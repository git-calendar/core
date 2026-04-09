package e2e

import (
	"encoding/json"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/git-calendar/core/pkg/core"
	"github.com/git-calendar/core/pkg/filesystem"
	"github.com/google/uuid"
)

func Test_AddEvent_CreatesJsonFile(t *testing.T) {
	c := core.NewCore()

	err := c.CreateCalendar(TestCalendarName, "")
	if err != nil {
		t.Fatalf("failed to init repo: %v", err)
	}

	id := uuid.New()
	eventIn := core.Event{
		Id:       id,
		Calendar: TestCalendarName,
		Title:    "Foo Event",
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

	b, err := os.ReadFile(filepath.Join(home, filesystem.DirName, TestCalendarName, core.EventsDirName, fmt.Sprintf("%s.json", id)))
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

	if parsedEvent.Title != "Foo Event" {
		t.Errorf("id is not the same as input: \nin:   %s\n!=\nfile: %s", "Foo Event", parsedEvent.Title)
	}
}

func Test_RemoveEvent_DeletesJsonFile(t *testing.T) {
	c := core.NewCore()

	err := c.CreateCalendar(TestCalendarName, "")
	if err != nil {
		t.Fatalf("failed to init repo: %v", err)
	}

	id := uuid.New()
	startTime := time.Now()
	eventIn := core.Event{
		Id:       id,
		Calendar: TestCalendarName,
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
	if out.Id != id {
		t.Errorf("id should be %s, got %s", id, out.Id)
	}

	home, err := os.UserHomeDir()
	if err != nil {
		t.Errorf("failed to get home dir: %v", err)
	}

	filePath := path.Join(home, filesystem.DirName, TestCalendarName, core.EventsDirName, fmt.Sprintf("%s.json", id))

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

func Test_AddEventAndGetEvent_Works(t *testing.T) {
	c := core.NewCore()

	err := c.CreateCalendar(TestCalendarName, "")
	if err != nil {
		t.Fatalf("failed to init repo: %v", err)
	}

	date := time.Date(2026, 1, 1, 9, 0, 0, 0, time.UTC)
	eventIn := core.Event{
		Id:       uuid.New(),
		Calendar: TestCalendarName,
		Title:    "Foo Event",
		From:     date,
		To:       date.Add(2 * time.Hour),
	}

	_, err = c.CreateEvent(eventIn)
	if err != nil {
		t.Errorf("failed to create an event: %v", err)
	}

	eventOut, err := c.GetEvent(eventIn.Id)
	if err != nil {
		t.Fatalf("failed to get an event by id: %v", err)
	}

	if !reflect.DeepEqual(eventIn, *eventOut) {
		t.Errorf("events are not the same: \nin:  %+v\n!=\nout: %+v", eventIn, *eventOut)
	}
}

func Test_AddEventsAndGetThemByInterval(t *testing.T) {
	c := core.NewCore()

	err := c.CreateCalendar(TestCalendarName, "")
	if err != nil {
		t.Fatalf("failed to init repo: %v", err)
	}

	date := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	numEvents := 5
	for i := range numEvents {
		eventIn := core.Event{
			Id:       uuid.New(),
			Calendar: TestCalendarName,
			Title:    fmt.Sprintf("Event %d", i+1),
			From:     date.AddDate(0, 0, i),
			To:       date.AddDate(0, 0, i).Add(time.Hour),
		}
		_, err = c.CreateEvent(eventIn)
		if err != nil {
			t.Errorf("failed to create an event: %v", err)
		}
	}

	eventsOut := c.GetEvents(date, date.AddDate(0, 1, 0))
	if len(eventsOut) != numEvents {
		t.Errorf("not the correct number of events: got %d, want %d", len(eventsOut), numEvents)
		t.Errorf("eventsOut: %v", eventsOut)
	}
}

func Test_AddNormalEventsAndRemoveEvent_Works(t *testing.T) {
	c := core.NewCore()

	err := c.CreateCalendar(TestCalendarName, "")
	if err != nil {
		t.Fatalf("failed to init repo: %v", err)
	}

	date := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	numEvents := 5
	for i := range numEvents {
		eventIn := core.Event{
			Id:       uuid.New(),
			Calendar: TestCalendarName,
			Title:    fmt.Sprintf("Event %d", i+1),
			From:     date.AddDate(0, 0, i),
			To:       date.AddDate(0, 0, i).Add(time.Hour),
		}
		_, err = c.CreateEvent(eventIn)
		if err != nil {
			t.Errorf("failed to create an event: %v", err)
		}
	}

	eventsOut := c.GetEvents(date, date.AddDate(0, 1, 0))
	if len(eventsOut) != numEvents {
		t.Errorf("not the correct number of events: got %d, want %d", len(eventsOut), numEvents)
		t.Errorf("eventsOut: %v", eventsOut)
	}

	err = c.RemoveEvent(eventsOut[0])
	if err != nil {
		t.Errorf("failed to remove event: %v", err)
	}

	eventsOut = c.GetEvents(date, date.AddDate(0, 1, 0))
	if len(eventsOut) != numEvents-1 {
		t.Errorf("not the correct number of events: got %d, want %d", len(eventsOut), numEvents)
		t.Errorf("eventsOut: %v", eventsOut)
	}
}

func Test_AddNormalEventsInSameIntervalAndRemoveEvents_Works(t *testing.T) {
	c := core.NewCore()

	err := c.CreateCalendar(TestCalendarName, "")
	if err != nil {
		t.Fatalf("failed to init repo: %v", err)
	}

	date := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	var events []core.Event
	for i := range 5 {
		eventIn := core.Event{
			Id:       uuid.New(),
			Calendar: TestCalendarName,
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
		c.RemoveEvent(events[i])
	}

	eventsOut := c.GetEvents(date, date.AddDate(0, 1, 0))
	if len(eventsOut) != 0 {
		t.Errorf("not the correct number of events: got %d, want %d", len(eventsOut), 0)
		t.Errorf("eventsOut: %v", eventsOut)
	}
}

func Test_UpdateStandardEvent_Works(t *testing.T) {
	c := core.NewCore()

	err := c.CreateCalendar(TestCalendarName, "")
	if err != nil {
		t.Fatalf("failed to init repo: %v", err)
	}

	startTime := time.Now()
	eventIn := core.Event{
		Id:       uuid.New(),
		Calendar: TestCalendarName,
		Title:    "Original Title",
		From:     startTime,
		To:       startTime.Add(time.Hour),
	}

	_, err = c.CreateEvent(eventIn)
	if err != nil {
		t.Errorf("failed to create an event: %v", err)
	}

	eventIn.Title = "Updated Title"
	eventIn.To = startTime.Add(2 * time.Hour)

	updatedEvent, err := c.UpdateEvent(eventIn)
	if err != nil {
		t.Errorf("failed to update event: %v", err)
	}

	eventOut, err := c.GetEvent(eventIn.Id)
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
