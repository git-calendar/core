package core

import (
	"strings"
	"testing"
	"time"

	rrule "github.com/teambition/rrule-go"
)

func TestValidateRecurrencePreservesDTStartLocation(t *testing.T) {
	start, err := time.Parse(time.RFC3339, "2026-08-06T11:15:00+02:00")
	if err != nil {
		t.Fatal(err)
	}
	set, err := rrule.StrToRRuleSet("DTSTART;TZID=Europe/Prague:20260806T111500\nRRULE:FREQ=WEEKLY;INTERVAL=1;BYDAY=TH")
	if err != nil {
		t.Fatal(err)
	}

	if err := validateRecurrence(set, start); err != nil {
		t.Fatal(err)
	}

	if got := set.GetDTStart().Location().String(); got != "Europe/Prague" {
		t.Fatalf("DTSTART location mismatch: got %q, want Europe/Prague", got)
	}
	if got := set.String(); !strings.Contains(got, "DTSTART;TZID=Europe/Prague:20260806T111500") {
		t.Fatalf("serialized recurrence lost its time zone: %q", got)
	}

	prague, err := time.LoadLocation("Europe/Prague")
	if err != nil {
		t.Fatal(err)
	}
	from := time.Date(2026, time.November, 1, 0, 0, 0, 0, prague)
	to := time.Date(2026, time.November, 30, 0, 0, 0, 0, prague)
	occurrences := recurrenceBetween(set, from, to)
	if len(occurrences) == 0 {
		t.Fatal("expected at least one winter occurrence")
	}
	for _, occurrence := range occurrences {
		if occurrence.Hour() != 11 || occurrence.Minute() != 15 {
			t.Fatalf("winter occurrence moved from 11:15 local time: %s", occurrence)
		}
		_, offset := occurrence.Zone()
		if offset != int(time.Hour/time.Second) {
			t.Fatalf("winter occurrence has offset %d, want 3600", offset)
		}
	}
}

func TestValidateRecurrenceRequiresRule(t *testing.T) {
	set := &rrule.Set{}
	set.DTStart(time.Date(2026, time.January, 1, 10, 0, 0, 0, time.UTC))

	if err := validateRecurrence(set, set.GetDTStart()); err == nil {
		t.Fatal("expected recurrence without RRULE to fail")
	}
}

func TestValidateRecurrenceRejectsCountAndUntil(t *testing.T) {
	set := mustRecurrence(t, "DTSTART:20260101T100000Z\nRRULE:FREQ=DAILY;COUNT=2;UNTIL=20260110T100000Z")

	if err := validateRecurrence(set, set.GetDTStart()); err == nil {
		t.Fatal("expected recurrence with COUNT and UNTIL to fail")
	}
}

func TestRecurrenceBetweenExcludesEnd(t *testing.T) {
	set := mustRecurrence(t, "DTSTART:20260101T100000Z\nRRULE:FREQ=DAILY;COUNT=3")
	start := set.GetDTStart()

	got := recurrenceBetween(set, start, start.Add(2*24*time.Hour))
	if len(got) != 2 {
		t.Fatalf("got %d occurrences, want 2", len(got))
	}
	if !got[0].Equal(start) || !got[1].Equal(start.Add(24*time.Hour)) {
		t.Fatalf("unexpected occurrences: %v", got)
	}
}

func TestRecurrenceIsUnbounded(t *testing.T) {
	unbounded := mustRecurrence(t, "DTSTART:20260101T100000Z\nRRULE:FREQ=WEEKLY")
	bounded := mustRecurrence(t, "DTSTART:20260101T100000Z\nRRULE:FREQ=WEEKLY;COUNT=3")

	if !recurrenceIsUnbounded(unbounded) {
		t.Fatal("expected recurrence without COUNT or UNTIL to be unbounded")
	}
	if recurrenceIsUnbounded(bounded) {
		t.Fatal("expected recurrence with COUNT to be bounded")
	}
	if recurrenceIsUnbounded(nil) {
		t.Fatal("nil recurrence must not be unbounded")
	}
}

func TestSplitTimes(t *testing.T) {
	cutoff := time.Date(2026, time.January, 2, 10, 0, 0, 0, time.UTC)
	times := []time.Time{
		cutoff.Add(-time.Hour),
		cutoff,
		cutoff.Add(time.Hour),
	}

	before, after := splitTimes(times, cutoff)
	if len(before) != 1 || !before[0].Equal(times[0]) {
		t.Fatalf("unexpected times before cutoff: %v", before)
	}
	if len(after) != 2 || !after[0].Equal(times[1]) || !after[1].Equal(times[2]) {
		t.Fatalf("unexpected times at or after cutoff: %v", after)
	}
}

func mustRecurrence(t *testing.T, value string) *rrule.Set {
	t.Helper()
	set, err := rrule.StrToRRuleSet(value)
	if err != nil {
		t.Fatalf("failed to parse recurrence: %v", err)
	}
	return set
}
