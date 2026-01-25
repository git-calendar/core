package core

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"path/filepath"
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
	eventTree *interval.SearchTree[uuid.UUID, time.Time]
	events    map[uuid.UUID]*Event
	repo      *gogit.Repository
	repoPath  string
	fs        billy.Filesystem // "/" for OPFS, "$HOME" for classic FS
	proxyUrl  *url.URL
}

// A "constructor" for CoreApi.
func NewCore() *Core {
	var c Core

	// alloc some vars
	c.eventTree = interval.NewSearchTree[uuid.UUID](func(x, y time.Time) int { return x.Compare(y) })
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
	c.eventTree = interval.NewSearchTree[uuid.UUID](func(x, y time.Time) int { return x.Compare(y) })

	return nil
}

func (c *Core) SetCorsProxy(proxyUrl string) error {
	var err error
	trimmed := strings.TrimSuffix(proxyUrl, "/") // remove trailing "/"
	c.proxyUrl, err = url.Parse(trimmed)
	return err
}

func (c *Core) CreateEvent(event Event) (*Event, error) {
	// TODO
	// just a prototype:

	// add to all events
	c.events[event.Id] = &event

	// -------- insert into tree --------
	err := c.eventTree.Insert(event.From, event.To, event.Id)
	if err != nil {
		return nil, fmt.Errorf("failed to insert into index tree: %w", err)
	}

	// -------- create json file --------
	data, err := json.MarshalIndent(event, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("failed to marshal event to JSON: %w", err)
	}

	dirPath := filepath.Join(c.repoPath, EventsDirName)
	err = c.fs.MkdirAll(dirPath, 0o755) // ensure the "events" folder exists
	if err != nil {
		return nil, fmt.Errorf("failed to create events directory: %w", err)
	}

	filename := fmt.Sprintf("%s.json", event.Id)
	filePath := filepath.Join(c.repoPath, EventsDirName, filename)

	// create a scope for the file operations
	{
		file, err := c.fs.Create(filePath)
		if err != nil {
			return nil, fmt.Errorf("failed to create event file: %w", err)
		}
		if _, err := file.Write(data); err != nil {
			file.Close()
			return nil, fmt.Errorf("failed to write event file: %w", err)
		}
		// close the file BEFORE git operations
		if err := file.Close(); err != nil {
			return nil, fmt.Errorf("failed to close event file: %w", err)
		}
	}

	if c.repo == nil {
		c.fs.Remove(filePath)
		return nil, fmt.Errorf("repo not loaded")
	}

	// -------- add to git repo --------
	w, err := c.repo.Worktree()
	if err != nil {
		c.fs.Remove(filePath)
		return nil, fmt.Errorf("failed to get worktree: %w", err)
	}

	// stage
	gitPath := filepath.ToSlash(filepath.Join(EventsDirName, filename)) // relative to git, not the fs root
	if _, err := w.Add(gitPath); err != nil {
		c.fs.Remove(filePath)
		return nil, fmt.Errorf("failed to stage event file: %w", err)
	}

	// commit
	_, err = w.Commit(
		fmt.Sprintf("CALENDAR: Added event '%s'", event.Title),
		&gogit.CommitOptions{
			Author: &object.Signature{
				Name:  "git-calendar",
				Email: "",
				When:  time.Now(),
			},
		},
	)
	if err != nil {
		if errors.Is(err, gogit.ErrEmptyCommit) {
			// nothing has changed
			return &event, nil
		}

		// TODO idk
		w.Remove(gitPath)
		c.fs.Remove(filePath)
		return nil, fmt.Errorf("failed to commit event: %w", err)
	}

	return &event, nil
}

func (c *Core) UpdateEvent(event Event) (*Event, error) {
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
	// TODO
	return nil
}

func (c *Core) GetEvent(id uuid.UUID) (*Event, error) {
	// TODO

	e, ok := c.events[id]
	if !ok {
		return nil, fmt.Errorf("event with this id doesnt exist")
	}

	return e, nil
}

func (c *Core) GetEvents(from, to time.Time) ([]Event, error) {
	// TODO

	now := time.Now()
	sampleEvents := []Event{
		{
			Id:       uuid.Must(uuid.NewV7()),
			Title:    "Meeting",
			Location: "Google Meet",
			From:     time.Date(now.Year(), now.Month(), now.Day(), 10, 0, 0, 0, now.Location()), // today at 10:00
			To:       time.Date(now.Year(), now.Month(), now.Day(), 13, 0, 0, 0, now.Location()), // today at 13:00
		},
		{
			Id:       uuid.Must(uuid.NewV7()),
			Title:    "Lunch with Joe",
			Location: "Restaurant",
			From:     time.Date(now.Year(), now.Month(), now.Day(), 11, 30, 0, 0, now.Location()), // today at 11:30
			To:       time.Date(now.Year(), now.Month(), now.Day(), 13, 30, 0, 0, now.Location()), // today at 13:30
		},
	}

	for _, v := range c.events {
		sampleEvents = append(sampleEvents, *v)
	}

	return sampleEvents, nil
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
