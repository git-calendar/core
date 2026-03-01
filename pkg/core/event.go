package core

import (
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
)

type Event struct {
	Id           uuid.UUID   `json:"id"` // use UUIDv4; shouldn't change (different id = different event)
	Title        string      `json:"title"`
	Location     string      `json:"location"`
	Description  string      `json:"description"`
	From         time.Time   `json:"from"`
	OriginalFrom time.Time   `json:"original_from,omitempty"`
	To           time.Time   `json:"to"`
	Calendar     string      `json:"calendar"`
	Tag          string      `json:"tag"`
	MasterId     uuid.UUID   `json:"master_id"` // uuid.Nil if master
	Repeat       *Repetition `json:"repeat"`    // nil if slave
}

type Repetition struct {
	Frequency  Freq        `json:"frequency"`  // Daily, Weekly, ... (None if master)
	Interval   int         `json:"interval"`   // 1..N (freq:Weekly + interval:2 => every other week)
	Until      time.Time   `json:"until"`      // the end of repetition by timestamp
	Count      int         `json:"count"`      // or by number of occurrences (only one condition can be present not both)
	Exceptions []Exception `json:"exceptions"` // an array of slaves "From" timestamps
}

type Exception struct {
	Id   uuid.UUID `json:"id"`
	Time time.Time `json:"time"`
}

func (e *Event) Validate() error {
	if e == nil {
		return nil
	}
	if e.Id != uuid.Nil { // if id is set
		if e.Id.Version() != 4 { // enforce version
			return errors.New("unsupported UUID version")
		}
	} else { // if id is unset
		e.Id = uuid.New() // create one if not specified
	}
	if e.Title == "" {
		return errors.New("event title cannot be empty")
	}
	if e.From.IsZero() || e.To.IsZero() {
		return errors.New("event timestamps cannot be 0")
	}
	if e.From.Compare(e.To) != -1 {
		return errors.New("event 'from' timestamp cannot be greater or equal than 'to' (cannot end before it starts)")
	}
	if err := e.Repeat.Validate(); err != nil {
		return fmt.Errorf("events repetition is invalid: %w", err)
	}
	return nil
}

func (r *Repetition) Validate() error {
	if r == nil {
		return nil
	}
	if !r.Frequency.IsValid() {
		return errors.New("repetition frequency is invalid")
	}
	for _, ex := range r.Exceptions {
		if ex.Validate() == nil {
			return errors.New("repetition exceptions are not valid")
		}
	}
	if r.Interval < 1 {
		return errors.New("repetition interval is invalid")
	}
	if r.Until.IsZero() && r.Count < 1 {
		return errors.New("repetition combination of Until & Count is invalid")
	}
	if !r.Until.IsZero() && r.Count >= 0 {
		return errors.New("repetition when Until date set Count must be negative")
	}

	return nil
}

func (e *Exception) Validate() error {
	if e == nil {
		return nil
	}
	if e.Time.IsZero() {
		return errors.New("exception time cannot be empty")
	}
	return nil
}
