package core

import (
	"bytes"
	"fmt"
	"io"
	"slices"
	"time"
	"uuid"

	ics "github.com/arran4/golang-ical"
	rrule "github.com/teambition/rrule-go"
	"github.com/thommeo/winianatz"
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
		normalizeTimeZones(source)
		event, err := parseICalEvent(source, calendar)
		if err != nil {
			return nil, fmt.Errorf("import event %d: %w", i+1, err)
		}
		if stableIDs {
			event.ID = icalEventID(calendar, source.Id(), i, event)
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

func normalizeTimeZones(event *ics.VEvent) {
	for i := range event.Properties {
		tzid := event.Properties[i].ICalParameters[string(ics.ParameterTzid)]
		if len(tzid) != 1 {
			continue
		}
		if zone, err := winianatz.FromMicrosoftAlias(tzid[0]); err == nil {
			tzid[0] = zone.IANA
		}
	}
}

func icalEventID(calendar, uid string, index int, event Event) uuid.UUID {
	if uid == "" {
		// fallback if uid is missing
		uid = fmt.Sprintf("%d\x00%s\x00%s", index, event.Title, event.From.Format(time.RFC3339Nano))
	}
	return newUUIDv5(uuid.Nil(), []byte("git-calendar:event\x00"+calendar+"\x00"+uid))
}

func serializeICal(calendarName string, events []Event) []byte {
	calendar := ics.NewCalendarFor("git-calendar")
	calendar.SetCalscale("GREGORIAN")
	if calendarName != "" {
		calendar.SetName(calendarName)
	}

	events = slices.Clone(events)
	slices.SortFunc(events, func(a, b Event) int {
		return bytes.Compare(a.ID[:], b.ID[:])
	})

	generatedAt := time.Now()
	for _, event := range events {
		addICalEvent(calendar, event, generatedAt)
	}

	return []byte(calendar.Serialize(ics.WithNewLineWindows))
}

func addICalEvent(calendar *ics.Calendar, event Event, generatedAt time.Time) {
	exported := calendar.AddEvent("urn:uuid:" + event.ID.String())
	exported.SetSummary(event.Title)
	exported.SetStartAt(event.From)
	exported.SetEndAt(event.To)

	if event.Location != "" {
		exported.SetLocation(event.Location)
	}
	if event.Description != "" {
		exported.SetDescription(event.Description)
	}

	stamp := event.UpdatedAt
	if stamp.IsZero() {
		stamp = generatedAt
	}
	exported.SetDtStampTime(stamp)
	if !event.UpdatedAt.IsZero() {
		exported.SetLastModifiedAt(event.UpdatedAt)
	}

	if event.Repeat == nil {
		return
	}
	rule := event.Repeat.GetRRule()
	if rule == nil {
		return
	}
	exported.AddRrule(rule.OrigOptions.RRuleString())
	for _, exception := range event.Repeat.GetExDate() {
		exported.AddExdate(exception.UTC().Format(rrule.DateTimeFormat))
	}
}
