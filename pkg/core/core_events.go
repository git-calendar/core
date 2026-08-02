package core

import (
	"errors"
	"fmt"
	"path"
	"slices"
	"time"

	"github.com/google/uuid"
	rrule "github.com/teambition/rrule-go"
)

// CreateEvent validates, stores, and commits a new event.
func (c *Core) CreateEvent(event Event) (*Event, error) {
	if _, ok := c.events[event.ID]; ok && event.ID != uuid.Nil {
		return nil, errors.New("an event with this id already exists")
	}

	cal, ok := c.calendars[event.Calendar]
	if !ok {
		return nil, errors.New("the specified calendar is missing")
	}
	if cal.Readonly {
		return nil, errors.New("the specified calendar is read-only")
	}

	if err := event.Validate(); err != nil {
		return nil, fmt.Errorf("invalid event: %w", err)
	}

	c.events[event.ID] = &event

	if err := c.intervalTree.InsertEvent(event); err != nil {
		return nil, fmt.Errorf("failed to insert into index tree: %w", err)
	}

	if err := c.saveAndCommitEvent(&event, fmt.Sprintf("Added event %q", event.ID)); err != nil {
		return nil, fmt.Errorf("failed to save event to repo: %w", err)
	}

	return &event, nil
}

// UpdateEvent updates a standalone event by ID.
// Use UpdateRepeatingEvent for generated child/occurances.
func (c *Core) UpdateEvent(event Event) (*Event, error) {
	if err := event.Validate(); err != nil {
		return nil, fmt.Errorf("invalid event: %w", err)
	}

	originalEvent, exists := c.events[event.ID]
	if !exists {
		return nil, fmt.Errorf("no event found with id %q", event.ID)
	}

	cal, ok := c.calendars[event.Calendar]
	if !ok {
		return nil, errors.New("the specified calendar is missing")
	}
	if cal.Readonly {
		return nil, errors.New("the specified calendar is read-only")
	}

	oldEnd := originalEvent.getTreeEndTime()
	newEnd := event.getTreeEndTime()

	if originalEvent.From != event.From || oldEnd != newEnd { // update the interval tree
		ids, found := c.intervalTree.tree.Find(originalEvent.From, oldEnd)
		if found {
			index := slices.Index(ids, originalEvent.ID)
			if index != -1 {
				updated := slices.Delete(ids, index, index+1)
				if len(updated) == 0 {
					_ = c.intervalTree.tree.Delete(originalEvent.From, oldEnd)
				} else {
					_ = c.intervalTree.tree.Insert(originalEvent.From, oldEnd, updated)
				}
			}
		}
		err := c.intervalTree.InsertEvent(event)
		if err != nil {
			return nil, fmt.Errorf("failed to reinsert event into tree: %w", err)
		}
	}

	newRepoCommitMsg := fmt.Sprintf("Updated event %q", event.ID)
	if originalEvent.Calendar != event.Calendar {
		newRepoCommitMsg = fmt.Sprintf("Moved event from another calendar %q", event.ID)
		// remove the old file from the previous calendar repository
		if err := c.deleteAndCommitEvent(originalEvent.ID, fmt.Sprintf("Moved event to another calendar %q", originalEvent.ID)); err != nil {
			return nil, err
		}
	}

	c.events[event.ID] = &event
	if err := c.saveAndCommitEvent(&event, newRepoCommitMsg); err != nil {
		return nil, err
	}

	return &event, nil
}

// UpdateRepeatingEvent updates occurrences in a recurring series.
func (c *Core) UpdateRepeatingEvent(old, new Event, strat UpdateStrategy) (*Event, error) {
	if err := old.Validate(); err != nil {
		return nil, fmt.Errorf("invalid old event: %w", err)
	}
	if err := new.Validate(); err != nil {
		return nil, fmt.Errorf("invalid new event: %w", err)
	}
	if !strat.IsValid() {
		return nil, errors.New("incorrect strategy provided")
	}
	if old.ID != new.ID {
		return nil, fmt.Errorf("invalid update event: id %q does not match parent id %q", old.ID, new.ID)
	}
	if !old.IsChild() || !new.IsChild() {
		return nil, errors.New("repeating update requires child events")
	}
	if *old.ParentID != *new.ParentID {
		return nil, errors.New("old and new child events have different parent ids")
	}

	cal, ok := c.calendars[new.Calendar]
	if !ok {
		return nil, errors.New("the specified calendar is missing")
	}
	if cal.Readonly {
		return nil, errors.New("the specified calendar is read-only")
	}

	switch strat {
	case Current:
		return c.updateCurrentChild(&old, &new)
	case Following:
		return c.updateFollowingChildren(&old, &new)
	case All:
		return c.updateAllChildren(&old, &new)
	default:
		return nil, fmt.Errorf("update strategy %d isn't implemented", strat)
	}
}

// RemoveEvent removes a standalone event or recurring-series parent.
// Use RemoveRepeatingEvent for generated child/occurances.
func (c *Core) RemoveEvent(event Event) error {
	if err := event.Validate(); err != nil {
		return fmt.Errorf("invalid event: %w", err)
	}

	if cal, ok := c.calendars[event.Calendar]; ok && cal.Readonly {
		return errors.New("the events calendar is read-only")
	}

	// delete file from disk and commit
	err := c.deleteAndCommitEvent(event.ID, fmt.Sprintf("Deleted event %q", event.ID))
	if err != nil {
		return fmt.Errorf("failed to delete event from git: %w", err)
	}

	err = c.intervalTree.RemoveEvent(event)
	if err != nil {
		return fmt.Errorf("failed to delete event from interval tree: %w", err)
	}

	delete(c.events, event.ID)
	return nil
}

// RemoveRepeatingEvent removes occurrences from a recurring series.
func (c *Core) RemoveRepeatingEvent(event Event, strat UpdateStrategy) error {
	if err := event.Validate(); err != nil {
		return fmt.Errorf("invalid event: %w", err)
	}

	if cal, ok := c.calendars[event.Calendar]; ok && cal.Readonly {
		return errors.New("the events calendar is read-only")
	}

	if !event.IsChild() {
		return errors.New("event has to be a child to be removed using this method")
	}

	switch strat {
	case Current:
		return c.removeCurrentChild(&event)
	case Following:
		return c.removeFollowingChildren(&event)
	case All:
		return c.removeAllChildren(&event)
	default:
		return fmt.Errorf("update strategy %d isn't implemented", strat)
	}
}

// GetEvent returns the event with the given ID, or an error if it doesn't exist.
func (c *Core) GetEvent(id uuid.UUID) (*Event, error) {
	e, ok := c.events[id]
	if !ok {
		return nil, fmt.Errorf("event with id: %q doesn't exist", id)
	}
	return e, nil
}

// GetEvents returns events intersecting the inclusive interval [from, to] and matching the optional filter.
func (c *Core) GetEvents(from, to time.Time, filter GetEventsFilter) []Event {
	// query the interval tree for intersecting event IDs
	intervalsMatched, found := c.intervalTree.tree.AllIntersections(from, to)
	if !found {
		return []Event{}
	}

	result := make([]Event, 0, len(intervalsMatched))

	for _, intersection := range intervalsMatched {
		for _, eID := range intersection {
			curEvent, ok := c.events[eID]
			if !ok {
				fmt.Printf("WARN: event %q is missing from the events map\n", eID)
				continue
			}

			// skip filtered out events
			if filter != nil && !checkFilter(curEvent, filter) {
				continue
			}

			// non-repeating events can be appended right away
			if curEvent.Repeat == nil {
				result = append(result, *c.events[eID])
				continue
			}

			repeat, err := rrule.StrToRRuleSet(curEvent.Repeat.String())
			if err != nil {
				continue
			}
			starts := recurrenceBetween(curEvent.Repeat, from, to)
			eventDuration := curEvent.To.Sub(curEvent.From)
			for _, start := range starts {
				result = append(result, Event{
					ID:          generateCustomUUID(curEvent.ID, start),
					Title:       curEvent.Title,
					Location:    curEvent.Location,
					Description: curEvent.Description,
					From:        start,
					To:          start.Add(eventDuration),
					Calendar:    curEvent.Calendar,
					TagID:       curEvent.TagID,
					ParentID:    &curEvent.ID,
					Repeat:      repeat,
				})
			}
		}
	}

	return result
}

// updateCurrentChild updates one generated child by excluding it and creating a detached event.
func (c *Core) updateCurrentChild(original, updated *Event) (*Event, error) {
	parent, ok := c.events[*updated.ParentID] // we check nil pointer in UpdateRepeatingEvent
	if !ok || parent == nil || !parent.IsParent() {
		return nil, errors.New("no valid parent found")
	}
	if parent.Repeat == nil {
		return nil, errors.New("parent is not a repeating event")
	}

	parent.Repeat.ExDate(original.From)
	if err := c.saveAndCommitEvent(parent, fmt.Sprintf("Added exception to parent %q", parent.ID)); err != nil {
		return nil, fmt.Errorf("failed to save parent event: %w", err)
	}

	// detach the updated event from series and creete a new standalone instance
	detachedEvent := *updated    // shallow copy
	detachedEvent.Repeat = nil   // not repeating anymore
	detachedEvent.ParentID = nil // not child anymore
	detachedEvent.ID = uuid.Nil  // set to uuid.Nil; CreateEvent will asign a new one

	return c.CreateEvent(detachedEvent) // save as new
}

// updateFollowingChildren splits a series at the selected child.
func (c *Core) updateFollowingChildren(old, new *Event) (*Event, error) {
	parent, ok := c.events[*old.ParentID]
	if !ok || parent == nil || !parent.IsParent() {
		return nil, errors.New("no valid parent found")
	}

	originalRepeat := parent.Repeat
	before, after, index, err := splitRecurrence(originalRepeat, old.From, new.From, new.Repeat)
	if err != nil {
		return nil, fmt.Errorf("failed to split recurrence: %w", err)
	}
	if index == 0 {
		return c.updateAllChildren(old, new)
	}

	if err := c.intervalTree.RemoveEvent(*parent); err != nil {
		return nil, fmt.Errorf("failed to remove parent from interval tree: %w", err)
	}
	parent.Repeat = before
	if err := c.intervalTree.InsertEvent(*parent); err != nil {
		parent.Repeat = originalRepeat
		if rollbackErr := c.intervalTree.InsertEvent(*parent); rollbackErr != nil {
			return nil, fmt.Errorf("failed to reinsert capped parent: %w; rollback failed too: %v", err, rollbackErr)
		}
		return nil, fmt.Errorf("failed to reinsert capped parent: %w", err)
	}
	if err := c.saveAndCommitEvent(parent, fmt.Sprintf("Capped parent event %q", parent.ID)); err != nil {
		_ = c.intervalTree.RemoveEvent(*parent)
		parent.Repeat = originalRepeat
		_ = c.intervalTree.InsertEvent(*parent)
		return nil, fmt.Errorf("failed to commit capped parent: %w", err)
	}

	newEvent := *new
	newEvent.ID = uuid.New()
	newEvent.ParentID = nil
	newEvent.Repeat = after

	created, err := c.CreateEvent(newEvent)
	if err == nil {
		return created, nil
	}

	_ = c.intervalTree.RemoveEvent(*parent)
	parent.Repeat = originalRepeat
	if rollbackErr := c.intervalTree.InsertEvent(*parent); rollbackErr != nil {
		return nil, fmt.Errorf("failed to create new series: %w; rollback failed too: %v", err, rollbackErr)
	}
	if rollbackErr := c.saveAndCommitEvent(parent, fmt.Sprintf("Rolled back parent %q", parent.ID)); rollbackErr != nil {
		return nil, fmt.Errorf("failed to create new series: %w; rollback failed too: %v", err, rollbackErr)
	}
	return nil, fmt.Errorf("failed to create new continuing series: %w", err)
}

// updateAllChildren applies a child update to its parent series.
func (c *Core) updateAllChildren(old, new *Event) (*Event, error) {
	parent, ok := c.events[*old.ParentID]
	if !ok || parent == nil || !parent.IsParent() {
		return nil, errors.New("no valid parent found")
	}

	original := *parent
	fromDiff := new.From.Sub(old.From)
	toDiff := new.To.Sub(old.To)
	updatedRepeat, err := shiftRecurrence(original.Repeat, parent.From, parent.From.Add(fromDiff), new.Repeat)
	if err != nil {
		return nil, fmt.Errorf("failed to update recurrence: %w", err)
	}

	if err := c.intervalTree.RemoveEvent(*parent); err != nil {
		return nil, fmt.Errorf("failed to remove parent from interval tree: %w", err)
	}

	parent.Title = new.Title
	parent.Location = new.Location
	parent.Description = new.Description
	parent.From = parent.From.Add(fromDiff)
	parent.To = parent.To.Add(toDiff)
	parent.TagID = new.TagID
	parent.Calendar = new.Calendar
	parent.Repeat = updatedRepeat

	if err := c.intervalTree.InsertEvent(*parent); err != nil {
		*parent = original
		_ = c.intervalTree.InsertEvent(*parent)
		return nil, fmt.Errorf("failed to reinsert parent: %w", err)
	}

	if err := c.saveAndCommitEvent(parent, fmt.Sprintf("Updated time series (parent %q)", parent.ID)); err != nil {
		return nil, fmt.Errorf("failed to save parent: %w", err)
	}
	return parent, nil
}

func (c *Core) removeCurrentChild(event *Event) error {
	parent, ok := c.events[*event.ParentID]
	if !ok || parent == nil || !parent.IsParent() {
		return errors.New("no valid parent found")
	}
	parent.Repeat.ExDate(event.From)
	if parent.Repeat.After(parent.From, true).IsZero() {
		return c.RemoveEvent(*parent)
	}
	if err := c.saveAndCommitEvent(parent, fmt.Sprintf("Updated event %q", event.ID)); err != nil {
		return fmt.Errorf("failed to save event to repo: %w", err)
	}
	return nil
}

func (c *Core) removeFollowingChildren(event *Event) error {
	parent, ok := c.events[*event.ParentID]
	if !ok || parent == nil || !parent.IsParent() {
		return errors.New("no valid parent found")
	}
	if parent.From.Equal(event.From) {
		return c.removeAllChildren(event)
	}

	capped, err := capRecurrenceBefore(parent.Repeat, event.From)
	if err != nil {
		return fmt.Errorf("failed to cap recurrence: %w", err)
	}
	original := parent.Repeat
	if err := c.intervalTree.RemoveEvent(*parent); err != nil {
		return fmt.Errorf("failed to remove parent from interval tree: %w", err)
	}
	parent.Repeat = capped
	if err := c.intervalTree.InsertEvent(*parent); err != nil {
		parent.Repeat = original
		_ = c.intervalTree.InsertEvent(*parent)
		return fmt.Errorf("failed to reinsert parent into interval tree: %w", err)
	}
	if err := c.saveAndCommitEvent(parent, fmt.Sprintf("Updated event %q", event.ID)); err != nil {
		return fmt.Errorf("failed to save event to repo: %w", err)
	}
	return nil
}

func (c *Core) removeAllChildren(event *Event) error {
	parent, ok := c.events[*event.ParentID] // we check nil pointer in RemoveRepeatingEvent
	if !ok || parent == nil || !parent.IsParent() {
		return errors.New("no valid parent found")
	}

	return c.RemoveEvent(*parent)
}

// saveAndCommitEvent serializes event to JSON, saves to file, stages and commits with given message.
func (c *Core) saveAndCommitEvent(event *Event, commitMsg string) error {
	event.UpdatedAt = time.Now() // force new time

	cal, ok := c.calendars[event.Calendar]
	if !ok {
		return fmt.Errorf("calendar not found: %s", event.Calendar)
	}
	if cal.repository == nil {
		return errors.New("calendar repo not initialized")
	}

	wt, err := cal.repository.Worktree()
	if err != nil {
		return fmt.Errorf("worktree: %w", err)
	}

	filename := fmt.Sprintf("%s.json", event.ID)
	gitPath := path.Join(EventsDirName, filename)

	// ensure the events directory exists
	if err := wt.Filesystem.MkdirAll(EventsDirName, 0o755); err != nil {
		return fmt.Errorf("mkdir events: %w", err)
	}

	// create truncates/overwrites the file if it exists
	file, err := wt.Filesystem.Create(gitPath)
	if err != nil {
		return fmt.Errorf("failed to create event file: %w", err)
	}
	defer file.Close()

	if err := event.WriteToFile(file, cal.EncryptionKey); err != nil {
		return fmt.Errorf("write event file: %w", err)
	}

	if err := file.Close(); err != nil {
		return fmt.Errorf("close event file: %w", err)
	}

	// stage
	if _, err := wt.Add(gitPath); err != nil {
		return fmt.Errorf("git add %q: %w", gitPath, err)
	}

	// commit
	return commitWorktree(wt, commitMsg)
}

// deleteAndCommitEvent removes event from filesystem and commits the change.
func (c *Core) deleteAndCommitEvent(eventID uuid.UUID, commitMsg string) error {
	event, ok := c.events[eventID]
	if !ok {
		return fmt.Errorf("event not found: %s", eventID)
	}

	cal, ok := c.calendars[event.Calendar]
	if !ok {
		return fmt.Errorf("calendar not found: %s", event.Calendar)
	}
	if cal.repository == nil {
		return errors.New("calendar repo not initialized")
	}

	wt, err := cal.repository.Worktree()
	if err != nil {
		return fmt.Errorf("failed to get worktree: %w", err)
	}

	gitPath := path.Join(EventsDirName, fmt.Sprintf("%s.json", eventID))

	if _, err := wt.Remove(gitPath); err != nil {
		return fmt.Errorf("git remove %q: %w", gitPath, err)
	}

	// commit
	return commitWorktree(wt, commitMsg)
}
