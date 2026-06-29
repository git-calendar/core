package core

import (
	"errors"
	"fmt"
	"path"
	"slices"
	"time"

	gogit "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/google/uuid"
)

// Creates a new event and save it into git.
func (c *Core) CreateEvent(event Event) (*Event, error) {
	if _, ok := c.events[event.Id]; ok && event.Id != uuid.Nil {
		return nil, fmt.Errorf("an event with this id already exists")
	}

	cal, ok := c.calendars[event.Calendar]
	if !ok {
		return nil, fmt.Errorf("the specified calendar is missing")
	}
	if cal.Readonly {
		return nil, fmt.Errorf("the specified calendar is read-only")
	}

	if err := event.Validate(); err != nil {
		return nil, fmt.Errorf("invalid event: %w", err)
	}

	c.events[event.Id] = &event

	if err := c.intervalTree.InsertEvent(event); err != nil {
		return nil, fmt.Errorf("failed to insert into index tree: %w", err)
	}

	err := c.saveAndCommitEvent(&event, fmt.Sprintf("Added event %q", event.Id))
	if err != nil {
		return nil, fmt.Errorf("failed to save event to repo: %w", err)
	}

	return &event, nil
}

// Updates a Basic event based on its id. Use UpdateRepeatingEvent method for repeating events.
func (c *Core) UpdateEvent(event Event) (*Event, error) {
	if err := event.Validate(); err != nil {
		return nil, fmt.Errorf("invalid event: %w", err)
	}

	originalEvent, exists := c.events[event.Id]
	if !exists {
		return nil, fmt.Errorf("no event found with id %q", event.Id)
	}

	cal, ok := c.calendars[event.Calendar]
	if !ok {
		return nil, fmt.Errorf("the specified calendar is missing")
	}
	if cal.Readonly {
		return nil, fmt.Errorf("the specified calendar is read-only")
	}

	oldEnd := originalEvent.getTreeEndTime()
	newEnd := event.getTreeEndTime()

	if originalEvent.From != event.From || oldEnd != newEnd { // update the interval tree
		ids, found := c.intervalTree.tree.Find(originalEvent.From, oldEnd)
		if found {
			index := slices.Index(ids, originalEvent.Id)
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

	newRepoCommitMsg := fmt.Sprintf("Updated event %q", event.Id)
	if originalEvent.Calendar != event.Calendar {
		newRepoCommitMsg = fmt.Sprintf("Moved event from another calendar %q", event.Id)
		if err := c.deleteAndCommitEvent(originalEvent.Id, fmt.Sprintf("Moved event to another calendar %q", originalEvent.Id)); err != nil { // remote the old file from previous calendar/repo
			return nil, err
		}
	}

	c.events[event.Id] = &event
	if err := c.saveAndCommitEvent(&event, newRepoCommitMsg); err != nil {
		return nil, err
	}

	return &event, nil
}

// Removes a child event by adding an exception to its parent repeat rule.
func (c *Core) UpdateRepeatingEvent(old, new Event, strat UpdateStrategy) (*Event, error) {
	if err := old.Validate(); err != nil {
		return nil, fmt.Errorf("invalid old event: %w", err)
	}
	if err := new.Validate(); err != nil {
		return nil, fmt.Errorf("invalid new event: %w", err)
	}
	if !strat.IsValid() {
		return nil, fmt.Errorf("incorrect strategy provided")
	}
	if old.Id != new.Id { // check if the event we are changing is the original Parent
		return nil, fmt.Errorf("invalid update event: id %q does not match parent id %q", old.Id, new.Id)
	}
	if !old.IsChild() || !new.IsChild() {
		return nil, fmt.Errorf("repeating update requires child events")
	}
	if old.ParentId != new.ParentId {
		return nil, fmt.Errorf("old and new child events have different parent ids")
	}

	cal, ok := c.calendars[new.Calendar]
	if !ok {
		return nil, fmt.Errorf("the specified calendar is missing")
	}
	if cal.Readonly {
		return nil, fmt.Errorf("the specified calendar is read-only")
	}

	switch strat {
	case Current:
		return c.updateCurrentChild(&new)
	case Following:
		return c.updateFollowingChildren(&old, &new)
	case All:
		return c.updateAllChildren(&old, &new)
	default:
		return nil, fmt.Errorf("update strategy %d isn't implemented", strat)
	}
}

// Removes a real (basic/parent) event from the calendar. Use RemoveRepeatingEvent method for repeating events.
func (c *Core) RemoveEvent(event Event) error {
	if err := event.Validate(); err != nil {
		return fmt.Errorf("invalid event: %w", err)
	}

	if cal, ok := c.calendars[event.Calendar]; ok && cal.Readonly {
		return fmt.Errorf("the events calendar is read-only")
	}

	err := c.intervalTree.RemoveEvent(event)
	if err != nil {
		return fmt.Errorf("failed to delete event from interval tree: %w", err)
	}

	// delete file from disk + git
	err = c.deleteAndCommitEvent(event.Id, fmt.Sprintf("Deleted event %q", event.Id))
	if err != nil {
		return fmt.Errorf("failed to delete event from git: %w", err)
	}

	delete(c.events, event.Id)
	return nil
}

// Removes a child event by adding an exception to its parent repeat rule.
func (c *Core) RemoveRepeatingEvent(event Event, strat UpdateStrategy) error {
	if err := event.Validate(); err != nil {
		return fmt.Errorf("invalid event: %w", err)
	}

	if cal, ok := c.calendars[event.Calendar]; ok && cal.Readonly {
		return fmt.Errorf("the events calendar is read-only")
	}

	if !event.IsChild() {
		return errors.New("event has to be a child to be removed using this method")
	}

	switch strat {
	case Current:
		return c.removeCurrentChild(&event)
	// TODO:
	// case Following:
	// 	return c.removeFollowingChildren(&event)
	// case All:
	// 	return c.removeAllChildren(&event)
	default:
		return fmt.Errorf("update strategy %d isn't implemented", strat)
	}
}

// Returns event by id, or an error if it doesn't exist.
func (c *Core) GetEvent(id uuid.UUID) (*Event, error) {
	e, ok := c.events[id]
	if !ok {
		return nil, fmt.Errorf("event with id: %q doesn't exist", id)
	}
	return e, nil
}

// Returns an array of events which fall into the specified interval [from, to].
func (c *Core) GetEvents(from, to time.Time) []Event {
	// query the interval tree
	intervalsMatched, found := c.intervalTree.tree.AllIntersections(from, to)
	if !found {
		return []Event{}
	}

	result := make([]Event, 0, len(intervalsMatched))

	for _, intersection := range intervalsMatched {
		for _, eId := range intersection {
			curEvent, ok := c.events[eId]
			if !ok {
				fmt.Printf("event with id: '%v' doesn't exist in events map WTF\n", eId)
				continue
			}

			// if it doesn't repeat, just plain append to result
			if curEvent.Repeat == nil {
				result = append(result, *c.events[eId])
				continue
			}

			eventDuration := curEvent.To.Sub(curEvent.From)
			firstStart, index := firstOccurrenceAtOrAfter(from, curEvent)

			if firstStart.IsZero() {
				continue // no occurrences >= from
			}

			for firstStart.Before(to) { // while child event fits in the wanted interval
				// logic when repeating until
				if curEvent.Repeat.Count == 0 && firstStart.After(curEvent.Repeat.Until) {
					break // new event exceeded the repetition end (Until)
				}
				// logic for repeating only N times (count)
				if curEvent.Repeat.Count != 0 && index >= curEvent.Repeat.Count {
					break // new event exceeded the max count of child events
				}

				index++
				child := Event{
					Id:          generateCustomUUID(curEvent.Id, firstStart),
					Title:       curEvent.Title,
					Location:    curEvent.Location,
					Description: curEvent.Description,
					From:        firstStart,
					To:          firstStart.Add(eventDuration),
					Calendar:    curEvent.Calendar,
					TagId:       curEvent.TagId,
					ParentId:    &curEvent.Id,
					Repeat:      curEvent.Repeat,
				}
				// ignore exceptions
				if !slices.Contains(curEvent.Repeat.Exceptions, child.Id) {
					result = append(result, child)
				}

				firstStart = addUnit(firstStart, curEvent.Repeat.Interval, curEvent.Repeat.Frequency) // next occurrence
			}
		}
	}

	return result
}

// ------------------------------------------------ Helpers -------------------------------------------------

// Updates single generated/child event by adding it to its Parent repeat exceptions and creating a brand new event instead.
func (c *Core) updateCurrentChild(updated *Event) (*Event, error) {
	parent, ok := c.events[*updated.ParentId] // we check nil pointer in UpdateRepeatingEvent
	if !ok || parent == nil || !parent.IsParent() {
		return nil, fmt.Errorf("no valid parent found")
	}
	if parent.Repeat == nil {
		return nil, fmt.Errorf("parent is not a repeating event, WTF")
	}

	// update parent event with the new exception
	parent.Repeat.Exceptions = append(parent.Repeat.Exceptions, updated.Id)
	if err := c.saveAndCommitEvent(parent, fmt.Sprintf("Added exception to parent %q", parent.Id)); err != nil {
		return nil, fmt.Errorf("failed to save parent event: %w", err)
	}

	// detach updated from repeating time series
	detachedEvent := *updated    // shallow copy
	detachedEvent.Repeat = nil   // not repeating anymore
	detachedEvent.ParentId = nil // not child anymore
	detachedEvent.Id = uuid.Nil  // set to uuid.Nil; CreateEvent will asign a new one

	return c.CreateEvent(detachedEvent) // save as new
}

// updateFollowingChildren splits the time series into two by stopping the original parent event from repeating further and creating brand new parent with updated properties.
func (c *Core) updateFollowingChildren(old, new *Event) (*Event, error) {
	parent, ok := c.events[*old.ParentId] // we check nil pointer in UpdateRepeatingEvent
	if !ok || parent == nil || !parent.IsParent() {
		return nil, fmt.Errorf("no valid parent found")
	}
	if parent.Repeat == nil {
		return nil, fmt.Errorf("parent is not a repeating event")
	}

	// keep originals for rollback
	originalRepeat := *parent.Repeat
	originalExceptions := append([]uuid.UUID(nil), parent.Repeat.Exceptions...)

	// find position in the series
	_, splitIndex := firstOccurrenceAtOrAfter(old.From, parent)
	if splitIndex == -1 {
		return nil, fmt.Errorf("could not find occurrence at or after %s", old.From)
	}

	fromDiff := new.From.Sub(old.From)

	// editing the first occurrence -> update entire series
	if splitIndex == 0 {
		return c.updateAllChildren(old, new)
	}

	// split exceptions before and after the edited occurrence
	exBefore, exAfter := splitExceptions(originalExceptions, old.From)

	// remove parent before we mutate it
	if err := c.intervalTree.RemoveEvent(*parent); err != nil {
		return nil, fmt.Errorf("failed to remove parent from interval tree: %w", err)
	}

	// ------------ cap the original parent ------------
	previousStart := addUnit(old.From, -originalRepeat.Interval, originalRepeat.Frequency)
	if !previousStart.After(parent.From) {
		// only one occurrence left -> turn it into a basic (non-repeating) event
		parent.Repeat = nil
	} else {
		// keep repeating but stop before the split point
		capped := originalRepeat
		capped.Until = previousStart
		capped.Count = 0
		capped.Exceptions = exBefore
		parent.Repeat = &capped
	}

	// save the capped parent
	if err := c.intervalTree.InsertEvent(*parent); err != nil {
		parent.Repeat = &originalRepeat
		parent.Repeat.Exceptions = originalExceptions
		return nil, fmt.Errorf("failed to reinsert capped parent: %w", err)
	}

	if err := c.saveAndCommitEvent(parent, fmt.Sprintf("Capped parent event %q", parent.Id)); err != nil {
		return nil, fmt.Errorf("failed to commit capped parent: %w", err)
	}

	// ------------ create the new continuing series ------------
	newEvent := *new
	newEvent.Id = uuid.New()
	newEvent.ParentId = nil // this is now its own parent

	if newEvent.Repeat != nil {
		repeat := *newEvent.Repeat
		repeat.Exceptions = nil

		// adjust count for remaining occurrences
		if originalRepeat.Count != 0 {
			remaining := originalRepeat.Count - splitIndex
			if remaining < 1 {
				remaining = 1
			}
			repeat.Count = remaining
		}

		// shift exceptions to the new parent
		for _, ex := range exAfter {
			exTime := getTimeFromUUID(ex).Add(fromDiff)
			repeat.Exceptions = append(repeat.Exceptions, generateCustomUUID(newEvent.Id, exTime))
		}

		newEvent.Repeat = &repeat
	}

	created, err := c.CreateEvent(newEvent)
	if err != nil {
		// rollback: restore original parent
		_ = c.intervalTree.RemoveEvent(*parent)
		parent.Repeat = &originalRepeat
		parent.Repeat.Exceptions = originalExceptions

		if rbErr := c.intervalTree.InsertEvent(*parent); rbErr != nil {
			return nil, fmt.Errorf("failed to create new series: %w; rollback failed too: %v", err, rbErr)
		}
		if rbErr := c.saveAndCommitEvent(parent, fmt.Sprintf("rolled back parent %q", parent.Id)); rbErr != nil {
			return nil, fmt.Errorf("failed to create new series: %w; rollback failed too: %v", err, rbErr)
		}
		return nil, fmt.Errorf("failed to create new continuing series: %w", err)
	}

	return created, nil
}

// Updates the entire repeating series by only modifying the parent. That means all generated child events get updated as well.
// Both old and new arguments are child events.
func (c *Core) updateAllChildren(old, new *Event) (*Event, error) {
	parent, ok := c.events[*old.ParentId] // we check nil pointer in UpdateRepeatingEvent
	if !ok || parent == nil || !parent.IsParent() {
		return nil, fmt.Errorf("no valid parent found")
	}

	fromDiff := new.From.Sub(old.From)
	toDiff := new.To.Sub(old.To)

	fromChanged := fromDiff != 0
	toChanged := toDiff != 0
	repeatChanged := repeatRuleChanged(parent.Repeat, new.Repeat)

	needsReindex := fromChanged || toChanged || repeatChanged

	if needsReindex {
		if err := c.intervalTree.RemoveEvent(*parent); err != nil {
			return nil, fmt.Errorf("failed to remove parent from interval tree: %w", err)
		}
	}

	var exceptions []uuid.UUID
	if parent.Repeat != nil {
		exceptions = append(exceptions, parent.Repeat.Exceptions...) // copy
	}

	// shift all exceptions by the time fromDiff
	if fromChanged {
		for i := range exceptions {
			exceptions[i] = getShiftedUUID(exceptions[i], fromDiff)
		}
	}

	if new.Repeat != nil {
		repeat := *new.Repeat
		repeat.Exceptions = exceptions
		parent.Repeat = &repeat
	} else {
		parent.Repeat = nil
	}

	parent.Title = new.Title
	parent.Location = new.Location
	parent.Description = new.Description
	parent.From = parent.From.Add(fromDiff)
	parent.To = parent.To.Add(toDiff)
	parent.TagId = new.TagId
	parent.Calendar = new.Calendar

	if needsReindex {
		if err := c.intervalTree.InsertEvent(*parent); err != nil {
			return nil, fmt.Errorf("failed to reinsert parent: %w", err)
		}
	}

	if err := c.saveAndCommitEvent(parent,
		fmt.Sprintf("Updated time series (parent %q)", parent.Id),
	); err != nil {
		return nil, fmt.Errorf("failed to save parent: %w", err)
	}

	return parent, nil
}

func (c *Core) removeCurrentChild(event *Event) error {
	parent, ok := c.events[*event.ParentId] // we check nil pointer in RemoveRepeatingEvent
	if !ok || parent == nil || !parent.IsParent() {
		return fmt.Errorf("no valid parent found")
	}

	// if exception doesn't exist yet
	if !slices.Contains(parent.Repeat.Exceptions, event.Id) {
		// add date to parent exceptions
		newException := event.Id
		parent.Repeat.Exceptions = append(parent.Repeat.Exceptions, newException)

		// update/overwrite the file in repo
		err := c.saveAndCommitEvent(parent, fmt.Sprintf("Updated event %q", event.Id))
		if err != nil {
			return fmt.Errorf("failed to save event to repo: %w", err)
		}
	}

	// TODO: finish this
	// cleanup the Parent if all Children are in Exceptions
	// either (Count != 0 and Count = len(Exceptions)) or TODO: hard to know from the Until
	if (parent.Repeat.Count != 0 && len(parent.Repeat.Exceptions) == parent.Repeat.Count) || (!parent.Repeat.Until.IsZero() && false) { // ughhh
		err := c.deleteAndCommitEvent(parent.Id, fmt.Sprintf("Deleted event %q", parent.Id))
		if err != nil {
			return fmt.Errorf("failed to delete event from git: %w", err)
		}
		delete(c.events, parent.Id)
	}

	return nil
}

// Serializes event to JSON, saves to file, stages and commits with given message.
func (c *Core) saveAndCommitEvent(event *Event, commitMsg string) error {
	event.UpdatedAt = time.Now() // force new time

	cal, ok := c.calendars[event.Calendar]
	if !ok {
		return fmt.Errorf("calendar not found: %s", event.Calendar)
	}
	if cal.repository == nil {
		return fmt.Errorf("calendar repo not initialized")
	}

	wt, err := cal.repository.Worktree()
	if err != nil {
		return fmt.Errorf("worktree: %w", err)
	}

	filename := fmt.Sprintf("%s.json", event.Id)
	gitPath := path.Join(EventsDirName, filename)

	// ensure events directory exists
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
	_, err = wt.Commit(commitMsg, &gogit.CommitOptions{
		Author: &object.Signature{
			Name:  GitAuthorName,
			Email: "",
			When:  time.Now(),
		},
	})
	if err != nil && !errors.Is(err, gogit.ErrEmptyCommit) {
		return fmt.Errorf("failed to git commit: %w", err)
	}

	return nil
}

// Removes event from filesystem and commits the change.
func (c *Core) deleteAndCommitEvent(eventId uuid.UUID, commitMsg string) error {
	event, ok := c.events[eventId]
	if !ok {
		return fmt.Errorf("event not found: %s", eventId)
	}

	cal, ok := c.calendars[event.Calendar]
	if !ok {
		return fmt.Errorf("calendar not found: %s", event.Calendar)
	}
	if cal.repository == nil {
		return fmt.Errorf("calendar repo not initialized")
	}

	wt, err := cal.repository.Worktree()
	if err != nil {
		return fmt.Errorf("failed to get worktree: %w", err)
	}

	gitPath := path.Join(EventsDirName, fmt.Sprintf("%s.json", eventId))

	if _, err := wt.Remove(gitPath); err != nil {
		return fmt.Errorf("git remove %q: %w", gitPath, err)
	}

	_, err = wt.Commit(commitMsg, &gogit.CommitOptions{
		Author: &object.Signature{
			Name:  GitAuthorName,
			Email: "",
			When:  time.Now(),
		},
	})
	if err != nil && !errors.Is(err, gogit.ErrEmptyCommit) {
		return fmt.Errorf("failed to git commit: %w", err)
	}

	return nil
}
