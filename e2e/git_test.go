package e2e

import (
	"slices"
	"testing"

	"github.com/git-calendar/core/pkg/core"
)

func TestUpdateRemotes_ReplacesExistingRemotes(t *testing.T) {
	c := core.NewCore()

	err := c.CreateCalendar(TestCalendarName, "")
	if err != nil {
		t.Fatalf("failed to create calendar: %v", err)
	}

	t.Cleanup(func() {
		_ = c.RemoveCalendar(TestCalendarName)
	})

	const oldCalName = "https://github.com/git-calendar/old-calendar.git"
	const newCalName = "https://github.com/git-calendar/new-calendar.git"

	err = c.UpdateRemotes(TestCalendarName, oldCalName)
	if err != nil {
		t.Fatalf("failed to set initial remotes: %v", err)
	}

	err = c.UpdateRemotes(TestCalendarName, newCalName)
	if err != nil {
		t.Fatalf("failed to replace remotes: %v", err)
	}

	calendars, err := c.ListCalendars()
	if err != nil {
		t.Fatalf("failed to list calendars: %v", err)
	}

	var found bool
	for _, calendar := range calendars {
		if calendar.Name != TestCalendarName {
			continue
		}

		want := []string{newCalName}
		if !slices.Equal(calendar.Remotes, want) {
			t.Fatalf("remotes mismatch: got %v, want %v", calendar.Remotes, want)
		}

		found = true
		break
	}

	if !found {
		t.Errorf("calendar %q not found in list", TestCalendarName)
	}
}

func TestUpdateRemotes_DeletesAllWhenEmpty(t *testing.T) {
	c := core.NewCore()

	err := c.CreateCalendar(TestCalendarName, "")
	if err != nil {
		t.Fatalf("failed to create calendar: %v", err)
	}

	t.Cleanup(func() {
		_ = c.RemoveCalendar(TestCalendarName)
	})

	err = c.UpdateRemotes(
		TestCalendarName,
		"https://github.com/git-calendar/calendar.git",
	)
	if err != nil {
		t.Fatalf("failed to set remotes: %v", err)
	}

	err = c.UpdateRemotes(TestCalendarName)
	if err != nil {
		t.Fatalf("failed to clear remotes: %v", err)
	}

	calendars, err := c.ListCalendars()
	if err != nil {
		t.Fatalf("failed to list calendars: %v", err)
	}

	var found bool
	for _, calendar := range calendars {
		if calendar.Name != TestCalendarName {
			continue
		}

		if len(calendar.Remotes) != 0 {
			t.Fatalf("expected no remotes, got %v", calendar.Remotes)
		}

		found = true
		break
	}

	if !found {
		t.Errorf("calendar %q not found in list", TestCalendarName)
	}
}

func TestUpdateRemotes_MissingCalendar(t *testing.T) {
	c := core.NewCore()

	err := c.RemoveCalendar(TestCalendarName)
	if err != nil {
		t.Fatal(err)
	}

	err = c.UpdateRemotes(TestCalendarName, "https://github.com/git-calendar/calendar.git")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}
