package core

import (
	"cmp"
	"slices"
	"time"
)

// Reminder schedules a notification relative to an event's start.
type Reminder struct {
	// MinutesBefore is non-negative; zero means notify at the start.
	MinutesBefore int64 `json:"minutes_before"`
}

// Notification describes a scheduled reminder for a standalone event or occurrence.
type Notification struct {
	Event Event     `json:"event"`
	At    time.Time `json:"at"`
}

// GetNotifications returns reminders due in [from, to), ordered by time, without marking them delivered.
// Clients own notification delivery and deduplication using the event ID and At.
func (c *Core) GetNotifications(from, to time.Time, filter GetEventsFilter) []Notification {
	result := make([]Notification, 0)
	if !from.Before(to) {
		return result
	}

	var lookahead time.Duration
	for _, event := range c.events {
		for _, reminder := range event.Reminders {
			if reminder.MinutesBefore < 0 || reminder.MinutesBefore > int64((1<<63-1)/time.Minute) {
				continue
			}
			lookahead = max(lookahead, time.Duration(reminder.MinutesBefore)*time.Minute)
		}
	}

	for _, event := range c.GetEvents(from, to.Add(lookahead), filter) {
		for _, reminder := range event.Reminders {
			if reminder.MinutesBefore < 0 || reminder.MinutesBefore > int64((1<<63-1)/time.Minute) {
				continue
			}
			at := event.From.Add(-time.Duration(reminder.MinutesBefore) * time.Minute)
			if !at.Before(from) && at.Before(to) {
				result = append(result, Notification{Event: event, At: at})
			}
		}
	}
	slices.SortFunc(result, func(a, b Notification) int {
		if order := a.At.Compare(b.At); order != 0 {
			return order
		}
		return cmp.Compare(a.Event.ID.String(), b.Event.ID.String())
	})
	return result
}
