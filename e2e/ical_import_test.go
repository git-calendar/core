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
	"github.com/git-calendar/core/pkg/errcode"
	"github.com/git-calendar/core/pkg/filesystem"
	gogit "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/object"
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
	if events[0].ID.Version() != 5 {
		t.Errorf("event ID version = %d, want 5", events[0].ID.Version())
	}
	id := events[0].ID

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
	if len(events) != 1 || events[0].ID != id {
		t.Fatalf("imported event did not survive reload: %+v", events)
	}
}

func TestImportICalFileIsIdempotentAndAtomic(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	const calendar = "test-ical-idempotent"
	c := core.NewCore()
	if err := c.CreateCalendar(calendar, ""); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = c.RemoveCalendar(calendar) })

	before := calendarCommitCount(t, calendar)
	feed := multiEventICalFeed("First title")
	if err := c.ImportICalFile(calendar, nil, strings.NewReader(feed)); err != nil {
		t.Fatal(err)
	}
	if got := calendarCommitCount(t, calendar); got != before+1 {
		t.Fatalf("first import created %d commits, want 1", got-before)
	}

	events := importedEvents(c, calendar)
	if len(events) != 2 {
		t.Fatalf("first import returned %d events, want 2: %+v", len(events), events)
	}
	ids := make(map[time.Time]string, len(events))
	for _, event := range events {
		ids[event.From] = event.ID.String()
	}

	if err := c.ImportICalFile(calendar, nil, strings.NewReader(feed)); err != nil {
		t.Fatal(err)
	}
	if got := calendarCommitCount(t, calendar); got != before+1 {
		t.Fatalf("identical reimport changed commit count to %d, want %d", got, before+1)
	}
	events = importedEvents(c, calendar)
	if len(events) != 2 {
		t.Fatalf("identical reimport returned %d events, want 2: %+v", len(events), events)
	}
	for _, event := range events {
		if event.ID.String() != ids[event.From] {
			t.Fatalf("event at %s changed ID from %s to %s", event.From, ids[event.From], event.ID)
		}
	}

	if err := c.ImportICalFile(calendar, nil, strings.NewReader(multiEventICalFeed("Updated title"))); err != nil {
		t.Fatal(err)
	}
	if got := calendarCommitCount(t, calendar); got != before+2 {
		t.Fatalf("updated reimport changed commit count to %d, want %d", got, before+2)
	}
	events = importedEvents(c, calendar)
	if len(events) != 2 {
		t.Fatalf("updated reimport returned %d events, want 2: %+v", len(events), events)
	}
	var foundUpdated bool
	for _, event := range events {
		if event.ID.String() != ids[event.From] {
			t.Fatalf("updated event at %s changed ID from %s to %s", event.From, ids[event.From], event.ID)
		}
		if event.Title == "Updated title" {
			foundUpdated = true
		}
	}
	if !foundUpdated {
		t.Fatalf("updated event title was not imported: %+v", events)
	}
}

func TestImportICalFileInvalidFeedDoesNotPartiallyImport(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	const (
		calendar = "test-ical-atomic-error"
		feed     = `BEGIN:VCALENDAR
VERSION:2.0
PRODID:-//git-calendar//test//EN
BEGIN:VEVENT
UID:valid@example.com
DTSTART:20260714T100000Z
DTEND:20260714T110000Z
SUMMARY:Valid event
END:VEVENT
BEGIN:VEVENT
UID:invalid@example.com
DTSTART:20260715T100000Z
SUMMARY:Missing end
END:VEVENT
END:VCALENDAR`
	)

	c := core.NewCore()
	if err := c.CreateCalendar(calendar, ""); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = c.RemoveCalendar(calendar) })

	before := calendarCommitCount(t, calendar)
	if err := c.ImportICalFile(calendar, nil, strings.NewReader(feed)); err == nil {
		t.Fatal("expected invalid iCalendar import to fail")
	}
	if got := calendarCommitCount(t, calendar); got != before {
		t.Fatalf("failed import changed commit count from %d to %d", before, got)
	}
	if events := importedEvents(c, calendar); len(events) != 0 {
		t.Fatalf("failed import left %d events: %+v", len(events), events)
	}

	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}
	eventsPath := filepath.Join(home, filesystem.DirName, calendar, core.EventsDirName)
	entries, err := os.ReadDir(eventsPath)
	if err != nil && !os.IsNotExist(err) {
		t.Fatalf("read events directory: %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("failed import left event files: %+v", entries)
	}
}

func TestUpdateICalURL(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	const name = "test-update-ical-url"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/first.ics":
			_, _ = w.Write([]byte(icalFeed("First title")))
		case "/second.ics":
			_, _ = w.Write([]byte(icalFeed("Second title")))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	firstURL, err := url.Parse(server.URL + "/first.ics")
	if err != nil {
		t.Fatal(err)
	}
	secondURL, err := url.Parse(server.URL + "/second.ics")
	if err != nil {
		t.Fatal(err)
	}

	c := core.NewCore()
	if err := c.ImportICalURL(name, firstURL); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = c.RemoveCalendar(name) })

	if err := c.UpdateICalURL(name, secondURL); err != nil {
		t.Fatal(err)
	}

	events := importedEvents(c, name)
	if len(events) != 1 || events[0].Title != "Second title" {
		t.Fatalf("updated URL import = %+v", events)
	}

	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}
	urlPath := filepath.Join(home, filesystem.DirName, name+core.ICalURLFileSuffix)
	data, err := os.ReadFile(urlPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != secondURL.String() {
		t.Errorf("URL file = %q, want %q", data, secondURL)
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
	if events[0].ID.Version() != 5 {
		t.Errorf("event ID version = %d, want 5", events[0].ID.Version())
	}
	id := events[0].ID

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

	if err := c.SyncAll(); err == nil {
		t.Fatal("SyncAll() succeeded while the feed was offline")
	} else {
		if code, ok := errcode.CodeOf(err); !ok || code != errcode.Network {
			t.Fatalf("SyncAll() error code = %q, %t, want %q, true: %v", code, ok, errcode.Network, err)
		}
		if calendar, ok := errcode.CalendarOf(err); !ok || calendar != name {
			t.Fatalf("SyncAll() error calendar = %q, %t, want %q, true: %v", calendar, ok, name, err)
		}
	}

	offline.Store(false)
	if err := c.SyncAll(); err != nil {
		t.Fatal(err)
	}
	events = importedEvents(c, name)
	if len(events) != 1 || events[0].Title != "Second title" {
		t.Fatalf("synced URL import = %+v", events)
	}
	if events[0].ID != id {
		t.Errorf("event ID changed after sync: got %s, want %s", events[0].ID, id)
	}
	if requests.Load() != 3 {
		t.Errorf("URL was fetched %d times, want 3", requests.Load())
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
	if requests.Load() != 3 {
		t.Errorf("URL was fetched %d times after rename, want 3", requests.Load())
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

func calendarCommitCount(t *testing.T, calendar string) int {
	t.Helper()

	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}
	repo, err := gogit.PlainOpen(filepath.Join(home, filesystem.DirName, calendar))
	if err != nil {
		t.Fatalf("open calendar repository: %v", err)
	}
	commits, err := repo.Log(&gogit.LogOptions{})
	if err != nil {
		t.Fatalf("list calendar commits: %v", err)
	}
	defer commits.Close()

	count := 0
	if err := commits.ForEach(func(*object.Commit) error {
		count++
		return nil
	}); err != nil {
		t.Fatalf("iterate calendar commits: %v", err)
	}
	return count
}

func importedEvents(c *core.Core, calendar string) []core.Event {
	from := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	return c.GetEvents(from, to, core.GetEventsFilter{calendar: {}})
}

func multiEventICalFeed(firstTitle string) string {
	return fmt.Sprintf(`BEGIN:VCALENDAR
VERSION:2.0
PRODID:-//git-calendar//test//EN
BEGIN:VEVENT
UID:first@example.com
DTSTART:20260714T100000Z
DTEND:20260714T110000Z
SUMMARY:%s
END:VEVENT
BEGIN:VEVENT
UID:second@example.com
DTSTART:20260715T100000Z
DTEND:20260715T110000Z
SUMMARY:Second title
END:VEVENT
END:VCALENDAR`, firstTitle)
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
