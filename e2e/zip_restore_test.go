package e2e

import (
	"archive/zip"
	"bytes"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/git-calendar/core/pkg/core"
	"github.com/google/uuid"
)

func TestRestoreZipRoundTrip(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	const calendar = "restored-calendar"
	source := core.NewCore()
	if err := source.CreateCalendar(calendar, "password"); err != nil {
		t.Fatal(err)
	}
	tag, err := source.CreateTag(calendar, core.Tag{ID: uuid.New(), Name: "Work", Color: "blue"})
	if err != nil {
		t.Fatal(err)
	}
	start := time.Date(2026, time.January, 1, 10, 0, 0, 0, time.UTC)
	event, err := source.CreateEvent(core.Event{
		ID:       uuid.New(),
		Title:    "Restored event",
		From:     start,
		To:       start.Add(time.Hour),
		Calendar: calendar,
		TagID:    &tag.ID,
	})
	if err != nil {
		t.Fatal(err)
	}

	backup, err := source.ExportZip("")
	if err != nil {
		t.Fatal(err)
	}

	t.Setenv("HOME", t.TempDir())
	restored := core.NewCore()
	if err := restored.RestoreZip(backup); err != nil {
		t.Fatalf("RestoreZip failed: %v", err)
	}
	if err := restored.CreateCalendar("removed-by-restore", ""); err != nil {
		t.Fatal(err)
	}
	if err := restored.RestoreZip(backup); err != nil {
		t.Fatalf("second RestoreZip failed: %v", err)
	}

	calendars, err := restored.ListCalendars()
	if err != nil {
		t.Fatal(err)
	}
	if len(calendars) != 1 || calendars[0].Name != calendar || !calendars[0].IsEncrypted() {
		t.Fatalf("restored calendars = %+v", calendars)
	}
	if len(calendars[0].Tags) != 1 || calendars[0].Tags[0].ID != tag.ID {
		t.Fatalf("restored tags = %+v", calendars[0].Tags)
	}

	got, err := restored.GetEvent(event.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Title != event.Title || got.TagID == nil || *got.TagID != tag.ID {
		t.Fatalf("restored event = %+v", got)
	}
}

func TestRestoreZipRejectsTraversalBeforeWriting(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	c := core.NewCore()
	if err := c.CreateCalendar("preserved", ""); err != nil {
		t.Fatal(err)
	}

	if err := c.RestoreZip(traversalZip(t)); err == nil {
		t.Fatal("expected traversal archive to fail")
	}
	calendars, err := c.ListCalendars()
	if err != nil {
		t.Fatal(err)
	}
	if len(calendars) != 1 || calendars[0].Name != "preserved" {
		t.Fatalf("invalid restore changed calendars: %+v", calendars)
	}
	if _, err := os.Stat(filepath.Join(home, "outside")); !os.IsNotExist(err) {
		t.Fatalf("traversal path exists or could not be checked: %v", err)
	}
}

func traversalZip(t *testing.T) []byte {
	t.Helper()

	var data bytes.Buffer
	func() {
		zw := zip.NewWriter(&data)
		defer zw.Close()

		valid, err := zw.Create("calendar/events/event.json")
		if err != nil {
			t.Fatal(err)
		}
		if _, err := valid.Write([]byte("{}")); err != nil {
			t.Fatal(err)
		}
		outside, err := zw.Create("../outside")
		if err != nil {
			t.Fatal(err)
		}
		if _, err := outside.Write([]byte("bad")); err != nil {
			t.Fatal(err)
		}
	}()
	return data.Bytes()
}
