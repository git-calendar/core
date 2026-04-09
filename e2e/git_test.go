package e2e

import (
	"testing"

	"github.com/git-calendar/core/pkg/core"
)

func Test_AddRemote_Works(t *testing.T) {
	c := core.NewCore()

	err := c.RemoveCalendar(TestCalendarName)
	if err != nil {
		t.Errorf("failed to delete existing repo: %v", err)
	}

	err = c.CreateCalendar(TestCalendarName, "")
	if err != nil {
		t.Errorf("failed to init repo: %v", err)
	}

	err = c.AddRemote(TestCalendarName, "github", "https://github.com/git-calendar/core.git")
	if err != nil {
		t.Errorf("failed to add remote: %v", err)
	}

	err = c.AddRemote(TestCalendarName, "github", "foo")
	if err == nil {
		t.Errorf("expected an error after adding an existing remote")
	}

	err = c.AddRemote(TestCalendarName, "foo", "invalid url bla bla")
	if err == nil {
		t.Errorf("expected an error after adding an invalid url")
	}

	err = c.AddRemote(TestCalendarName, "bar", "https://github.com/git-calendar/core")
	if err == nil {
		t.Errorf("expected an error after adding an non-git url")
	}
}
