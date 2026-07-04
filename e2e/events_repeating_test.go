// It is kinda e2e, but not entirely. TODO rethink this.
package e2e

import (
	"testing"
	"time"

	"github.com/git-calendar/core/pkg/core"
	"github.com/google/uuid"
)

func TestRepeatingEvent_GetEvents_UntilWeekly_GeneratesOccurrencesInRange(t *testing.T) {
	c := newTestCore(t)

	startTime := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	parent := createRepeatingEvent(t, c, "Repeating Event", startTime, time.Hour*4, core.Repetition{
		Frequency: core.Week,
		Interval:  1,
		Until:     startTime.AddDate(1, 0, 0),
	})

	stored := requireEvent(t, c, parent.Id)
	if !stored.From.Equal(parent.From) {
		t.Fatalf("stored parent From mismatch: expected %s, got %s", parent.From, stored.From)
	}
	if stored.Repeat == nil {
		t.Fatalf("stored parent should be repeating")
	}

	events := c.GetEvents(startTime, startTime.AddDate(1, 0, 0), nil)
	if len(events) != 53 {
		t.Fatalf("expected 53 weekly events in 2026, got %d: %+v", len(events), events)
	}
}

func TestRepeatingEvent_GetEvents_CountWeekly_GeneratesExactCount(t *testing.T) {
	c := newTestCore(t)

	const count = 6
	startTime := time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC)
	createRepeatingEvent(t, c, "Repeating Event", startTime, time.Hour*4, core.Repetition{
		Frequency: core.Week,
		Interval:  1,
		Count:     count,
	})

	events := c.GetEvents(startTime.Add(-time.Hour), startTime.AddDate(0, 0, count*7+1), nil)
	assertEventStarts(t, events,
		startTime,
		startTime.AddDate(0, 0, 7),
		startTime.AddDate(0, 0, 14),
		startTime.AddDate(0, 0, 21),
		startTime.AddDate(0, 0, 28),
		startTime.AddDate(0, 0, 35),
	)
}

func TestRepeatingEvent_Remove_Current_AddsParentExceptionAndHidesChild(t *testing.T) {
	c := newTestCore(t)

	const count = 6
	startTime := time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC)
	parent := createRepeatingEvent(t, c, "Repeating Event", startTime, time.Hour, core.Repetition{
		Frequency: core.Day,
		Interval:  1,
		Count:     count,
	})

	events := c.GetEvents(startTime, startTime.AddDate(0, 0, count), nil)
	assertEventStarts(t, events,
		startTime,
		startTime.AddDate(0, 0, 1),
		startTime.AddDate(0, 0, 2),
		startTime.AddDate(0, 0, 3),
		startTime.AddDate(0, 0, 4),
		startTime.AddDate(0, 0, 5),
	)

	removed := requireEventAt(t, events, startTime.AddDate(0, 0, 2))
	if err := c.RemoveRepeatingEvent(removed, core.Current); err != nil {
		t.Fatalf("failed to remove child event: %v", err)
	}

	storedParent := requireEvent(t, c, parent.Id)
	assertParentHasException(t, storedParent, removed.Id)

	events = c.GetEvents(startTime, startTime.AddDate(0, 0, count), nil)
	assertNoEventAt(t, events, removed.From)
	if len(events) != count-1 {
		t.Fatalf("expected %d events after delete, got %d: %+v", count-1, len(events), events)
	}
}

func TestRepeatingEvent_Remove_Current_RejectsParentEvent(t *testing.T) {
	c := newTestCore(t)

	startTime := time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC)
	parent := createRepeatingEvent(t, c, "Repeating Event", startTime, time.Hour, core.Repetition{
		Frequency: core.Day,
		Interval:  1,
		Count:     3,
	})

	err := c.RemoveRepeatingEvent(parent, core.Current)
	if err == nil {
		t.Fatalf("expected parent delete through RemoveRepeatingEvent to fail")
	}
}

func TestRepeatingEvent_Update_Current_DetachesOnlyTargetChild(t *testing.T) {
	c := newTestCore(t)

	const count = 5
	startTime := time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC)
	parent := createRepeatingEvent(t, c, "Daily event", startTime, time.Hour, core.Repetition{
		Frequency: core.Day,
		Interval:  1,
		Count:     count,
	})

	events := c.GetEvents(startTime, startTime.AddDate(0, 0, count), nil)
	target := requireEventAt(t, events, startTime.AddDate(0, 0, 2))

	updated := cloneEvent(target)
	updated.Title = "Daily event - update"
	updated.From = target.From.Add(time.Hour)
	updated.To = target.To.Add(time.Hour)

	_, err := c.UpdateRepeatingEvent(target, updated, core.Current)
	if err != nil {
		t.Fatalf("failed to update child event with Current strategy: %v", err)
	}

	storedParent := requireEvent(t, c, parent.Id)
	assertParentHasException(t, storedParent, target.Id)

	events = c.GetEvents(startTime, startTime.AddDate(0, 0, count), nil)
	assertEventStarts(t, events,
		startTime,
		startTime.AddDate(0, 0, 1),
		startTime.AddDate(0, 0, 2).Add(time.Hour),
		startTime.AddDate(0, 0, 3),
		startTime.AddDate(0, 0, 4),
	)
	assertNoEventAt(t, events, target.From)

	detached := requireEventAt(t, events, updated.From)
	if detached.Repeat != nil {
		t.Fatalf("detached event should not repeat")
	}
	if detached.ParentId != nil {
		t.Fatalf("detached event should not have parent id, got %s", detached.ParentId)
	}
	if detached.Title != updated.Title {
		t.Fatalf("detached title mismatch: expected %q, got %q", updated.Title, detached.Title)
	}
}

func TestRepeatingEvent_Update_StrategiesRejectParentEvents(t *testing.T) {
	c := newTestCore(t)

	startTime := time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC)
	parent := createRepeatingEvent(t, c, "Repeating Event", startTime, time.Hour, core.Repetition{
		Frequency: core.Day,
		Interval:  1,
		Count:     3,
	})

	tests := []struct {
		name     string
		strategy core.UpdateStrategy
	}{
		{name: "Current", strategy: core.Current},
		{name: "Following", strategy: core.Following},
		{name: "All", strategy: core.All},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			updated := cloneEvent(parent)
			updated.Title = "Should fail"

			_, err := c.UpdateRepeatingEvent(parent, updated, tt.strategy)
			if err == nil {
				t.Fatalf("expected parent update with %s strategy to fail", tt.name)
			}
		})
	}
}

func TestRepeatingEvent_Update_Following_SplitsSeriesFromTargetChild(t *testing.T) {
	c := newTestCore(t)

	parentId := uuid.New()
	startTime := time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC)
	parent := core.Event{
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
	createEvent(t, c, parent)

	events := c.GetEvents(startTime, startTime.AddDate(0, 0, 21), nil)
	target := requireEventAt(t, events, startTime.AddDate(0, 0, 2))
	previous := requireEventAt(t, events, startTime.AddDate(0, 0, 1))

	updated := cloneEvent(target)
	updated.Title = "Daily Meeting - New Phase"
	updated.Repeat = &core.Repetition{
		Frequency: core.Day,
		Interval:  1,
		Until:     startTime.AddDate(0, 1, 0),
	}

	newParent, err := c.UpdateRepeatingEvent(target, updated, core.Following)
	if err != nil {
		t.Fatalf("failed to update child event with Following strategy: %v", err)
	}

	if newParent.ParentId != nil {
		t.Fatalf("new event should be a parent, got ParentId %s", newParent.ParentId)
	}
	if newParent.Title != updated.Title {
		t.Fatalf("new parent title mismatch: expected %q, got %q", updated.Title, newParent.Title)
	}
	if !newParent.From.Equal(target.From) {
		t.Fatalf("new parent From mismatch: expected %s, got %s", target.From, newParent.From)
	}

	oldParent := requireEvent(t, c, parentId)
	if oldParent.Repeat == nil {
		t.Fatalf("old parent should still repeat before the split")
	}
	if !oldParent.Repeat.Until.Equal(previous.From) {
		t.Fatalf("old parent Until mismatch: expected %s, got %s", previous.From, oldParent.Repeat.Until)
	}
	if oldParent.Repeat.Count != 0 {
		t.Fatalf("old parent Count should be reset to 0, got %d", oldParent.Repeat.Count)
	}
}

func TestRepeatingEvent_Update_Following_SecondChild_DoesNotLeaveInvalidOldParent(t *testing.T) {
	c := newTestCore(t)

	parentId := uuid.New()
	startTime := time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC)
	parent := core.Event{
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
	createEvent(t, c, parent)

	events := c.GetEvents(startTime, startTime.AddDate(0, 0, 21), nil)
	target := requireEventAt(t, events, startTime.AddDate(0, 0, 1))

	updated := cloneEvent(target)
	updated.Title = "Daily Meeting - New Phase"
	updated.Repeat = &core.Repetition{
		Frequency: core.Day,
		Interval:  1,
		Until:     startTime.AddDate(0, 1, 0),
	}

	newParent, err := c.UpdateRepeatingEvent(target, updated, core.Following)
	if err != nil {
		t.Fatalf("failed to update second child with Following strategy: %v", err)
	}
	if newParent.ParentId != nil {
		t.Fatalf("new event should be a parent, got ParentId %s", newParent.ParentId)
	}

	oldParent := requireEvent(t, c, parentId)
	if oldParent.Repeat != nil && !oldParent.Repeat.Until.After(oldParent.From) {
		t.Fatalf("old parent has invalid repetition boundary: From=%s Until=%s", oldParent.From, oldParent.Repeat.Until)
	}
}

func TestRepeatingEvent_Update_Following_CarriesAndShiftsFutureExceptions(t *testing.T) {
	c := newTestCore(t)

	startTime := time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC)
	createRepeatingEvent(t, c, "Daily Standup", startTime, time.Hour, core.Repetition{
		Frequency: core.Day,
		Interval:  1,
		Until:     startTime.AddDate(0, 0, 10),
	})

	events := c.GetEvents(startTime, startTime.AddDate(0, 0, 6), nil)
	second := cloneEvent(requireEventAt(t, events, startTime.AddDate(0, 0, 1)))
	fourth := requireEventAt(t, events, startTime.AddDate(0, 0, 3))

	if err := c.RemoveRepeatingEvent(fourth, core.Current); err != nil {
		t.Fatalf("failed to remove fourth child: %v", err)
	}

	shift := time.Hour
	updatedSecond := cloneEvent(second)
	updatedSecond.From = second.From.Add(shift)
	updatedSecond.To = second.To.Add(shift)
	updatedSecond.Repeat = &core.Repetition{
		Frequency: core.Day,
		Interval:  1,
		Until:     startTime.AddDate(0, 0, 10),
	}

	_, err := c.UpdateRepeatingEvent(second, updatedSecond, core.Following)
	if err != nil {
		t.Fatalf("failed to update second child with Following strategy: %v", err)
	}

	events = c.GetEvents(startTime, startTime.AddDate(0, 0, 5), nil)
	assertEventStarts(t, events,
		startTime,
		startTime.AddDate(0, 0, 1).Add(shift),
		startTime.AddDate(0, 0, 2).Add(shift),
		startTime.AddDate(0, 0, 4).Add(shift),
	)
	assertNoEventAt(t, events, startTime.AddDate(0, 0, 3).Add(shift))
}

func TestRepeatingEvent_Update_Following_FirstChild_ShiftBackDoesNotKeepOriginalParent(t *testing.T) {
	c := newTestCore(t)

	startTime := time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC)
	createRepeatingEvent(t, c, "Daily Standup", startTime, time.Hour, core.Repetition{
		Frequency: core.Day,
		Interval:  1,
		Count:     3,
	})

	events := c.GetEvents(startTime.Add(-time.Hour), startTime.AddDate(0, 0, 3), nil)
	first := requireEventAt(t, events, startTime)

	shift := -2 * time.Hour
	updatedFirst := cloneEvent(first)
	updatedFirst.From = first.From.Add(shift)
	updatedFirst.To = first.To.Add(shift)

	_, err := c.UpdateRepeatingEvent(first, updatedFirst, core.Following)
	if err != nil {
		t.Fatalf("failed to update first child with Following strategy: %v", err)
	}

	events = c.GetEvents(startTime.Add(shift), startTime.AddDate(0, 0, 3), nil)
	assertEventStarts(t, events,
		startTime.Add(shift),
		startTime.AddDate(0, 0, 1).Add(shift),
		startTime.AddDate(0, 0, 2).Add(shift),
	)
	assertNoEventAt(t, events, startTime)
}

func TestRepeatingEvent_Update_Following_TitleOnlyKeepsFutureDeletedChildHidden(t *testing.T) {
	c := newTestCore(t)

	startTime := time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC)
	createRepeatingEvent(t, c, "Daily Standup", startTime, time.Hour, core.Repetition{
		Frequency: core.Day,
		Interval:  1,
		Until:     startTime.AddDate(0, 0, 10),
	})

	events := c.GetEvents(startTime, startTime.AddDate(0, 0, 6), nil)
	second := cloneEvent(requireEventAt(t, events, startTime.AddDate(0, 0, 1)))
	fourth := requireEventAt(t, events, startTime.AddDate(0, 0, 3))

	if err := c.RemoveRepeatingEvent(fourth, core.Current); err != nil {
		t.Fatalf("failed to remove fourth child: %v", err)
	}

	updatedSecond := cloneEvent(second)
	updatedSecond.Title = "Daily Standup - New Phase"
	updatedSecond.Repeat = &core.Repetition{
		Frequency: core.Day,
		Interval:  1,
		Until:     startTime.AddDate(0, 0, 10),
	}

	_, err := c.UpdateRepeatingEvent(second, updatedSecond, core.Following)
	if err != nil {
		t.Fatalf("failed to update second child with Following strategy: %v", err)
	}

	events = c.GetEvents(startTime, startTime.AddDate(0, 0, 5), nil)
	assertEventStarts(t, events,
		startTime,
		startTime.AddDate(0, 0, 1),
		startTime.AddDate(0, 0, 2),
		startTime.AddDate(0, 0, 4),
	)
	assertNoEventAt(t, events, startTime.AddDate(0, 0, 3))
}

func TestRepeatingEvent_Update_Following_SplitsExceptionsAtOriginalTargetTime(t *testing.T) {
	c := newTestCore(t)

	startTime := time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC)
	createRepeatingEvent(t, c, "Daily Standup", startTime, time.Hour, core.Repetition{
		Frequency: core.Day,
		Interval:  1,
		Until:     startTime.AddDate(0, 0, 10),
	})

	events := c.GetEvents(startTime, startTime.AddDate(0, 0, 6), nil)
	second := cloneEvent(requireEventAt(t, events, startTime.AddDate(0, 0, 1)))
	third := requireEventAt(t, events, startTime.AddDate(0, 0, 2))

	if err := c.RemoveRepeatingEvent(third, core.Current); err != nil {
		t.Fatalf("failed to remove third child: %v", err)
	}

	shift := 72 * time.Hour
	updatedSecond := cloneEvent(second)
	updatedSecond.From = second.From.Add(shift)
	updatedSecond.To = second.To.Add(shift)
	updatedSecond.Repeat = &core.Repetition{
		Frequency: core.Day,
		Interval:  1,
		Until:     startTime.AddDate(0, 0, 10),
	}

	_, err := c.UpdateRepeatingEvent(second, updatedSecond, core.Following)
	if err != nil {
		t.Fatalf("failed to update second child with Following strategy: %v", err)
	}

	events = c.GetEvents(startTime, startTime.AddDate(0, 0, 11), nil)
	requireEventAt(t, events, startTime)
	requireEventAt(t, events, second.From.Add(shift))
	requireEventAt(t, events, startTime.AddDate(0, 0, 3).Add(shift))
	assertNoEventAt(t, events, third.From.Add(shift))
}

func TestRepeatingEvent_Update_Following_CountSeriesKeepsRemainingSlotsAfterFutureDeletedChild(t *testing.T) {
	c := newTestCore(t)

	const count = 5
	startTime := time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC)
	createRepeatingEvent(t, c, "Daily Standup", startTime, time.Hour, core.Repetition{
		Frequency: core.Day,
		Interval:  1,
		Count:     count,
	})

	events := c.GetEvents(startTime, startTime.AddDate(0, 0, count+1), nil)
	second := cloneEvent(requireEventAt(t, events, startTime.AddDate(0, 0, 1)))
	fourth := requireEventAt(t, events, startTime.AddDate(0, 0, 3))

	if err := c.RemoveRepeatingEvent(fourth, core.Current); err != nil {
		t.Fatalf("failed to remove fourth child: %v", err)
	}

	shift := time.Hour
	updatedSecond := cloneEvent(second)
	updatedSecond.From = second.From.Add(shift)
	updatedSecond.To = second.To.Add(shift)
	updatedSecond.Repeat = &core.Repetition{
		Frequency: core.Day,
		Interval:  1,
		Count:     count,
	}

	_, err := c.UpdateRepeatingEvent(second, updatedSecond, core.Following)
	if err != nil {
		t.Fatalf("failed to update second child with Following strategy: %v", err)
	}

	events = c.GetEvents(startTime, startTime.AddDate(0, 0, count+1), nil)
	assertEventStarts(t, events,
		startTime,
		startTime.AddDate(0, 0, 1).Add(shift),
		startTime.AddDate(0, 0, 2).Add(shift),
		startTime.AddDate(0, 0, 4).Add(shift),
	)
	assertNoEventAt(t, events, startTime.AddDate(0, 0, 3).Add(shift))
}

func TestRepeatingEvent_Update_All_ShiftsWholeSeriesFromChild(t *testing.T) {
	c := newTestCore(t)

	startTime := time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC)
	parent := createRepeatingEvent(t, c, "Monthly Review", startTime, time.Hour, core.Repetition{
		Frequency: core.Month,
		Interval:  1,
		Count:     5,
	})

	events := c.GetEvents(startTime, startTime.AddDate(0, 6, 0), nil)
	first := requireEventAt(t, events, startTime)

	shift := 2 * time.Hour
	updated := cloneEvent(first)
	updated.From = first.From.Add(shift)
	updated.To = first.To.Add(shift)
	updated.Title = "Monthly Review - Shifted"

	_, err := c.UpdateRepeatingEvent(first, updated, core.All)
	if err != nil {
		t.Fatalf("failed to update child event with All strategy: %v", err)
	}

	storedParent := requireEvent(t, c, parent.Id)
	if !storedParent.From.Equal(startTime.Add(shift)) {
		t.Fatalf("parent From mismatch: expected %s, got %s", startTime.Add(shift), storedParent.From)
	}
	if storedParent.Title != updated.Title {
		t.Fatalf("parent title mismatch: expected %q, got %q", updated.Title, storedParent.Title)
	}

	events = c.GetEvents(startTime.Add(shift), startTime.AddDate(0, 6, 0).Add(shift), nil)
	assertEventStarts(t, events,
		startTime.Add(shift),
		startTime.AddDate(0, 1, 0).Add(shift),
		startTime.AddDate(0, 2, 0).Add(shift),
		startTime.AddDate(0, 3, 0).Add(shift),
		startTime.AddDate(0, 4, 0).Add(shift),
	)
}

func TestRepeatingEvent_Update_All_ShiftsExceptionsAndKeepsDeletedChildHidden(t *testing.T) {
	c := newTestCore(t)

	startTime := time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC)
	createRepeatingEvent(t, c, "Repeating", startTime, time.Hour, core.Repetition{
		Frequency: core.Day,
		Interval:  1,
		Count:     3,
	})

	events := c.GetEvents(startTime, startTime.AddDate(0, 0, 3), nil)
	middle := requireEventAt(t, events, startTime.AddDate(0, 0, 1))

	if err := c.RemoveRepeatingEvent(middle, core.Current); err != nil {
		t.Fatalf("failed to remove middle child: %v", err)
	}

	events = c.GetEvents(startTime, startTime.AddDate(0, 0, 3), nil)
	first := requireEventAt(t, events, startTime)

	shift := -time.Hour
	updated := cloneEvent(first)
	updated.From = first.From.Add(shift)
	updated.To = first.To.Add(shift)
	updated.Title = "Repeating - Shifted"

	_, err := c.UpdateRepeatingEvent(first, updated, core.All)
	if err != nil {
		t.Fatalf("failed to update child event with All strategy: %v", err)
	}

	events = c.GetEvents(startTime.Add(shift), startTime.AddDate(0, 0, 3), nil)
	assertEventStarts(t, events,
		startTime.Add(shift),
		startTime.AddDate(0, 0, 2).Add(shift),
	)
	assertNoEventAt(t, events, startTime.AddDate(0, 0, 1).Add(shift))
}

func TestRepeatingEvent_Update_All_RepeatRuleChangeKeepsStoredExceptions(t *testing.T) {
	c := newTestCore(t)

	startTime := time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC)
	createRepeatingEvent(t, c, "Repeating", startTime, time.Hour, core.Repetition{
		Frequency: core.Day,
		Interval:  1,
		Count:     3,
	})

	events := c.GetEvents(startTime, startTime.AddDate(0, 0, 3), nil)
	middle := requireEventAt(t, events, startTime.AddDate(0, 0, 1))

	if err := c.RemoveRepeatingEvent(middle, core.Current); err != nil {
		t.Fatalf("failed to remove middle child: %v", err)
	}

	events = c.GetEvents(startTime, startTime.AddDate(0, 0, 3), nil)
	first := requireEventAt(t, events, startTime)

	shift := -time.Hour
	updated := cloneEvent(first)
	updated.From = first.From.Add(shift)
	updated.To = first.To.Add(shift)
	updated.Title = "Repeating - Shifted"
	updated.Repeat = &core.Repetition{
		Frequency:  core.Day,
		Interval:   1,
		Count:      4,
		Exceptions: []uuid.UUID{},
	}

	_, err := c.UpdateRepeatingEvent(first, updated, core.All)
	if err != nil {
		t.Fatalf("failed to update child event with All strategy: %v", err)
	}

	events = c.GetEvents(startTime.Add(shift), startTime.AddDate(0, 0, 5), nil)
	assertEventStarts(t, events,
		startTime.Add(shift),
		startTime.AddDate(0, 0, 2).Add(shift),
		startTime.AddDate(0, 0, 3).Add(shift),
	)
	assertNoEventAt(t, events, startTime.AddDate(0, 0, 1).Add(shift))
}

func TestEvent_Update_StandardToRepeating_GeneratesChildren(t *testing.T) {
	c := newTestCore(t)

	startTime := time.Date(2026, 5, 5, 15, 0, 0, 0, time.UTC)
	event := core.Event{
		Id:       uuid.New(),
		Calendar: TestCalendarName,
		Title:    "One-time meeting",
		From:     startTime,
		To:       startTime.Add(time.Hour),
	}
	createEvent(t, c, event)

	updated := event
	updated.Title = "Weekly meeting"
	updated.Repeat = &core.Repetition{
		Frequency: core.Week,
		Interval:  1,
		Count:     3,
	}

	_, err := c.UpdateEvent(updated)
	if err != nil {
		t.Fatalf("failed to update standard event to repeating: %v", err)
	}

	events := c.GetEvents(startTime, startTime.AddDate(0, 1, 0), nil)
	assertEventStarts(t, events,
		startTime,
		startTime.AddDate(0, 0, 7),
		startTime.AddDate(0, 0, 14),
	)

	stored := requireEvent(t, c, event.Id)
	if stored.Repeat == nil {
		t.Fatalf("updated parent should repeat")
	}
	if stored.Repeat.Count != 3 {
		t.Fatalf("updated parent Count mismatch: expected 3, got %d", stored.Repeat.Count)
	}
}

func newTestCore(t *testing.T) *core.Core {
	t.Helper()

	c := core.NewCore()
	if err := c.CreateCalendar(TestCalendarName, ""); err != nil {
		t.Fatalf("failed to init repo: %v", err)
	}

	t.Cleanup(func() {
		_ = c.RemoveCalendar(TestCalendarName)
	})

	return c
}

func createRepeatingEvent(t *testing.T, c *core.Core, title string, from time.Time, duration time.Duration, repeat core.Repetition) core.Event {
	t.Helper()

	event := core.Event{
		Id:       uuid.New(),
		Calendar: TestCalendarName,
		Title:    title,
		From:     from,
		To:       from.Add(duration),
		Repeat:   &repeat,
	}
	createEvent(t, c, event)

	return event
}

func createEvent(t *testing.T, c *core.Core, event core.Event) core.Event {
	t.Helper()

	created, err := c.CreateEvent(event)
	if err != nil {
		t.Fatalf("failed to create event: %v", err)
	}

	return *created
}

func requireEvent(t *testing.T, c *core.Core, id uuid.UUID) core.Event {
	t.Helper()

	event, err := c.GetEvent(id)
	if err != nil {
		t.Fatalf("failed to get event %s: %v", id, err)
	}

	return *event
}

func requireEventAt(t *testing.T, events []core.Event, from time.Time) core.Event {
	t.Helper()

	event, ok := findEventByFrom(events, from)
	if !ok {
		t.Fatalf("expected event at %s, got starts %v", from, eventStarts(events))
	}

	return event
}

func assertEventStarts(t *testing.T, events []core.Event, wants ...time.Time) {
	t.Helper()

	if len(events) != len(wants) {
		t.Fatalf("expected %d events, got %d: %v", len(wants), len(events), eventStarts(events))
	}

	for _, want := range wants {
		if _, ok := findEventByFrom(events, want); !ok {
			t.Errorf("missing event at %s; got starts %v", want, eventStarts(events))
		}
	}
}

func assertNoEventAt(t *testing.T, events []core.Event, from time.Time) {
	t.Helper()

	if event, ok := findEventByFrom(events, from); ok {
		t.Fatalf("expected no event at %s, got %+v", from, event)
	}
}

func assertParentHasException(t *testing.T, parent core.Event, exception uuid.UUID) {
	t.Helper()

	if parent.Repeat == nil {
		t.Fatalf("parent %s should be repeating", parent.Id)
	}
	if !containsUUID(parent.Repeat.Exceptions, exception) {
		t.Fatalf("parent %s does not contain exception %s; exceptions: %v", parent.Id, exception, parent.Repeat.Exceptions)
	}
}

func cloneEvent(event core.Event) core.Event {
	cloned := event

	if event.Repeat != nil {
		repeat := *event.Repeat
		repeat.Exceptions = append([]uuid.UUID(nil), event.Repeat.Exceptions...)
		cloned.Repeat = &repeat
	}

	return cloned
}

func findEventByFrom(events []core.Event, from time.Time) (core.Event, bool) {
	for _, event := range events {
		if event.From.Equal(from) {
			return event, true
		}
	}

	return core.Event{}, false
}

func containsUUID(ids []uuid.UUID, id uuid.UUID) bool {
	for _, cur := range ids {
		if cur == id {
			return true
		}
	}

	return false
}

func eventStarts(events []core.Event) []time.Time {
	starts := make([]time.Time, 0, len(events))
	for _, event := range events {
		starts = append(starts, event.From)
	}

	return starts
}
