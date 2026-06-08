package e2e

import (
	"testing"

	"github.com/git-calendar/core/pkg/core"
)

func TestUpdateRemote_ReplacesExistingRemote(t *testing.T) {
	c := core.NewCore()

	err := c.CreateCalendar(TestCalendarName, "")
	if err != nil {
		t.Fatalf("failed to create calendar: %v", err)
	}

	t.Cleanup(func() {
		_ = c.RemoveCalendar(TestCalendarName)
	})

	oldCalRemote := "https://github.com/git-calendar/old-calendar.git"
	newCalRemote := "https://github.com/git-calendar/new-calendar.git"

	err = c.UpdateRemote(TestCalendarName, mustParseUrl(oldCalRemote))
	if err != nil {
		t.Fatalf("failed to set initial remotes: %v", err)
	}

	err = c.UpdateRemote(TestCalendarName, mustParseUrl(newCalRemote))
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
		if calendar.RemoteUrl != newCalRemote {
			t.Fatalf("remote url mismatch: got %v, want %v", calendar.RemoteUrl, newCalRemote)
		}
		found = true
		break
	}

	if !found {
		t.Errorf("calendar %q not found in list", TestCalendarName)
	}
}

func TestUpdateRemote_DeletesWhenEmpty(t *testing.T) {
	c := core.NewCore()

	err := c.CreateCalendar(TestCalendarName, "")
	if err != nil {
		t.Fatalf("failed to create calendar: %v", err)
	}

	t.Cleanup(func() {
		_ = c.RemoveCalendar(TestCalendarName)
	})

	err = c.UpdateRemote(TestCalendarName, mustParseUrl("https://github.com/git-calendar/calendar.git"))
	if err != nil {
		t.Fatalf("failed to set remotes: %v", err)
	}

	err = c.UpdateRemote(TestCalendarName, nil)
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
		if calendar.RemoteUrl != "" {
			t.Fatalf("remote url mismatch: got %v, want \"\"", calendar.RemoteUrl)
		}
		found = true
		break
	}

	if !found {
		t.Errorf("calendar %q not found in list", TestCalendarName)
	}
}

func TestUpdateRemote_MissingCalendar(t *testing.T) {
	c := core.NewCore()

	err := c.RemoveCalendar(TestCalendarName)
	if err != nil {
		t.Fatal(err)
	}

	err = c.UpdateRemote(TestCalendarName, mustParseUrl("https://github.com/git-calendar/calendar.git"))
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}
