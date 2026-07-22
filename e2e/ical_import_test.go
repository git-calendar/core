package e2e

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/git-calendar/core/pkg/core"
	"github.com/git-calendar/core/pkg/filesystem"
)

func TestImportICalFilePersistsEvents(t *testing.T) {
	const calendar = "test-ical-file"
	c := core.NewCore()
	if err := c.CreateCalendar(calendar, ""); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = c.RemoveCalendar(calendar) })

	if err := c.ImportICalFile(calendar, strings.NewReader(icalFeed("Imported event"))); err != nil {
		t.Fatal(err)
	}

	events := importedEvents(c, calendar)
	if len(events) != 1 {
		t.Fatalf("got %d imported events, want 1", len(events))
	}
	if events[0].Id.Version() != 4 {
		t.Errorf("event ID version = %d, want 4", events[0].Id.Version())
	}
	id := events[0].Id

	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}
	eventPath := filepath.Join(home, filesystem.DirName, calendar, core.EventsDirName, id.String()+".json")
	if _, err := os.Stat(eventPath); err != nil {
		t.Fatalf("imported event file was not saved: %v", err)
	}

	if err := c.LoadCalendars(); err != nil {
		t.Fatal(err)
	}
	events = importedEvents(c, calendar)
	if len(events) != 1 || events[0].Id != id {
		t.Fatalf("imported event did not survive reload: %+v", events)
	}
}

func TestImportICalURLRefetchesOnLoad(t *testing.T) {
	const name = "test-ical-url"

	var feed atomic.Value
	feed.Store(icalFeed("First title"))
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		_, _ = w.Write([]byte(feed.Load().(string)))
	}))
	defer server.Close()

	sourceURL, err := url.Parse(server.URL)
	if err != nil {
		t.Fatal(err)
	}

	c := core.NewCore()
	if err := c.ImportICalURL(name, sourceURL); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = c.RemoveCalendar(name) })

	events := importedEvents(c, name)
	if len(events) != 1 || events[0].Title != "First title" {
		t.Fatalf("first URL import = %+v", events)
	}
	if events[0].Id.Version() != 8 {
		t.Errorf("event ID version = %d, want 8", events[0].Id.Version())
	}
	id := events[0].Id

	calendars, err := c.ListCalendars()
	if err != nil {
		t.Fatal(err)
	}
	for _, calendar := range calendars {
		if calendar.Name == name && !calendar.Readonly {
			t.Fatal("URL calendar is not read-only")
		}
	}

	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(home, filesystem.DirName, name))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != server.URL {
		t.Errorf("URL file = %q, want %q", data, server.URL)
	}

	feed.Store(icalFeed("Second title"))
	if err := c.LoadCalendars(); err != nil {
		t.Fatal(err)
	}

	events = importedEvents(c, name)
	if len(events) != 1 || events[0].Title != "Second title" {
		t.Fatalf("refetched URL import = %+v", events)
	}
	if events[0].Id != id {
		t.Errorf("event ID changed after refetch: got %s, want %s", events[0].Id, id)
	}
	if requests.Load() != 2 {
		t.Errorf("URL was fetched %d times, want 2", requests.Load())
	}
}

func importedEvents(c *core.Core, calendar string) []core.Event {
	from := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	return c.GetEvents(from, to, core.GetEventsFilter{calendar: nil})
}

func icalFeed(title string) string {
	return fmt.Sprintf(`BEGIN:VCALENDAR
VERSION:2.0
PRODID:-//git-calendar//test//EN
BEGIN:VEVENT
UID:stable@example.com
DTSTART:20260714T100000Z
DTEND:20260714T110000Z
SUMMARY:%s
END:VEVENT
END:VCALENDAR`, title)
}
