package api

import (
	"encoding/json/v2"
	"testing"
)

func TestNotificationLifecycle(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	a := NewApi()
	if err := a.CreateCalendar("Work", ""); err != nil {
		t.Fatal(err)
	}
	created, err := a.CreateEvent(`{"title":"Meeting","calendar":"Work","from":"2026-01-01T10:00:00Z","to":"2026-01-01T11:00:00Z","reminders":[{"minutes_before":15},{"minutes_before":0}]}`)
	if err != nil {
		t.Fatal(err)
	}
	if err := a.LoadCalendars(); err != nil {
		t.Fatal(err)
	}
	query := func() string {
		t.Helper()
		data, err := a.GetNotifications("2026-01-01T09:45:00Z", "2026-01-01T10:00:00Z", "")
		if err != nil {
			t.Fatal(err)
		}
		return data
	}
	var notifications []struct {
		Event eventJSON `json:"event"`
		At    string    `json:"at"`
	}
	if err := json.Unmarshal([]byte(query()), &notifications); err != nil {
		t.Fatal(err)
	}
	if len(notifications) != 1 || notifications[0].At != "2026-01-01T09:45:00Z" || len(notifications[0].Event.Reminders) != 2 {
		t.Fatalf("unexpected notifications: %+v", notifications)
	}
	if data, err := a.GetNotifications("bad", "2026-01-01T10:00:00Z", ""); err == nil || data != "[]" {
		t.Fatalf("invalid timestamp: data = %s, err = %v", data, err)
	}
	if _, err := a.GetNotifications("2026-01-01T10:00:00Z", "2026-01-01T09:00:00Z", ""); err == nil {
		t.Fatal("reversed interval must be rejected")
	}
	event, err := unmarshalEvent(created)
	if err != nil {
		t.Fatal(err)
	}
	event.Reminders = nil
	updated, err := marshalEvent(&event)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := a.UpdateEvent(updated); err != nil {
		t.Fatal(err)
	}
	if got := query(); got != "[]" {
		t.Fatalf("disabling reminders returned %s", got)
	}
	if _, err := a.UpdateEvent(created); err != nil {
		t.Fatal(err)
	}
	if err := a.RemoveEvent(created); err != nil {
		t.Fatal(err)
	}
	if got := query(); got != "[]" {
		t.Fatalf("deleted event returned notifications: %s", got)
	}
}
