package core

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/git-calendar/core/pkg/encryption"
	"github.com/go-git/go-billy/v5"
	"github.com/google/uuid"
)

// Event represents a single calendar entry.
//
// Can be one of these:
//  1. Basic:   A standalone event that does not repeat (ParentId is nil, Repeat is nil).
//  2. Parent:  The "source of truth" for a recurring series (ParentId is nil, Repeat defines the rule).
//  3. Child:   A generated occurrence from a Parent (ParentId points to its Parent, Repeat copies the Parent rule).
type Event struct {
	Id          uuid.UUID   `json:"id,omitzero"`       // Should not change (different id = different event). Only UUIDv4 or UUIDv8 (for children) is being used.
	Title       string      `json:"title,omitzero"`    // Should not be empty.
	Location    string      `json:"location,omitzero"` // Physical or virtual location (e.g., URL).
	Description string      `json:"description,omitzero"`
	From        time.Time   `json:"from,omitzero"`
	To          time.Time   `json:"to,omitzero"`
	Calendar    string      `json:"calendar,omitzero"`  // The name of the calendar the event belongs to.
	Tag         string      `json:"tag,omitzero"`       // User-defined category or label.
	ParentId    uuid.UUID   `json:"parent_id,omitzero"` // Specific for child events. It is uuid.Nil if the event is basic or parent.
	Repeat      *Repetition `json:"repeat,omitzero"`    // nil if child
}

// Repetition defines the recurrence rules for a Parent event.
//
// A Repetition object exists only on Parent events to generate Children.
// A series must be capped by either Until (date) or Count (occurrences). Not both.
type Repetition struct {
	Frequency  Freq        `json:"frequency,omitzero"`  // The unit of time for recurrence (Day, Week, Month, etc.).
	Interval   int         `json:"interval,omitzero"`   // The multiplier for Frequency (e.g., Interval:2 * Frequency:Week = every other week).
	Until      time.Time   `json:"until,omitzero"`      // Hard stop date for the series.
	Count      int         `json:"count,omitzero"`      // Total number of occurrences to generate.
	Exceptions []uuid.UUID `json:"exceptions,omitzero"` // List of Child IDs that deviate from the base rule (edited or cancelled).
}

func (e *Event) Validate() error {
	if e == nil {
		return nil
	}
	if e.Id != uuid.Nil {
		// if id is set
		if e.Id.Version() != 4 && e.Id.Version() != 8 { // enforce version
			return errors.New("unsupported UUID version")
		}
	} else { // if id is unset
		e.Id = uuid.New() // create one if not specified
	}
	if e.Title == "" {
		return errors.New("Title cannot be empty")
	}
	if e.From.IsZero() || e.To.IsZero() {
		return errors.New("timestamps From & To cannot be 0")
	}
	if e.From.Compare(e.To) != -1 {
		return errors.New("From timestamp cannot be greater or equal than To (cannot end before it starts)")
	}
	if err := e.Repeat.Validate(); err != nil {
		return fmt.Errorf("repetition is invalid: %w", err)
	}
	return nil
}

func (r *Repetition) Validate() error {
	if r == nil {
		return nil
	}
	if !r.Frequency.IsValid() {
		return errors.New("frequency is invalid")
	}
	if r.Interval < 1 {
		return errors.New("interval is invalid")
	}
	if r.Until.IsZero() && r.Count < 1 {
		return errors.New("combination of Until & Count is invalid")
	}
	if !r.Until.IsZero() && r.Count > 0 {
		return errors.New("Count must be 0 when Until date is set")
	}

	return nil
}

func (e Event) IsBasic() bool {
	return !e.IsChild() && !e.IsParent() // e.ParentId == uuid.Nil && e.Repeat == nil
}

func (e Event) IsChild() bool {
	return e.ParentId != uuid.Nil
}

func (e Event) IsParent() bool {
	return e.ParentId == uuid.Nil && e.Repeat != nil
}

// Returns either the To time.Time for Basic non-repeating event, or calculates the last occurrence of a repeating Parent event and returns its To.
func (e Event) getTreeEndTime() time.Time {
	if e.Repeat == nil {
		return e.To
	}

	eventEnd := e.To
	if e.Repeat != nil {
		eventEnd = e.Repeat.Until // if repeating, use interval [From, Repetition.Until]
		if e.Repeat.Count >= 1 {  // if repeating on count basis
			eventEnd = addUnit(e.To, e.Repeat.Interval*e.Repeat.Count, e.Repeat.Frequency)
		}
	}
	return eventEnd
}

func (e Event) WriteToFile(file billy.File, key []byte) error {
	// not needed to be stored in the file
	id := e.Id
	e.Id = uuid.Nil

	// marshal normally
	raw, err := json.MarshalIndent(e, "", "  ")
	if err != nil {
		return err
	}

	if len(key) == 0 { // no encryption, just use the plaintext
		_, err = file.Write(raw)
		return err
	}

	if id == uuid.Nil {
		return errors.New("event Id has to be set for encryption")
	}

	// unmarshal into generic map
	var data map[string]any
	if err := json.Unmarshal(raw, &data); err != nil {
		return err
	}

	// encrypt everything recursively
	encData, err := encryption.EncryptFields(data, key, id[:])
	if err != nil {
		return err
	}

	// marshal again
	finalRaw, err := json.MarshalIndent(encData, "", "  ")
	if err != nil {
		return err
	}

	_, err = file.Write(finalRaw)
	return err
}

func (e *Event) LoadFromFile(file billy.File, decryptionKey []byte) error {
	raw, err := io.ReadAll(file)
	if err != nil {
		return fmt.Errorf("failed to read file '%s': %w\n", file.Name(), err)
	}

	e.Id, err = uuid.Parse(strings.Split(file.Name(), ".")[0]) // get event id from the name
	if err != nil {
		return fmt.Errorf("file name is not UUID.json but '%s': %w\n", file.Name(), err)
	}

	if len(decryptionKey) == 0 { // no encryption, just use the plaintext
		return json.Unmarshal(raw, e)
	}

	var encryptedData map[string]any
	if err := json.Unmarshal(raw, &encryptedData); err != nil {
		return err
	}

	decryptedData, err := encryption.DecryptFields(encryptedData, decryptionKey, e.Id[:])
	if err != nil {
		return err
	}

	// eww (map to struct)
	tmp, err1 := json.Marshal(decryptedData)
	err2 := json.Unmarshal(tmp, e)

	return errors.Join(err1, err2)
}
