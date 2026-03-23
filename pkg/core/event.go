package core

import (
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/firu11/git-calendar-core/pkg/core/encryption"
	"github.com/google/uuid"
	// deterministic AEAD
)

type Event struct {
	Id          uuid.UUID   `json:"-"` // use UUIDv4; shouldn't change (different id = different event)
	Title       string      `json:"title"`
	Location    string      `json:"location,omitzero"`
	Description string      `json:"description,omitzero"`
	From        time.Time   `json:"from"`
	To          time.Time   `json:"to"`
	Calendar    string      `json:"calendar"`
	Tag         string      `json:"tag"`
	MasterId    uuid.UUID   `json:"-"`               // uuid.Nil if basic event or repeating master event
	Repeat      *Repetition `json:"repeat,omitzero"` // nil if slave
}

type Repetition struct {
	Frequency  Freq        `json:"frequency"`      // Day, Week, ... (None if master)
	Interval   int         `json:"interval"`       // 1..N (freq:Week + interval:2 => every other week)
	Until      time.Time   `json:"until,omitzero"` // the end of repetition by timestamp
	Count      int         `json:"count,omitzero"` // or by number of occurrences (only one condition can be present not both)
	Exceptions []uuid.UUID `json:"exceptions"`     // an array of slaves ids
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

func (e *Event) MarshalJSON() ([]byte, error) {
	type plainEvent Event // create a new type based on Event just to strip away its methods to avoid infinite recursion of MarshalJSON()

	enc, err := encryption.EncryptFields((*plainEvent)(e))
	if err != nil {
		return nil, err
	}

	return json.Marshal(enc)
}

func (e *Event) UnmarshalJSON(data []byte) error {
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}

	return nil // TODO: decrypt
}
