package core

import (
	"strings"
	"testing"
	"time"
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
	if first.Id.Version() != 4 {
		t.Errorf("event ID version = %d, want 4", first.Id.Version())
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
	if second.Repeat.Frequency != Week {
		t.Errorf("Frequency = %v, want %v", second.Repeat.Frequency, Week)
	}
	if second.Repeat.Interval != 2 {
		t.Errorf("Interval = %d, want 2", second.Repeat.Interval)
	}
	if second.Repeat.Count != 4 {
		t.Errorf("Count = %d, want 4", second.Repeat.Count)
	}
}

func TestParseICalRejectsUnsupportedRecurrence(t *testing.T) {
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

	_, err := parseICal(strings.NewReader(input), "Work", false)
	if err == nil || !strings.Contains(err.Error(), "recurrence modifiers are not supported") {
		t.Fatalf("error = %v, want unsupported recurrence modifier error", err)
	}
}

func assertICalTime(t *testing.T, got, want time.Time) {
	t.Helper()
	if !got.Equal(want) {
		t.Errorf("time = %v, want %v", got, want)
	}
}
