package e2e

import (
	"encoding/json"
	"fmt"
	"os"
	"path"
	"reflect"
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

	if !reflect.DeepEqual(eventIn, eventOut) {
		t.Errorf("events are not the same: \nin:  %+v\n!=\nout: %+v", eventIn, eventOut)
	}
}
