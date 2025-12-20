package core

import (
	"errors"
)

type Event struct {
	Id       int    `json:"id"` // shouldnt change (different id = different event)
	Title    string `json:"title"`
	Location string `json:"location"`
	From     int64  `json:"from"` // unix timestamp in minutes (not using time.Time for cross lang. compatibility)
	To       int64  `json:"to"`   // unix timestamp in minutes (not using time.Time for cross lang. compatibility)
}

func (e *Event) Validate() error {
	if e.Title == "" {
		return errors.New("event title cannot be empty")
	}
	if e.From < 0 || e.To < 0 {
		return errors.New("event timestamps cannot be before epoch (less than 0)")
	}
	if e.From >= e.To {
		return errors.New("event 'from' timestamp cannot be greater or equal than 'to' (cannot end before it starts)")
	}
	return nil
}
