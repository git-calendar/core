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

func TestAddInfinitelyRepeatingEventAndGetEvents(t *testing.T) {
	c := core.NewCore()

	err := c.CreateCalendar(TestCalendarName, "")
	if err != nil {
		t.Fatalf("failed to init repo: %v", err)
	}

	t.Cleanup(func() {
		_ = c.RemoveCalendar(TestCalendarName)
	})

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

func TestAddCountRepeatingEventAndGetEvents(t *testing.T) {
	c := core.NewCore()

	err := c.CreateCalendar(TestCalendarName, "")
	if err != nil {
		t.Fatalf("failed to init repo: %v", err)
	}

	t.Cleanup(func() {
		_ = c.RemoveCalendar(TestCalendarName)
	})

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

func TestAddRepeatingEventsAndRemoveRepeatingEvent(t *testing.T) {
	c := core.NewCore()

	err := c.CreateCalendar(TestCalendarName, "")
	if err != nil {
		t.Fatalf("failed to init repo: %v", err)
	}

	t.Cleanup(func() {
		_ = c.RemoveCalendar(TestCalendarName)
	})

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

func TestUpdateRepeatingEvent_Current(t *testing.T) {
	c := core.NewCore()

	err := c.CreateCalendar(TestCalendarName, "")
	if err != nil {
		t.Fatalf("failed to init repo: %v", err)
	}

	t.Cleanup(func() {
		_ = c.RemoveCalendar(TestCalendarName)
	})

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
		t.Fatalf("expected child events, got %d", len(eventsOut))
	}

	targetEvent := eventsOut[2]
	updatedTarget := targetEvent
	originalFrom := targetEvent.From

	updatedTarget.Title = "Daily event - update"
	updatedTarget.From = startTime.Add(time.Hour)
	updatedTarget.To = startTime.Add(2 * time.Hour)

	_, err = c.UpdateRepeatingEvent(targetEvent, updatedTarget, core.Current)
	if err != nil {
		t.Errorf("failed to update child event (Current): %v", err)
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

func TestUpdateRepeatingEvent_Following(t *testing.T) {
	c := core.NewCore()

	err := c.CreateCalendar(TestCalendarName, "")
	if err != nil {
		t.Fatalf("failed to init repo: %v", err)
	}

	t.Cleanup(func() {
		_ = c.RemoveCalendar(TestCalendarName)
	})

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
	expectedUntilCap := eventsOut[1].From

	updatedTarget.Title = "Weekly Meeting - New Phase"
	updatedTarget.Repeat = &core.Repetition{
		Frequency: core.Day,
		Interval:  1,
		Until:     startTime.AddDate(0, 1, 0),
	}
	newParentOut, err := c.UpdateRepeatingEvent(targetEvent, updatedTarget, core.Following)
	if err != nil {
		t.Errorf("failed to update child event (Following): %v", err)
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
	if !olderParentOut.Repeat.Until.Equal(expectedUntilCap) {
		t.Errorf("parent event Until was not capped correctly. Expected %s, got %s", expectedUntilCap, olderParentOut.Repeat.Until)
	}
	if olderParentOut.Repeat.Count != 0 {
		t.Errorf("parent event Count should be overridden to 0, got %d", olderParentOut.Repeat.Count)
	}
}

func TestUpdateRepeatingEvent_All(t *testing.T) {
	c := core.NewCore()

	err := c.CreateCalendar(TestCalendarName, "")
	if err != nil {
		t.Fatalf("failed to init repo: %v", err)
	}

	t.Cleanup(func() {
		_ = c.RemoveCalendar(TestCalendarName)
	})

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

	_, err = c.UpdateEvent(*targetEvent)
	if err != nil {
		t.Errorf("failed to update child event (All): %v", err)
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

func TestUpdateEvent_FromStandardToRepeating(t *testing.T) {
	c := core.NewCore()

	err := c.CreateCalendar(TestCalendarName, "")
	if err != nil {
		t.Fatalf("failed to init repo: %v", err)
	}

	t.Cleanup(func() {
		_ = c.RemoveCalendar(TestCalendarName)
	})

	startTime := time.Date(2026, 5, 5, 15, 0, 0, 0, time.UTC)
	eventIn := core.Event{
		Id:       uuid.New(),
		Calendar: TestCalendarName,
		Title:    "One-time meeting",
		From:     startTime,
		To:       startTime.Add(time.Hour),
	}

	_, err = c.CreateEvent(eventIn)
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

func TestUpdateFollowing_ExceptionCarriedToNewParent(t *testing.T) {
	c := core.NewCore()

	err := c.CreateCalendar(TestCalendarName, "")
	if err != nil {
		t.Fatalf("failed to init repo: %v", err)
	}

	t.Cleanup(func() {
		_ = c.RemoveCalendar(TestCalendarName)
	})

	const count = 5
	startTime := time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC)
	parentEvent := core.Event{
		Id:       uuid.New(),
		Calendar: TestCalendarName,
		Title:    "Daily Standup",
		From:     startTime,
		To:       startTime.Add(time.Hour),
		Repeat: &core.Repetition{
			Frequency: core.Day,
			Interval:  1,
			Count:     count,
		},
	}
	_, err = c.CreateEvent(parentEvent)
	if err != nil {
		t.Fatalf("failed to create event: %v", err)
	}

	allEvents := c.GetEvents(startTime, startTime.AddDate(0, 0, count+5))
	if len(allEvents) != count {
		t.Fatalf("setup: expected %d events, got %d", count, len(allEvents))
	}

	// delete the 4th child - creates an exception on the parent
	if err := c.RemoveRepeatingEvent(allEvents[3], core.Current); err != nil {
		t.Fatalf("failed to remove 4th event: %v", err)
	}

	// update the 2nd child (and all following) by shifting it 1h
	const shift = time.Hour
	secondEvent := allEvents[1]
	updatedSecond := secondEvent
	updatedSecond.From = secondEvent.From.Add(shift)
	updatedSecond.To = secondEvent.To.Add(shift)
	updatedSecond.Repeat = &core.Repetition{
		Frequency: core.Day,
		Interval:  1,
		Count:     count,
	}

	_, err = c.UpdateRepeatingEvent(secondEvent, updatedSecond, core.Following)
	if err != nil {
		t.Fatalf("failed to Following-update second event: %v", err)
	}

	result := c.GetEvents(startTime, startTime.AddDate(0, 0, count+5))
	if len(result) != count-1 {
		t.Fatalf("expected %d events after delete+split, got %d: %+v", count-1, len(result), result)
	}

	type expected struct {
		from time.Time
		desc string
	}
	wants := []expected{
		{startTime, "day 0 unshifted (original parent, capped exclusively)"},
		{startTime.AddDate(0, 0, 1).Add(shift), "day 1 shifted"},
		{startTime.AddDate(0, 0, 2).Add(shift), "day 2 shifted"},
		// day 3 gap - exception should carry and time-adjust
		{startTime.AddDate(0, 0, 4).Add(shift), "day 4 shifted"},
	}

	for i, w := range wants {
		if !result[i].From.Equal(w.from) {
			t.Errorf("result[%d] (%s): expected From %s, got %s", i, w.desc, w.from, result[i].From)
		}
	}
}
