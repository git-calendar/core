package e2e

import (
	"encoding/json"
	"fmt"
	"os"
	"path"
	"reflect"
	"strconv"
	"testing"
	"time"

	"github.com/firu11/git-calendar-core/pkg/core"
	"github.com/firu11/git-calendar-core/pkg/filesystem"
	"github.com/google/uuid"
)

// It is kinda e2e, but not entirely. TODO rethink this.

func Test_AddEvent_CreatesJsonFile(t *testing.T) {
	a := core.NewCore()

	err := a.Initialize()
	if err != nil {
		t.Errorf("failed to init repo: %v", err)
	}

	id := uuid.Must(uuid.NewV7())
	eventIn := core.Event{
		Id:    id,
		Title: "Foo Event",
		From:  time.Now(),
		To:    time.Now().Add(2 * time.Hour),
	}

	_, err = a.CreateEvent(eventIn)
	if err != nil {
		t.Errorf("failed to create an event: %v", err)
	}

	home, err := os.UserHomeDir()
	if err != nil {
		t.Errorf("failed to get home dir: %v", err)
	}

	b, err := os.ReadFile(path.Join(home, filesystem.RepoDirName, core.EventsDirName, fmt.Sprintf("%s.json", id)))
	if err != nil {
		t.Errorf("failed to read event json file: %v", err)
	}

	var parsedEvent struct {
		Id    uuid.UUID `json:"id"`
		Title string    `json:"title"`
	}
	err = json.Unmarshal(b, &parsedEvent)
	if err != nil {
		t.Errorf("failed to parse event json file: %v", err)
	}

	if parsedEvent.Id != id {
		t.Errorf("id is not the same as input: \nin:   %d\n!=\nfile: %v", 1, parsedEvent.Id)
	}
	if parsedEvent.Title != "Foo Event" {
		t.Errorf("id is not the same as input: \nin:   %s\n!=\nfile: %s", "Foo Event", parsedEvent.Title)
	}
}

func Test_AddEventAndGetEvent_Works(t *testing.T) {
	a := core.NewCore()

	err := a.Initialize()
	if err != nil {
		t.Errorf("failed to init repo: %v", err)
	}

	id := uuid.Must(uuid.NewV7())
	eventIn := core.Event{
		Id:    id,
		Title: "Foo Event",
		From:  time.Now(),                    // right now
		To:    time.Now().Add(2 * time.Hour), // two hours from now
	}

	_, err = a.CreateEvent(eventIn)
	if err != nil {
		t.Errorf("failed to create an event: %v", err)
	}

	eventOut, err := a.GetEvent(id)
	if err != nil {
		t.Errorf("failed to get an event by id: %v", err)
	}

	if !reflect.DeepEqual(eventIn, *eventOut) {
		t.Errorf("events are not the same: \nin:  %+v\n!=\nout: %+v", eventIn, eventOut)
	}
}

func Test_AddEventsAndGetThemByInterval(t *testing.T) {
	a := core.NewCore()

	err := a.Initialize()
	if err != nil {
		t.Errorf("failed to init repo: %v", err)
	}
	date := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	numEvents := 5
	for i := 0; i < numEvents; i++ {
		id := uuid.Must(uuid.NewV7())
		from := date.AddDate(0, 0, i)
		to := date.AddDate(0, 0, i).Add(time.Hour)
		eventIn := core.Event{
			Id:         id,
			Title:      "Evet" + strconv.Itoa(i),
			From:       from,
			To:         to,
			Duration:   time.Hour,
			Repetition: core.None,
		}
		_, err = a.CreateEvent(eventIn)
		if err != nil {
			t.Errorf("failed to create an event: %v", err)
		}
	}
	eventsOut, err := a.GetEvents(date, date.AddDate(0, 1, 0))
	if err != nil {
		t.Errorf("failed to get events: %v", err)
	}
	if len(eventsOut) != numEvents {
		t.Errorf("not the correct number of events: got %d, want %d", len(eventsOut), numEvents)
		t.Errorf("eventsOut: %v", eventsOut)
	}
}

func Test_AddRepeatingEventAndGetEvents_Works(t *testing.T) {
	a := core.NewCore()

	err := a.Initialize()
	if err != nil {
		t.Errorf("failed to init repo: %v", err)
	}

	id := uuid.Must(uuid.NewV7())
	eventIn := core.Event{
		Id:         id,
		Title:      "Repeating Event",
		From:       time.Now(),
		To:         time.Date(9999, 12, 31, 23, 59, 59, 0, time.UTC),
		Duration:   time.Hour * 8,
		Repetition: core.Week,
	}
	_, err = a.CreateEvent(eventIn)
	if err != nil {
		t.Errorf("failed to create an event: %v", err)
	}

	eventOut, err := a.GetEvent(id)
	if err != nil {
		t.Errorf("failed to get an event by id: %v", err)
	}
	if !reflect.DeepEqual(eventIn, *eventOut) {
		t.Errorf("events are not the same: \nin:  %+v\n!=\nout: %+v", eventIn, eventOut)
	}

	eventsOut, err := a.GetEvents(time.Now().AddDate(0, 0, 7), time.Now().AddDate(0, 1, 7))
	if err != nil {
		t.Errorf("failed to get an events by interval: %v", err)
	}
	if len(eventsOut) != 4 {
		t.Errorf("not all events were generated: %v", err)
		t.Errorf("eventsOut: %v", eventsOut)
	}
}
