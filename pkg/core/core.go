package core

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/firu11/git-calendar-core/pkg/filesystem"
	"github.com/go-git/go-billy/v5"
	gogitutil "github.com/go-git/go-billy/v5/util"
	gogit "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing/cache"
	"github.com/go-git/go-git/v5/plumbing/object"
	gogitfs "github.com/go-git/go-git/v5/storage/filesystem"
	"github.com/google/uuid"
	"github.com/rdleal/intervalst/interval"
)

// The real API.
//
// Works with raw Go structs, use api.Api to work with JSON.
type Core struct {
	eventTree *interval.SearchTree[[]uuid.UUID, time.Time] // for each interval we can have multiple events ([time.Time, time.Time] -> []uuid.UUID)
	events    map[uuid.UUID]*Event
	repo      *gogit.Repository
	repoPath  string
	fs        billy.Filesystem // "/" for OPFS, "$HOME" for classic FS
	proxyUrl  *url.URL
}

// A "constructor" for Core.
func NewCore() *Core {
	var c Core

	// alloc some vars
	c.eventTree = interval.NewSearchTree[[]uuid.UUID](
		func(x, y time.Time) int {
			return x.Compare(y)
		},
	)
	c.events = make(map[uuid.UUID]*Event)

	// get the fs; go tags handle which one (classic/wasm)
	var err error
	c.fs, c.repoPath, err = filesystem.GetRepoFS()
	if err != nil {
		panic(err)
	}

	return &c
}

func (c *Core) Initialize() error {
	if c.repo != nil {
		return nil // already initialized
	}

	if err := c.fs.MkdirAll(c.repoPath, 0o755); err != nil {
		return fmt.Errorf("create repo dir: %w", err)
	}

	repoFS, err := c.fs.Chroot(c.repoPath)
	if err != nil {
		return fmt.Errorf("chroot repo dir: %w", err)
	}

	if err := repoFS.MkdirAll(".git", 0o755); err != nil {
		return fmt.Errorf("create .git dir: %w", err)
	}

	dotGitFS, err := repoFS.Chroot(".git")
	if err != nil {
		return fmt.Errorf("chroot .git dir: %w", err)
	}

	storage := gogitfs.NewStorage(dotGitFS, cache.NewObjectLRUDefault())

	repo, err := gogit.Init(storage, repoFS)
	if errors.Is(err, gogit.ErrRepositoryAlreadyExists) {
		repo, err = gogit.Open(storage, repoFS)
		if err != nil {
			return err
		}
	} else if err != nil {
		return err
	}

	c.repo = repo

	return c.setupInitialRepoStructure()
}

func (c *Core) Clone(repoUrl string) error {
	if c.repo != nil {
		return errors.New("repo already exists")
	}

	// make sure that the repo dir is created
	if err := c.fs.MkdirAll(c.repoPath, 0o755); err != nil {
		return fmt.Errorf("create repo dir: %w", err)
	}
	repoFS, err := c.fs.Chroot(c.repoPath)
	if err != nil {
		return fmt.Errorf("chroot repo dir: %w", err)
	}

	// make sure that .git dir exists
	if err := repoFS.MkdirAll(".git", 0o755); err != nil {
		return fmt.Errorf("create .git: %w", err)
	}
	dotGitFS, err := repoFS.Chroot(".git")
	if err != nil {
		return fmt.Errorf("chroot .git: %w", err)
	}

	storage := gogitfs.NewStorage(dotGitFS, cache.NewObjectLRUDefault())

	// add proxy if specified (only needed for the browser)
	// like so: http://cors-proxy.abc/?url=https://github.com/firu11/git-calendar-core
	var finalRepoUrl string
	if c.proxyUrl != nil {
		final := *c.proxyUrl          // copy
		q := final.Query()            // get parsed query (a copy)
		q.Set("url", repoUrl)         // add the param
		final.RawQuery = q.Encode()   // put it back
		finalRepoUrl = final.String() // get the final string
	} else {
		finalRepoUrl = repoUrl
	}

	// clone now
	c.repo, err = gogit.Clone(storage, repoFS, &gogit.CloneOptions{
		RemoteName: "github",
		URL:        finalRepoUrl,
	})
	if err != nil {
		return fmt.Errorf("git clone failed: %w", err)
	}

	return err
}

func (c *Core) AddRemote(name, remoteUrl string) error {
	var validUrl string
	{
		// validate URL (git doesnt do that when adding a remote, it fails afterwards with e.g. git fetch)
		u := strings.TrimSuffix(remoteUrl, "/") // remove trailing "/"
		if !strings.HasSuffix(u, ".git") {
			return errors.New("remote url doesn't end with '.git'")
		}
		parsedUrl, err := url.Parse(u)
		if err != nil {
			return fmt.Errorf("cannot parse remote url: %w", err)
		}
		validUrl = parsedUrl.String()
	}

	_, err := c.repo.CreateRemote(&config.RemoteConfig{
		Name: name,
		URLs: []string{validUrl},
	})
	if err != nil {
		return fmt.Errorf("failed to create a remote: %w", err)
	}

	return nil
}

func (c *Core) Delete() error {
	c.repo = nil
	err := gogitutil.RemoveAll(c.fs, c.repoPath)
	if err != nil {
		return fmt.Errorf("failed to remove repo directory: %w", err)
	}

	c.events = make(map[uuid.UUID]*Event)
	c.eventTree = interval.NewSearchTree[[]uuid.UUID](
		func(x, y time.Time) int {
			return x.Compare(y)
		},
	)

	return nil
}

func (c *Core) SetCorsProxy(proxyUrl string) error {
	var err error
	trimmed := strings.TrimSuffix(proxyUrl, "/") // remove trailing "/"
	c.proxyUrl, err = url.Parse(trimmed)
	return err
}

func (c *Core) CreateEvent(event Event) (*Event, error) {
	if err := event.Validate(); err != nil {
		return nil, fmt.Errorf("invalid event: %w", err)
	}
	// add to all events
	c.events[event.Id] = &event

	// -------- insert into tree --------

	eventEnd := event.To
	if event.Repeat != nil {
		eventEnd = event.Repeat.Until // if repeating, insert interval [From, Repetition.Until]
		if event.Repeat.Count >= 1 /* if repeating on count basis */ {
			eventEnd = addUnit(event.To, event.Repeat.Interval*event.Repeat.Count, event.Repeat.Frequency)
		}
	}
	ids, _ := c.eventTree.Find(event.From, eventEnd) // find existing interval
	updated := append(ids, event.Id)                 // if not found, ids is nil -> append makes [event.Id]

	err := c.eventTree.Insert(event.From, eventEnd, updated)
	if err != nil {
		return nil, fmt.Errorf("failed to insert into index tree: %w", err)
	}

	// -------- create json file and add to git --------
	err = c.saveEventToRepo(&event, fmt.Sprintf("CALENDAR: Added event '%s'", event.Title))
	if err != nil {
		return nil, fmt.Errorf("failed to save event to repo: %w", err)
	}
	return &event, nil
}

func (c *Core) UpdateEvent(event Event) (*Event, error) {
	if err := event.Validate(); err != nil {
		return nil, fmt.Errorf("invalid event: %w", err)
	}
	// TODO

	// var e Event
	// err := json.Unmarshal([]byte(eventJson), &e)
	// if err != nil {
	// 	return fmt.Errorf("failed to parse event json: %w", err)
	// }

	// if err := e.Validate(); err != nil {
	// 	return fmt.Errorf("invalid event data: %w", err)
	// }

	// // check if it exists
	// _, ok := a.events[e.Id]
	// if !ok {
	// 	return fmt.Errorf("event with this id doesnt exist")
	// }

	// // replace the pointer
	// a.events[e.Id] = &e

	return nil, nil
}

func (c *Core) RemoveEvent(event Event) error {
	if err := event.Validate(); err != nil {
		return fmt.Errorf("invalid event: %w", err)
	}

	// real event, must be deleted entirely
	if event.MasterId == uuid.Nil {
		delete(c.events, event.Id)

		// find last slave and its To
		eventEnd := event.To
		if event.Repeat != nil {
			eventEnd = event.Repeat.Until
			if event.Repeat.Count >= 1 {
				eventEnd = addUnit(event.To, event.Repeat.Interval*event.Repeat.Count, event.Repeat.Frequency)
			}
		}

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
		err := c.deleteEventFromRepo(event.Id, fmt.Sprintf("CALENDAR: Delete event '%s'", event.Title))
		if err != nil {
			return fmt.Errorf("failed to delete event: %w", err)
		}

		return nil
	}

	// generated repeating event, must be added to repeat exceptions
	if event.Repeat == nil && event.MasterId != uuid.Nil {
		delete(c.events, event.Id)

		// get master event
		masterEvent := c.events[event.MasterId]
		if masterEvent == nil || masterEvent.Repeat == nil {
			return fmt.Errorf("master event not found")
		}

		// add date to exceptions
		if !slices.Contains(masterEvent.Repeat.Exceptions, event.From) {
			masterEvent.Repeat.Exceptions = append(masterEvent.Repeat.Exceptions, event.From)
			err := c.saveEventToRepo(masterEvent, fmt.Sprintf("CALENDAR: Updated event '%s'", event.Title))
			if err != nil {
				return fmt.Errorf("failed to save event to repo: %w", err)
			}
		}

		return nil
	}

	return errors.New("something went wrong, event was not removed")
}

func (c *Core) GetEvent(id uuid.UUID) (*Event, error) {
	e, ok := c.events[id]
	if !ok {
		return nil, fmt.Errorf("event with this id doesnt exist")
	}
	return e, nil
}

func (c *Core) GetEvents(from, to time.Time) ([]Event, error) {
	// query the interval tree
	intervalsMatched, found := c.eventTree.AllIntersections(from, to)
	if !found {
		return []Event{}, nil
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
					Id:          uuid.New(),
					Title:       curEvent.Title,
					Location:    curEvent.Location,
					Description: curEvent.Description,
					From:        tmpEventTime,
					To:          tmpEventTime.Add(duration),
					MasterId:    curEvent.Id,
					Repeat:      nil,
				}
				result = append(result, generatedEvent)

				tmpEventTime = addUnit(tmpEventTime, 1, curEvent.Repeat.Frequency) // next occurance
			}
		}
	}

	return result, nil
}

// ------------------------------------------------ Helpers -------------------------------------------------

// Helper function to setup the initial "events" folder etc.
func (c *Core) setupInitialRepoStructure() error {
	// TODO

	// eventsDirPath := path.Join(a.repoPath, EventsDirName)
	// err := a.fs.MkdirAll(eventsDirPath, 0o755)
	// if err != nil {
	// 	return fmt.Errorf("failed to create folder '%s': %w", eventsDirPath, err)
	// }

	return nil
}

// TODO remove
func sampleEvents() []Event {
	now := time.Now()
	return []Event{
		{
			Id:       uuid.New(),
			Title:    "Meeting",
			Location: "Google Meet",
			From:     time.Date(now.Year(), now.Month(), now.Day(), 10, 0, 0, 0, now.Location()), // today at 10:00
			To:       time.Date(now.Year(), now.Month(), now.Day(), 13, 0, 0, 0, now.Location()), // today at 13:00
		},
		{
			Id:       uuid.New(),
			Title:    "Lunch with Joe",
			Location: "Restaurant",
			From:     time.Date(now.Year(), now.Month(), now.Day(), 11, 30, 0, 0, now.Location()), // today at 11:30
			To:       time.Date(now.Year(), now.Month(), now.Day(), 13, 30, 0, 0, now.Location()), // today at 13:30
		},
	}
}

func (c *Core) saveEventToRepo(event *Event, commitMsg string) error {
	// marshal event
	data, err := json.MarshalIndent(event, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal event: %w", err)
	}

	// write JSON content
	filename := fmt.Sprintf("%s.json", event.Id)

	// ensure directory exists
	dirPath := filepath.Join(c.repoPath, EventsDirName)
	if err := c.fs.MkdirAll(dirPath, 0o755); err != nil {
		return fmt.Errorf("failed mkdir events: %w", err)
	}

	filePath := filepath.Join(dirPath, filename)

	// create truncates/overwrites the file if it exists
	file, err := c.fs.Create(filePath)
	if err != nil {
		return fmt.Errorf("failed to create file: %w", err)
	}

	if _, err := file.Write(data); err != nil {
		_ = file.Close()
		return fmt.Errorf("failed to write file: %w", err)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("failed to close file: %w", err)
	}

	if c.repo == nil {
		return fmt.Errorf("repo not initialized")
	}

	// -------- add to git repo --------
	w, err := c.repo.Worktree()
	if err != nil {
		return fmt.Errorf("failed to get worktree: %w", err)
	}

	// stage
	gitPath := filepath.ToSlash(filepath.Join(EventsDirName, filename))
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

func (c *Core) deleteEventFromRepo(eventId uuid.UUID, commitMsg string) error {
	filename := fmt.Sprintf("%s.json", eventId)

	// -------- remove from filesystem --------
	dirPath := filepath.Join(c.repoPath, EventsDirName)
	filePath := filepath.Join(dirPath, filename)

	if err := c.fs.Remove(filePath); err != nil {
		// TODO maybe continue, to clean the git from this file
		return fmt.Errorf("failed to remove file from disk: %w", err)
	}

	// -------- remove from git --------
	if c.repo == nil {
		return fmt.Errorf("repo not initialized")
	}

	w, err := c.repo.Worktree()
	if err != nil {
		return fmt.Errorf("failed to get worktree: %w", err)
	}

	gitPath := filepath.ToSlash(filepath.Join(EventsDirName, filename))

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
