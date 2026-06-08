package core

import (
	"bytes"
	"errors"
	"fmt"
	"net/url"
	"strings"

	"github.com/git-calendar/core/pkg/export"
	"github.com/git-calendar/core/pkg/filesystem"
	"github.com/go-git/go-billy/v5"
	gogit "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/cache"
	gogitfs "github.com/go-git/go-git/v5/storage/filesystem"
	"github.com/google/uuid"
)

// The real API.
//
// Works with raw Go structs, use api.Api to work with JSON.
type Core struct {
	intervalTree *IntervalTree
	events       map[uuid.UUID]*Event
	calendars    map[string]*calendar
	fs           billy.Filesystem // root "/" for OPFS, "$HOME" for classic FS
	proxyUrl     *url.URL         // cors proxy, that works with "url" query param (like https://cors-proxy.abc/?url=https://github.com/...) (only needed for the browser!)
}

// A "constructor" for Core.
func NewCore() *Core {
	var c Core
	c.resetCore()

	// get the fs; go tags handle which one (classic/wasm)
	var err error
	c.fs, err = filesystem.GetFS()
	if err != nil {
		panic(err)
	}

	return &c
}

// Sets a url for CORS proxy. This is only needed inside a browser.
func (c *Core) SetCorsProxy(proxyUrl string) error {
	var err error
	trimmed := strings.TrimSuffix(proxyUrl, "/") // remove trailing "/"
	c.proxyUrl, err = url.ParseRequestURI(trimmed)
	return err
}

// Update all remotes for all repositories.
func (c *Core) PushAll() error {
	var errs error
	for _, cal := range c.calendars {
		remotes, err := cal.repository.Remotes()
		if err != nil {
			errs = errors.Join(err)
		}

		for _, remote := range remotes {
			err = remote.Push(&gogit.PushOptions{})
			if err == gogit.NoErrAlreadyUpToDate {
				continue // this is ok
			}
			if err != nil {
				errs = errors.Join(err)
			}
		}
	}
	return errs
}

func (c *Core) PullAll() error {
	var resultErr error

	for _, cal := range c.calendars {
		fmt.Println("pulling", cal.Name)

		wt, err := cal.repository.Worktree()
		if err != nil || wt == nil {
			continue
		}

		remotes, err := cal.repository.Remotes()
		if err != nil {
			resultErr = errors.Join(resultErr, err)
			continue
		}

		if len(remotes) == 0 {
			resultErr = errors.Join(resultErr, fmt.Errorf("%s: no remotes configured", cal.Name))
			continue
		}

		remoteName := remotes[0].Config().Name

		err = wt.Pull(&gogit.PullOptions{
			RemoteName: remoteName,
		})

		if errors.Is(err, gogit.NoErrAlreadyUpToDate) {
			continue
		}

		if err != nil {
			resultErr = errors.Join(
				resultErr,
				fmt.Errorf("%s: pull from remote %q failed: %w", cal.Name, remoteName, err),
			)
			continue
		}
	}

	return resultErr
}

func (c *Core) ExportZip(calendar string) ([]byte, error) {
	var buf bytes.Buffer
	var fs billy.Filesystem

	if calendar == "" {
		fs = c.fs
	} else {
		cal, ok := c.calendars[calendar]
		if !ok {
			return nil, fmt.Errorf("calendar not found: %s", calendar)
		}

		wt, err := cal.repository.Worktree()
		if err != nil {
			return nil, err
		}

		fs = wt.Filesystem
	}

	if err := export.Zip(fs, &buf); err != nil {
		return nil, err
	}

	return append([]byte(nil), buf.Bytes()...), nil
}

// ------------------------------------------------ Helpers -------------------------------------------------

// Resets the Core internal variables and reallocates them.
func (c *Core) resetCore() {
	c.intervalTree = NewIntervalTree()
	c.events = make(map[uuid.UUID]*Event)
	c.calendars = make(map[string]*calendar)
}

// Loads, if exists, or creates new repository with the given name.
func (c *Core) initCalendarRepo(name string) (*gogit.Repository, error) {
	if err := c.fs.MkdirAll(name, 0o755); err != nil {
		return nil, fmt.Errorf("create repo dir: %w", err)
	}

	repoFS, err := c.fs.Chroot(name)
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
