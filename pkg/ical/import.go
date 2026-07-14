package ical

import (
	"errors"
	"fmt"
	"io"

	ics "github.com/arran4/golang-ical"
	"github.com/git-calendar/core/pkg/core"
)

// Import parses iCalendar events and assigns them to calendar.
func Import(r io.Reader, calendar string) ([]core.Event, error) {
	cal, err := ics.ParseCalendar(r)
	if err != nil {
		return nil, fmt.Errorf("parse iCalendar: %w", err)
	}

	sourceEvents := cal.Events()
	events := make([]core.Event, 0, len(sourceEvents))
	for i, source := range sourceEvents {
		event, err := importEvent(source, calendar)
		if err != nil {
			return nil, fmt.Errorf("import event %d: %w", i+1, err)
		}
		events = append(events, event)
	}

	return events, nil
}

func importEvent(source *ics.VEvent, calendar string) (core.Event, error) {
	from, err := source.GetStartAt()
	if err != nil {
		return core.Event{}, fmt.Errorf("read DTSTART: %w", err)
	}

	to, err := source.GetEndAt()
	if err != nil {
		return core.Event{}, fmt.Errorf("read DTEND: %w", err)
	}

	repeat, err := importRepetition(source)
	if err != nil {
		return core.Event{}, err
	}

	event := core.Event{
		Title:       text(source, ics.ComponentPropertySummary),
		Location:    text(source, ics.ComponentPropertyLocation),
		Description: text(source, ics.ComponentPropertyDescription),
		From:        from,
		To:          to,
		Calendar:    calendar,
		Repeat:      repeat,
	}
	if err := event.Validate(); err != nil {
		return core.Event{}, err
	}

	return event, nil
}

func importRepetition(event *ics.VEvent) (*core.Repetition, error) {
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
	if hasModifiers(rule) {
		return nil, errors.New("recurrence modifiers are not supported")
	}

	frequency, err := importFrequency(rule.Freq)
	if err != nil {
		return nil, err
	}

	return &core.Repetition{
		Frequency: frequency,
		Interval:  rule.Interval,
		Until:     rule.Until,
		Count:     rule.Count,
	}, nil
}

func importFrequency(frequency ics.Frequency) (core.Freq, error) {
	switch frequency {
	case ics.FrequencyDaily:
		return core.Day, nil
	case ics.FrequencyWeekly:
		return core.Week, nil
	case ics.FrequencyMonthly:
		return core.Month, nil
	case ics.FrequencyYearly:
		return core.Year, nil
	default:
		return core.Invalid, fmt.Errorf("unsupported recurrence frequency %q", frequency)
	}
}

func hasModifiers(rule *ics.RecurrenceRule) bool {
	return len(rule.BySecond)+len(rule.ByMinute)+len(rule.ByHour)+
		len(rule.ByDay)+len(rule.ByMonthDay)+len(rule.ByYearDay)+
		len(rule.ByWeekNo)+len(rule.ByMonth)+len(rule.BySetPos) > 0 ||
		rule.Wkst != ""
}

func text(event *ics.VEvent, property ics.ComponentProperty) string {
	value := event.GetProperty(property)
	if value == nil {
		return ""
	}
	return value.Value
}
