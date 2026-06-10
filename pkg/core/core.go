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
	"github.com/go-git/go-git/v5/plumbing"
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

func (c *Core) SyncAll() error {
	var resultErr error

	for _, cal := range c.calendars {
		if cal == nil || cal.repository == nil {
			continue // important to check here; syncCalendar and other do assume this
		}

		if err := c.syncCalendar(cal); err != nil {
			resultErr = errors.Join(resultErr, fmt.Errorf("%q: sync failed: %w", cal.Name, err))
		}
	}

	if err := c.LoadCalendars(); err != nil { // reload events from disk
		resultErr = errors.Join(resultErr, err)
	}

	return resultErr
}

// syncCalendar assumes the worktree is clean and all local calendar changes have already been committed.
func (c *Core) syncCalendar(cal *calendar) error {
	if err := fetchCalendar(cal, c.proxyUrl); err != nil {
		if errors.Is(err, gogit.ErrRemoteNotFound) {
			return nil // this is ok
		}
		return err
	}

	localRef, err := localMainRef(cal.repository)
	if err != nil {
		return err
	}

	remoteRef, err := remoteMainRef(cal.repository)
	if err != nil {
		return err
	}

	switch {
	case localRef.Hash() == remoteRef.Hash():
		return nil

	case isAncestor(cal.repository, localRef.Hash(), remoteRef.Hash()):
		// remote is ahead
		return fastForwardCalendar(cal, remoteRef.Hash())

	case isAncestor(cal.repository, remoteRef.Hash(), localRef.Hash()):
		// local is ahead
		return pushCalendar(cal, c.proxyUrl)

	default:
		// cant simply push or pull (history diverged) -> try merge

		fmt.Printf("Diverged history detected on %q, trying to merge...\n", cal.Name)
		if err := customMerge(cal, c.proxyUrl); err != nil {
			return fmt.Errorf("failed to merge: %w", err)
		}
		fmt.Printf("Custom merge successfull for %q\n", cal.Name)

		return pushCalendar(cal, c.proxyUrl)
	}
}

func pushCalendar(cal *calendar, proxyUrl *url.URL) error {
	fmt.Println("pushing", cal.Name)

	repoUrl, err := repoUrlFromCalendar(cal)
	if err != nil {
		return err
	}

	finalUrl, auth := prepareRepoUrl(repoUrl, proxyUrl)
	err = cal.repository.Push(&gogit.PushOptions{
		RemoteName: GitRemoteName,
		RemoteURL:  finalUrl.String(),
		Auth:       auth,
	})
	if err != nil {
		if errors.Is(err, gogit.NoErrAlreadyUpToDate) {
			return nil // this is ok
		}
		return err
	}

	return nil
}

func fetchCalendar(cal *calendar, proxyUrl *url.URL) error {
	fmt.Println("fetching", cal.Name)

	repoUrl, err := repoUrlFromCalendar(cal)
	if err != nil {
		return err
	}

	finalUrl, auth := prepareRepoUrl(repoUrl, proxyUrl)
	err = cal.repository.Fetch(&gogit.FetchOptions{
		RemoteName: GitRemoteName,
		RemoteURL:  finalUrl.String(),
		Auth:       auth,
	})
	if err != nil {
		if errors.Is(err, gogit.NoErrAlreadyUpToDate) {
			return nil // this is ok
		}
		return err
	}

	return nil
}

func fastForwardCalendar(cal *calendar, hash plumbing.Hash) error {
	fmt.Println("fast-forward", cal.Name)

	wt, err := cal.repository.Worktree()
	if err != nil {
		return err
	}

	if err := wt.Checkout(&gogit.CheckoutOptions{
		Branch: plumbing.NewBranchReferenceName(GitBranchName),
	}); err != nil {
		return fmt.Errorf("failed to checkout %s: %w", GitBranchName, err)
	}

	if err := wt.Reset(&gogit.ResetOptions{
		Commit: hash,
		Mode:   gogit.HardReset,
	}); err != nil {
		return fmt.Errorf("failed to fast-forward %s to %s: %w", GitBranchName, hash, err)
	}

	return nil
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
