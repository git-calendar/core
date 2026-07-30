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
	t.Setenv("HOME", t.TempDir())

	const calendar = "test-ical-file"
	c := core.NewCore()
	if err := c.CreateCalendar(calendar, ""); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = c.RemoveCalendar(calendar) })

	if err := c.ImportICalFile(calendar, nil, strings.NewReader(icalFeed("Imported event"))); err != nil {
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

func TestImportICalURLCachesUntilSync(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	const (
		name        = "test-ical-url"
		renamedName = "test-ical-url-renamed"
	)

	var feed atomic.Value
	feed.Store(icalFeed("First title"))
	var requests atomic.Int32
	var offline atomic.Bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		if offline.Load() {
			http.Error(w, "offline", http.StatusServiceUnavailable)
			return
		}
		_, _ = w.Write([]byte(feed.Load().(string)))
	}))
	defer server.Close()

	sourceURL, err := url.Parse(server.URL + "/calendar.ics")
	if err != nil {
		t.Fatal(err)
	}

	c := core.NewCore()
	if err := c.ImportICalURL(name, sourceURL); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = c.RemoveCalendar(name)
		_ = c.RemoveCalendar(renamedName)
	})

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
	found := false
	for _, calendar := range calendars {
		if calendar.Name != name {
			continue
		}
		found = true
		if !calendar.Readonly {
			t.Fatal("URL calendar is not read-only")
		}
	}
	if !found {
		t.Fatal("URL calendar was not registered")
	}

	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}
	calendarRoot := filepath.Join(home, filesystem.DirName)
	if _, err := os.Stat(filepath.Join(calendarRoot, name)); !os.IsNotExist(err) {
		t.Fatalf("unsuffixed URL file exists or could not be checked: %v", err)
	}
	urlFilePath := filepath.Join(calendarRoot, name+".url")
	data, err := os.ReadFile(urlFilePath)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != sourceURL.String() {
		t.Errorf("URL file = %q, want %q", data, sourceURL)
	}
	icalFilePath := filepath.Join(calendarRoot, name+core.ICalFileSuffix)
	if data, err := os.ReadFile(icalFilePath); err != nil {
		t.Fatal(err)
	} else if string(data) != icalFeed("First title") {
		t.Errorf("cached iCalendar = %q", data)
	}

	feed.Store(icalFeed("Second title"))
	offline.Store(true)
	if err := c.LoadCalendars(); err != nil {
		t.Fatal(err)
	}

	events = importedEvents(c, name)
	if len(events) != 1 || events[0].Title != "First title" {
		t.Fatalf("cached URL import = %+v", events)
	}
	if requests.Load() != 1 {
		t.Errorf("URL was fetched %d times during load, want 1", requests.Load())
	}

	offline.Store(false)
	if err := c.SyncAll(); err != nil {
		t.Fatal(err)
	}
	events = importedEvents(c, name)
	if len(events) != 1 || events[0].Title != "Second title" {
		t.Fatalf("synced URL import = %+v", events)
	}
	if events[0].Id != id {
		t.Errorf("event ID changed after sync: got %s, want %s", events[0].Id, id)
	}
	if requests.Load() != 2 {
		t.Errorf("URL was fetched %d times, want 2", requests.Load())
	}

	if err := c.RenameCalendar(name, renamedName); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(urlFilePath); !os.IsNotExist(err) {
		t.Fatalf("old URL file still exists or could not be checked after rename: %v", err)
	}
	if _, err := os.Stat(icalFilePath); !os.IsNotExist(err) {
		t.Fatalf("old cached iCalendar still exists or could not be checked after rename: %v", err)
	}
	renamedURLFilePath := filepath.Join(calendarRoot, renamedName+".url")
	if data, err := os.ReadFile(renamedURLFilePath); err != nil {
		t.Fatal(err)
	} else if string(data) != sourceURL.String() {
		t.Errorf("renamed URL file = %q, want %q", data, sourceURL)
	}
	renamedICalFilePath := filepath.Join(calendarRoot, renamedName+core.ICalFileSuffix)
	if data, err := os.ReadFile(renamedICalFilePath); err != nil {
		t.Fatal(err)
	} else if string(data) != icalFeed("Second title") {
		t.Errorf("renamed cached iCalendar = %q", data)
	}
	if requests.Load() != 2 {
		t.Errorf("URL was fetched %d times after rename, want 2", requests.Load())
	}

	if err := c.RemoveCalendar(renamedName); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(renamedURLFilePath); !os.IsNotExist(err) {
		t.Fatalf("URL file still exists or could not be checked after removal: %v", err)
	}
	if _, err := os.Stat(renamedICalFilePath); !os.IsNotExist(err) {
		t.Fatalf("cached iCalendar still exists or could not be checked after removal: %v", err)
	}
}

func importedEvents(c *core.Core, calendar string) []core.Event {
	from := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	return c.GetEvents(from, to, core.GetEventsFilter{calendar: {}})
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
