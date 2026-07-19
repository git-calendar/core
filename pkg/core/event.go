package core

import (
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	rrule "github.com/teambition/rrule-go"
)

// Event represents a single calendar entry.
//
// Can be one of these:
//  1. Basic:   A standalone event that does not repeat (ParentId is nil, Repeat is nil).
//  2. Parent:  The "source of truth" for a recurring series (ParentId is nil, Repeat defines the rule).
//  3. Child:   A generated occurrence from a Parent (ParentId points to its Parent, Repeat is nil).
type Event struct {
	Id          uuid.UUID  `json:"id"`       // Should not change (different id = different event). Only UUIDv4 or UUIDv8 (for children) is being used.
	Title       string     `json:"title"`    // Should not be empty.
	Location    string     `json:"location"` // Physical or virtual location (e.g., URL).
	Description string     `json:"description"`
	From        time.Time  `json:"from"`
	To          time.Time  `json:"to"`
	Calendar    string     `json:"calendar"`  // The name of the calendar the event belongs to.
	TagId       *uuid.UUID `json:"tag_id"`    // A user-defined tag/category. Can be nil.
	ParentId    *uuid.UUID `json:"parent_id"` // Specific for child events. It is nil (not uuid.Nil) if the event is basic or parent.
	Repeat      *rrule.Set `json:"-"`
	UpdatedAt   time.Time  `json:"-"` // Used for git conflict resolution; latest wins. Client doesn't need to see this -> json:"-".
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
	if e.ParentId == nil {
		if err := validateRecurrence(e.Repeat, e.From); err != nil {
			return fmt.Errorf("recurrence is invalid: %w", err)
		}
	}
	return nil
}

func (e Event) IsBasic() bool {
	return !e.IsChild() && !e.IsParent()
}

func (e Event) IsChild() bool {
	return e.ParentId != nil
}

func (e Event) IsParent() bool {
	return e.ParentId == nil && e.Repeat != nil
}

// getTreeEndTime returns the end of the final generated child.
func (e Event) getTreeEndTime() time.Time {
	if e.Repeat == nil {
		return e.To
	}

	last := recurrenceLast(e.Repeat)
	if last.IsZero() {
		return e.To
	}
	return last.Add(e.To.Sub(e.From))
}
