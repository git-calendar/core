package core

import (
	"fmt"
	"strings"
	"testing"
	"time"

	rrule "github.com/teambition/rrule-go"
)

func TestParseICal(t *testing.T) {
	input := `BEGIN:VCALENDAR
VERSION:2.0
PRODID:-//git-calendar//test//EN
BEGIN:VEVENT
UID:one@example.com
DTSTART:20260714T100000Z
DTEND:20260714T113000Z
SUMMARY:Planning\, review
LOCATION:https://meeting.abc/123
DESCRIPTION:Line one\nLine two
END:VEVENT
BEGIN:VEVENT
UID:two@example.com
DTSTART:20260715T090000Z
DTEND:20260715T100000Z
SUMMARY:Fortnightly sync
RRULE:FREQ=WEEKLY;INTERVAL=2;COUNT=4
END:VEVENT
END:VCALENDAR`

	events, err := parseICal(strings.NewReader(input), "Work", false)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 2 {
		t.Fatalf("got %d events, want 2", len(events))
	}

	first := events[0]
	if first.ID.Version() != 4 {
		t.Errorf("event ID version = %d, want 4", first.ID.Version())
	}
	if first.Title != "Planning, review" {
		t.Errorf("Title = %q, want %q", first.Title, "Planning, review")
	}
	if first.Location != "https://meeting.abc/123" {
		t.Errorf("Location = %q, want %q", first.Location, "https://meeting.abc/123")
	}
	if first.Description != "Line one\nLine two" {
		t.Errorf("Description = %q, want %q", first.Description, "Line one\nLine two")
	}
	if first.Calendar != "Work" {
		t.Errorf("Calendar = %q, want %q", first.Calendar, "Work")
	}
	assertICalTime(t, first.From, time.Date(2026, 7, 14, 10, 0, 0, 0, time.UTC))
	assertICalTime(t, first.To, time.Date(2026, 7, 14, 11, 30, 0, 0, time.UTC))

	second := events[1]
	if second.Repeat == nil {
		t.Fatal("Repeat is nil")
	}
	rule := second.Repeat.GetRRule()
	if rule == nil {
		t.Fatal("RRULE is nil")
	}
	option := rule.OrigOptions
	if option.Freq != rrule.WEEKLY {
		t.Errorf("Frequency = %v, want %v", option.Freq, rrule.WEEKLY)
	}
	if option.Interval != 2 {
		t.Errorf("Interval = %d, want 2", option.Interval)
	}
	if option.Count != 4 {
		t.Errorf("Count = %d, want 4", option.Count)
	}
}

func TestParseICalDefaultsAllDayEventWithoutEndToOneDay(t *testing.T) {
	input := `BEGIN:VCALENDAR
VERSION:2.0
PRODID:icalendar-ruby
BEGIN:VEVENT
UID:holiday@example.com
DTSTART;VALUE=DATE:20240101
SUMMARY:Nový rok
RRULE:FREQ=YEARLY;COUNT=6
END:VEVENT
END:VCALENDAR`

	events, err := parseICal(strings.NewReader(input), "Holidays", false)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 {
		t.Fatalf("got %d events, want 1", len(events))
	}

	assertICalTime(t, events[0].From, time.Date(2024, 1, 1, 0, 0, 0, 0, time.Local))
	assertICalTime(t, events[0].To, time.Date(2024, 1, 2, 0, 0, 0, 0, time.Local))
	if events[0].Repeat == nil {
		t.Fatal("Repeat is nil")
	}
}

func TestParseICalSupportsRecurrenceModifiers(t *testing.T) {
	input := `BEGIN:VCALENDAR
VERSION:2.0
PRODID:-//git-calendar//test//EN
BEGIN:VEVENT
UID:one@example.com
DTSTART:20260714T100000Z
DTEND:20260714T110000Z
SUMMARY:Several days
RRULE:FREQ=WEEKLY;COUNT=4;BYDAY=MO,WE
END:VEVENT
END:VCALENDAR`

	events, err := parseICal(strings.NewReader(input), "Work", false)
	if err != nil {
		t.Fatal(err)
	}

	weekdays := events[0].Repeat.GetRRule().OrigOptions.Byweekday
	if len(weekdays) != 2 || weekdays[0].String() != "MO" || weekdays[1].String() != "WE" {
		t.Fatalf("Byweekday = %v, want [MO WE]", weekdays)
	}
}

// fuck you microslop
func TestParseICalSupportsWindowsTimeZones(t *testing.T) {
	tests := []struct {
		name      string
		tzid      string
		start     string
		end       string
		wantStart time.Time
		wantEnd   time.Time
	}{
		{
			name:      "Central Europe daylight time",
			tzid:      "Central Europe Standard Time",
			start:     "20260729T160000",
			end:       "20260729T183000",
			wantStart: time.Date(2026, 7, 29, 14, 0, 0, 0, time.UTC),
			wantEnd:   time.Date(2026, 7, 29, 16, 30, 0, 0, time.UTC),
		},
		{
			name:      "Pacific standard time",
			tzid:      "Pacific Standard Time",
			start:     "20260115T100000",
			end:       "20260115T110000",
			wantStart: time.Date(2026, 1, 15, 18, 0, 0, 0, time.UTC),
			wantEnd:   time.Date(2026, 1, 15, 19, 0, 0, 0, time.UTC),
		},
		{
			name:      "India half-hour offset",
			tzid:      "India Standard Time",
			start:     "20260115T100000",
			end:       "20260115T110000",
			wantStart: time.Date(2026, 1, 15, 4, 30, 0, 0, time.UTC),
			wantEnd:   time.Date(2026, 1, 15, 5, 30, 0, 0, time.UTC),
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			input := fmt.Sprintf(`BEGIN:VCALENDAR
VERSION:2.0
BEGIN:VEVENT
UID:microsoft@example.com
SUMMARY:MS event
DTSTART;TZID=%s:%s
DTEND;TZID=%s:%s
END:VEVENT
END:VCALENDAR`, test.tzid, test.start, test.tzid, test.end)

			events, err := parseICal(strings.NewReader(input), "Microsoft", true)
			if err != nil {
				t.Fatal(err)
			}
			if len(events) != 1 {
				t.Fatalf("got %d events, want 1", len(events))
			}

			assertICalTime(t, events[0].From, test.wantStart)
			assertICalTime(t, events[0].To, test.wantEnd)
		})
	}
}

func assertICalTime(t *testing.T, got, want time.Time) {
	t.Helper()
	if !got.Equal(want) {
		t.Errorf("time = %v, want %v", got, want)
	}
}
