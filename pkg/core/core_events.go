package core

import (
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"slices"
	"time"

	"github.com/firu11/git-calendar-core/pkg/filesystem"
	gogit "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/google/uuid"
)

// Creates a new event.
func (c *Core) CreateEvent(event Event) (*Event, error) {
	if err := event.Validate(); err != nil {
		return nil, fmt.Errorf("invalid event: %w", err)
	}

	c.events[event.Id] = &event

	if err := insertEventIntoTree(c.eventTree, event); err != nil {
		return nil, fmt.Errorf("failed to insert into index tree: %w", err)
	}

	err := c.saveAndCommitEvent(&event, fmt.Sprintf("CALENDAR: Added event '%s'", event.Title))
	if err != nil {
		return nil, fmt.Errorf("failed to save event to repo: %w", err)
	}

	return &event, nil
}

// Updates an event based on its id. You can specify an UpdateOption to control the behaviour for updating a repeating event,
func (c *Core) UpdateEvent(event Event, opts ...UpdateOption) (*Event, error) {
	if err := event.Validate(); err != nil {
		return nil, fmt.Errorf("invalid event: %w", err)
	}

	if event.isGenerated() {
		return c.updateGenerated(event, opts...)
	}
	// ------- normal event or normal exception event -------

	originalEvent, exists := c.events[event.Id]
	if !exists {
		return nil, fmt.Errorf("no event found with id '%s'", event.Id)
	}

	oldEnd := originalEvent.getTreeEndTime()
	newEnd := event.getTreeEndTime()

	if originalEvent.From != event.From || oldEnd != newEnd { // update the eventTree
		ids, found := c.eventTree.Find(originalEvent.From, oldEnd)
		if found {
			index := slices.Index(ids, originalEvent.Id)
			if index != -1 {
				updated := slices.Delete(ids, index, index+1)
				if len(updated) == 0 {
					_ = c.eventTree.Delete(originalEvent.From, oldEnd)
				} else {
					_ = c.eventTree.Insert(originalEvent.From, oldEnd, updated)
				}
			}
		}
		err := insertEventIntoTree(c.eventTree, event)
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
		return nil, fmt.Errorf("event with this id doesnt exist")
	}
	return e, nil
}

// Returns an array of events which fall into the specified interval.
func (c *Core) GetEvents(from, to time.Time) []Event {
	// query the interval tree
	intervalsMatched, found := c.eventTree.AllIntersections(from, to)
	if !found {
		return []Event{}
	}

	result := make([]Event, 0, len(intervalsMatched))

	for _, intersection := range intervalsMatched {
		for _, eId := range intersection {
			curEvent := c.events[eId]

			// if it doesn't repeat, just plain append to result
			if curEvent.Repeat == nil {
				result = append(result, *c.events[eId])
				continue
			}

			duration := curEvent.To.Sub(curEvent.From)
			tmpEventTime, index := getFirstCandidate(from, curEvent)

			for tmpEventTime.Before(to) { // while generated event fits in the wanted interval
				if tmpEventTime.Add(duration).Before(from) { // if the generated event ends before our wanted interval -> skip
					tmpEventTime = addUnit(tmpEventTime, 1, curEvent.Repeat.Frequency) // next occurrence
					continue
				}
				// logic when repeating until
				if curEvent.Repeat.Count == -1 && tmpEventTime.After(curEvent.Repeat.Until) {
					break // new event exceeded the repetition end (Until)
				}
				// logic for repeating only N times (count)
				if curEvent.Repeat.Count != -1 && index >= curEvent.Repeat.Count {
					break // new event exceeded the max count of generated events
				}

				index++
				generatedEvent := Event{
					Id:           uuid.New(),
					Title:        curEvent.Title,
					Location:     curEvent.Location,
					Description:  curEvent.Description,
					From:         tmpEventTime,
					OriginalFrom: tmpEventTime,
					To:           tmpEventTime.Add(duration),
					Calendar:     curEvent.Calendar,
					Tag:          curEvent.Tag,
					MasterId:     curEvent.Id,
					Repeat:       curEvent.Repeat, // TODO send the repeat struct
				}
				// ignore exceptions
				if !containsTime(curEvent.Repeat.Exceptions, tmpEventTime) {
					result = append(result, generatedEvent)
				}
				tmpEventTime = addUnit(tmpEventTime, 1, curEvent.Repeat.Frequency) // next occurrence
			}
		}
	}

	return result
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
			Name:  "git-calendar",
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
			Name:  "git-calendar",
			Email: "",
			When:  time.Now(),
		},
	})
	if err != nil {
		return fmt.Errorf("failed to git commit: %w", err)
	}

	return nil
}

func (c *Core) updateGeneratedCurrent(event Event, master *Event) (*Event, error) {
	// ----- update master event with the new exception -----
	exceptionTime := event.OriginalFrom
	if exceptionTime.IsZero() {
		exceptionTime = event.From
	}
	exception := Exception{
		event.Id,
		exceptionTime,
	}
	master.Repeat.Exceptions = append(master.Repeat.Exceptions, exception)

	if err := c.saveAndCommitEvent(master, fmt.Sprintf("CALENDAR: Added exception to master '%s'", master.Title)); err != nil {
		return nil, fmt.Errorf("failed to save master event: %w", err)
	}

	// ----- update master event with the new exception -----
	event.Repeat = nil // detach from repeating time series

	c.events[event.Id] = &event

	if err := insertEventIntoTree(c.eventTree, event); err != nil {
		return nil, fmt.Errorf("failed to insert into index tree: %w", err)
	}

	if err := c.saveAndCommitEvent(&event, fmt.Sprintf("CALENDAR: Saved exception event '%s'", event.Title)); err != nil {
		return nil, err
	}

	return &event, nil
}

// Stops the original event from repeating anymore and creates new repeating event with new updated properties
func (c *Core) updateGeneratedFollowing(event Event, master *Event) (*Event, error) {
	master.Repeat.Until = event.From // cap master at start of change
	master.Repeat.Count = -1         // enforce "Until" logic over "Count"

	if err := c.saveAndCommitEvent(master, fmt.Sprintf("CALENDAR: Capped master event '%s'", master.Title)); err != nil {
		return nil, fmt.Errorf("failed to cap master event: %w", err)
	}

	// make the incoming event new Master event
	event.MasterId = uuid.Nil
	c.events[event.Id] = &event

	if err := insertEventIntoTree(c.eventTree, event); err != nil {
		return nil, fmt.Errorf("failed to insert into index tree: %w", err)
	}
	if err := c.saveAndCommitEvent(&event, fmt.Sprintf("CALENDAR: Created new master event '%s'", event.Title)); err != nil {
		return nil, err
	}
	return &event, nil
}

// Updates all event related to the master
func (c *Core) updateGeneratedAll(event Event, master *Event) (*Event, error) {
	fromChanged := event.From != master.From
	toChanged := event.To != master.To
	repeatChanged := event.Repeat != master.Repeat

	if fromChanged { // shift all exceptions by the time difference
		distance := event.From.Sub(master.From)
		for i := range master.Repeat.Exceptions {
			master.Repeat.Exceptions[i].Time = master.Repeat.Exceptions[i].Time.Add(distance)
		}
	}

	if fromChanged || toChanged || repeatChanged {
		err := moveEventInTree(c.eventTree, master, &event)
		if err != nil {
			return nil, fmt.Errorf("failed to rebuild tree for event: %w", err)
		}
	}
	master.Title = event.Title
	master.Location = event.Location
	master.Description = event.Description
	master.From = event.From
	master.To = event.To
	master.Tag = event.Tag
	master.Repeat = event.Repeat

	if err := c.saveAndCommitEvent(master, fmt.Sprintf("CALENDAR: Updated master event '%s'", master.Title)); err != nil {
		return nil, fmt.Errorf("failed to save event to repo: %w", err)
	}
	return master, nil
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

func (c *Core) removeReal(event Event) error {
	// find last slave and its To
	eventEnd := event.getTreeEndTime()

	// get the full interval
	ids, found := c.eventTree.Find(event.From, eventEnd)
	if !found {
		return fmt.Errorf("event not found in search tree")
	}

	// find index of our event
	index := slices.Index(ids, event.Id)
	if index == -1 {
		return errors.New("")
	}

	// delete event from interval
	updated := slices.Delete(ids, index, index+1)

	if len(updated) == 0 { // interval now empty -> delete from tree
		if err := c.eventTree.Delete(event.From, eventEnd); err != nil {
			return fmt.Errorf("failed to delete tree node: %w", err)
		}
	} else { // not empty -> overwrite
		if err := c.eventTree.Insert(event.From, eventEnd, updated); err != nil {
			return fmt.Errorf("failed to reinsert node into tree: %w", err)
		}
	}

	// delete file from disk + git
	err := c.deleteAndCommitEvent(event.Id, fmt.Sprintf("CALENDAR: Delete event '%s'", event.Title))
	if err != nil {
		return fmt.Errorf("failed to delete event: %w", err)
	}

	delete(c.events, event.Id)
	return nil
}

func (c *Core) removeGenerated(event Event) error {
	masterEvent := c.events[event.MasterId]
	if masterEvent == nil || masterEvent.Repeat == nil {
		return fmt.Errorf("master event not found")
	}

	// if exception doesn't exist yet
	if !containsTime(masterEvent.Repeat.Exceptions, event.From) {
		// add date to master exceptions
		newException := Exception{
			event.Id,
			event.From,
		}
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
