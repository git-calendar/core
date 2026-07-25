package core

import (
	"crypto/sha256"
	"fmt"
	"io"
	"time"

	ics "github.com/arran4/golang-ical"
	"github.com/google/uuid"
	rrule "github.com/teambition/rrule-go"
)

// parseICal parses ical events from r to []Event.
// calendar is copied to each event and used to scope deterministic IDs.
// When stableIDs is false, each event gets a random ID.
func parseICal(r io.Reader, calendar string, stableIDs bool) ([]Event, error) {
	cal, err := ics.ParseCalendar(r)
	if err != nil {
		return nil, fmt.Errorf("parse iCalendar: %w", err)
	}

	sourceEvents := cal.Events()
	events := make([]Event, 0, len(sourceEvents))
	for i, source := range sourceEvents {
		event, err := parseICalEvent(source, calendar)
		if err != nil {
			return nil, fmt.Errorf("import event %d: %w", i+1, err)
		}
		if stableIDs {
			event.Id = icalEventID(calendar, source.Id(), i, event)
		}
		if err := event.Validate(); err != nil {
			return nil, fmt.Errorf("import event %d: %w", i+1, err)
		}
		events = append(events, event)
	}

	return events, nil
}

func parseICalEvent(source *ics.VEvent, calendar string) (Event, error) {
	from, err := source.GetStartAt()
	if err != nil {
		return Event{}, fmt.Errorf("read DTSTART: %w", err)
	}

	to, err := icalEventEnd(source, from)
	if err != nil {
		return Event{}, err
	}

	repeat, err := parseICalRRule(source, from)
	if err != nil {
		return Event{}, err
	}

	return Event{
		Title:       icalText(source, ics.ComponentPropertySummary),
		Location:    icalText(source, ics.ComponentPropertyLocation),
		Description: icalText(source, ics.ComponentPropertyDescription),
		From:        from,
		To:          to,
		Calendar:    calendar,
		Repeat:      repeat,
	}, nil
}

func icalEventEnd(event *ics.VEvent, start time.Time) (time.Time, error) {
	if event.HasProperty(ics.ComponentPropertyDtEnd) {
		end, err := event.GetEndAt()
		if err != nil {
			return time.Time{}, fmt.Errorf("read DTEND: %w", err)
		}
		return end, nil
	}

	startProperty := event.GetProperty(ics.ComponentPropertyDtStart)
	if startProperty != nil && startProperty.GetValueType() == ics.ValueDataTypeDate {
		// RFC 5545 defines a date-only VEVENT without DTEND as lasting one day.
		return start.AddDate(0, 0, 1), nil
	}

	return time.Time{}, fmt.Errorf("read DTEND: %w: %s", ics.ErrorPropertyNotFound, ics.ComponentPropertyDtEnd)
}

func parseICalRRule(event *ics.VEvent, start time.Time) (*rrule.Set, error) {
	value := icalText(event, ics.ComponentPropertyRrule)
	if value == "" {
		return nil, nil
	}

	option, err := rrule.StrToROptionInLocation(value, start.Location())
	if err != nil {
		return nil, fmt.Errorf("parse RRULE: %w", err)
	}
	option.Dtstart = start
	return newRecurrence(*option, nil)
}

func icalText(event *ics.VEvent, property ics.ComponentProperty) string {
	value := event.GetProperty(property)
	if value == nil {
		return ""
	}
	return value.Value
}

func icalEventID(calendar, uid string, index int, event Event) uuid.UUID {
	if uid == "" {
		// fallback it uid is missing
		uid = fmt.Sprintf("%d\x00%s\x00%s", index, event.Title, event.From.Format(time.RFC3339Nano))
	}
	return uuid.NewHash(sha256.New(), uuid.Nil, []byte(calendar+"\x00"+uid), 8)
}
