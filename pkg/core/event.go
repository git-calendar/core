package core

import (
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
)

type Event struct {
	Id          uuid.UUID   `json:"id"` // shouldn't change (different id = different event); only UUIDv4 or UUIDv8 for children
	Title       string      `json:"title"`
	Location    string      `json:"location"`
	Description string      `json:"description"`
	From        time.Time   `json:"from"`
	To          time.Time   `json:"to"`
	Calendar    string      `json:"calendar"`
	Tag         string      `json:"tag"`
	ParentId    uuid.UUID   `json:"parent_id"` // specific for child events; its uuid.Nil if the event is basic or parent
	Repeat      *Repetition `json:"repeat"`    // nil if child
}

type Repetition struct {
	Frequency  Freq        `json:"frequency"`  // Day, Week, ... (None if parent)
	Interval   int         `json:"interval"`   // 1..N (freq:Week + interval:2 => every other week)
	Until      time.Time   `json:"until"`      // the end of repetition by timestamp
	Count      int         `json:"count"`      // or by number of occurrences (only one condition can be present not both)
	Exceptions []uuid.UUID `json:"exceptions"` // an array of children ids
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

func (e Event) isChild() bool {
	return e.ParentId != uuid.Nil
}

func (e Event) isParent() bool {
	return e.ParentId == uuid.Nil && e.Repeat != nil
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
