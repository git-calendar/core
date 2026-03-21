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

	if event.isGenerated() {
		return c.updateGenerated(event, opts...)
	}

	return c.updateReal(event)
}

// Removes an event from calendar
func (c *Core) RemoveEvent(event Event) error {
	if err := event.Validate(); err != nil {
		return fmt.Errorf("invalid event: %w", err)
	}

	if !event.isGenerated() {
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
					MasterId:    curEvent.Id,
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

	master, ok := c.events[event.MasterId]
	if !ok || master == nil || master.Repeat == nil {
		return nil, fmt.Errorf("invalid update event: no valid master found")
	}

	switch opts[0] {
	case Current:
		return c.updateGeneratedCurrent(event, master)
	case Following:
		return c.updateGeneratedFollowing(event, master)
	case All:
		return c.updateGeneratedAll(event, master)
	default:
		return nil, fmt.Errorf("update option %d isn't implemented", opts[0])
	}
}

// Updates single generated/slave event by adding a repeat rule to its master and creating a brand new one.
func (c *Core) updateGeneratedCurrent(event Event, master *Event) (*Event, error) {
	// update master event with the new exception
	exceptionTime, err := getTimeFromUUID(event.Id)
	if err != nil {
		return nil, err
	}
	if exceptionTime.IsZero() {
		exceptionTime = event.From
	}
	exception := event.Id
	master.Repeat.Exceptions = append(master.Repeat.Exceptions, exception)

	if err := c.saveAndCommitEvent(master, fmt.Sprintf("CALENDAR: Added exception to master '%s'", master.Title)); err != nil {
		return nil, fmt.Errorf("failed to save master event: %w", err)
	}

	event.Repeat = nil          // detach from repeating time series
	return c.CreateEvent(event) // save as new
}

// Splits the time series into two by stopping the original master event from repeating further and creating brand new repeating event with updated properties.
func (c *Core) updateGeneratedFollowing(event Event, master *Event) (*Event, error) {
	master.Repeat.Until = event.From // cap master at start of change
	master.Repeat.Count = 0          // enforce "Until" logic over "Count"

	if err := c.saveAndCommitEvent(master, fmt.Sprintf("CALENDAR: Capped master event '%s'", master.Title)); err != nil {
		return nil, fmt.Errorf("failed to commit master event: %w", err)
	}

	// create the new master for the second half of the time series
	event.MasterId = uuid.Nil // not slave anymore
	event.Id = uuid.Nil       // let it create a new one, don't keep the same as its old master

	newMaster, err := c.CreateEvent(event)
	if err != nil {
		return nil, fmt.Errorf("failed to create new event: %w", err)
	}

	return newMaster, nil
}

// Updates the master event only. That means all generated slave events get updated too.
func (c *Core) updateGeneratedAll(updated Event, master *Event) (*Event, error) {
	fromChanged := !updated.From.Equal(master.From)
	toChanged := !updated.To.Equal(master.To)
	repeatChanged := !reflect.DeepEqual(*updated.Repeat, *master.Repeat)

	if fromChanged || toChanged || repeatChanged {
		if err := c.intervalTree.RemoveEvent(*master); err != nil {
			return nil, fmt.Errorf("failed to rebuild tree for event: %w", err)
		}
	}

	master.Title = updated.Title
	master.Location = updated.Location
	master.Description = updated.Description
	master.From = withTimeOfDay(master.From, updated.From)
	master.To = withTimeOfDay(master.To, updated.To)
	master.Tag = updated.Tag
	master.Repeat = updated.Repeat
	master.Calendar = updated.Calendar

	if fromChanged || toChanged || repeatChanged {
		if err := c.intervalTree.InsertEvent(*master); err != nil {
			return nil, fmt.Errorf("failed to rebuild tree for event: %w", err)
		}
	}

	if err := c.saveAndCommitEvent(master, fmt.Sprintf("CALENDAR: Updated master event '%s'", master.Title)); err != nil {
		return nil, fmt.Errorf("failed to save event to repo: %w", err)
	}

	return master, nil
}

// Deletes a real (basic/repeating master) event from memory as well as from git.
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

// Deletes a generated/slave event by adding an exception to its master repeat rule.
func (c *Core) removeGenerated(event Event) error {
	masterEvent := c.events[event.MasterId]
	if masterEvent == nil || masterEvent.Repeat == nil {
		return fmt.Errorf("master event not found")
	}

	// if exception doesn't exist yet
	if !slices.Contains(masterEvent.Repeat.Exceptions, event.Id) {
		// add date to master exceptions
		newException := event.Id
		masterEvent.Repeat.Exceptions = append(masterEvent.Repeat.Exceptions, newException)

		// update/overwrite the file in repo
		err := c.saveAndCommitEvent(masterEvent, fmt.Sprintf("CALENDAR: Updated event '%s'", event.Title))
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
