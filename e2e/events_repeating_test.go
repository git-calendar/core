// It is kinda e2e, but not entirely. TODO rethink this.
package e2e

import (
	"fmt"
	"testing"
	"time"

	"github.com/git-calendar/core/pkg/core"
	"github.com/google/uuid"
	rrule "github.com/teambition/rrule-go"
)

func TestRepeatingEvent_GetEvents_UntilWeekly_GeneratesOccurrencesInRange(t *testing.T) {
	c := newTestCore(t)

	startTime := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	parent := createRepeatingEvent(t, c, "Repeating Event", startTime, time.Hour*4, recurrenceUntilEndOfDay(t, startTime, "WEEKLY", startTime.AddDate(1, 0, 0)))

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
	createRepeatingEvent(t, c, "Repeating Event", startTime, time.Hour*4, recurrenceWithCount(t, startTime, "WEEKLY", count))

	events := c.GetEvents(startTime.Add(-time.Hour), startTime.AddDate(0, 0, count*7+1), nil)
	for _, event := range events {
		if event.Repeat != nil {
			t.Fatalf("generated child %s should not own its parent's recurrence", event.Id)
		}
	}
	assertEventStarts(
		t, events,
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
	parent := createRepeatingEvent(t, c, "Repeating Event", startTime, time.Hour, recurrenceWithCount(t, startTime, "DAILY", count))

	events := c.GetEvents(startTime, startTime.AddDate(0, 0, count), nil)
	assertEventStarts(
		t, events,
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
	assertParentHasException(t, storedParent, removed.From)

	events = c.GetEvents(startTime, startTime.AddDate(0, 0, count), nil)
	assertNoEventAt(t, events, removed.From)
	if len(events) != count-1 {
		t.Fatalf("expected %d events after delete, got %d: %+v", count-1, len(events), events)
	}
}

func TestRepeatingEvent_Remove_Current_RejectsParentEvent(t *testing.T) {
	c := newTestCore(t)

	startTime := time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC)
	parent := createRepeatingEvent(t, c, "Repeating Event", startTime, time.Hour, recurrenceWithCount(t, startTime, "DAILY", 3))

	err := c.RemoveRepeatingEvent(parent, core.Current)
	if err == nil {
		t.Fatalf("expected parent delete through RemoveRepeatingEvent to fail")
	}
}

func TestRepeatingEvent_Remove_Current_RemovesOnlyTargetChild(t *testing.T) {
	c := newTestCore(t)

	const count = 5
	startTime := time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC)
	parent := createRepeatingEvent(t, c, "Daily event", startTime, time.Hour, recurrenceWithCount(t, startTime, "DAILY", count))

	events := c.GetEvents(startTime, startTime.AddDate(0, 0, count), nil)
	target := requireEventAt(t, events, startTime.AddDate(0, 0, 2))

	err := c.RemoveRepeatingEvent(target, core.Current)
	if err != nil {
		t.Fatalf("failed to remove current child: %v", err)
	}

	storedParent := requireEvent(t, c, parent.Id)
	assertParentHasException(t, storedParent, target.From)

	events = c.GetEvents(startTime, startTime.AddDate(0, 0, count), nil)
	assertEventStarts(
		t, events,
		startTime,
		startTime.AddDate(0, 0, 1),
		startTime.AddDate(0, 0, 3),
		startTime.AddDate(0, 0, 4),
	)
	assertNoEventAt(t, events, target.From)
}

func TestRepeatingEvent_Remove_Following_RemovesTargetAndFollowingChildren(t *testing.T) {
	c := newTestCore(t)

	const count = 5
	startTime := time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC)
	_ = createRepeatingEvent(t, c, "Daily event", startTime, time.Hour, recurrenceWithCount(t, startTime, "DAILY", count))

	events := c.GetEvents(startTime, startTime.AddDate(0, 0, count), nil)
	target := requireEventAt(t, events, startTime.AddDate(0, 0, 2))

	err := c.RemoveRepeatingEvent(target, core.Following)
	if err != nil {
		t.Fatalf("failed to remove following children: %v", err)
	}

	events = c.GetEvents(startTime, startTime.AddDate(0, 0, count), nil)
	assertEventStarts(
		t, events,
		startTime,
		startTime.AddDate(0, 0, 1),
	)

	assertNoEventAt(t, events, startTime.AddDate(0, 0, 2))
	assertNoEventAt(t, events, startTime.AddDate(0, 0, 3))
	assertNoEventAt(t, events, startTime.AddDate(0, 0, 4))
}

func TestRepeatingEvent_Remove_All_RemovesWholeSeries(t *testing.T) {
	c := newTestCore(t)

	const count = 5
	startTime := time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC)
	_ = createRepeatingEvent(t, c, "Daily event", startTime, time.Hour, recurrenceWithCount(t, startTime, "DAILY", count))

	events := c.GetEvents(startTime, startTime.AddDate(0, 0, count), nil)
	target := requireEventAt(t, events, startTime.AddDate(0, 0, 2))

	err := c.RemoveRepeatingEvent(target, core.All)
	if err != nil {
		t.Fatalf("failed to remove whole repeating series: %v", err)
	}

	events = c.GetEvents(startTime, startTime.AddDate(0, 0, count), nil)
	if len(events) != 0 {
		t.Fatalf("expected whole repeating series to be removed, got %d events", len(events))
	}
}

func TestRepeatingEvent_Update_Current_DetachesOnlyTargetChild(t *testing.T) {
	c := newTestCore(t)

	const count = 5
	startTime := time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC)
	parent := createRepeatingEvent(t, c, "Daily event", startTime, time.Hour, recurrenceWithCount(t, startTime, "DAILY", count))

	events := c.GetEvents(startTime, startTime.AddDate(0, 0, count), nil)
	target := requireEventAt(t, events, startTime.AddDate(0, 0, 2))

	updated := cloneEvent(t, target)
	updated.Title = "Daily event - update"
	updated.From = target.From.Add(time.Hour)
	updated.To = target.To.Add(time.Hour)

	_, err := c.UpdateRepeatingEvent(target, updated, core.Current)
	if err != nil {
		t.Fatalf("failed to update child event with Current strategy: %v", err)
	}

	storedParent := requireEvent(t, c, parent.Id)
	assertParentHasException(t, storedParent, target.From)

	events = c.GetEvents(startTime, startTime.AddDate(0, 0, count), nil)
	assertEventStarts(
		t, events,
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
	parent := createRepeatingEvent(t, c, "Repeating Event", startTime, time.Hour, recurrenceWithCount(t, startTime, "DAILY", 3))

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
			updated := cloneEvent(t, parent)
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
		Repeat:   recurrenceUntilEndOfDay(t, startTime, "DAILY", startTime.AddDate(0, 1, 0)),
	}
	createEvent(t, c, parent)

	events := c.GetEvents(startTime, startTime.AddDate(0, 0, 21), nil)
	target := requireEventAt(t, events, startTime.AddDate(0, 0, 2))
	previous := requireEventAt(t, events, startTime.AddDate(0, 0, 1))

	updated := cloneEvent(t, target)
	updated.Title = "Daily Meeting - New Phase"
	updated.Repeat = recurrenceUntilEndOfDay(t, updated.From, "DAILY", startTime.AddDate(0, 1, 0))

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
	wantRRule := recurrenceUntil(t, oldParent.From, "DAILY", previous.From).GetRRule().OrigOptions.RRuleString()
	gotRRule := oldParent.Repeat.GetRRule().OrigOptions.RRuleString()
	if gotRRule != wantRRule {
		t.Fatalf("old parent RRULE mismatch: expected %q, got %q", wantRRule, gotRRule)
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
		Repeat:   recurrenceUntilEndOfDay(t, startTime, "DAILY", startTime.AddDate(0, 1, 0)),
	}
	createEvent(t, c, parent)

	events := c.GetEvents(startTime, startTime.AddDate(0, 0, 21), nil)
	target := requireEventAt(t, events, startTime.AddDate(0, 0, 1))

	updated := cloneEvent(t, target)
	updated.Title = "Daily Meeting - New Phase"
	updated.Repeat = recurrenceUntilEndOfDay(t, updated.From, "DAILY", startTime.AddDate(0, 1, 0))

	newParent, err := c.UpdateRepeatingEvent(target, updated, core.Following)
	if err != nil {
		t.Fatalf("failed to update second child with Following strategy: %v", err)
	}
	if newParent.ParentId != nil {
		t.Fatalf("new event should be a parent, got ParentId %s", newParent.ParentId)
	}

	oldParent := requireEvent(t, c, parentId)
	if oldParent.Repeat != nil {
		t.Fatalf("old parent should not repeat when only its first occurrence remains; got RRULE %q", oldParent.Repeat.GetRRule().OrigOptions.RRuleString())
	}
}

func TestRepeatingEvent_Update_Following_CarriesAndShiftsFutureExceptions(t *testing.T) {
	c := newTestCore(t)

	startTime := time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC)
	createRepeatingEvent(t, c, "Daily Standup", startTime, time.Hour, recurrenceUntilEndOfDay(t, startTime, "DAILY", startTime.AddDate(0, 0, 10)))

	events := c.GetEvents(startTime, startTime.AddDate(0, 0, 6), nil)
	second := cloneEvent(t, requireEventAt(t, events, startTime.AddDate(0, 0, 1)))
	fourth := requireEventAt(t, events, startTime.AddDate(0, 0, 3))

	if err := c.RemoveRepeatingEvent(fourth, core.Current); err != nil {
		t.Fatalf("failed to remove fourth child: %v", err)
	}

	shift := time.Hour
	updatedSecond := cloneEvent(t, second)
	updatedSecond.From = second.From.Add(shift)
	updatedSecond.To = second.To.Add(shift)
	updatedSecond.Repeat = recurrenceUntilEndOfDay(t, updatedSecond.From, "DAILY", startTime.AddDate(0, 0, 10))

	_, err := c.UpdateRepeatingEvent(second, updatedSecond, core.Following)
	if err != nil {
		t.Fatalf("failed to update second child with Following strategy: %v", err)
	}

	events = c.GetEvents(startTime, startTime.AddDate(0, 0, 5), nil)
	assertEventStarts(
		t, events,
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
	createRepeatingEvent(t, c, "Daily Standup", startTime, time.Hour, recurrenceWithCount(t, startTime, "DAILY", 3))

	events := c.GetEvents(startTime.Add(-time.Hour), startTime.AddDate(0, 0, 3), nil)
	first := requireEventAt(t, events, startTime)

	shift := -2 * time.Hour
	updatedFirst := cloneEvent(t, first)
	updatedFirst.From = first.From.Add(shift)
	updatedFirst.To = first.To.Add(shift)

	_, err := c.UpdateRepeatingEvent(first, updatedFirst, core.Following)
	if err != nil {
		t.Fatalf("failed to update first child with Following strategy: %v", err)
	}

	events = c.GetEvents(startTime.Add(shift), startTime.AddDate(0, 0, 3), nil)
	assertEventStarts(
		t, events,
		startTime.Add(shift),
		startTime.AddDate(0, 0, 1).Add(shift),
		startTime.AddDate(0, 0, 2).Add(shift),
	)
	assertNoEventAt(t, events, startTime)
}

func TestRepeatingEvent_Update_Following_TitleOnlyKeepsFutureDeletedChildHidden(t *testing.T) {
	c := newTestCore(t)

	startTime := time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC)
	createRepeatingEvent(t, c, "Daily Standup", startTime, time.Hour, recurrenceUntilEndOfDay(t, startTime, "DAILY", startTime.AddDate(0, 0, 10)))

	events := c.GetEvents(startTime, startTime.AddDate(0, 0, 6), nil)
	second := cloneEvent(t, requireEventAt(t, events, startTime.AddDate(0, 0, 1)))
	fourth := requireEventAt(t, events, startTime.AddDate(0, 0, 3))

	if err := c.RemoveRepeatingEvent(fourth, core.Current); err != nil {
		t.Fatalf("failed to remove fourth child: %v", err)
	}

	updatedSecond := cloneEvent(t, second)
	updatedSecond.Title = "Daily Standup - New Phase"
	updatedSecond.Repeat = nil // children do not own the recurrence

	_, err := c.UpdateRepeatingEvent(second, updatedSecond, core.Following)
	if err != nil {
		t.Fatalf("failed to update second child with Following strategy: %v", err)
	}

	events = c.GetEvents(startTime, startTime.AddDate(0, 0, 5), nil)
	assertEventStarts(
		t, events,
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
	createRepeatingEvent(t, c, "Daily Standup", startTime, time.Hour, recurrenceUntilEndOfDay(t, startTime, "DAILY", startTime.AddDate(0, 0, 10)))

	events := c.GetEvents(startTime, startTime.AddDate(0, 0, 6), nil)
	second := cloneEvent(t, requireEventAt(t, events, startTime.AddDate(0, 0, 1)))
	third := requireEventAt(t, events, startTime.AddDate(0, 0, 2))

	if err := c.RemoveRepeatingEvent(third, core.Current); err != nil {
		t.Fatalf("failed to remove third child: %v", err)
	}

	shift := 72 * time.Hour
	updatedSecond := cloneEvent(t, second)
	updatedSecond.From = second.From.Add(shift)
	updatedSecond.To = second.To.Add(shift)
	updatedSecond.Repeat = recurrenceUntilEndOfDay(t, updatedSecond.From, "DAILY", startTime.AddDate(0, 0, 10))

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
	createRepeatingEvent(t, c, "Daily Standup", startTime, time.Hour, recurrenceWithCount(t, startTime, "DAILY", count))

	events := c.GetEvents(startTime, startTime.AddDate(0, 0, count+1), nil)
	second := cloneEvent(t, requireEventAt(t, events, startTime.AddDate(0, 0, 1)))
	fourth := requireEventAt(t, events, startTime.AddDate(0, 0, 3))

	if err := c.RemoveRepeatingEvent(fourth, core.Current); err != nil {
		t.Fatalf("failed to remove fourth child: %v", err)
	}

	shift := time.Hour
	updatedSecond := cloneEvent(t, second)
	updatedSecond.From = second.From.Add(shift)
	updatedSecond.To = second.To.Add(shift)
	updatedSecond.Repeat = recurrenceWithCount(t, updatedSecond.From, "DAILY", count)

	_, err := c.UpdateRepeatingEvent(second, updatedSecond, core.Following)
	if err != nil {
		t.Fatalf("failed to update second child with Following strategy: %v", err)
	}

	events = c.GetEvents(startTime, startTime.AddDate(0, 0, count+1), nil)
	assertEventStarts(
		t, events,
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
	parent := createRepeatingEvent(t, c, "Monthly Review", startTime, time.Hour, recurrenceWithCount(t, startTime, "MONTHLY", 5))

	events := c.GetEvents(startTime, startTime.AddDate(0, 6, 0), nil)
	first := requireEventAt(t, events, startTime)

	shift := 2 * time.Hour
	updated := cloneEvent(t, first)
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
	assertEventStarts(
		t, events,
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
	createRepeatingEvent(t, c, "Repeating", startTime, time.Hour, recurrenceWithCount(t, startTime, "DAILY", 3))

	events := c.GetEvents(startTime, startTime.AddDate(0, 0, 3), nil)
	middle := requireEventAt(t, events, startTime.AddDate(0, 0, 1))

	if err := c.RemoveRepeatingEvent(middle, core.Current); err != nil {
		t.Fatalf("failed to remove middle child: %v", err)
	}

	events = c.GetEvents(startTime, startTime.AddDate(0, 0, 3), nil)
	first := requireEventAt(t, events, startTime)

	shift := -time.Hour
	updated := cloneEvent(t, first)
	updated.From = first.From.Add(shift)
	updated.To = first.To.Add(shift)
	updated.Title = "Repeating - Shifted"

	_, err := c.UpdateRepeatingEvent(first, updated, core.All)
	if err != nil {
		t.Fatalf("failed to update child event with All strategy: %v", err)
	}

	events = c.GetEvents(startTime.Add(shift), startTime.AddDate(0, 0, 3), nil)
	assertEventStarts(
		t, events,
		startTime.Add(shift),
		startTime.AddDate(0, 0, 2).Add(shift),
	)
	assertNoEventAt(t, events, startTime.AddDate(0, 0, 1).Add(shift))
}

func TestRepeatingEvent_Update_All_RepeatRuleChangeKeepsStoredExceptions(t *testing.T) {
	c := newTestCore(t)

	startTime := time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC)
	createRepeatingEvent(t, c, "Repeating", startTime, time.Hour, recurrenceWithCount(t, startTime, "DAILY", 3))

	events := c.GetEvents(startTime, startTime.AddDate(0, 0, 3), nil)
	middle := requireEventAt(t, events, startTime.AddDate(0, 0, 1))

	if err := c.RemoveRepeatingEvent(middle, core.Current); err != nil {
		t.Fatalf("failed to remove middle child: %v", err)
	}

	events = c.GetEvents(startTime, startTime.AddDate(0, 0, 3), nil)
	first := requireEventAt(t, events, startTime)

	shift := -time.Hour
	updated := cloneEvent(t, first)
	updated.From = first.From.Add(shift)
	updated.To = first.To.Add(shift)
	updated.Title = "Repeating - Shifted"
	updated.Repeat = recurrenceWithCount(t, updated.From, "DAILY", 4)
	updated.Repeat.SetExDates(nil)

	_, err := c.UpdateRepeatingEvent(first, updated, core.All)
	if err != nil {
		t.Fatalf("failed to update child event with All strategy: %v", err)
	}

	events = c.GetEvents(startTime.Add(shift), startTime.AddDate(0, 0, 5), nil)
	assertEventStarts(
		t, events,
		startTime.Add(shift),
		startTime.AddDate(0, 0, 2).Add(shift),
		startTime.AddDate(0, 0, 3).Add(shift),
	)
	assertNoEventAt(t, events, startTime.AddDate(0, 0, 1).Add(shift))
}

func TestRepeatingEvent_RemoveFollowingThenUpdateAllShiftTime_DoesNotLoseEvents(t *testing.T) {
	c := newTestCore(t)

	const count = 5
	startTime := time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC)
	duration := time.Hour

	createRepeatingEvent(t, c, "Daily event", startTime, duration, recurrenceWithCount(t, startTime, "DAILY", count))

	events := c.GetEvents(startTime, startTime.AddDate(0, 0, count), nil)
	third := requireEventAt(t, events, startTime.AddDate(0, 0, 2))

	if err := c.RemoveRepeatingEvent(third, core.Following); err != nil {
		t.Fatalf("failed to remove following from 3rd: %v", err)
	}

	events = c.GetEvents(startTime, startTime.AddDate(0, 0, count), nil)
	assertEventStarts(t, events, startTime, startTime.AddDate(0, 0, 1))

	second := requireEventAt(t, events, startTime.AddDate(0, 0, 1))

	newFrom := second.From.Add(-time.Hour)
	newTo := second.To.Add(-time.Hour)

	// Model this the way a real caller would build a child update:
	// only the mutable fields, no Repeat field carried over — children
	// don't own the repeat rule, the parent does.
	newEvent := second
	newEvent.From = newFrom
	newEvent.To = newTo
	newEvent.Repeat = nil

	updated, err := c.UpdateRepeatingEvent(second, newEvent, core.All)
	if err != nil {
		t.Fatalf("failed to update with All strategy: %v", err)
	}
	if updated == nil {
		t.Fatalf("expected updated event, got nil")
	}

	// Both remaining occurrences must survive the shift, just moved -1h.
	events = c.GetEvents(startTime.Add(-2*time.Hour), startTime.AddDate(0, 0, count), nil)
	if len(events) != 2 {
		t.Fatalf("expected 2 events after All-strategy time shift, got %d: %+v", len(events), events)
	}

	expectedFirst := startTime.Add(-time.Hour)
	expectedSecond := startTime.AddDate(0, 0, 1).Add(-time.Hour)
	assertEventStarts(t, events, expectedFirst, expectedSecond)
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
	updated.Repeat = recurrenceWithCount(t, updated.From, "WEEKLY", 3)

	_, err := c.UpdateEvent(updated)
	if err != nil {
		t.Fatalf("failed to update standard event to repeating: %v", err)
	}

	events := c.GetEvents(startTime, startTime.AddDate(0, 1, 0), nil)
	assertEventStarts(
		t, events,
		startTime,
		startTime.AddDate(0, 0, 7),
		startTime.AddDate(0, 0, 14),
	)

	stored := requireEvent(t, c, event.Id)
	if stored.Repeat == nil {
		t.Fatalf("updated parent should repeat")
	}
	wantRRule := recurrenceWithCount(t, stored.From, "WEEKLY", 3).GetRRule().OrigOptions.RRuleString()
	gotRRule := stored.Repeat.GetRRule().OrigOptions.RRuleString()
	if gotRRule != wantRRule {
		t.Fatalf("updated parent RRULE mismatch: expected %q, got %q", wantRRule, gotRRule)
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

func createRepeatingEvent(t *testing.T, c *core.Core, title string, from time.Time, duration time.Duration, repeat *rrule.Set) core.Event {
	t.Helper()

	event := core.Event{
		Id:       uuid.New(),
		Calendar: TestCalendarName,
		Title:    title,
		From:     from,
		To:       from.Add(duration),
		Repeat:   repeat,
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

func assertParentHasException(t *testing.T, parent core.Event, exception time.Time) {
	t.Helper()

	if parent.Repeat == nil {
		t.Fatalf("parent %s should be repeating", parent.Id)
	}
	exceptions := parent.Repeat.GetExDate()
	if !containsTime(exceptions, exception) {
		t.Fatalf("parent %s does not contain exception %s; exceptions: %v", parent.Id, exception, exceptions)
	}
}

func cloneEvent(t *testing.T, event core.Event) core.Event {
	t.Helper()
	cloned := event

	if event.Repeat != nil {
		repeat, err := rrule.StrToRRuleSet(event.Repeat.String())
		if err != nil {
			t.Fatalf("failed to clone recurrence: %v", err)
		}
		cloned.Repeat = repeat
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

func containsTime(times []time.Time, want time.Time) bool {
	for _, current := range times {
		if current.Equal(want) {
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

func recurrenceWithCount(t *testing.T, start time.Time, frequency string, count int) *rrule.Set {
	t.Helper()
	return recurrence(t, start, fmt.Sprintf("FREQ=%s;INTERVAL=1;COUNT=%d", frequency, count))
}

func recurrenceUntil(t *testing.T, start time.Time, frequency string, until time.Time) *rrule.Set {
	t.Helper()
	return recurrence(t, start, fmt.Sprintf("FREQ=%s;INTERVAL=1;UNTIL=%s", frequency, until.UTC().Format("20060102T150405Z")))
}

func recurrenceUntilEndOfDay(t *testing.T, start time.Time, frequency string, day time.Time) *rrule.Set {
	t.Helper()
	until := time.Date(
		day.Year(), day.Month(), day.Day(),
		23, 59, 59, 0,
		day.Location(),
	)
	return recurrenceUntil(t, start, frequency, until)
}

func recurrence(t *testing.T, start time.Time, value string) *rrule.Set {
	t.Helper()
	option, err := rrule.StrToROptionInLocation(value, start.Location())
	if err != nil {
		t.Fatalf("failed to parse RRULE %q: %v", value, err)
	}
	option.Dtstart = start

	rule, err := rrule.NewRRule(*option)
	if err != nil {
		t.Fatalf("failed to create RRULE %q: %v", value, err)
	}
	set := &rrule.Set{}
	set.RRule(rule)
	return set
}
