package core

import (
	"errors"
	"time"

	"github.com/google/uuid"
)

type Event struct {
	Id          uuid.UUID   `json:"id"` // shouldn't change (different id = different event)
	Title       string      `json:"title"`
	Location    string      `json:"location"`
	From        time.Time   `json:"from"`
	To          time.Time   `json:"to"`
	Description string      `json:"description"`
	Repeat      *Repetition `json:"repeat"` // nil if not repeating
}

type Repetition struct {
	Frequency  TimeUnit    `json:"frequency"`
	Interval   uint        `json:"interval"`
	Until      time.Time   `json:"until"`
	Count      uint        `json:"count"`
	Exceptions []time.Time `json:"exceptions"`
	ParentId   uuid.UUID   `json:"parentId"`
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
	return nil
}
