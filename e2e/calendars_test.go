package e2e

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"testing"
	"time"
	"uuid"

	"github.com/git-calendar/core/pkg/core"
	"github.com/git-calendar/core/pkg/filesystem"
	gogit "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/object"
)

const testCalendarName = "test"

func TestCreateCalendar(t *testing.T) {
	c := core.NewCore()

	err := c.CreateCalendar(testCalendarName, "")
	if err != nil {
		t.Fatalf("failed to init repo: %v", err)
	}

	t.Cleanup(func() {
		_ = c.RemoveCalendar(testCalendarName)
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
		if d.Name() == testCalendarName {
			found = true
			break
		}
	}
	if !found {
		t.Error("directory not found")
	}
}

func TestCloneCalendarLoadsEventsImmediately(t *testing.T) {
	remotePath := filepath.Join(t.TempDir(), "cloned-calendar.git")
	repo, err := gogit.PlainInit(remotePath, false)
	if err != nil {
		t.Fatalf("failed to initialize source repository: %v", err)
	}
	wt, err := repo.Worktree()
	if err != nil {
		t.Fatalf("failed to get source worktree: %v", err)
	}
	if err := wt.Filesystem.MkdirAll(core.EventsDirName, 0o755); err != nil {
		t.Fatalf("failed to create source events directory: %v", err)
	}

	start := time.Date(2026, time.January, 1, 10, 0, 0, 0, time.UTC)
	event := core.Event{
		ID:        uuid.New(),
		Title:     "Cloned event",
		From:      start,
		To:        start.Add(time.Hour),
		Calendar:  "cloned-calendar",
		UpdatedAt: start,
	}
	filePath := filepath.Join(core.EventsDirName, event.ID.String()+".json")
	file, err := wt.Filesystem.Create(filePath)
	if err != nil {
		t.Fatalf("failed to create source event file: %v", err)
	}
	if err := event.WriteToFile(file, nil); err != nil {
		_ = file.Close()
		t.Fatalf("failed to write source event: %v", err)
	}
	if err := file.Close(); err != nil {
		t.Fatalf("failed to close source event: %v", err)
	}
	if _, err := wt.Add(filePath); err != nil {
		t.Fatalf("failed to stage source event: %v", err)
	}
	if _, err := wt.Commit("add event", &gogit.CommitOptions{
		Author: &object.Signature{Name: "Test", When: start},
	}); err != nil {
		t.Fatalf("failed to commit source event: %v", err)
	}

	c := core.NewCore()
	remoteURL := &url.URL{Scheme: "file", Path: remotePath}
	if err := c.CloneCalendar(remoteURL, "", false); err != nil {
		t.Fatalf("failed to clone calendar: %v", err)
	}
	t.Cleanup(func() {
		_ = c.RemoveCalendar("cloned-calendar")
	})

	got, err := c.GetEvent(event.ID)
	if err != nil {
		t.Fatalf("cloned event was not loaded immediately: %v", err)
	}
	if got.Title != event.Title || !got.From.Equal(event.From) || !got.To.Equal(event.To) {
		t.Fatalf("cloned event = %+v, want %+v", got, event)
	}
	if got.Calendar != "cloned-calendar" {
		t.Fatalf("cloned event calendar = %q, want %q", got.Calendar, "cloned-calendar")
	}
}

func TestListCalendars(t *testing.T) {
	c := core.NewCore()

	err := c.CreateCalendar(testCalendarName, "")
	if err != nil {
		t.Fatalf("failed to create calendar: %v", err)
	}

	t.Cleanup(func() {
		_ = c.RemoveCalendar(testCalendarName)
	})

	calendars, err := c.ListCalendars()
	if err != nil {
		t.Fatalf("failed to list calendars: %v", err)
	}

	var found bool
	for _, calendar := range calendars {
		if calendar.Name != testCalendarName {
			continue
		}

		if calendar.IsEncrypted() {
			t.Fatal("calendar should not be encrypted")
		}
		if len(calendar.Tags) != 0 {
			t.Fatal("there should be no tags")
		}

		found = true
		break
	}

	if !found {
		t.Errorf("calendar %q not found in list", testCalendarName)
	}
}

func TestListCalendars_WithRemote(t *testing.T) {
	c := core.NewCore()

	err := c.CreateCalendar(testCalendarName, "")
	if err != nil {
		t.Fatalf("failed to create calendar: %v", err)
	}

	t.Cleanup(func() {
		_ = c.RemoveCalendar(testCalendarName)
	})

	remoteURL := "https://github.com/git-calendar/calendar.git"

	err = c.UpdateRemote(testCalendarName, mustParseURL(remoteURL), false)
	if err != nil {
		t.Fatalf("failed to update remotes: %v", err)
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
		rurl, err := calendar.RemoteURL()
		if err != nil {
			t.Fatalf("failed to get remote url: %v", err)
		}
		if rurl != remoteURL {
			t.Fatalf("remote url mismatch: got %v, want %v", rurl, remoteURL)
		}
		found = true
		break
	}

	if !found {
		t.Errorf("calendar %q not found in list", testCalendarName)
	}
}

func TestRemoveCalendar(t *testing.T) {
	c := core.NewCore()

	err := c.CreateCalendar(testCalendarName, "")
	if err != nil {
		t.Fatalf("failed to create calendar: %v", err)
	}
	if err := c.UpdateRemote(testCalendarName, mustParseURL("https://example.com/calendar.git"), true); err != nil {
		t.Fatalf("failed to make calendar read-only: %v", err)
	}

	err = c.RemoveCalendar(testCalendarName)
	if err != nil {
		t.Fatalf("failed to remove calendar: %v", err)
	}

	home, err := os.UserHomeDir()
	if err != nil {
		t.Errorf("failed to get home dir: %v", err)
	}

	calendarPath := filepath.Join(home, filesystem.DirName, testCalendarName)

	_, err = os.Stat(calendarPath)
	if err == nil {
		t.Error("calendar directory still exists")
	}

	if err != nil && !os.IsNotExist(err) {
		t.Errorf("failed to check calendar directory: %v", err)
	}

	readonlyPath := filepath.Join(home, filesystem.DirName, testCalendarName+core.ReadonlyFileSuffix)
	if _, err := os.Stat(readonlyPath); !os.IsNotExist(err) {
		t.Errorf("read-only marker still exists or could not be checked: %v", err)
	}
}

func TestRenameCalendar(t *testing.T) {
	c := core.NewCore()

	oldName := testCalendarName + "_old"
	newName := testCalendarName + "_new"

	err := c.CreateCalendar(oldName, "")
	if err != nil {
		t.Fatalf("failed to create calendar: %v", err)
	}

	t.Cleanup(func() {
		_ = c.RemoveCalendar(oldName)
	})

	start := time.Date(2026, 1, 1, 9, 0, 0, 0, time.UTC)
	event, err := c.CreateEvent(core.Event{
		ID:       uuid.New(),
		Title:    "Renamed calendar event",
		From:     start,
		To:       start.Add(time.Hour),
		Calendar: oldName,
	})
	if err != nil {
		t.Fatalf("failed to create event: %v", err)
	}

	err = c.RenameCalendar(oldName, newName)
	if err != nil {
		t.Fatalf("failed to rename calendar: %v", err)
	}

	t.Cleanup(func() {
		_ = c.RemoveCalendar(newName)
	})

	renamedEvent, err := c.GetEvent(event.ID)
	if err != nil {
		t.Fatalf("failed to get event after rename: %v", err)
	}
	if renamedEvent.Calendar != newName {
		t.Fatalf("event calendar = %q, want %q", renamedEvent.Calendar, newName)
	}

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

	err := c.CreateCalendar(testCalendarName, "")
	if err != nil {
		t.Fatalf("failed to create calendar: %v", err)
	}
	t.Cleanup(func() {
		_ = c.RemoveCalendar(testCalendarName)
	})

	err = c.RenameCalendar(testCalendarName, testCalendarName)
	if err != nil {
		t.Fatalf("renaming calendar to same name should not fail: %v", err)
	}

	home, err := os.UserHomeDir()
	if err != nil {
		t.Errorf("failed to get home dir: %v", err)
	}

	calendarPath := filepath.Join(home, filesystem.DirName, testCalendarName)

	_, err = os.Stat(calendarPath)
	if err != nil {
		t.Errorf("calendar directory should still exist: %v", err)
	}
}

func TestRenameCalendar_MissingCalendar(t *testing.T) {
	c := core.NewCore()

	c.RemoveCalendar(testCalendarName)

	err := c.RenameCalendar(testCalendarName, "new-calendar")
	if err == nil {
		t.Fatal("expected error when renaming missing calendar")
	}
}

func TestRenameCalendar_AlreadyExists(t *testing.T) {
	c := core.NewCore()

	oldName := testCalendarName + "_old"
	newName := testCalendarName + "_new"

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

// Helper
func mustParseURL(raw string) *url.URL {
	u, err := url.Parse(raw)
	if err != nil {
		panic(fmt.Sprintf("failed to parse url: %v", err))
	}
	return u
}
