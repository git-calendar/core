package e2e

import (
	"strings"
	"testing"
	"time"

	ics "github.com/arran4/golang-ical"
	"github.com/git-calendar/core/pkg/core"
)

func TestExportICalExportsOnlyTheNamedCalendar(t *testing.T) {
	const (
		firstCalendar  = "test-ical-export-first"
		secondCalendar = "test-ical-export-second"
	)
	calendarCore := core.NewCore()
	for _, name := range []string{firstCalendar, secondCalendar} {
		if err := calendarCore.CreateCalendar(name, ""); err != nil {
			t.Fatal(err)
		}
		name := name
		t.Cleanup(func() { _ = calendarCore.RemoveCalendar(name) })
	}

	start := time.Date(2026, time.August, 16, 10, 0, 0, 0, time.UTC)
	for _, event := range []core.Event{
		{Title: "First event", From: start, To: start.Add(time.Hour), Calendar: firstCalendar},
		{Title: "Second event", From: start, To: start.Add(time.Hour), Calendar: secondCalendar},
	} {
		if _, err := calendarCore.CreateEvent(event); err != nil {
			t.Fatal(err)
		}
	}

	data, err := calendarCore.ExportICal(firstCalendar)
	if err != nil {
		t.Fatal(err)
	}
	exported, err := ics.ParseCalendar(strings.NewReader(string(data)))
	if err != nil {
		t.Fatal(err)
	}
	if len(exported.Events()) != 1 {
		t.Fatalf("got %d exported events, want 1", len(exported.Events()))
	}
	if summary := exported.Events()[0].GetProperty(ics.ComponentPropertySummary); summary == nil || summary.Value != "First event" {
		t.Fatalf("exported SUMMARY = %+v, want First event", summary)
	}
}

func TestExportICalEmptyCalendar(t *testing.T) {
	const name = "test-ical-export-empty"
	calendarCore := core.NewCore()
	if err := calendarCore.CreateCalendar(name, ""); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = calendarCore.RemoveCalendar(name) })

	data, err := calendarCore.ExportICal(name)
	if err != nil {
		t.Fatal(err)
	}
	exported, err := ics.ParseCalendar(strings.NewReader(string(data)))
	if err != nil {
		t.Fatal(err)
	}
	if len(exported.Events()) != 0 {
		t.Fatalf("got %d events, want 0", len(exported.Events()))
	}
}

func TestExportICalRejectsUnknownCalendar(t *testing.T) {
	calendarCore := core.NewCore()
	if _, err := calendarCore.ExportICal("missing"); err == nil {
		t.Fatal("expected unknown calendar export to fail")
	}
}
