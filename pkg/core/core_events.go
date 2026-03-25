package core

import (
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"reflect"
	"slices"
	"time"

	"github.com/firu11/git-calendar-core/pkg/filesystem"
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

	err := c.saveAndCommitEvent(&event, fmt.Sprintf("CALENDAR: Added event '%s'", event.Title))
	if err != nil {
		return nil, fmt.Errorf("failed to save event to repo: %w", err)
	}

	return &event, nil
}

// Updates an event based on its id. You have to specify an UpdateOption to control the behaviour for updating a repeating event.
func (c *Core) UpdateEvent(event Event, opts ...UpdateOption) (*Event, error) {
	if err := event.Validate(); err != nil {
		return nil, fmt.Errorf("invalid event: %w", err)
	}

	if event.isChild() {
		return c.updateGenerated(event, opts...)
	}

	return c.updateReal(event)
}

// Removes an event from calendar
func (c *Core) RemoveEvent(event Event) error {
	if err := event.Validate(); err != nil {
		return fmt.Errorf("invalid event: %w", err)
	}

	if !event.isChild() {
		return c.removeReal(event) // must be deleted entirely
	}
	return c.removeGenerated(event) // must be added to exceptions
}

// Returns event by id, or an error if it doesn't exist.
func (c *Core) GetEvent(id uuid.UUID) (*Event, error) {
	e, ok := c.events[id]
	if !ok {
		return nil, fmt.Errorf("event with id: '%s' doesn't exist", id)
	}
	return e, nil
}

// Returns an array of events which fall into the specified interval <from, to>.
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

func (c *Core) updateReal(event Event) (*Event, error) {
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
	if err := c.saveAndCommitEvent(&event, fmt.Sprintf("CALENDAR: Updated event '%s'", event.Title)); err != nil {
		return nil, err
	}

	return &event, nil
}

// Updates generated event. Assumes that input event is already validated.
func (c *Core) updateGenerated(event Event, opts ...UpdateOption) (*Event, error) {
	if len(opts) != 1 || !opts[0].IsValid() {
		return nil, fmt.Errorf("invalid update event: incorrect options provided")
	}

	parent, ok := c.events[event.ParentId]
	if !ok || parent == nil || !parent.isParent() {
		return nil, fmt.Errorf("invalid update event: no valid parent found")
	}

	switch opts[0] {
	case Current:
		return c.updateGeneratedCurrent(event, parent)
	case Following:
		return c.updateGeneratedFollowing(event, parent)
	case All:
		return c.updateGeneratedAll(event, parent)
	default:
		return nil, fmt.Errorf("update option %d isn't implemented", opts[0])
	}
}

// Updates single generated/child event by adding a repeat rule to its parent and creating a brand new one.
func (c *Core) updateGeneratedCurrent(event Event, parent *Event) (*Event, error) {
	// update parent event with the new exception
	exceptionTime, err := getTimeFromUUID(event.Id)
	if err != nil {
		return nil, err
	}
	if exceptionTime.IsZero() {
		exceptionTime = event.From
	}
	exception := event.Id
	parent.Repeat.Exceptions = append(parent.Repeat.Exceptions, exception)

	if err := c.saveAndCommitEvent(parent, fmt.Sprintf("CALENDAR: Added exception to parent '%s'", parent.Title)); err != nil {
		return nil, fmt.Errorf("failed to save parent event: %w", err)
	}

	event.Repeat = nil          // detach from repeating time series
	return c.CreateEvent(event) // save as new
}

// Splits the time series into two by stopping the original parent event from repeating further and creating brand new repeating event with updated properties.
func (c *Core) updateGeneratedFollowing(event Event, parent *Event) (*Event, error) {
	parent.Repeat.Until = event.From // cap parent at start of change
	parent.Repeat.Count = 0          // enforce "Until" logic over "Count"

	if err := c.saveAndCommitEvent(parent, fmt.Sprintf("CALENDAR: Capped parent event '%s'", parent.Title)); err != nil {
		return nil, fmt.Errorf("failed to commit parent event: %w", err)
	}

	// create the new parent for the second half of the time series
	event.ParentId = uuid.Nil // not child anymore
	event.Id = uuid.Nil       // let it create a new one, don't keep the same as its old parent

	newParent, err := c.CreateEvent(event)
	if err != nil {
		return nil, fmt.Errorf("failed to create new event: %w", err)
	}

	return newParent, nil
}

// Updates the parent event only. That means all generated child events get updated too.
// Keep in mind that function expect Parents only!
func (c *Core) updateGeneratedAll(updated Event, parent *Event) (*Event, error) {
	// check if the event we are changing is the original Parent
	if updated.Id != parent.Id {
		return nil, fmt.Errorf("invalid update event: id '%s' does not match parent id '%s'", updated.Id, parent.Id)
	}

	fromChanged := !updated.From.Equal(parent.From)
	toChanged := !updated.To.Equal(parent.To)
	repeatChanged := !reflect.DeepEqual(updated.Repeat, parent.Repeat)

	if fromChanged || toChanged || repeatChanged {
		if err := c.intervalTree.RemoveEvent(*parent); err != nil {
			return nil, fmt.Errorf("failed to rebuild tree for updated: %w", err)
		}
	}

	// shift all exceptions by the difference between updated.From and parent.From
	difference := updated.From.Sub(parent.From)
	if fromChanged && updated.Repeat != nil {
		for i := range updated.Repeat.Exceptions {
			updated.Repeat.Exceptions[i] = getShiftedUUID(updated.Repeat.Exceptions[i], difference)
		}
	}

	parent.Title = updated.Title
	parent.Location = updated.Location
	parent.Description = updated.Description
	parent.From = updated.From
	parent.To = updated.To
	parent.Tag = updated.Tag
	parent.Repeat = updated.Repeat
	parent.Calendar = updated.Calendar

	if fromChanged || toChanged || repeatChanged {
		if err := c.intervalTree.InsertEvent(*parent); err != nil {
			return nil, fmt.Errorf("failed to rebuild tree for updated: %w", err)
		}
	}

	if err := c.saveAndCommitEvent(parent, fmt.Sprintf("CALENDAR: Updated parent updated '%s'", parent.Title)); err != nil {
		return nil, fmt.Errorf("failed to save updated to repo: %w", err)
	}

	return parent, nil
}

// Deletes a real (basic/repeating parent) event from memory as well as from git.
func (c *Core) removeReal(event Event) error {
	err := c.intervalTree.RemoveEvent(event)
	if err != nil {
		return fmt.Errorf("failed to delete event from interval tree: %w", err)
	}

	// delete file from disk + git
	err = c.deleteAndCommitEvent(event.Id, fmt.Sprintf("CALENDAR: Delete event '%s'", event.Title))
	if err != nil {
		return fmt.Errorf("failed to delete event from git: %w", err)
	}

	delete(c.events, event.Id)
	return nil
}

// Deletes a child event by adding an exception to its parent repeat rule.
func (c *Core) removeGenerated(event Event) error {
	parentEvent := c.events[event.ParentId]
	if parentEvent == nil || parentEvent.Repeat == nil {
		return fmt.Errorf("parent event not found")
	}

	// if exception doesn't exist yet
	if !slices.Contains(parentEvent.Repeat.Exceptions, event.Id) {
		// add date to parent exceptions
		newException := event.Id
		parentEvent.Repeat.Exceptions = append(parentEvent.Repeat.Exceptions, newException)

		// update/overwrite the file in repo
		err := c.saveAndCommitEvent(parentEvent, fmt.Sprintf("CALENDAR: Updated event '%s'", event.Title))
		if err != nil {
			return fmt.Errorf("failed to save event to repo: %w", err)
		}
	}

	delete(c.events, event.Id) // TODO is this needed? It's no-op, but can there be a generated event in events?
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
