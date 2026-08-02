package e2e

import (
	"testing"

	"github.com/git-calendar/core/pkg/core"
)

func TestUpdateRemote_ReplacesExistingRemote(t *testing.T) {
	c := core.NewCore()

	err := c.CreateCalendar(testCalendarName, "")
	if err != nil {
		t.Fatalf("failed to create calendar: %v", err)
	}

	t.Cleanup(func() {
		_ = c.RemoveCalendar(testCalendarName)
	})

	oldCalRemote := "https://github.com/git-calendar/old-calendar.git"
	newCalRemote := "https://github.com/git-calendar/new-calendar.git"

	err = c.UpdateRemote(testCalendarName, mustParseUrl(oldCalRemote), false)
	if err != nil {
		t.Fatalf("failed to set initial remotes: %v", err)
	}

	err = c.UpdateRemote(testCalendarName, mustParseUrl(newCalRemote), true)
	if err != nil {
		t.Fatalf("failed to replace remotes: %v", err)
	}
	if err := c.LoadCalendars(); err != nil {
		t.Fatalf("failed to reload calendars: %v", err)
	}

	calendars, err := c.ListCalendars()
	if err != nil {
		t.Fatalf("failed to list calendars: %v", err)
	}

	var found bool
	for _, calendar := range calendars {
		if calendar.Name != testCalendarName {
			continue
		}
		remoteUrl, err := calendar.RemoteURL()
		if err != nil {
			t.Fatalf("failed to get remote url: %v", err)
		}
		if remoteUrl != newCalRemote {
			t.Fatalf("remote url mismatch: got %v, want %v", remoteUrl, newCalRemote)
		}
		if !calendar.Readonly {
			t.Fatal("calendar is not read-only after updating the remote")
		}
		found = true
		break
	}

	if !found {
		t.Errorf("calendar %q not found in list", testCalendarName)
	}
}

func TestUpdateRemote_DeletesWhenEmpty(t *testing.T) {
	c := core.NewCore()

	err := c.CreateCalendar(testCalendarName, "")
	if err != nil {
		t.Fatalf("failed to create calendar: %v", err)
	}

	t.Cleanup(func() {
		_ = c.RemoveCalendar(testCalendarName)
	})

	err = c.UpdateRemote(testCalendarName, mustParseUrl("https://github.com/git-calendar/calendar.git"), true)
	if err != nil {
		t.Fatalf("failed to set remotes: %v", err)
	}

	err = c.UpdateRemote(testCalendarName, nil, false)
	if err != nil {
		t.Fatalf("failed to clear remotes: %v", err)
	}
	if err := c.LoadCalendars(); err != nil {
		t.Fatalf("failed to reload calendars: %v", err)
	}

	calendars, err := c.ListCalendars()
	if err != nil {
		t.Fatalf("failed to list calendars: %v", err)
	}

	var found bool
	for _, calendar := range calendars {
		if calendar.Name != testCalendarName {
			continue
		}
		remoteUrl, err := calendar.RemoteURL()
		if err != nil {
			t.Fatalf("failed to get remote url: %v", err)
		}
		if remoteUrl != "" {
			t.Fatalf("remote url mismatch: got %v, want \"\"", remoteUrl)
		}
		if calendar.Readonly {
			t.Fatal("calendar is still read-only after clearing the remote")
		}
		found = true
		break
	}

	if !found {
		t.Errorf("calendar %q not found in list", testCalendarName)
	}
}

func TestUpdateRemote_MissingCalendar(t *testing.T) {
	c := core.NewCore()

	err := c.RemoveCalendar(testCalendarName)
	if err != nil {
		t.Fatal(err)
	}

	err = c.UpdateRemote(testCalendarName, mustParseUrl("https://github.com/git-calendar/calendar.git"), false)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}
