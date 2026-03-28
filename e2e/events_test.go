// It is kinda e2e, but not entirely. TODO rethink this.
package e2e

import (
	"encoding/binary"
	"reflect"
	"slices"
	"testing"
	"time"

	"github.com/git-calendar/core/pkg/core"
	"github.com/google/uuid"
)

const TestCalendarName = "test"

func Test_AddInfinitelyRepeatingEventAndGetEvents_Works(t *testing.T) {
	c := core.NewCore()

	err := c.CreateCalendar(TestCalendarName)
	if err != nil {
		t.Fatalf("failed to init repo: %v", err)
	}

	id := uuid.New()
	startTime := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	eventIn := core.Event{
		Id:       id,
		Calendar: TestCalendarName,
		Title:    "Repeating Event",
		From:     startTime,
		To:       startTime.Add(time.Hour * 4),
		Repeat: &core.Repetition{
			Frequency: core.Week,
			Interval:  1,
			Until:     time.Date(9999, 12, 31, 23, 59, 59, 0, time.UTC),
		},
	}
	_, err = c.CreateEvent(eventIn)
	if err != nil {
		t.Errorf("failed to create an event: %v", err)
	}

	eventOut, err := c.GetEvent(id)
	if err != nil {
		t.Fatalf("failed to get an event by id: %v", err)
	}
	if !reflect.DeepEqual(eventIn, *eventOut) {
		t.Errorf("events are not the same: \nin:  %+v\n!=\nout: %+v", eventIn, *eventOut)
	}

	queryTo := startTime.AddDate(1, 0, 0)
	eventsOut := c.GetEvents(startTime, queryTo)
	if len(eventsOut) != 53 { // 2026 has 53 weeks
		t.Errorf("not all events were generated; eventsOut: %d: %+v", len(eventsOut), eventsOut)
	}
}

func Test_AddCountRepeatingEventAndGetEvents_Works(t *testing.T) {
	c := core.NewCore()

	err := c.CreateCalendar(TestCalendarName)
	if err != nil {
		t.Fatalf("failed to init repo: %v", err)
	}

	const COUNT = 6
	id := uuid.New()
	startTime := time.Now()
	eventIn := core.Event{
		Id:       id,
		Calendar: TestCalendarName,
		Title:    "Repeating Event",
		From:     startTime,
		To:       startTime.Add(time.Hour * 4),
		Repeat: &core.Repetition{
			Frequency: core.Week,
			Interval:  1,
			Count:     COUNT,
		},
	}
	_, err = c.CreateEvent(eventIn)
	if err != nil {
		t.Errorf("failed to create an event: %v", err)
	}

	eventOut, err := c.GetEvent(id)
	if err != nil {
		t.Fatalf("failed to get an event by id: %v", err)
	}
	if !reflect.DeepEqual(eventIn, *eventOut) {
		t.Errorf("events are not the same: \nin:  %+v\n!=\nout: %+v", eventIn, *eventOut)
	}

	queryFrom := time.Now().AddDate(-1, 0, 0)
	queryTo := time.Now().AddDate(1, 0, 0)
	eventsOut := c.GetEvents(queryFrom, queryTo)
	if len(eventsOut) != COUNT {
		t.Errorf("not all events were generated; eventsOut: %d: %+v", len(eventsOut), eventsOut)
	}
}

func Test_AddRepeatingEventsAndRemoveGeneratedEvent_Works(t *testing.T) {
	c := core.NewCore()

	err := c.CreateCalendar(TestCalendarName)
	if err != nil {
		t.Fatalf("failed to init repo: %v", err)
	}

	const COUNT = 6
	id := uuid.New()
	startTime := time.Now()
	eventIn := core.Event{
		Id:       id,
		Calendar: TestCalendarName,
		Title:    "Repeating Event",
		From:     startTime,
		To:       startTime.Add(time.Hour * 4),
		Repeat: &core.Repetition{
			Frequency: core.Week,
			Interval:  1,
			Count:     COUNT,
		},
	}
	_, err = c.CreateEvent(eventIn)
	if err != nil {
		t.Errorf("failed to create an event: %v", err)
	}

	eventOut, err := c.GetEvent(id)
	if err != nil {
		t.Fatalf("failed to get an event by id: %v", err)
	}
	if !reflect.DeepEqual(eventIn, *eventOut) {
		t.Errorf("events are not the same: \nin:  %+v\n!=\nout: %+v", eventIn, *eventOut)
	}

	queryFrom := time.Now().AddDate(-1, 0, 0)
	queryTo := time.Now().AddDate(1, 0, 0)
	eventsOut := c.GetEvents(queryFrom, queryTo)
	if len(eventsOut) != COUNT {
		t.Errorf("not all events were generated; eventsOut: %d: %+v", len(eventsOut), eventsOut)
		return
	}
	eventToRemove := eventsOut[0]
	if err := c.RemoveRepeatingEvent(eventToRemove, core.Current); err != nil {
		t.Errorf("failed to remove event: %v", err)
	}

	eventsOut = c.GetEvents(queryFrom, queryTo)
	if len(eventsOut) != COUNT-1 || slices.Contains(eventsOut, eventToRemove) {
		t.Errorf("event wasn't removed correctly; eventsOut: %d: %+v", len(eventsOut), eventsOut)
	}
}

func Test_UpdateGeneratedEvent_Current_Works(t *testing.T) {
	c := core.NewCore()
	_ = c.CreateCalendar(TestCalendarName)

	parentId := uuid.New()
	startTime := time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC)
	parentEvent := core.Event{
		Id:       parentId,
		Calendar: TestCalendarName,
		Title:    "Daily event",
		From:     startTime,
		To:       startTime.Add(time.Hour),
		Repeat: &core.Repetition{
			Frequency: core.Day,
			Interval:  1,
			Count:     5,
		},
	}
	_, _ = c.CreateEvent(parentEvent)

	eventsOut := c.GetEvents(startTime, startTime.AddDate(0, 0, 5))
	if len(eventsOut) != 5 {
		t.Fatalf("expected generated events, got %d", len(eventsOut))
	}

	targetEvent := eventsOut[2]
	updatedTarget := targetEvent
	originalFrom := targetEvent.From

	updatedTarget.Title = "Daily event - update"
	updatedTarget.From = startTime.Add(time.Hour)
	updatedTarget.To = startTime.Add(2 * time.Hour)

	_, err := c.UpdateRepeatingEvent(targetEvent, updatedTarget, core.Current)
	if err != nil {
		t.Errorf("failed to update generated event (Current): %v", err)
	}

	parentOut, _ := c.GetEvent(parentId)
	foundException := false
	for _, ex := range parentOut.Repeat.Exceptions {
		t := time.Unix(int64(binary.BigEndian.Uint32(ex[12:16])), 0)
		if t.Equal(originalFrom) {
			foundException = true
			break
		}
	}
	if !foundException {
		t.Errorf("parent event did not receive the exception for time: %s", originalFrom)
	}

	isolatedOut := c.GetEvents(updatedTarget.From, updatedTarget.To)[0]
	if !isolatedOut.From.Equal(startTime.Add(time.Hour)) {
		t.Errorf("isolated event doesnt have the right From")
	}
	if !isolatedOut.To.Equal(startTime.Add(2 * time.Hour)) {
		t.Errorf("isolated event doesnt have the right To")
	}
	if isolatedOut.Repeat != nil {
		t.Errorf("isolated event should not have a repeat struct")
	}
}

func Test_UpdateGeneratedEvent_Following_Works(t *testing.T) {
	c := core.NewCore()
	_ = c.CreateCalendar(TestCalendarName)

	parentId := uuid.New()
	startTime := time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC)
	parentEvent := core.Event{
		Id:       parentId,
		Calendar: TestCalendarName,
		Title:    "Daily Meeting",
		From:     startTime,
		To:       startTime.Add(time.Hour),
		Repeat: &core.Repetition{
			Frequency: core.Day,
			Interval:  1,
			Until:     startTime.AddDate(0, 1, 0),
		},
	}
	_, _ = c.CreateEvent(parentEvent)

	eventsOut := c.GetEvents(startTime, startTime.AddDate(0, 0, 21))
	targetEvent := eventsOut[2]
	updatedTarget := targetEvent
	originalFrom := targetEvent.From

	updatedTarget.Title = "Weekly Meeting - New Phase"
	updatedTarget.Repeat = &core.Repetition{
		Frequency: core.Day,
		Interval:  1,
		Until:     startTime.AddDate(0, 1, 0),
	}
	newParentOut, err := c.UpdateRepeatingEvent(targetEvent, updatedTarget, core.Following)
	if err != nil {
		t.Errorf("failed to update generated event (Following): %v", err)
	}
	if newParentOut.ParentId != uuid.Nil {
		t.Errorf("new event should be a parent, but ParentId is %s", newParentOut.ParentId)
	}
	if newParentOut.Title != "Weekly Meeting - New Phase" {
		t.Errorf("title not updated on new parent")
	}

	olderParentOut, err := c.GetEvent(parentId)
	if err != nil {
		t.Fatalf("failed to get parent out: %v", err)
	}
	if !olderParentOut.Repeat.Until.Equal(originalFrom) {
		t.Errorf("parent event Until was not capped correctly. Expected %s, got %s", originalFrom, olderParentOut.Repeat.Until)
	}
	if olderParentOut.Repeat.Count != 0 {
		t.Errorf("parent event Count should be overridden to 0, got %d", olderParentOut.Repeat.Count)
	}
}

func Test_UpdateGeneratedEvent_All_Works(t *testing.T) {
	c := core.NewCore()
	_ = c.CreateCalendar(TestCalendarName)

	parentId := uuid.New()
	startTime := time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC)
	parentEvent := core.Event{
		Id:       parentId,
		Calendar: TestCalendarName,
		Title:    "Monthly Review",
		From:     startTime,
		To:       startTime.Add(time.Hour),
		Repeat: &core.Repetition{
			Frequency: core.Month,
			Interval:  1,
			Count:     5,
		},
	}

	_, _ = c.CreateEvent(parentEvent)

	eventsOut := c.GetEvents(startTime, startTime.AddDate(0, 6, 0))
	targetEvent, _ := c.GetEvent(eventsOut[0].ParentId)

	shift := 2 * time.Hour
	targetEvent.From = targetEvent.From.Add(shift)
	targetEvent.To = targetEvent.To.Add(shift)
	targetEvent.Title = "Monthly Review - Shifted"
	targetEvent.Repeat = &core.Repetition{
		Frequency: core.Month,
		Interval:  1,
		Count:     5,
	}

	_, err := c.UpdateEvent(*targetEvent)
	if err != nil {
		t.Errorf("failed to update generated event (All): %v", err)
	}

	parentOut, _ := c.GetEvent(parentId)
	expectedNewFrom := startTime.Add(shift)
	if !parentOut.From.Equal(expectedNewFrom) {
		t.Errorf("parent event From was not shifted. Expected %s, got %s", expectedNewFrom, parentOut.From)
	}
	if parentOut.Title != "Monthly Review - Shifted" {
		t.Errorf("parent event Title was not updated")
	}
}

func Test_UpdateEvent_FromStandardToRepeating_Works(t *testing.T) {
	c := core.NewCore()
	_ = c.CreateCalendar(TestCalendarName)

	startTime := time.Date(2026, 5, 5, 15, 0, 0, 0, time.UTC)
	eventIn := core.Event{
		Id:       uuid.New(),
		Calendar: TestCalendarName,
		Title:    "One-time meeting",
		From:     startTime,
		To:       startTime.Add(time.Hour),
	}

	_, err := c.CreateEvent(eventIn)
	if err != nil {
		t.Fatalf("failed to create an event: %v", err)
	}

	// now update it to be a repeating event
	eventIn.Title = "Weekly meeting"
	eventIn.Repeat = &core.Repetition{
		Frequency: core.Week,
		Interval:  1,
		Count:     3,
	}

	_, err = c.UpdateEvent(eventIn)
	if err != nil {
		t.Fatalf("failed to update event to repeating: %v", err)
	}

	eventsOut := c.GetEvents(startTime, startTime.AddDate(0, 1, 0))
	if len(eventsOut) != 3 {
		t.Errorf("expected 3 events after update, got %d", len(eventsOut))
	}

	updatedParent, err := c.GetEvent(eventIn.Id)
	if err != nil {
		t.Fatalf("failed to get parent event after update: %v", err)
	}
	if updatedParent.Repeat == nil || updatedParent.Repeat.Count != 3 {
		t.Errorf("parent event was not correctly updated to be repeating")
	}
}
