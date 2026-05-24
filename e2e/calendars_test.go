package e2e

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/git-calendar/core/pkg/core"
	"github.com/git-calendar/core/pkg/filesystem"
)

func TestCreateCalendar(t *testing.T) {
	c := core.NewCore()

	err := c.CreateCalendar(TestCalendarName, "")
	if err != nil {
		t.Fatalf("failed to init repo: %v", err)
	}

	t.Cleanup(func() {
		_ = c.RemoveCalendar(TestCalendarName)
	})

	home, err := os.UserHomeDir()
	if err != nil {
		t.Errorf("failed to get home dir: %v", err)
	}

	dirs, err := os.ReadDir(filepath.Join(home, filesystem.DirName))
	if err != nil {
		t.Errorf("failed to read event json file: %v", err)
	}

	var found bool
	for _, d := range dirs {
		if d.Name() == TestCalendarName {
			found = true
			break
		}
	}
	if !found {
		t.Error("directory not found")
	}
}

func TestListCalendars(t *testing.T) {
	c := core.NewCore()

	err := c.CreateCalendar(TestCalendarName, "")
	if err != nil {
		t.Fatalf("failed to create calendar: %v", err)
	}

	t.Cleanup(func() {
		_ = c.RemoveCalendar(TestCalendarName)
	})

	var found bool
	for _, calendar := range c.ListCalendars() {
		if calendar == TestCalendarName {
			found = true
			break
		}
	}

	if !found {
		t.Errorf("calendar %q not found in list", TestCalendarName)
	}
}

func TestRemoveCalendar(t *testing.T) {
	c := core.NewCore()

	err := c.CreateCalendar(TestCalendarName, "")
	if err != nil {
		t.Fatalf("failed to create calendar: %v", err)
	}

	err = c.RemoveCalendar(TestCalendarName)
	if err != nil {
		t.Fatalf("failed to remove calendar: %v", err)
	}

	home, err := os.UserHomeDir()
	if err != nil {
		t.Errorf("failed to get home dir: %v", err)
	}

	calendarPath := filepath.Join(home, filesystem.DirName, TestCalendarName)

	_, err = os.Stat(calendarPath)
	if err == nil {
		t.Error("calendar directory still exists")
	}

	if err != nil && !os.IsNotExist(err) {
		t.Errorf("failed to check calendar directory: %v", err)
	}
}
