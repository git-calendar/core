package core

import (
	"errors"
	"time"

	"github.com/google/uuid"
)

type Event struct {
	Id       uuid.UUID `json:"id"` // shouldn't change (different id = different event)
	Title    string    `json:"title"`
	Location string    `json:"location"`
	From     time.Time `json:"from"`
	To       time.Time `json:"to"`
}

func (e *Event) Validate() error {
	if e.Title == "" {
		return errors.New("event title cannot be empty")
	}
	if e.From.IsZero() || e.To.IsZero() {
		return errors.New("event timestamps cannot be 0")
	}
	if e.From.Compare(e.To) <= 0 {
		return errors.New("event 'from' timestamp cannot be greater or equal than 'to' (cannot end before it starts)")
	}
	return nil
}
