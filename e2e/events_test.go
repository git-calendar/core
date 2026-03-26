package e2e

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"reflect"
	"slices"
	"strconv"
	"testing"
	"time"

	"github.com/firu11/git-calendar-core/pkg/core"
	"github.com/firu11/git-calendar-core/pkg/filesystem"
	"github.com/google/uuid"
)

// It is kinda e2e, but not entirely. TODO rethink this.

const TestCalendarName = "test"

func Test_CreateCalendar_Works(t *testing.T) {
	a := core.NewCore()

	err := a.CreateCalendar(TestCalendarName)
	if err != nil {
		t.Fatalf("failed to init repo: %v", err)
	}

	home, err := os.UserHomeDir()
	if err != nil {
		t.Errorf("failed to get home dir: %v", err)
	}

	dirs, err := os.ReadDir(filepath.Join(home, filesystem.DirName))
	if err != nil {
		t.Errorf("failed to read event json file: %v", err)
	}

	var found bool
	for _, d := range dirs {
		if d.Name() == TestCalendarName {
			found = true
			break
		}
	}
	if !found {
		t.Error("directory not found")
	}
}

func Test_AddEvent_CreatesJsonFile(t *testing.T) {
	a := core.NewCore()

	err := a.CreateCalendar(TestCalendarName)
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

	_, err = a.CreateEvent(eventIn)
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
		Id    uuid.UUID `json:"id"`
		Title string    `json:"title"`
	}
	err = json.Unmarshal(b, &parsedEvent)
	if err != nil {
		t.Fatalf("failed to parse event json file: %v", err)
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

	err := a.CreateCalendar(TestCalendarName)
	if err != nil {
		t.Fatalf("failed to init repo: %v", err)
	}

	id := uuid.New()
	eventIn := core.Event{
		Id:       id,
		Calendar: TestCalendarName,
		Title:    "Foo Event",
		From:     time.Now(),                    // right now
		To:       time.Now().Add(2 * time.Hour), // two hours from now
	}

	_, err = a.CreateEvent(eventIn)
	if err != nil {
		t.Errorf("failed to create an event: %v", err)
	}

	eventOut, err := a.GetEvent(id)
	if err != nil {
		t.Fatalf("failed to get an event by id: %v", err)
	}

	if !reflect.DeepEqual(eventIn, *eventOut) {
		t.Errorf("events are not the same: \nin:  %+v\n!=\nout: %+v", eventIn, *eventOut)
	}
}

func Test_AddEventsAndGetThemByInterval(t *testing.T) {
	a := core.NewCore()

	err := a.CreateCalendar(TestCalendarName)
	if err != nil {
		t.Fatalf("failed to init repo: %v", err)
	}

	date := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	numEvents := 5
	for i := range numEvents {
		id := uuid.New()
		from := date.AddDate(0, 0, i)
		to := date.AddDate(0, 0, i).Add(time.Hour)
		eventIn := core.Event{
			Id:       id,
			Calendar: TestCalendarName,
			Title:    "Event" + strconv.Itoa(i),
			From:     from,
			To:       to,
			Repeat:   nil,
		}
		_, err = a.CreateEvent(eventIn)
		if err != nil {
			t.Errorf("failed to create an event: %v", err)
		}
	}

	eventsOut := a.GetEvents(date, date.AddDate(0, 1, 0))
	if len(eventsOut) != numEvents {
		t.Errorf("not the correct number of events: got %d, want %d", len(eventsOut), numEvents)
		t.Errorf("eventsOut: %v", eventsOut)
	}
}

func Test_AddInfinitelyRepeatingEventAndGetEvents_Works(t *testing.T) {
	a := core.NewCore()

	err := a.CreateCalendar(TestCalendarName)
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
	_, err = a.CreateEvent(eventIn)
	if err != nil {
		t.Errorf("failed to create an event: %v", err)
	}

	eventOut, err := a.GetEvent(id)
	if err != nil {
		t.Fatalf("failed to get an event by id: %v", err)
	}
	if !reflect.DeepEqual(eventIn, *eventOut) {
		t.Errorf("events are not the same: \nin:  %+v\n!=\nout: %+v", eventIn, *eventOut)
	}

	queryTo := startTime.AddDate(1, 0, 0)
	eventsOut := a.GetEvents(startTime, queryTo)
	if len(eventsOut) != 53 { // 2026 has 53 weeks
		t.Errorf("not all events were generated; eventsOut: %d: %+v", len(eventsOut), eventsOut)
	}
}

func Test_AddCountRepeatingEventAndGetEvents_Works(t *testing.T) {
	a := core.NewCore()

	err := a.CreateCalendar(TestCalendarName)
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
	_, err = a.CreateEvent(eventIn)
	if err != nil {
		t.Errorf("failed to create an event: %v", err)
	}

	eventOut, err := a.GetEvent(id)
	if err != nil {
		t.Fatalf("failed to get an event by id: %v", err)
	}
	if !reflect.DeepEqual(eventIn, *eventOut) {
		t.Errorf("events are not the same: \nin:  %+v\n!=\nout: %+v", eventIn, *eventOut)
	}

	queryFrom := time.Now().AddDate(-1, 0, 0)
	queryTo := time.Now().AddDate(1, 0, 0)
	eventsOut := a.GetEvents(queryFrom, queryTo)
	if len(eventsOut) != COUNT {
		t.Errorf("not all events were generated; eventsOut: %d: %+v", len(eventsOut), eventsOut)
	}
}

func Test_AddNormalEventsAndRemoveEvent_Works(t *testing.T) {
	a := core.NewCore()

	err := a.CreateCalendar(TestCalendarName)
	if err != nil {
		t.Fatalf("failed to init repo: %v", err)
	}

	date := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	numEvents := 5
	events := make([]core.Event, numEvents)
	for i := range numEvents {
		id := uuid.New()
		from := date.AddDate(0, 0, i)
		to := date.AddDate(0, 0, i).Add(time.Hour)
		eventIn := core.Event{
			Id:       id,
			Calendar: TestCalendarName,
			Title:    "Event" + strconv.Itoa(i),
			From:     from,
			To:       to,
			Repeat:   nil,
		}
		events[i] = eventIn
		_, err = a.CreateEvent(eventIn)
		if err != nil {
			t.Errorf("failed to create an event: %v", err)
		}
	}

	eventsOut := a.GetEvents(date, date.AddDate(0, 1, 0))
	if len(eventsOut) != numEvents {
		t.Errorf("not the correct number of events: got %d, want %d", len(eventsOut), numEvents)
		t.Errorf("eventsOut: %v", eventsOut)
	}

	a.RemoveEvent(events[0])
	eventsOut = a.GetEvents(date, date.AddDate(0, 1, 0))
	if len(eventsOut) != numEvents-1 {
		t.Errorf("not the correct number of events: got %d, want %d", len(eventsOut), numEvents)
		t.Errorf("eventsOut: %v", eventsOut)
	}
}

func Test_AddNormalEventsInSameIntervalAndRemoveEvents_Works(t *testing.T) {
	a := core.NewCore()

	err := a.CreateCalendar(TestCalendarName)
	if err != nil {
		t.Fatalf("failed to init repo: %v", err)
	}

	date := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	numEvents := 5
	events := make([]core.Event, numEvents)
	for i := range numEvents {
		id := uuid.New()
		from := date.AddDate(0, 0, 1)
		to := date.AddDate(0, 0, 1).Add(time.Hour)
		eventIn := core.Event{
			Id:       id,
			Calendar: TestCalendarName,
			Title:    "Event" + strconv.Itoa(i),
			From:     from,
			To:       to,
			Repeat:   nil,
		}
		events[i] = eventIn
		_, err = a.CreateEvent(eventIn)
		if err != nil {
			t.Errorf("failed to create an event: %v", err)
		}
	}

	for i := range events {
		a.RemoveEvent(events[i])
	}

	eventsOut := a.GetEvents(date, date.AddDate(0, 1, 0))
	if len(eventsOut) != 0 {
		t.Errorf("not the correct number of events: got %d, want %d", len(eventsOut), 0)
		t.Errorf("eventsOut: %v", eventsOut)
	}
}

func Test_AddRepeatingEventsAndRemoveGeneratedEvent_Works(t *testing.T) {
	a := core.NewCore()

	err := a.CreateCalendar(TestCalendarName)
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
	_, err = a.CreateEvent(eventIn)
	if err != nil {
		t.Errorf("failed to create an event: %v", err)
	}

	eventOut, err := a.GetEvent(id)
	if err != nil {
		t.Fatalf("failed to get an event by id: %v", err)
	}
	if !reflect.DeepEqual(eventIn, *eventOut) {
		t.Errorf("events are not the same: \nin:  %+v\n!=\nout: %+v", eventIn, *eventOut)
	}

	queryFrom := time.Now().AddDate(-1, 0, 0)
	queryTo := time.Now().AddDate(1, 0, 0)
	eventsOut := a.GetEvents(queryFrom, queryTo)
	if len(eventsOut) != COUNT {
		t.Errorf("not all events were generated; eventsOut: %d: %+v", len(eventsOut), eventsOut)
		return
	}
	eventToRemove := eventsOut[0]
	if err := a.RemoveRepeatingEvent(eventToRemove, core.Current); err != nil {
		t.Errorf("failed to remove event: %v", err)
	}

	eventsOut = a.GetEvents(queryFrom, queryTo)
	if len(eventsOut) != COUNT-1 || slices.Contains(eventsOut, eventToRemove) {
		t.Errorf("event wasn't removed correctly; eventsOut: %d: %+v", len(eventsOut), eventsOut)
	}
}

func Test_RemoveEvent_DeletesJsonFile(t *testing.T) {
	a := core.NewCore()

	err := a.CreateCalendar(TestCalendarName)
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

	_, err = a.CreateEvent(eventIn)
	if err != nil {
		t.Fatalf("failed to create an event: %v", err)
	}

	out, err := a.GetEvent(id)
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

	err = a.RemoveEvent(eventIn)
	if err != nil {
		t.Errorf("failed to remove event: %v", err)
	}

	if _, err := os.Stat(filePath); !os.IsNotExist(err) {
		t.Errorf("file was not deleted: %s", filePath)
	}
}

func Test_UpdateStandardEvent_Works(t *testing.T) {
	a := core.NewCore()

	err := a.CreateCalendar(TestCalendarName)
	if err != nil {
		t.Fatalf("failed to init repo: %v", err)
	}

	id := uuid.New()
	startTime := time.Now()
	eventIn := core.Event{
		Id:       id,
		Calendar: TestCalendarName,
		Title:    "Original Title",
		From:     startTime,
		To:       startTime.Add(time.Hour),
	}

	_, err = a.CreateEvent(eventIn)
	if err != nil {
		t.Errorf("failed to create an event: %v", err)
	}

	eventIn.Title = "Updated Title"
	eventIn.To = startTime.Add(2 * time.Hour)

	updatedEvent, err := a.UpdateEvent(eventIn)
	if err != nil {
		t.Errorf("failed to update event: %v", err)
	}

	eventOut, err := a.GetEvent(id)
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

func Test_UpdateGeneratedEvent_Current_Works(t *testing.T) {
	a := core.NewCore()
	_ = a.CreateCalendar(TestCalendarName)

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
	_, _ = a.CreateEvent(parentEvent)

	eventsOut := a.GetEvents(startTime, startTime.AddDate(0, 0, 5))
	if len(eventsOut) != 5 {
		t.Fatalf("expected generated events, got %d", len(eventsOut))
	}

	targetEvent := eventsOut[2]
	updatedTarget := targetEvent
	originalFrom := targetEvent.From

	updatedTarget.Title = "Daily event - update"
	updatedTarget.From = startTime.Add(time.Hour)
	updatedTarget.To = startTime.Add(2 * time.Hour)

	_, err := a.UpdateRepeatingEvent(targetEvent, updatedTarget, core.Current)
	if err != nil {
		t.Errorf("failed to update generated event (Current): %v", err)
	}

	parentOut, _ := a.GetEvent(parentId)
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

	isolatedOut := a.GetEvents(updatedTarget.From, updatedTarget.To)[0]
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
	a := core.NewCore()
	_ = a.CreateCalendar(TestCalendarName)

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
	_, _ = a.CreateEvent(parentEvent)

	eventsOut := a.GetEvents(startTime, startTime.AddDate(0, 0, 21))
	targetEvent := eventsOut[2]
	updatedTarget := targetEvent
	originalFrom := targetEvent.From

	updatedTarget.Title = "Weekly Meeting - New Phase"
	updatedTarget.Repeat = &core.Repetition{
		Frequency: core.Day,
		Interval:  1,
		Until:     startTime.AddDate(0, 1, 0),
	}
	newParentOut, err := a.UpdateRepeatingEvent(targetEvent, updatedTarget, core.Following)
	if err != nil {
		t.Errorf("failed to update generated event (Following): %v", err)
	}
	if newParentOut.ParentId != uuid.Nil {
		t.Errorf("new event should be a parent, but ParentId is %s", newParentOut.ParentId)
	}
	if newParentOut.Title != "Weekly Meeting - New Phase" {
		t.Errorf("title not updated on new parent")
	}

	olderParentOut, err := a.GetEvent(parentId)
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
	a := core.NewCore()
	_ = a.CreateCalendar(TestCalendarName)

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

	_, _ = a.CreateEvent(parentEvent)

	eventsOut := a.GetEvents(startTime, startTime.AddDate(0, 6, 0))
	targetEvent, _ := a.GetEvent(eventsOut[0].ParentId)

	shift := 2 * time.Hour
	targetEvent.From = targetEvent.From.Add(shift)
	targetEvent.To = targetEvent.To.Add(shift)
	targetEvent.Title = "Monthly Review - Shifted"
	targetEvent.Repeat = &core.Repetition{
		Frequency: core.Month,
		Interval:  1,
		Count:     5,
	}

	_, err := a.UpdateEvent(*targetEvent)
	if err != nil {
		t.Errorf("failed to update generated event (All): %v", err)
	}

	parentOut, _ := a.GetEvent(parentId)
	expectedNewFrom := startTime.Add(shift)
	if !parentOut.From.Equal(expectedNewFrom) {
		t.Errorf("parent event From was not shifted. Expected %s, got %s", expectedNewFrom, parentOut.From)
	}
	if parentOut.Title != "Monthly Review - Shifted" {
		t.Errorf("parent event Title was not updated")
	}
}

func Test_UpdateEvent_FromStandardToRepeating_Works(t *testing.T) {
	a := core.NewCore()
	_ = a.CreateCalendar(TestCalendarName)

	id := uuid.New()
	startTime := time.Date(2026, 5, 5, 15, 0, 0, 0, time.UTC)
	eventIn := core.Event{
		Id:       id,
		Calendar: TestCalendarName,
		Title:    "One-time meeting",
		From:     startTime,
		To:       startTime.Add(time.Hour),
	}

	_, err := a.CreateEvent(eventIn)
	if err != nil {
		t.Fatalf("failed to create an event: %v", err)
	}

	// Now, update it to be a repeating event
	eventIn.Title = "Weekly meeting"
	eventIn.Repeat = &core.Repetition{
		Frequency: core.Week,
		Interval:  1,
		Count:     3,
	}

	_, err = a.UpdateEvent(eventIn)
	if err != nil {
		t.Fatalf("failed to update event to repeating: %v", err)
	}

	eventsOut := a.GetEvents(startTime, startTime.AddDate(0, 1, 0))
	if len(eventsOut) != 3 {
		t.Errorf("expected 3 events after update, got %d", len(eventsOut))
	}

	updatedParent, err := a.GetEvent(id)
	if err != nil {
		t.Fatalf("failed to get parent event after update: %v", err)
	}
	if updatedParent.Repeat == nil || updatedParent.Repeat.Count != 3 {
		t.Errorf("parent event was not correctly updated to be repeating")
	}
}
