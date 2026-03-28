package core

import (
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"reflect"
	"slices"
	"time"

	"github.com/git-calendar/core/pkg/filesystem"
	gogit "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/google/uuid"
)

// Creates a new event and save it into git.
func (c *Core) CreateEvent(event Event) (*Event, error) {
	if _, ok := c.events[event.Id]; ok && event.Id != uuid.Nil {
		return nil, fmt.Errorf("an event with this id already exists")
	}

	if err := event.Validate(); err != nil {
		return nil, fmt.Errorf("invalid event: %w", err)
	}

	c.events[event.Id] = &event

	if err := c.intervalTree.InsertEvent(event); err != nil {
		return nil, fmt.Errorf("failed to insert into index tree: %w", err)
	}

	err := c.saveAndCommitEvent(&event, fmt.Sprintf("Added event '%s'", event.Id))
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
		return nil, fmt.Errorf("no event found with id '%s'", event.Id)
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

	c.events[event.Id] = &event
	if err := c.saveAndCommitEvent(&event, fmt.Sprintf("Updated event '%s'", event.Id)); err != nil {
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
		return nil, fmt.Errorf("invalid update event: id '%s' does not match parent id '%s'", old.Id, new.Id)
	}

	switch strat {
	case Current:
		return c.updateCurrentChild(&new)
	case Following:
		return c.updateFollowingChildren(&new)
	case All:
		return c.updateAllChildren(&old, &new)
	default:
		return nil, fmt.Errorf("update strategy %d isn't implemented", strat)
	}
}

// Removes a real (basic/repeating parent) event from the calendar. Use RemoveRepeatingEvent method for repeating events.
func (c *Core) RemoveEvent(event Event) error {
	if err := event.Validate(); err != nil {
		return fmt.Errorf("invalid event: %w", err)
	}

	err := c.intervalTree.RemoveEvent(event)
	if err != nil {
		return fmt.Errorf("failed to delete event from interval tree: %w", err)
	}

	// delete file from disk + git
	err = c.deleteAndCommitEvent(event.Id, fmt.Sprintf("Delete event '%s'", event.Id))
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
		return nil, fmt.Errorf("event with id: '%s' doesn't exist", id)
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

			for firstStart.Before(to) { // while generated event fits in the wanted interval
				if firstStart.Add(eventDuration).Before(from) { // if the generated event ends before our wanted interval -> skip
					firstStart = addUnit(firstStart, curEvent.Repeat.Interval, curEvent.Repeat.Frequency) // next occurrence
					continue
				}
				// logic when repeating until
				if curEvent.Repeat.Count == 0 && firstStart.After(curEvent.Repeat.Until) {
					break // new event exceeded the repetition end (Until)
				}
				// logic for repeating only N times (count)
				if curEvent.Repeat.Count != 0 && index >= curEvent.Repeat.Count {
					break // new event exceeded the max count of generated events
				}

				index++
				generatedEvent := Event{
					Id:          generateCustomUUID(curEvent.Id, firstStart),
					Title:       curEvent.Title,
					Location:    curEvent.Location,
					Description: curEvent.Description,
					From:        firstStart,
					To:          firstStart.Add(eventDuration),
					Calendar:    curEvent.Calendar,
					Tag:         curEvent.Tag,
					ParentId:    curEvent.Id,
					Repeat:      curEvent.Repeat,
				}
				// ignore exceptions
				if !slices.Contains(curEvent.Repeat.Exceptions, generatedEvent.Id) {
					result = append(result, generatedEvent)
				}

				firstStart = addUnit(firstStart, curEvent.Repeat.Interval, curEvent.Repeat.Frequency) // next occurrence
			}
		}
	}

	return result
}

// ------------------------------------------------ Helpers -------------------------------------------------

// Updates single generated/child event by adding a repeat exception to its Parent and creating a brand new event instead.
func (c *Core) updateCurrentChild(updated *Event) (*Event, error) {
	parent, ok := c.events[updated.ParentId]
	if !ok || parent == nil || !parent.IsParent() {
		return nil, fmt.Errorf("no valid parent found")
	}

	// update parent event with the new exception
	parent.Repeat.Exceptions = append(updated.Repeat.Exceptions, updated.Id)
	if err := c.saveAndCommitEvent(updated, fmt.Sprintf("Added exception to parent '%s'", updated.Id)); err != nil {
		return nil, fmt.Errorf("failed to save parent event: %w", err)
	}

	// detach updated from repeating time series
	updated.Repeat = nil
	updated.ParentId = uuid.Nil
	updated.Id = uuid.Nil // let it create a new one

	return c.CreateEvent(*updated) // save as new
}

// Splits the time series into two by stopping the original parent event from repeating further and creating brand new parent with updated properties.
func (c *Core) updateFollowingChildren(event *Event) (*Event, error) {
	parent, ok := c.events[event.ParentId]
	if !ok || parent == nil || !parent.IsParent() {
		return nil, fmt.Errorf("no valid parent found")
	}

	parent.Repeat.Until = event.From // cap parent at start of change
	parent.Repeat.Count = 0          // enforce Until logic over Count

	if err := c.saveAndCommitEvent(parent, fmt.Sprintf("Capped parent event '%s'", parent.Id)); err != nil {
		return nil, fmt.Errorf("failed to commit parent event: %w", err)
	}

	// create the new parent for the second half of the time series
	event.ParentId = uuid.Nil // not child anymore
	event.Id = uuid.Nil       // set to nil; CreateEvent will asign a new one

	if event.Repeat.Count != 0 {
		// TODO: shorten the repeat for the second half
	}

	newParent, err := c.CreateEvent(*event)
	if err != nil {
		return nil, fmt.Errorf("failed to create new event: %w", err)
	}

	return newParent, nil
}

// Updates the entire repeating series by only modifying the parent. That means all generated child events get updated as well.
// Both old and new arguments are child events.
func (c *Core) updateAllChildren(old, new *Event) (*Event, error) {
	if old.IsParent() || new.IsParent() {
		return nil, fmt.Errorf("updateRepeatingAll works with child event")
	}

	parent, ok := c.events[new.ParentId]
	if !ok || parent == nil || !parent.IsParent() {
		return nil, fmt.Errorf("no valid parent found")
	}

	fromDiff := new.From.Sub(old.From)
	toDiff := new.To.Sub(old.To)
	toChanged := toDiff != 0
	fromChanged := fromDiff != 0
	repeatChanged := !reflect.DeepEqual(old.Repeat, new.Repeat)

	if fromChanged || toChanged || repeatChanged {
		if err := c.intervalTree.RemoveEvent(*parent); err != nil {
			return nil, fmt.Errorf("failed to remove the parent from interval tree: %w", err)
		}
	}

	// shift all exceptions by the time fromDiff
	if fromChanged && parent.Repeat != nil {
		for i := range parent.Repeat.Exceptions {
			parent.Repeat.Exceptions[i] = getShiftedUUID(parent.Repeat.Exceptions[i], fromDiff)
		}
	}

	if new.Repeat == nil { // updated event does not repeat
		parent.Repeat = nil
	}

	parent.Title = new.Title
	parent.Location = new.Location
	parent.Description = new.Description
	parent.From = parent.From.Add(fromDiff)
	parent.To = parent.To.Add(toDiff)
	parent.Tag = new.Tag
	parent.Calendar = new.Calendar

	c.events[parent.Id] = parent

	if fromChanged || toChanged || repeatChanged {
		if err := c.intervalTree.InsertEvent(*parent); err != nil {
			return nil, fmt.Errorf("failed to rebuild tree for updated: %w", err)
		}
	}

	if err := c.saveAndCommitEvent(parent, fmt.Sprintf("Updated parent updated '%s'", new.Id)); err != nil {
		return nil, fmt.Errorf("failed to save updated to repo: %w", err)
	}

	return new, nil
}

func (c *Core) removeCurrentChild(event *Event) error {
	parent, ok := c.events[event.ParentId]
	if !ok || parent == nil || !parent.IsParent() {
		return fmt.Errorf("no valid parent found")
	}

	// if exception doesn't exist yet
	if !slices.Contains(parent.Repeat.Exceptions, event.Id) {
		// add date to parent exceptions
		newException := event.Id
		parent.Repeat.Exceptions = append(parent.Repeat.Exceptions, newException)

		// update/overwrite the file in repo
		err := c.saveAndCommitEvent(parent, fmt.Sprintf("Updated event '%s'", event.Id))
		if err != nil {
			return fmt.Errorf("failed to save event to repo: %w", err)
		}
	}

	// TODO: cleanup the parent if all children are in exceptions
	// either Count != 0 and count = len(exceptions) or hard to know from the Until
	if (parent.Repeat.Count != 0 && len(parent.Repeat.Exceptions) == parent.Repeat.Count) || (!parent.Repeat.Until.IsZero() && false) { // ughhh
		err := c.deleteAndCommitEvent(parent.ParentId, fmt.Sprintf("Delete event '%s'", event.Id))
		if err != nil {
			return fmt.Errorf("failed to delete event from git: %w", err)
		}
		delete(c.events, parent.Id)
	}

	return nil
}

// Serializes event to JSON, saves to file, stages and commits with given message.
func (c *Core) saveAndCommitEvent(event *Event, commitMsg string) error {
	// marshal event
	data, err := json.MarshalIndent(event, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal event: %w", err)
	}

	// ensure events directory exists
	dirPath := c.fs.Join(filesystem.DirName, event.Calendar, EventsDirName)
	if err := c.fs.MkdirAll(dirPath, 0o755); err != nil {
		return fmt.Errorf("failed mkdir events: %w", err)
	}

	filename := fmt.Sprintf("%s.json", event.Id)
	filePath := c.fs.Join(dirPath, filename)

	// create truncates/overwrites the file if it exists
	file, err := c.fs.Create(filePath)
	if err != nil {
		return fmt.Errorf("failed to create file: %w", err)
	}

	// write JSON content
	if _, err := file.Write(data); err != nil {
		_ = file.Close()
		return fmt.Errorf("failed to write file: %w", err)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("failed to close file: %w", err)
	}

	if c.repos[event.Calendar] == nil {
		return fmt.Errorf("calendar repo not initialized")
	}

	// -------- add to git repo --------
	w, err := c.repos[event.Calendar].Worktree()
	if err != nil {
		return fmt.Errorf("failed to get worktree: %w", err)
	}

	// stage
	gitPath := filepath.ToSlash(c.fs.Join(EventsDirName, filename))
	if _, err := w.Add(gitPath); err != nil {
		return fmt.Errorf("git add: %w", err)
	}

	// commit
	_, err = w.Commit(commitMsg, &gogit.CommitOptions{
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
		return fmt.Errorf("failed to find event by id")
	}
	filename := fmt.Sprintf("%s.json", eventId)

	// -------- remove from filesystem --------
	filePath := c.fs.Join(filesystem.DirName, event.Calendar, EventsDirName, filename)
	if err := c.fs.Remove(filePath); err != nil {
		// TODO maybe continue, to clean the git from this file
		return fmt.Errorf("failed to remove file from disk: %w", err)
	}

	// -------- remove from git --------
	repo, ok := c.repos[event.Calendar]
	if !ok {
		return fmt.Errorf("calendar repo not initialized")
	}

	w, err := repo.Worktree()
	if err != nil {
		return fmt.Errorf("failed to get worktree: %w", err)
	}

	gitPath := filepath.ToSlash(c.fs.Join(EventsDirName, filename))

	if _, err := w.Remove(gitPath); err != nil {
		return fmt.Errorf("git remove: %w", err)
	}

	_, err = w.Commit(commitMsg, &gogit.CommitOptions{
		Author: &object.Signature{
			Name:  GitAuthorName,
			Email: "",
			When:  time.Now(),
		},
	})
	if err != nil {
		return fmt.Errorf("failed to git commit: %w", err)
	}

	return nil
}
