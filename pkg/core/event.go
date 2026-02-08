package core

import (
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
)

type Event struct {
	Id          uuid.UUID   `json:"id"` // shouldn't change (different id = different event)
	Title       string      `json:"title"`
	Location    string      `json:"location"`
	Description string      `json:"description"`
	From        time.Time   `json:"from"`
	To          time.Time   `json:"to"`
	MasterId    uuid.UUID   `json:"master_id"` // uuid.Nil if master
	Repeat      *Repetition `json:"repeat"`    // nil if slave
}

type Repetition struct {
	Frequency TimeUnit  `json:"frequency"` // Daily, Weekly, ... (None if master)
	Interval  uint      `json:"interval"`  // 1..N (freq:Weekly + interval:2 => every other week)
	Until     time.Time `json:"until"`     // the end of repetition by timestamp
	// Count      uint        `json:"count"`      // or by number of occurances TODO
	Exceptions []time.Time `json:"exceptions"` // an array of slaves "From" timestamps
}

func (e *Event) Validate() error {
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

func (e *Repetition) Validate() error {
	// TODO
	return nil
}
