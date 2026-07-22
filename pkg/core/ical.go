package core

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"time"

	ics "github.com/arran4/golang-ical"
	"github.com/google/uuid"
)

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
			event.Id = icalEventID(calendar, icalText(source, ics.ComponentPropertyUniqueId), i, event)
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

	to, err := source.GetEndAt()
	if err != nil {
		return Event{}, fmt.Errorf("read DTEND: %w", err)
	}

	repeat, err := parseICalRepetition(source)
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

func parseICalRepetition(event *ics.VEvent) (*Repetition, error) {
	if event.HasProperty(ics.ComponentPropertyRdate) ||
		event.HasProperty(ics.ComponentPropertyExdate) ||
		event.HasProperty(ics.ComponentPropertyExrule) ||
		event.HasProperty(ics.ComponentPropertyRecurrenceId) {
		return nil, errors.New("recurrence dates and exceptions are not supported")
	}

	rules, err := event.GetRRules()
	if err != nil {
		return nil, err
	}
	if len(rules) == 0 {
		return nil, nil
	}
	if len(rules) != 1 {
		return nil, errors.New("multiple recurrence rules are not supported")
	}

	rule := rules[0]
	if hasICalModifiers(rule) {
		return nil, errors.New("recurrence modifiers are not supported")
	}

	frequency, err := parseICalFrequency(rule.Freq)
	if err != nil {
		return nil, err
	}

	return &Repetition{
		Frequency: frequency,
		Interval:  rule.Interval,
		Until:     rule.Until,
		Count:     rule.Count,
	}, nil
}

func parseICalFrequency(frequency ics.Frequency) (Freq, error) {
	switch frequency {
	case ics.FrequencyDaily:
		return Day, nil
	case ics.FrequencyWeekly:
		return Week, nil
	case ics.FrequencyMonthly:
		return Month, nil
	case ics.FrequencyYearly:
		return Year, nil
	default:
		return Invalid, fmt.Errorf("unsupported recurrence frequency %q", frequency)
	}
}

func hasICalModifiers(rule *ics.RecurrenceRule) bool {
	return len(rule.BySecond)+len(rule.ByMinute)+len(rule.ByHour)+
		len(rule.ByDay)+len(rule.ByMonthDay)+len(rule.ByYearDay)+
		len(rule.ByWeekNo)+len(rule.ByMonth)+len(rule.BySetPos) > 0 ||
		rule.Wkst != ""
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
		uid = fmt.Sprintf("%d\x00%s\x00%s", index, event.Title, event.From.Format(time.RFC3339Nano))
	}
	sum := sha256.Sum256([]byte(calendar + "\x00" + uid))

	var id uuid.UUID
	copy(id[:], sum[:16])
	id[6] = id[6]&0x0f | 0x80
	id[8] = id[8]&0x3f | 0x80
	return id
}
