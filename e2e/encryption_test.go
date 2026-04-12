package e2e

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/git-calendar/core/pkg/core"
	"github.com/git-calendar/core/pkg/filesystem"
	"github.com/google/uuid"
	aessiv "github.com/jedisct1/go-aes-siv"
)

func TestCreateCalendarWithPassword_CreatesKeyFile(t *testing.T) {
	c := core.NewCore()

	err := c.CreateCalendar(TestCalendarName, "somepassword")
	if err != nil {
		t.Fatalf("failed to init repo: %v", err)
	}

	home, err := os.UserHomeDir()
	if err != nil {
		t.Errorf("failed to get home dir: %v", err)
	}

	b, err := os.ReadFile(filepath.Join(home, filesystem.DirName, fmt.Sprintf("%s.key", TestCalendarName)))
	if err != nil {
		t.Errorf("failed to read key file: %v", err)
	}

	if len(b) != aessiv.KeySize256 {
		t.Errorf("unexpected key size: %v", len(b))
	}
}

func TestCreateCalendarWithPasswordAndCreateEvent_CreatesJsonFile(t *testing.T) {
	c := core.NewCore()

	err := c.CreateCalendar(TestCalendarName, "somepassword")
	if err != nil {
		t.Fatalf("failed to init repo: %v", err)
	}

	id := uuid.New()
	title := "Foo Event"
	eventIn := core.Event{
		Id:       id,
		Calendar: TestCalendarName,
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

	if parsedEvent.Title == title {
		t.Errorf("title is not encrypted: \nin:   %s\nfile: %s", title, parsedEvent.Title)
	}
}
