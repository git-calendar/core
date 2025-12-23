package core

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"time"

	"github.com/firu11/git-calendar-core/filesystem"
	"github.com/go-git/go-billy/v5"
	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/cache"
	"github.com/go-git/go-git/v5/plumbing/object"
	gogitfs "github.com/go-git/go-git/v5/storage/filesystem"
	"github.com/rdleal/intervalst/interval"
)

type (
	// The exposed API interface
	//
	// cannot expose channels, maps or some goofy types which do not have bindings to other languages
	// As even an array does not work, Ive decided to use json everywhere instead of Event, even though you can return a *Event from a function. You cannot pass it as argument, return a array of events or anything else. Using json everywhere as a rest api would...
	Api interface {
		Initialize(repoPath string) error
		Clone(repoUrl, repoPath string) error
		// AddRemote()

		AddEvent(eventJson string) error // TODO: check that it gets translated to a throwing exception for Kotlin/JS
		UpdateEvent(eventJson string) error
		RemoveEvent(eventJson string) error
		GetEvent(id int) (string, error)
		GetEvents(from int64, to int64) (string, error)
	}

	apiImpl struct {
		eventTree *interval.SearchTree[int, int64] // int: id; int64: timestamp end and start
		events    map[int]*Event
		repoPath  string
		repo      *git.Repository
		fs        billy.Filesystem
	}
)

func NewApi() Api {
	var api apiImpl

	api.eventTree = interval.NewSearchTree[int](func(x, y int64) int { return int(x - y) })
	api.events = make(map[int]*Event)

	var err error
	api.fs, err = filesystem.GetRepoFS(RootDirName)
	if err != nil {
		panic(err)
	}

	return &api
}

func (a *apiImpl) AddEvent(eventJson string) error {
	var e Event
	err := json.Unmarshal([]byte(eventJson), &e)

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
	err = a.fs.MkdirAll(dirPath, 0o755) // Ensure the 'events' folder exists
	if err != nil {
		return fmt.Errorf("failed to create events directory: %w", err)
	}

	filename := fmt.Sprintf("%d.json", e.Id)
	filePath := filepath.Join(a.repoPath, EventsDirName, filename)
	file, err := a.fs.Create(filePath)
	if err != nil {
		return fmt.Errorf("failed to create event file: %w", err)
	}
	if _, err := file.Write(data); err != nil {
		return fmt.Errorf("failed to write event file: %w", err)
	}
	file.Close()

	if a.repo == nil {
		return fmt.Errorf("repo not initialized")
	}

	// -------- add to git repo --------
	w, err := a.repo.Worktree()
	if err != nil {
		return fmt.Errorf("failed to get worktree: %w", err)
	}

	gitPath := filepath.ToSlash(filePath)
	if _, err := w.Add(gitPath); err != nil {
		return fmt.Errorf("failed to stage event file: %w", err)
	}

	_, err = w.Commit(
		fmt.Sprintf("CALENDAR: Added event '%s'", e.Title),
		&git.CommitOptions{
			Author: &object.Signature{
				Name:  "git-calendar",
				Email: "",
				When:  time.Now(),
			},
		},
	)
	if err != nil {
		// TODO idk
		w.Remove(gitPath)
		return fmt.Errorf("failed to commit event: %w", err)
	}

	return err
}

func (a *apiImpl) UpdateEvent(eventJson string) error {
	var e Event
	err := json.Unmarshal([]byte(eventJson), &e)
	if err != nil {
		return fmt.Errorf("failed to parse event json: %w", err)
	}

	if err := e.Validate(); err != nil {
		return fmt.Errorf("invalid event data: %w", err)
	}

	// check if it exists
	_, ok := a.events[e.Id]
	if !ok {
		return fmt.Errorf("event with this id doesnt exist")
	}

	// replace the pointer
	a.events[e.Id] = &e

	return nil
}

func (a *apiImpl) RemoveEvent(eventJson string) error {
	return nil
}

func (a *apiImpl) GetEvent(id int) (string, error) {
	e, ok := a.events[id]
	if !ok {
		return "", fmt.Errorf("event with this id doesnt exist")
	}

	jsonBytes, err := json.Marshal(e)
	if err != nil {
		return "", fmt.Errorf("failed to marshal event: %w", err)
	}

	return string(jsonBytes), nil
}

func (a *apiImpl) GetEvents(from int64, to int64) (string, error) {
	return "test test hahhah", nil
}

func (a *apiImpl) Initialize(repoPath string) error {
	a.repoPath = repoPath

	_, err := a.fs.Stat(".git")

	dotGitFs, _ := a.fs.Chroot(".git")
	storage := gogitfs.NewStorage(dotGitFs, cache.NewObjectLRUDefault())

	if err == nil {
		// Repo exists! Open it instead of Init
		repo, err := git.Open(storage, a.fs)
		if err != nil {
			return fmt.Errorf("failed to open existing repo: %w", err)
		}
		a.repo = repo
		return nil
	}

	repo, err := git.Init(storage, a.fs)
	if err != nil {
		return fmt.Errorf("failed to init repo")
	}

	a.repo = repo

	// create the events directory and an initial commit to ensure a master branch exists
	// err = a.setupInitialRepoStructure(repoPath)
	return err
}

func (a *apiImpl) Clone(repoUrl, repoPath string) error {
	// // check if the directory already exists and is non-empty
	// if _, err := os.Stat(repoPath); err == nil {
	// 	// if the directory exists, try to open it instead of cloning over it.
	// 	// if the user meant to re-clone, they should delete the directory first.
	// 	return a.Initialize(repoPath)
	// }

	// repo, err := git.Clone(memory.NewStorage(), a.fs, &git.CloneOptions{
	// 	URL:      repoUrl,
	// 	Progress: os.Stdout, // optional: for logging clone progress
	// })
	// if err != nil {
	// 	return fmt.Errorf("failed to clone repository from '%s': %w", repoUrl, err)
	// }

	// a.repo = repo
	return nil
}

// func (a *apiImpl) setupInitialRepoStructure(repoPath string) error {
// 	err := os.Mkdir(path.Join(repoPath, EventsDirName), 0o755)
// 	if err != nil {
// 		return fmt.Errorf("failed to create folder '%s': %w", path.Join(repoPath, EventsDirName), err)
// 	}

// 	return nil
// }
