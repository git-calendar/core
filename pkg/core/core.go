package core

import (
	"bytes"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/git-calendar/core/pkg/export"
	"github.com/git-calendar/core/pkg/filesystem"
	"github.com/git-calendar/core/pkg/gitmerge"
	"github.com/go-git/go-billy/v5"
	gogit "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/cache"
	"github.com/go-git/go-git/v5/plumbing/object"
	gogitfs "github.com/go-git/go-git/v5/storage/filesystem"
	"github.com/google/uuid"
)

// The real API.
//
// Works with raw Go structs, use api.Api to work with JSON.
type Core struct {
	intervalTree *IntervalTree
	events       map[uuid.UUID]*Event
	calendars    map[string]*Calendar
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
	if proxyUrl == "" {
		c.proxyUrl = nil
		return nil
	}

	var err error
	trimmed := strings.TrimSuffix(proxyUrl, "/") // remove trailing "/"
	c.proxyUrl, err = url.ParseRequestURI(trimmed)
	return err
}

// SyncAll tries to synchronize all calendars with its remotes.
func (c *Core) SyncAll() error {
	var wg sync.WaitGroup
	errs := make(chan error, len(c.calendars))

	for _, cal := range c.calendars {
		if cal == nil || cal.repository == nil {
			continue // important to check here; syncCalendar and other do assume this
		}

		wg.Add(1)
		go func() {
			if err := c.syncCalendar(cal); err != nil {
				errs <- fmt.Errorf("%q: sync failed: %w", cal.Name, err)
			}
			wg.Done()
		}()
	}

	wg.Wait() // wait for all calendar syncs to finish
	close(errs)

	var resultErr error
	for err := range errs { // collect all errors
		resultErr = errors.Join(resultErr, err)
	}

	if err := c.LoadCalendars(); err != nil { // reload events from disk
		resultErr = errors.Join(resultErr, err)
	}

	return resultErr
}

// syncCalendar assumes the worktree is clean and all local calendar changes have already been committed.
func (c *Core) syncCalendar(cal *Calendar) error {
	if err := fetchCalendar(cal, c.proxyUrl); err != nil {
		if errors.Is(err, gogit.ErrRemoteNotFound) {
			return nil // this is ok
		}
		return err
	}

	localCommit, remoteCommit, err := gitmerge.GetCommits(cal.repository, GitBranchName, GitRemoteName)
	if err != nil {
		return err
	}

	switch {
	case localCommit.Hash == remoteCommit.Hash:
		return nil // already in sync

	case isAncestor(localCommit, remoteCommit):
		// remote is ahead
		return fastForwardCalendar(cal, remoteCommit.Hash)

	case isAncestor(remoteCommit, localCommit):
		// local is ahead
		return pushCalendar(cal, c.proxyUrl)

	default:
		// cant simply push or pull (history diverged) -> try merge

		fmt.Printf("Diverged history detected on %q, trying to merge...\n", cal.Name)
		if err := mergeOriginMain(cal.repository, cal.Name, cal.EncryptionKey); err != nil {
			return fmt.Errorf("failed to merge: %w", err)
		}
		fmt.Printf("Custom merge successfull for %q\n", cal.Name)

		return pushCalendar(cal, c.proxyUrl)
	}
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
	c.calendars = make(map[string]*Calendar)
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

	repo, err := gogit.InitWithOptions(storage, repoFS, gogit.InitOptions{
		DefaultBranch: plumbing.NewBranchReferenceName(GitBranchName),
	})
	if errors.Is(err, gogit.ErrRepositoryAlreadyExists) {
		repo, err = gogit.Open(storage, repoFS)
		if err != nil {
			return nil, err
		}
		return repo, nil
	} else if err != nil {
		return nil, err
	}

	if err := firstCommit(repo); err != nil {
		return nil, fmt.Errorf("initial commit failed: %w", err)
	}
	return repo, nil
}

func firstCommit(repo *gogit.Repository) error {
	wt, err := repo.Worktree()
	if err != nil {
		return err
	}

	_, err = wt.Commit("Initial calendar empty commit", &gogit.CommitOptions{
		AllowEmptyCommits: true,
		Author: &object.Signature{
			Name: GitAuthorName,
			When: time.Now(),
		},
	})
	return err
}
