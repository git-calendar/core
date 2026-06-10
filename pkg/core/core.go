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

func (c *Core) PushAll() error {
	var resultErr error

	for _, cal := range c.calendars {
		if cal == nil || cal.repository == nil {
			continue
		}

		fmt.Println("pushing", cal.Name)

		repoUrl, err := repoUrlFromCalendar(cal)
		if err != nil {
			if errors.Is(err, gogit.ErrRemoteNotFound) {
				continue // this is ok
			}
			resultErr = errors.Join(resultErr, err)
			continue
		}

		finalUrl, auth := prepareRepoUrl(repoUrl, c.proxyUrl)
		err = cal.repository.Push(&gogit.PushOptions{
			RemoteName: GitRemoteName,
			RemoteURL:  finalUrl.String(),
			Auth:       auth,
		})
		if err != nil {
			if errors.Is(err, gogit.NoErrAlreadyUpToDate) {
				continue // this is ok
			}
			resultErr = errors.Join(resultErr, err)
		}
	}

	return resultErr
}

func (c *Core) PullAll() error {
	var resultErr error
	var needPushAfter bool

	for _, cal := range c.calendars {
		if cal == nil || cal.repository == nil {
			continue
		}

		fmt.Println("pulling", cal.Name)

		wt, err := cal.repository.Worktree()
		if err != nil {
			resultErr = errors.Join(resultErr, fmt.Errorf("%q: get worktree: %w", cal.Name, err))
			continue
		}
		if wt == nil {
			continue
		}

		repoUrl, err := repoUrlFromCalendar(cal)
		if err != nil {
			if errors.Is(err, gogit.ErrRemoteNotFound) {
				continue // this is ok
			}
			resultErr = errors.Join(resultErr, err)
			continue
		}

		finalUrl, auth := prepareRepoUrl(repoUrl, c.proxyUrl)
		err = wt.Pull(&gogit.PullOptions{
			RemoteName: GitRemoteName,
			RemoteURL:  finalUrl.String(),
			Auth:       auth,
		})
		if err != nil {
			if errors.Is(err, gogit.NoErrAlreadyUpToDate) {
				continue // good
			}

			// histories have diverged -> merge needed
			if errors.Is(err, gogit.ErrNonFastForwardUpdate) {
				fmt.Printf("Diverged history detected on %q, trying to merge...\n", cal.Name)

				err := customMergeRemote(cal, c.proxyUrl)
				if err != nil {
					resultErr = errors.Join(resultErr, fmt.Errorf("%q: custom merge failed: %w", cal.Name, err))
					continue
				}
				fmt.Printf("Custom merge successfull\n", cal.Name)
				needPushAfter = true

				continue
			}

			// some other error happened
			resultErr = errors.Join(resultErr, fmt.Errorf("%q: pull from remote failed: %w", cal.Name, err))
			continue
		}
	}

	if needPushAfter {
		if err := c.PushAll(); err != nil {
			resultErr = errors.Join(resultErr, fmt.Errorf("push to remotes failed: %w", err))
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
