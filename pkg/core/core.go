package core

import (
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/firu11/git-calendar-core/pkg/filesystem"
	"github.com/go-git/go-billy/v5"
	gogit "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/cache"
	gogitfs "github.com/go-git/go-git/v5/storage/filesystem"
	"github.com/google/uuid"
	"github.com/rdleal/intervalst/interval"
)

// The real API.
//
// Works with raw Go structs, use api.Api to work with JSON.
type Core struct {
	tree     EventTree
	events   map[uuid.UUID]*Event
	repos    map[string]*gogit.Repository
	fs       billy.Filesystem // root "/" for OPFS, "$HOME" for classic FS
	proxyUrl *url.URL         // cors proxy, that works with "url" query param (like https://cors-proxy.abc/?url=https://github.com/...) (only needed for the browser!)
	// tags      map[string][]string // might not be needed to "cache" it like this
}

// A "constructor" for Core.
func NewCore() *Core {
	var c Core
	c.eraseAndAlloc()

	// get the fs; go tags handle which one (classic/wasm)
	var err error
	c.fs, err = filesystem.GetFS()
	if err != nil {
		panic(err)
	}

	err = c.fs.MkdirAll(filesystem.DirName, 0o755)
	if err != nil {
		panic(err)
	}

	return &c
}

// Sets a url for CORS proxy. This is only needed inside a browser.
func (c *Core) SetCorsProxy(proxyUrl string) error {
	var err error
	trimmed := strings.TrimSuffix(proxyUrl, "/") // remove trailing "/"
	c.proxyUrl, err = url.Parse(trimmed)
	return err
}

// Update all repotes for all repositories.
func (c *Core) PushAll() error {
	// TODO idk if it works

	var err error
	for _, repo := range c.repos {
		errx := repo.Push(&gogit.PushOptions{})
		if errx == gogit.NoErrAlreadyUpToDate {
			continue // this is ok
		}
		if errx != nil {
			err = errors.Join(errx)
		}
	}
	return err
}

// Update all repositories from remotes.
func (c *Core) PullAll() error {
	// TODO idk if it works

	var err error
	for _, repo := range c.repos {
		wt, errx := repo.Worktree()
		if errx != nil || wt == nil { // only fails if repo is bare (aka. only .git/ folder exists, no files) which should not happen ever haha
			continue
		}

		errx = wt.Pull(&gogit.PullOptions{})
		if errx == gogit.NoErrAlreadyUpToDate {
			continue // this is ok
		}
		if errx != nil {
			err = errors.Join(errx)
		}
	}
	return err
}

// ------------------------------------------------ Helpers -------------------------------------------------

// Resets the Core internal variables and reallocates them.
func (c *Core) eraseAndAlloc() {
	c.tree = interval.NewSearchTree[[]uuid.UUID](
		func(x, y time.Time) int {
			return x.Compare(y)
		},
	)
	c.events = make(map[uuid.UUID]*Event)
	c.repos = make(map[string]*gogit.Repository)
}

// Loads, if exists, or creates new repository with the given name.
func (c *Core) initCalendarRepo(name string) (*gogit.Repository, error) {
	repoPath := c.fs.Join(filesystem.DirName, name)

	if err := c.fs.MkdirAll(repoPath, 0o755); err != nil {
		return nil, fmt.Errorf("create repo dir: %w", err)
	}

	repoFS, err := c.fs.Chroot(repoPath)
	if err != nil {
		return nil, fmt.Errorf("chroot repo dir: %w", err)
	}

	if err := repoFS.MkdirAll(".git", 0o755); err != nil {
		return nil, fmt.Errorf("create .git dir: %w", err)
	}

	dotGitFS, err := repoFS.Chroot(".git")
	if err != nil {
		return nil, fmt.Errorf("chroot .git dir: %w", err)
	}

	storage := gogitfs.NewStorage(dotGitFS, cache.NewObjectLRUDefault())

	repo, err := gogit.Init(storage, repoFS)
	if errors.Is(err, gogit.ErrRepositoryAlreadyExists) {
		repo, err = gogit.Open(storage, repoFS)
		if err != nil {
			return nil, err
		}
	} else if err != nil {
		return nil, err
	}

	return repo, nil
}
