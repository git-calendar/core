package core

import (
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/google/uuid"
	"github.com/rdleal/intervalst/interval"
)

type IntervalTree struct {
	tree interval.SearchTree[[]uuid.UUID, time.Time] // for each interval we can have multiple events; <time.Time, time.Time> -> []uuid.UUID
}

func NewIntervalTree() *IntervalTree {
	return &IntervalTree{
		tree: *interval.NewSearchTree[[]uuid.UUID](
			func(x, y time.Time) int {
				return x.Compare(y)
			},
		),
	}
}

// Inserts an Event to its interval in the tree.
func (et *IntervalTree) InsertEvent(event Event) error {
	eventEnd := event.getTreeEndTime()
	ids, _ := et.tree.Find(event.From, eventEnd) // find existing interval
	updated := append(ids, event.Id)             // if not found, ids is nil -> append makes [event.Id]

	err := et.tree.Insert(event.From, eventEnd, updated)
	return err
}

// Deletes an Event from the interval tree.
func (et *IntervalTree) RemoveEvent(event Event) error {
	// find last slave and its To
	eventEnd := event.getTreeEndTime()

	// get the full interval
	ids, found := et.tree.Find(event.From, eventEnd)
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
		if err := et.tree.Delete(event.From, eventEnd); err != nil {
			return fmt.Errorf("failed to delete tree node: %w", err)
		}
	} else { // not empty -> replace
		if err := et.tree.Insert(event.From, eventEnd, updated); err != nil {
			return fmt.Errorf("failed to reinsert node into tree: %w", err)
		}
	}

	return nil
}
