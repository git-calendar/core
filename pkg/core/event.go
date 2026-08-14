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
//  1. Basic:   A standalone event that does not repeat (ParentID is nil, Repeat is nil).
//  2. Parent:  The "source of truth" for a recurring series (ParentID is nil, Repeat defines the rule).
//  3. Child:   A generated occurrence from a Parent (ParentID points to its Parent, Repeat copies the Parent rule).
type Event struct {
	// ID must not change (different id = different event).
	// Standalone events and parents use UUIDv4; generated occurrences/children use UUIDv8 with start time embedded.
	ID uuid.UUID `json:"id"`
	// Title is the event's non-empty display name.
	Title string `json:"title"`
	// Location is the event's physical or virtual location.
	Location string `json:"location"`
	// Description contains additional event details.
	Description string `json:"description"`
	// From is the event's start time.
	From time.Time `json:"from"`
	// To is the event's end time.
	To time.Time `json:"to"`
	// Calendar names the calendar containing the event.
	Calendar string `json:"calendar"`
	// TagID optionally associates the event with a tag.
	TagID *uuid.UUID `json:"tag_id"`
	// ParentID identifies a generated occurrence's parent.
	// Standalone events and recurring-series parents use nil, not uuid.Nil.
	ParentID *uuid.UUID `json:"parent_id"`
	// Repeat is an internal recurrence representation. It is serialized as RFC 5545 text.
	Repeat *rrule.Set `json:"-"`
	// UpdatedAt resolves Git conflicts by latest value and is excluded from public JSON.
	UpdatedAt time.Time `json:"-"`
}

// Validate checks the event and assigns an ID when one is missing.
func (e *Event) Validate() error {
	if e == nil {
		return nil
	}
	if e.ID != uuid.Nil {
		if e.ID.Version() != 4 && e.ID.Version() != 8 { // enforce version
			return errors.New("unsupported UUID version")
		}
	} else {
		e.ID = uuid.New()
	}
	if e.Title == "" {
		return errors.New("title cannot be empty")
	}
	if e.From.IsZero() || e.To.IsZero() {
		return errors.New("timestamps From & To cannot be 0")
	}
	if e.From.Compare(e.To) != -1 {
		return errors.New("from timestamp cannot be greater or equal than to (cannot end before it starts)")
	}
	if e.ParentID == nil {
		if err := validateRecurrence(e.Repeat, e.From); err != nil {
			return fmt.Errorf("recurrence is invalid: %w", err)
		}
	}
	return nil
}

// IsBasic reports whether the event is standalone and non-recurring.
func (e Event) IsBasic() bool {
	return !e.IsChild() && !e.IsParent()
}

// IsChild reports whether the event is a generated recurring occurrence.
func (e Event) IsChild() bool {
	return e.ParentID != nil
}

// IsParent reports whether the event defines a recurring series.
func (e Event) IsParent() bool {
	return e.ParentID == nil && e.Repeat != nil
}

// getTreeEndTime returns the end of the final generated child.
func (e Event) getTreeEndTime() time.Time {
	if e.Repeat == nil {
		return e.To
	}
	if recurrenceIsUnbounded(e.Repeat) {
		return maxRecurrenceEnd
	}

	last := recurrenceLast(e.Repeat)
	if last.IsZero() {
		return e.To
	}
	return last.Add(e.To.Sub(e.From))
}
