package core

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"path/filepath"
	"strings"
	"time"

	"github.com/firu11/git-calendar-core/filesystem"
	"github.com/go-git/go-billy/v5"
	gogit "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/cache"
	"github.com/go-git/go-git/v5/plumbing/object"
	gogitfs "github.com/go-git/go-git/v5/storage/filesystem"
	"github.com/rdleal/intervalst/interval"
)

type (
	// The exposed API interface
	//
	// Cannot expose channels, maps or some goofy types which do not have bindings to other languages...
	// As even an array does not work, Ive decided to use json everywhere instead of Event, even though you can return a *Event from a function. You cannot pass it as argument, return a array of events or anything else. Let's use JSON everywhere as a REST API would...
	Api interface {
		Initialize() error
		Clone(repoUrl string) error
		// AddRemote()
		// Delete()

		SetCorsProxy(proxyUrl string) error

		AddEvent(eventJson string) error
		UpdateEvent(eventJson string) error
		RemoveEvent(eventJson string) error
		GetEvent(id int) (string, error)
		GetEvents(from int64, to int64) (string, error)
	}

	// Private implementation of Api
	apiImpl struct {
		eventTree *interval.SearchTree[int, int64] // int: id; int64: timestamp end and start
		events    map[int]*Event
		repo      *gogit.Repository
		repoPath  string
		fs        billy.Filesystem
		proxyUrl  *url.URL
	}
)

func NewApi() Api {
	var api apiImpl

	// alloc some vars
	api.eventTree = interval.NewSearchTree[int](func(x, y int64) int { return int(x - y) })
	api.events = make(map[int]*Event)

	// get the fs, go tags handle which one (classic/wasm)
	var err error
	api.fs, api.repoPath, err = filesystem.GetRepoFS()
	if err != nil {
		panic(err)
	}

	return &api
}

func (a *apiImpl) AddEvent(eventJson string) error {
	// TODO
	// just a prototype:
	var e Event
	err := json.Unmarshal([]byte(eventJson), &e)
	if err != nil {
		return fmt.Errorf("failed to parse event data: %w", err)
	}

	if err := e.Validate(); err != nil {
		return fmt.Errorf("invalid event data: %w", err)
	}

	// add to all events
	a.events[e.Id] = &e

	// -------- insert into tree --------
	err = a.eventTree.Insert(e.From, e.To, e.Id)
	if err != nil {
		return fmt.Errorf("failed to insert into index tree: %w", err)
	}

	// -------- create json file --------
	data, err := json.MarshalIndent(e, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal event to JSON: %w", err)
	}

	dirPath := filepath.Join(a.repoPath, EventsDirName)
	err = a.fs.MkdirAll(dirPath, 0o755) // ensure the "events" folder exists
	if err != nil {
		return fmt.Errorf("failed to create events directory: %w", err)
	}

	filename := fmt.Sprintf("%d.json", e.Id)
	filePath := filepath.Join(a.repoPath, EventsDirName, filename)

	// create a scope for the file operations
	{
		file, err := a.fs.Create(filePath)
		if err != nil {
			return fmt.Errorf("failed to create event file: %w", err)
		}
		if _, err := file.Write(data); err != nil {
			file.Close()
			return fmt.Errorf("failed to write event file: %w", err)
		}
		// close the file BEFORE git operations
		if err := file.Close(); err != nil {
			return fmt.Errorf("failed to close event file: %w", err)
		}
	}

	if a.repo == nil {
		return fmt.Errorf("repo not initialized")
	}

	// -------- add to git repo --------
	w, err := a.repo.Worktree()
	if err != nil {
		return fmt.Errorf("failed to get worktree: %w", err)
	}

	// stage
	gitPath := filepath.ToSlash(filepath.Join(EventsDirName, filename)) // relative to git, not the fs root
	if _, err := w.Add(gitPath); err != nil {
		return fmt.Errorf("failed to stage event file: %w", err)
	}

	// commit
	_, err = w.Commit(
		fmt.Sprintf("CALENDAR: Added event '%s'", e.Title),
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
			return nil
		}

		// TODO idk
		w.Remove(gitPath)
		return fmt.Errorf("failed to commit event: %w", err)
	}

	return err
}

func (a *apiImpl) UpdateEvent(eventJson string) error {
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

	return nil
}

func (a *apiImpl) RemoveEvent(eventJson string) error {
	// TODO
	return nil
}

func (a *apiImpl) GetEvent(id int) (string, error) {
	// TODO

	// e, ok := a.events[id]
	// if !ok {
	// 	return "", fmt.Errorf("event with this id doesnt exist")
	// }

	// jsonBytes, err := json.Marshal(e)
	// if err != nil {
	// 	return "", fmt.Errorf("failed to marshal event: %w", err)
	// }

	return "", nil
}

func (a *apiImpl) GetEvents(from int64, to int64) (string, error) {
	// TODO
	return "", nil
}

func (a *apiImpl) Initialize() error {
	// a.fs is OPFS root
	if err := a.fs.MkdirAll(a.repoPath, 0o755); err != nil {
		return fmt.Errorf("create repo dir: %w", err)
	}

	repoFS, err := a.fs.Chroot(a.repoPath)
	if err != nil {
		return fmt.Errorf("chroot repo dir: %w", err)
	}

	if err := repoFS.MkdirAll(".git", 0o755); err != nil {
		return fmt.Errorf("create .git: %w", err)
	}

	dotGitFS, err := repoFS.Chroot(".git")
	if err != nil {
		return fmt.Errorf("chroot .git: %w", err)
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

	a.repo = repo

	return a.setupInitialRepoStructure()
}

func (a *apiImpl) SetCorsProxy(proxyUrl string) error {
	var err error
	trimmed := strings.TrimSuffix(proxyUrl, "/") // remove trailing "/"
	a.proxyUrl, err = url.Parse(trimmed)
	return err
}

func (a *apiImpl) Clone(repoUrl string) error {
	// make sure that the repo dir is created
	if err := a.fs.MkdirAll(a.repoPath, 0o755); err != nil {
		return fmt.Errorf("create repo dir: %w", err)
	}
	repoFS, err := a.fs.Chroot(a.repoPath)
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
	if a.proxyUrl != nil {
		final := *a.proxyUrl          // copy
		q := final.Query()            // get parsed query (a copy)
		q.Set("url", repoUrl)         // add the param
		final.RawQuery = q.Encode()   // put it back
		finalRepoUrl = final.String() // get the final string
	} else {
		finalRepoUrl = repoUrl
	}
	a.repo, err = gogit.Clone(storage, repoFS, &gogit.CloneOptions{
		RemoteName: "github",
		URL:        finalRepoUrl,
	})
	if err != nil {
		return fmt.Errorf("clone repo: %w", err)
	}

	return err
}

// helper function to setup the inital "events" folder etc.
func (a *apiImpl) setupInitialRepoStructure() error {
	// TODO

	// eventsDirPath := path.Join(a.repoPath, EventsDirName)
	// err := a.fs.MkdirAll(eventsDirPath, 0o755)
	// if err != nil {
	// 	return fmt.Errorf("failed to create folder '%s': %w", eventsDirPath, err)
	// }

	return nil
}
