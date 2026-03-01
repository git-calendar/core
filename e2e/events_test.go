package e2e

import (
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
		t.Errorf("failed to init repo: %v", err)
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
		t.Errorf("failed to init repo: %v", err)
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

	err := a.CreateCalendar(TestCalendarName)
	if err != nil {
		t.Errorf("failed to init repo: %v", err)
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
		t.Errorf("failed to get an event by id: %v", err)
	}

	if !reflect.DeepEqual(eventIn, *eventOut) {
		t.Errorf("events are not the same: \nin:  %+v\n!=\nout: %+v", eventIn, eventOut)
	}
}

func Test_AddEventsAndGetThemByInterval(t *testing.T) {
	a := core.NewCore()

	err := a.CreateCalendar(TestCalendarName)
	if err != nil {
		t.Errorf("failed to init repo: %v", err)
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
	eventsOut, err := a.GetEvents(date, date.AddDate(0, 1, 0))
	if err != nil {
		t.Errorf("failed to get events: %v", err)
	}
	if len(eventsOut) != numEvents {
		t.Errorf("not the correct number of events: got %d, want %d", len(eventsOut), numEvents)
		t.Errorf("eventsOut: %v", eventsOut)
	}
}

func Test_AddInfinitelyRepeatingEventAndGetEvents_Works(t *testing.T) {
	a := core.NewCore()

	err := a.CreateCalendar(TestCalendarName)
	if err != nil {
		t.Errorf("failed to init repo: %v", err)
	}

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
			Count:     -1,
			Until:     time.Date(9999, 12, 31, 23, 59, 59, 0, time.UTC),
		},
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

	queryFrom := time.Now().AddDate(0, 0, 6)
	queryTo := queryFrom.AddDate(0, 1, 0)
	eventsOut, err := a.GetEvents(queryFrom, queryTo)
	if err != nil {
		t.Errorf("failed to get an events by interval: %v", err)
	}
	if !(len(eventsOut) == 4 || len(eventsOut) == 5) { // can fit 5 weeks
		t.Errorf("not all events were generated: %v", err)
		t.Errorf("eventsOut: %d: %+v", len(eventsOut), eventsOut)
	}
}

func Test_AddCountRepeatingEventAndGetEvents_Works(t *testing.T) {
	a := core.NewCore()

	err := a.CreateCalendar(TestCalendarName)
	if err != nil {
		t.Errorf("failed to init repo: %v", err)
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
			Until:     time.Time{},
		},
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

	queryFrom := time.Now().AddDate(0, 0, 6)
	queryTo := queryFrom.AddDate(0, 2, 0)
	eventsOut, err := a.GetEvents(queryFrom, queryTo)
	if err != nil {
		t.Errorf("failed to get an events by interval: %v", err)
	}
	if len(eventsOut) != COUNT {
		t.Errorf("not all events were generated: %v", err)
		t.Errorf("eventsOut: %d: %+v", len(eventsOut), eventsOut)
	}
}

func Test_AddNormalEventsAndRemoveEvent_Works(t *testing.T) {
	a := core.NewCore()

	err := a.CreateCalendar(TestCalendarName)
	if err != nil {
		t.Errorf("failed to init repo: %v", err)
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
	eventsOut, err := a.GetEvents(date, date.AddDate(0, 1, 0))
	if err != nil {
		t.Errorf("failed to get events: %v", err)
	}

	if len(eventsOut) != numEvents {
		t.Errorf("not the correct number of events: got %d, want %d", len(eventsOut), numEvents)
		t.Errorf("eventsOut: %v", eventsOut)
	}

	a.RemoveEvent(events[0])
	eventsOut, err = a.GetEvents(date, date.AddDate(0, 1, 0))
	if err != nil {
		t.Errorf("failed to get events: %v", err)
	}

	if len(eventsOut) != numEvents-1 {
		t.Errorf("not the correct number of events: got %d, want %d", len(eventsOut), numEvents)
		t.Errorf("eventsOut: %v", eventsOut)
	}
}

func Test_AddNormalEventsInSameIntervalAndRemoveEvents_Works(t *testing.T) {
	a := core.NewCore()

	err := a.CreateCalendar(TestCalendarName)
	if err != nil {
		t.Errorf("failed to init repo: %v", err)
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
	eventsOut, err := a.GetEvents(date, date.AddDate(0, 1, 0))
	if err != nil {
		t.Errorf("failed to get events: %v", err)
	}

	if len(eventsOut) != 0 {
		t.Errorf("not the correct number of events: got %d, want %d", len(eventsOut), 0)
		t.Errorf("eventsOut: %v", eventsOut)
	}
}

func Test_AddRepeatingEventsAndRemoveGeneratedEvent_Works(t *testing.T) {
	a := core.NewCore()

	err := a.CreateCalendar(TestCalendarName)
	if err != nil {
		t.Errorf("failed to init repo: %v", err)
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
			Until:     time.Time{},
		},
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

	queryFrom := time.Now().AddDate(0, 0, 6)
	queryTo := queryFrom.AddDate(0, 2, 0)
	eventsOut, err := a.GetEvents(queryFrom, queryTo)
	if err != nil {
		t.Errorf("failed to get an events by interval: %v", err)
	}

	if len(eventsOut) != COUNT {
		t.Errorf("not all events were generated: %v", err)
		t.Errorf("eventsOut: %d: %+v", len(eventsOut), eventsOut)
	}
	eventToRemove := eventsOut[0]
	err = a.RemoveEvent(eventToRemove)
	if err != nil {
		t.Errorf("failed to remove event: %v", err)
	}

	eventsOut, err = a.GetEvents(queryFrom, queryTo)
	if err != nil {
		t.Errorf("failed to get an events by interval: %v", err)
	}

	if len(eventsOut) != COUNT-1 && slices.Contains(eventsOut, eventToRemove) {
		t.Errorf("event wasn't removed correctly %v", err)
		t.Errorf("eventsOut: %d: %+v", len(eventsOut), eventsOut)
	}
}

func Test_RemoveEvent_DeletesJsonFile(t *testing.T) {
	a := core.NewCore()

	err := a.CreateCalendar(TestCalendarName)
	if err != nil {
		t.Errorf("failed to init repo: %v", err)
	}

	from := time.Now()
	to := from.Add(1 * time.Hour)
	id := uuid.New()
	eventIn := core.Event{
		Id:       id,
		Calendar: TestCalendarName,
		Title:    "Event To Delete",
		From:     from,
		To:       to,
	}

	_, err = a.CreateEvent(eventIn)
	if err != nil {
		t.Errorf("failed to create an event: %v", err)
	}

	out, err := a.GetEvent(id)
	if err != nil || out == nil {
		t.Errorf("failed to get an event by id: %v", err)
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
		t.Errorf("failed to init repo: %v", err)
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
		t.Errorf("failed to get updated event: %v", err)
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

	masterId := uuid.New()
	startTime := time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC)
	masterEvent := core.Event{
		Id:       masterId,
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
	_, _ = a.CreateEvent(masterEvent)

	eventsOut, _ := a.GetEvents(startTime, startTime.AddDate(0, 0, 5))
	if len(eventsOut) != 5 {
		t.Fatalf("expected generated events, got %d", len(eventsOut))
	}

	targetEvent := eventsOut[2]
	originalFrom := targetEvent.From

	targetEvent.Title = "Daily event - update"
	targetEvent.From = startTime.Add(time.Hour)
	targetEvent.To = startTime.Add(time.Hour * 2)
	targetEvent.OriginalFrom = originalFrom

	_, err := a.UpdateEvent(targetEvent, core.Current)
	if err != nil {
		t.Errorf("failed to update generated event (Current): %v", err)
	}

	masterOut, _ := a.GetEvent(masterId)
	foundException := false
	for _, ex := range masterOut.Repeat.Exceptions {
		if ex.Time.Equal(originalFrom) {
			foundException = true
			break
		}
	}
	if !foundException {
		t.Errorf("master event did not receive the exception for time: %s", originalFrom)
	}

	isolatedOut, err := a.GetEvent(targetEvent.Id)
	if err != nil {
		t.Errorf("isolated event was not created: %v", err)
	}
	if isolatedOut.Repeat != nil {
		t.Errorf("isolated event should not have a repeat struct")
	}
}

func Test_UpdateGeneratedEvent_Following_Works(t *testing.T) {
	a := core.NewCore()
	_ = a.CreateCalendar(TestCalendarName)

	masterId := uuid.New()
	startTime := time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC)
	masterEvent := core.Event{
		Id:       masterId,
		Calendar: TestCalendarName,
		Title:    "Daily Meeting",
		From:     startTime,
		To:       startTime.Add(time.Hour),
		Repeat: &core.Repetition{
			Frequency: core.Day,
			Interval:  1,
			Until:     startTime.AddDate(0, 1, 0),
			Count:     -1,
		},
	}
	_, _ = a.CreateEvent(masterEvent)

	eventsOut, _ := a.GetEvents(startTime, startTime.AddDate(0, 0, 21))
	targetEvent := eventsOut[2]
	originalFrom := targetEvent.From
	targetEvent.Title = "Weekly Meeting - New Phase"
	targetEvent.Repeat = &core.Repetition{
		Frequency: core.Day,
		Interval:  1,
		Until:     startTime.AddDate(0, 1, 0),
		Count:     -1,
	}
	_, err := a.UpdateEvent(targetEvent, core.Following)
	if err != nil {
		t.Errorf("failed to update generated event (Following): %v", err)
	}

	masterOut, _ := a.GetEvent(masterId)
	if !masterOut.Repeat.Until.Equal(originalFrom) {
		t.Errorf("master event Until was not capped correctly. Expected %s, got %s", originalFrom, masterOut.Repeat.Until)
	}
	if masterOut.Repeat.Count != -1 {
		t.Errorf("master event Count should be overridden to -1, got %d", masterOut.Repeat.Count)
	}

	newMasterOut, _ := a.GetEvent(targetEvent.Id)
	if newMasterOut.MasterId != uuid.Nil {
		t.Errorf("new event should be a master, but MasterId is %s", newMasterOut.MasterId)
	}
	if newMasterOut.Title != "Weekly Meeting - New Phase" {
		t.Errorf("title not updated on new master")
	}
}

func Test_UpdateGeneratedEvent_All_Works(t *testing.T) {
	a := core.NewCore()
	_ = a.CreateCalendar(TestCalendarName)

	masterId := uuid.New()
	startTime := time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC)
	masterEvent := core.Event{
		Id:       masterId,
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

	_, _ = a.CreateEvent(masterEvent)

	eventsOut, _ := a.GetEvents(startTime, startTime.AddDate(0, 6, 0))
	targetEvent := eventsOut[0]

	shift := 2 * time.Hour
	targetEvent.From = targetEvent.From.Add(shift)
	targetEvent.To = targetEvent.To.Add(shift)
	targetEvent.Title = "Monthly Review - Shifted"
	targetEvent.Repeat = &core.Repetition{
		Frequency: core.Month,
		Interval:  1,
		Count:     5,
	}

	_, err := a.UpdateEvent(targetEvent, core.All)
	if err != nil {
		t.Errorf("failed to update generated event (All): %v", err)
	}

	masterOut, _ := a.GetEvent(masterId)
	expectedNewFrom := startTime.Add(shift)
	if !masterOut.From.Equal(expectedNewFrom) {
		t.Errorf("master event From was not shifted. Expected %s, got %s", expectedNewFrom, masterOut.From)
	}
	if masterOut.Title != "Monthly Review - Shifted" {
		t.Errorf("master event Title was not updated")
	}
}
