package core

import (
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/git-calendar/core/pkg/core/encryption"
	"github.com/google/uuid"
)

// Event represents a single calendar entry.
//
// Can be one of these:
//  1. Basic:   A standalone event that does not repeat (ParentId is nil, Repeat is nil).
//  2. Parent:  The "source of truth" for a recurring series (ParentId is nil, Repeat defines the rule).
//  3. Child:   A generated occurrence from a Parent (ParentId points to its Parent, Repeat copies the Parent rule).
type Event struct {
	Id          uuid.UUID   `json:"id" encrypt:"-"`              // Should not change (different id = different event). Only UUIDv4 or UUIDv8 (for children) is being used.
	Title       string      `json:"title" encrypt:"title"`       // Should not be empty.
	Location    string      `json:"location" encrypt:"location"` // Physical or virtual location (e.g., URL).
	Description string      `json:"description" encrypt:"description"`
	From        time.Time   `json:"from" encrypt:"from"`
	To          time.Time   `json:"to" encrypt:"to"`
	Calendar    string      `json:"calendar" encrypt:"calendar"` // The name of the calendar the event belongs to.
	Tag         string      `json:"tag" encrypt:"tag"`           // User-defined category or label.
	ParentId    uuid.UUID   `json:"parent_id" encrypt:"-"`       // Specific for child events. It is uuid.Nil if the event is basic or parent.
	Repeat      *Repetition `json:"repeat" encrypt:"repeat"`     // nil if child
}

// Repetition defines the recurrence rules for a Parent event.
//
// A Repetition object exists only on Parent events to generate Children.
// A series must be capped by either Until (date) or Count (occurrences). Not both.
type Repetition struct {
	Frequency  Freq        `json:"frequency" encrypt:"frequency"`   // The unit of time for recurrence (Day, Week, Month, etc.).
	Interval   int         `json:"interval" encrypt:"interval"`     // The multiplier for Frequency (e.g., Interval:2 * Frequency:Week = every other week).
	Until      time.Time   `json:"until" encrypt:"until"`           // Hard stop date for the series.
	Count      int         `json:"count" encrypt:"count"`           // Total number of occurrences to generate.
	Exceptions []uuid.UUID `json:"exceptions" encrypt:"exceptions"` // List of Child IDs that deviate from the base rule (edited or cancelled).
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

// Returns either the To time.Time for Basic non-repeating event, or calculates the last occurrence of Parent event and returns its To.
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

// Returns the marshaled and encrypted (if key was set) JSON.
func (e *Event) EncryptToIndentedJSON() ([]byte, error) {
	idBytes, _ := e.Id.MarshalBinary() // err always nil

	enc, err := encryption.EncryptFields(e, idBytes)
	if err != nil {
		return nil, err
	}

	return json.MarshalIndent(enc, "", "  ")
}

// Unmarshals and decrypts (if key was set) JSON.
func (e *Event) DecryptFromJSON(data []byte) error {
	idBytes, _ := e.Id.MarshalBinary() // err always nil

	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}

	return encryption.DecryptFields(e, raw, idBytes)
}
