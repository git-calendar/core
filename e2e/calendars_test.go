package e2e

import (
	"os"
	"path/filepath"
	"slices"
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

	calendars, err := c.ListCalendars()
	if err != nil {
		t.Fatalf("failed to list calendars: %v", err)
	}

	var found bool
	for _, calendar := range calendars {
		if calendar.Name != TestCalendarName {
			continue
		}

		if calendar.Encrypted {
			t.Fatal("calendar should not be encrypted")
		}
		if len(calendar.Tags) != 0 {
			t.Fatal("there should be no tags")
		}
		if len(calendar.Remotes) != 0 {
			t.Fatal("there should be no remotes")
		}

		found = true
		break
	}

	if !found {
		t.Errorf("calendar %q not found in list", TestCalendarName)
	}
}

func TestListCalendars_WithRemotes(t *testing.T) {
	c := core.NewCore()

	err := c.CreateCalendar(TestCalendarName, "")
	if err != nil {
		t.Fatalf("failed to create calendar: %v", err)
	}

	t.Cleanup(func() {
		_ = c.RemoveCalendar(TestCalendarName)
	})

	remotes := []string{"https://github.com/git-calendar/calendar.git", "https://gitlab.com/git-calendar/calendar2.git"}
	err = c.UpdateRemotes(TestCalendarName, remotes...)
	if err != nil {
		t.Fatalf("failed to update remotes: %v", err)
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

		if !slices.Equal(calendar.Remotes, remotes) {
			t.Fatalf("remotes mismatch: got %v, want %v", calendar.Remotes, remotes)
		}

		found = true
		break
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

func TestRenameCalendar(t *testing.T) {
	c := core.NewCore()

	oldName := TestCalendarName + "_old"
	newName := TestCalendarName + "_new"

	err := c.CreateCalendar(oldName, "")
	if err != nil {
		t.Fatalf("failed to create calendar: %v", err)
	}

	t.Cleanup(func() {
		_ = c.RemoveCalendar(oldName)
	})

	err = c.RenameCalendar(oldName, newName)
	if err != nil {
		t.Fatalf("failed to rename calendar: %v", err)
	}

	t.Cleanup(func() {
		_ = c.RemoveCalendar(newName)
	})

	home, err := os.UserHomeDir()
	if err != nil {
		t.Errorf("failed to get home dir: %v", err)
	}

	oldCalendarPath := filepath.Join(home, filesystem.DirName, oldName)
	newCalendarPath := filepath.Join(home, filesystem.DirName, newName)

	_, err = os.Stat(oldCalendarPath)
	if err == nil {
		t.Error("old calendar directory still exists")
	}

	if err != nil && !os.IsNotExist(err) {
		t.Errorf("failed to check old calendar directory: %v", err)
	}

	_, err = os.Stat(newCalendarPath)
	if err != nil {
		t.Errorf("new calendar directory does not exist: %v", err)
	}
}

func TestRenameCalendar_SameName(t *testing.T) {
	c := core.NewCore()

	err := c.CreateCalendar(TestCalendarName, "")
	if err != nil {
		t.Fatalf("failed to create calendar: %v", err)
	}
	t.Cleanup(func() {
		_ = c.RemoveCalendar(TestCalendarName)
	})

	err = c.RenameCalendar(TestCalendarName, TestCalendarName)
	if err != nil {
		t.Fatalf("renaming calendar to same name should not fail: %v", err)
	}

	home, err := os.UserHomeDir()
	if err != nil {
		t.Errorf("failed to get home dir: %v", err)
	}

	calendarPath := filepath.Join(home, filesystem.DirName, TestCalendarName)

	_, err = os.Stat(calendarPath)
	if err != nil {
		t.Errorf("calendar directory should still exist: %v", err)
	}
}

func TestRenameCalendar_MissingCalendar(t *testing.T) {
	c := core.NewCore()

	c.RemoveCalendar(TestCalendarName)

	err := c.RenameCalendar(TestCalendarName, "new-calendar")
	if err == nil {
		t.Fatal("expected error when renaming missing calendar")
	}
}

func TestRenameCalendar_AlreadyExists(t *testing.T) {
	c := core.NewCore()

	oldName := TestCalendarName + "_old"
	newName := TestCalendarName + "_new"

	err := c.CreateCalendar(oldName, "")
	if err != nil {
		t.Fatalf("failed to create old calendar: %v", err)
	}

	t.Cleanup(func() {
		_ = c.RemoveCalendar(oldName)
	})

	err = c.CreateCalendar(newName, "")
	if err != nil {
		t.Fatalf("failed to create new calendar: %v", err)
	}

	t.Cleanup(func() {
		_ = c.RemoveCalendar(newName)
	})

	err = c.RenameCalendar(oldName, newName)
	if err == nil {
		t.Fatal("expected error when renaming calendar to existing name")
	}

	home, err := os.UserHomeDir()
	if err != nil {
		t.Errorf("failed to get home dir: %v", err)
	}

	oldCalendarPath := filepath.Join(home, filesystem.DirName, oldName)
	newCalendarPath := filepath.Join(home, filesystem.DirName, newName)

	_, err = os.Stat(oldCalendarPath)
	if err != nil {
		t.Errorf("old calendar directory should still exist: %v", err)
	}

	_, err = os.Stat(newCalendarPath)
	if err != nil {
		t.Errorf("existing new calendar directory should still exist: %v", err)
	}
}
