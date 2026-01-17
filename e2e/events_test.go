package tests

import (
	"encoding/json"
	"os"
	"path"
	"reflect"
	"testing"
	"time"

	"github.com/firu11/git-calendar-core/pkg/core"
	"github.com/firu11/git-calendar-core/pkg/filesystem"
)

// It is kinda e2e, but not entirely. TODO rethink this.

func Test_AddEvent_CreatesJsonFile(t *testing.T) {
	a := core.NewApi()

	err := a.Initialize()
	if err != nil {
		t.Errorf("failed to init repo: %v", err)
	}

	eventIn := core.Event{
		Id:    1,
		Title: "Foo Event",
		From:  uint32(time.Now().Unix() / 1000),
		To:    uint32(time.Now().Add(2*time.Hour).Unix() / 1000),
	}

	err = a.AddEvent(eventIn)
	if err != nil {
		t.Errorf("failed to create an event: %v", err)
	}

	home, err := os.UserHomeDir()
	if err != nil {
		t.Errorf("failed to get home dir: %v", err)
	}

	b, err := os.ReadFile(path.Join(home, filesystem.RepoDirName, core.EventsDirName, "1.json"))
	if err != nil {
		t.Errorf("failed to read event json file: %v", err)
	}

	var parsedEvent struct {
		Id    int    `json:"id"`
		Title string `json:"title"`
	}
	err = json.Unmarshal(b, &parsedEvent)
	if err != nil {
		t.Errorf("failed to parse event json file: %v", err)
	}

	if parsedEvent.Id != 1 {
		t.Errorf("id is not the same as input: \nin:   %d\n!=\nfile: %v", 1, parsedEvent.Id)
	}
	if parsedEvent.Title != "Foo Event" {
		t.Errorf("id is not the same as input: \nin:   %s\n!=\nfile: %s", "Foo Event", parsedEvent.Title)
	}
}

func Test_AddEventAndGetEvent_Works(t *testing.T) {
	a := core.NewApi()

	err := a.Initialize()
	if err != nil {
		t.Errorf("failed to init repo: %v", err)
	}

	eventIn := core.Event{
		Id:    1,
		Title: "Foo Event",
		From:  uint32(time.Now().Unix() / 1000),                  // right now
		To:    uint32(time.Now().Add(2*time.Hour).Unix() / 1000), // two hours from now
	}

	err = a.AddEvent(eventIn)
	if err != nil {
		t.Errorf("failed to create an event: %v", err)
	}

	eventOut, err := a.GetEvent(1)
	if err != nil {
		t.Errorf("failed to get an event by id: %v", err)
	}

	if !reflect.DeepEqual(eventIn, eventOut) {
		t.Errorf("events are not the same: \nin:  %+v\n!=\nout: %+v", eventIn, eventOut)
	}
}
