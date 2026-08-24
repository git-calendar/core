package e2e

import (
	"encoding/json/v2"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"
	"uuid"

	"github.com/git-calendar/core/pkg/core"
	"github.com/git-calendar/core/pkg/filesystem"
)

func TestAddEvent_CreatesJsonFile(t *testing.T) {
	c := core.NewCore()

	err := c.CreateCalendar(testCalendarName, "")
	if err != nil {
		t.Fatalf("failed to init repo: %v", err)
	}

	t.Cleanup(func() {
		_ = c.RemoveCalendar(testCalendarName)
	})

	id := uuid.New()
	title := "Foo Event"
	eventIn := core.Event{
		ID:       id,
		Calendar: testCalendarName,
		Title:    title,
		From:     time.Now(),
		To:       time.Now().Add(2 * time.Hour),
	}

	_, err = c.CreateEvent(eventIn)
	if err != nil {
		t.Errorf("failed to create an event: %v", err)
	}

	home, err := os.UserHomeDir()
	if err != nil {
		t.Errorf("failed to get home dir: %v", err)
	}

	b, err := os.ReadFile(filepath.Join(home, filesystem.DirName, testCalendarName, core.EventsDirName, fmt.Sprintf("%s.json", id)))
	if err != nil {
		t.Errorf("failed to read event json file: %v", err)
	}

	var parsedEvent struct {
		Title string `json:"title"`
	}
	err = json.Unmarshal(b, &parsedEvent)
	if err != nil {
		t.Fatalf("failed to parse event json file: %v", err)
	}

	if parsedEvent.Title != title {
		t.Errorf("title is not the same as input: \nin:   %s\n!=\nfile: %s", title, parsedEvent.Title)
	}
}

func TestRemoveEvent_DeletesJsonFile(t *testing.T) {
	c := core.NewCore()

	err := c.CreateCalendar(testCalendarName, "")
	if err != nil {
		t.Fatalf("failed to init repo: %v", err)
	}

	t.Cleanup(func() {
		_ = c.RemoveCalendar(testCalendarName)
	})

	id := uuid.New()
	startTime := time.Now()
	eventIn := core.Event{
		ID:       id,
		Calendar: testCalendarName,
		Title:    "Event To Delete",
		From:     startTime,
		To:       startTime.Add(1 * time.Hour),
	}

	_, err = c.CreateEvent(eventIn)
	if err != nil {
		t.Fatalf("failed to create an event: %v", err)
	}

	out, err := c.GetEvent(id)
	if err != nil || out == nil {
		t.Fatalf("failed to get an event by id: %v", err)
	}
	if out.ID != id {
		t.Errorf("id should be %s, got %s", id, out.ID)
	}

	home, err := os.UserHomeDir()
	if err != nil {
		t.Errorf("failed to get home dir: %v", err)
	}

	filePath := filepath.Join(home, filesystem.DirName, testCalendarName, core.EventsDirName, fmt.Sprintf("%s.json", id))

	if _, err := os.Stat(filePath); err != nil {
		if os.IsNotExist(err) {
			t.Errorf("file should exist before deletion")
		} else {
			t.Error(err)
		}
	}

	err = c.RemoveEvent(eventIn)
	if err != nil {
		t.Errorf("failed to remove event: %v", err)
	}

	if _, err := os.Stat(filePath); !os.IsNotExist(err) {
		t.Errorf("file was not deleted: %s", filePath)
	}
}

func TestAddEventAndGetEvent(t *testing.T) {
	c := core.NewCore()

	err := c.CreateCalendar(testCalendarName, "")
	if err != nil {
		t.Fatalf("failed to init repo: %v", err)
	}

	t.Cleanup(func() {
		_ = c.RemoveCalendar(testCalendarName)
	})

	date := time.Date(2026, 1, 1, 9, 0, 0, 0, time.UTC)
	eventIn := core.Event{
		ID:       uuid.New(),
		Calendar: testCalendarName,
		Title:    "Foo Event",
		From:     date,
		To:       date.Add(2 * time.Hour),
	}

	_, err = c.CreateEvent(eventIn)
	if err != nil {
		t.Errorf("failed to create an event: %v", err)
	}

	eventOut, err := c.GetEvent(eventIn.ID)
	if err != nil {
		t.Fatalf("failed to get an event by id: %v", err)
	}
	eventIn.UpdatedAt = eventOut.UpdatedAt

	if !reflect.DeepEqual(eventIn, *eventOut) {
		t.Errorf("events are not the same: \nin:  %+v\n!=\nout: %+v", eventIn, *eventOut)
	}
}

func TestAddEventsAndGetThemByInterval(t *testing.T) {
	c := core.NewCore()

	err := c.CreateCalendar(testCalendarName, "")
	if err != nil {
		t.Fatalf("failed to init repo: %v", err)
	}

	t.Cleanup(func() {
		_ = c.RemoveCalendar(testCalendarName)
	})

	date := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	numEvents := 5
	for i := range numEvents {
		eventIn := core.Event{
			ID:       uuid.New(),
			Calendar: testCalendarName,
			Title:    fmt.Sprintf("Event %d", i+1),
			From:     date.AddDate(0, 0, i),
			To:       date.AddDate(0, 0, i).Add(time.Hour),
		}
		_, err = c.CreateEvent(eventIn)
		if err != nil {
			t.Errorf("failed to create an event: %v", err)
		}
	}

	eventsOut := c.GetEvents(date, date.AddDate(0, 1, 0), nil)
	if len(eventsOut) != numEvents {
		t.Errorf("not the correct number of events: got %d, want %d", len(eventsOut), numEvents)
		t.Errorf("eventsOut: %v", eventsOut)
	}
}

func TestAddNormalEventsAndRemoveEvent(t *testing.T) {
	c := core.NewCore()

	err := c.CreateCalendar(testCalendarName, "")
	if err != nil {
		t.Fatalf("failed to init repo: %v", err)
	}

	t.Cleanup(func() {
		_ = c.RemoveCalendar(testCalendarName)
	})

	date := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	numEvents := 5
	for i := range numEvents {
		eventIn := core.Event{
			ID:       uuid.New(),
			Calendar: testCalendarName,
			Title:    fmt.Sprintf("Event %d", i+1),
			From:     date.AddDate(0, 0, i),
			To:       date.AddDate(0, 0, i).Add(time.Hour),
		}
		if _, err = c.CreateEvent(eventIn); err != nil {
			t.Fatalf("failed to create an event: %v", err)
		}
	}

	eventsOut := c.GetEvents(date, date.AddDate(0, 1, 0), nil)
	if len(eventsOut) != numEvents {
		t.Fatalf("not the correct number of events: got %d, want %d; events: %v", len(eventsOut), numEvents, eventsOut)
	}

	if err = c.RemoveEvent(eventsOut[0]); err != nil {
		t.Fatalf("failed to remove event: %v", err)
	}

	eventsOut = c.GetEvents(date, date.AddDate(0, 1, 0), nil)
	if len(eventsOut) != numEvents-1 {
		t.Fatalf("not the correct number of events: got %d, want %d; events: %v", len(eventsOut), numEvents-1, eventsOut)
	}
}

func TestAddNormalEventsInSameIntervalAndRemoveEvents(t *testing.T) {
	c := core.NewCore()

	err := c.CreateCalendar(testCalendarName, "")
	if err != nil {
		t.Fatalf("failed to init repo: %v", err)
	}

	t.Cleanup(func() {
		_ = c.RemoveCalendar(testCalendarName)
	})

	date := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	var events []core.Event
	for i := range 5 {
		eventIn := core.Event{
			ID:       uuid.New(),
			Calendar: testCalendarName,
			Title:    fmt.Sprintf("Event %d", i+1),
			From:     date.AddDate(0, 0, 1),
			To:       date.AddDate(0, 0, 1).Add(time.Hour),
		}

		events = append(events, eventIn)

		_, err = c.CreateEvent(eventIn)
		if err != nil {
			t.Errorf("failed to create an event: %v", err)
		}
	}

	for i := range events {
		if err := c.RemoveEvent(events[i]); err != nil {
			t.Fatalf("failed to remove event: %v", err)
		}
	}

	eventsOut := c.GetEvents(date, date.AddDate(0, 1, 0), nil)
	if len(eventsOut) != 0 {
		t.Errorf("not the correct number of events: got %d, want %d", len(eventsOut), 0)
		t.Errorf("eventsOut: %v", eventsOut)
	}
}

func TestUpdateStandardEvent(t *testing.T) {
	c := core.NewCore()

	err := c.CreateCalendar(testCalendarName, "")
	if err != nil {
		t.Fatalf("failed to init repo: %v", err)
	}

	t.Cleanup(func() {
		_ = c.RemoveCalendar(testCalendarName)
	})

	startTime := time.Now()
	eventIn := core.Event{
		ID:       uuid.New(),
		Calendar: testCalendarName,
		Title:    "Original Title",
		From:     startTime,
		To:       startTime.Add(time.Hour),
	}

	_, err = c.CreateEvent(eventIn)
	if err != nil {
		t.Fatalf("failed to create an event: %v", err)
	}

	eventIn.Title = "Updated Title"
	eventIn.To = startTime.Add(2 * time.Hour)

	updatedEvent, err := c.UpdateEvent(eventIn)
	if err != nil {
		t.Fatalf("failed to update event: %v", err)
	}
	if updatedEvent == nil {
		t.Fatal("updated event is nil")
	}

	eventOut, err := c.GetEvent(eventIn.ID)
	if err != nil {
		t.Fatalf("failed to get updated event: %v", err)
	}

	if eventOut.Title != "Updated Title" {
		t.Errorf("title was not updated, got: %s", eventOut.Title)
	}
	if !eventOut.To.Equal(updatedEvent.To) {
		t.Errorf("time was not updated, got: %s", eventOut.To)
	}
}

func TestGetEvents_FilterByCalendarAndTag(t *testing.T) {
	c := core.NewCore()

	calendarA := testCalendarName + "-filter-a"
	calendarB := testCalendarName + "-filter-b"

	if err := c.CreateCalendar(calendarA, ""); err != nil {
		t.Fatalf("failed to create calendar A: %v", err)
	}
	t.Cleanup(func() {
		_ = c.RemoveCalendar(calendarA)
	})

	if err := c.CreateCalendar(calendarB, ""); err != nil {
		t.Fatalf("failed to create calendar B: %v", err)
	}
	t.Cleanup(func() {
		_ = c.RemoveCalendar(calendarB)
	})

	from := time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC)
	to := from.Add(time.Hour)

	tagA := uuid.New()
	tagB := uuid.New()

	matchingID := uuid.New()
	wrongTagID := uuid.New()
	wrongCalendarID := uuid.New()

	events := []core.Event{
		{
			ID:       matchingID,
			Calendar: calendarA,
			Title:    "matching event",
			From:     from,
			To:       to,
			TagID:    new(tagA),
		},
		{
			ID:       wrongTagID,
			Calendar: calendarA,
			Title:    "wrong tag",
			From:     from,
			To:       to,
			TagID:    new(tagB),
		},
		{
			ID:       wrongCalendarID,
			Calendar: calendarB,
			Title:    "wrong calendar",
			From:     from,
			To:       to,
			TagID:    new(tagA),
		},
	}

	for _, e := range events {
		if _, err := c.CreateEvent(e); err != nil {
			t.Fatalf("failed to create event %q: %v", e.Title, err)
		}
	}

	got := c.GetEvents(from.Add(-time.Hour), to.Add(time.Hour), core.GetEventsFilter{
		calendarA: {HiddenTagIDs: []uuid.UUID{tagB}},
		calendarB: {HiddenTagIDs: []uuid.UUID{tagA}},
	})

	if len(got) != 1 {
		t.Fatalf("expected 1 event, got %d: %#v", len(got), got)
	}

	if got[0].ID != matchingID {
		t.Errorf("expected event %s, got %s", matchingID, got[0].ID)
	}
}

func TestGetEvents_NilFilterReturnsAllEvents(t *testing.T) {
	c := core.NewCore()

	calendar := testCalendarName + "-nil-filter"

	if err := c.CreateCalendar(calendar, ""); err != nil {
		t.Fatalf("failed to create calendar: %v", err)
	}
	t.Cleanup(func() {
		_ = c.RemoveCalendar(calendar)
	})

	from := time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC)
	to := from.Add(time.Hour)

	tagA := uuid.New()
	tagB := uuid.New()

	eventA := core.Event{
		ID:       uuid.New(),
		Calendar: calendar,
		Title:    "event A",
		From:     from,
		To:       to,
		TagID:    new(tagA),
	}
	eventB := core.Event{
		ID:       uuid.New(),
		Calendar: calendar,
		Title:    "event B",
		From:     from.Add(30 * time.Minute),
		To:       to.Add(30 * time.Minute),
		TagID:    new(tagB),
	}

	if _, err := c.CreateEvent(eventA); err != nil {
		t.Fatalf("failed to create event A: %v", err)
	}
	if _, err := c.CreateEvent(eventB); err != nil {
		t.Fatalf("failed to create event B: %v", err)
	}

	got := c.GetEvents(from.Add(-time.Hour), to.Add(time.Hour), nil)

	if len(got) != 2 {
		t.Fatalf("expected 2 events, got %d: %#v", len(got), got)
	}

	if !hasEvent(got, eventA.ID) {
		t.Errorf("missing event A")
	}
	if !hasEvent(got, eventB.ID) {
		t.Errorf("missing event B")
	}
}

func hasEvent(events []core.Event, id uuid.UUID) bool {
	for _, e := range events {
		if e.ID == id {
			return true
		}
	}
	return false
}
