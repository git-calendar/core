package core

import (
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/firu11/git-calendar-core/pkg/core/encryption"
	"github.com/google/uuid"
)

type Event struct {
	Id          uuid.UUID   `json:"id" encrypt:"-"` // use UUIDv4; shouldn't change (different id = different event)
	Title       string      `json:"title"  encrypt:"title"`
	Location    string      `json:"location,omitzero"  encrypt:"location"`
	Description string      `json:"description,omitzero"  encrypt:"description"`
	From        time.Time   `json:"from" encrypt:"from"`
	To          time.Time   `json:"to" encrypt:"to"`
	Calendar    string      `json:"calendar" encrypt:"calendar"`
	Tag         string      `json:"tag" encrypt:"tag"`
	MasterId    uuid.UUID   `json:"master_id" encrypt:"-"`            // uuid.Nil if basic event or repeating master event
	Repeat      *Repetition `json:"repeat,omitzero" encrypt:"repeat"` // nil if slave
}

type Repetition struct {
	Frequency  Freq        `json:"frequency" encrypt:"frequency"`   // Day, Week, ... (None if master)
	Interval   int         `json:"interval" encrypt:"interval"`     // 1..N (freq:Week + interval:2 => every other week)
	Until      time.Time   `json:"until,omitzero" encrypt:"until"`  // the end of repetition by timestamp
	Count      int         `json:"count,omitzero" encrypt:"count"`  // or by number of occurrences (only one condition can be present not both)
	Exceptions []uuid.UUID `json:"exceptions" encrypt:"exceptions"` // an array of slaves ids
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

func (e Event) isGenerated() bool {
	return e.MasterId != uuid.Nil
}

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
