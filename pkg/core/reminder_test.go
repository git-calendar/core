package core

import (
	"bytes"
	"reflect"
	"testing"
	"time"

	"github.com/go-git/go-billy/v5/memfs"
)

func TestReminderValidation(t *testing.T) {
	start := time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC)
	for _, test := range []struct {
		name      string
		reminders []Reminder
		valid     bool
	}{
		{"disabled", nil, true},
		{"at start and before", []Reminder{{0}, {15}}, true},
		{"negative", []Reminder{{-1}}, false},
		{"duplicate", []Reminder{{15}, {15}}, false},
		{"overflow", []Reminder{{1<<63 - 1}}, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			event := Event{Title: "Test", From: start, To: start.Add(time.Hour), Reminders: test.reminders}
			if err := event.Validate(); (err == nil) != test.valid {
				t.Fatalf("Validate() = %v, valid = %v", err, test.valid)
			}
		})
	}
}

func TestRemindersPersist(t *testing.T) {
	for _, key := range [][]byte{nil, bytes.Repeat([]byte{1}, 32)} {
		event := Event{Title: "Test", From: time.Now(), To: time.Now().Add(time.Hour), Reminders: []Reminder{{0}, {15}}}
		if err := event.Validate(); err != nil {
			t.Fatal(err)
		}
		file, err := memfs.New().Create(event.ID.String() + ".json")
		if err != nil {
			t.Fatal(err)
		}
		defer file.Close()
		if err := event.WriteToFile(file, key); err != nil {
			t.Fatal(err)
		}
		if _, err := file.Seek(0, 0); err != nil {
			t.Fatal(err)
		}
		var loaded Event
		if err := loaded.LoadFromFile(file, "Work", key); err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(loaded.Reminders, event.Reminders) {
			t.Fatalf("reminders = %v, want %v", loaded.Reminders, event.Reminders)
		}
	}
}

func TestGetNotifications(t *testing.T) {
	var c Core
	c.resetCore()
	repeat := mustRecurrence(t, "DTSTART:20260101T100000Z\nRRULE:FREQ=DAILY;COUNT=3")
	start := repeat.GetDTStart()
	repeat.ExDate(start.Add(24 * time.Hour))
	parent := Event{Title: "Daily", Calendar: "Work", From: start, To: start.Add(time.Hour), Repeat: repeat, Reminders: []Reminder{{0}, {15}, {1440}}}
	if err := parent.Validate(); err != nil {
		t.Fatal(err)
	}
	c.events[parent.ID] = &parent
	if err := c.intervalTree.InsertEvent(parent); err != nil {
		t.Fatal(err)
	}

	// the second occurrence is excluded, including its day-before reminder
	got := c.GetNotifications(start.Add(-15*time.Minute), start.Add(48*time.Hour), nil)
	want := []time.Time{start.Add(-15 * time.Minute), start, start.Add(24 * time.Hour), start.Add(48*time.Hour - 15*time.Minute)}
	if len(got) != len(want) {
		t.Fatalf("got %d notifications, want %d: %v", len(got), len(want), got)
	}
	for i, notification := range got {
		if !notification.At.Equal(want[i]) || !notification.Event.IsChild() || len(notification.Event.Reminders) != 3 {
			t.Errorf("notification %d = %+v, want at %v with child event and reminders", i, notification, want[i])
		}
	}
	if got[0].Event.ID == got[2].Event.ID {
		t.Fatal("different occurrences must have different notification event IDs")
	}
	if got := c.GetNotifications(start, start.Add(time.Hour), GetEventsFilter{"Work": {HideUntagged: true}}); len(got) != 0 {
		t.Fatalf("filter should exclude untagged events: %v", got)
	}
	if got := c.GetNotifications(start, start, nil); len(got) != 0 {
		t.Fatalf("empty interval returned notifications: %v", got)
	}
}
