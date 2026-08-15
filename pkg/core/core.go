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
	gogitutil "github.com/go-git/go-billy/v5/util"
	gogit "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/cache"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/go-git/go-git/v5/plumbing/transport"
	gogitfs "github.com/go-git/go-git/v5/storage/filesystem"
	"github.com/google/uuid"
)

// Core manages calendars, events, tags, synchronization, and persistence using Go values.
// Works with raw Go structs, use api.Api to work with JSON.
type Core struct {
	intervalTree *IntervalTree
	events       map[uuid.UUID]*Event
	calendars    map[string]*Calendar
	fs           billy.Filesystem // root "/" for OPFS/IDB, "$HOME" for classic FS
	proxyURL     *url.URL         // cors proxy, that works like https://cors-proxy.abc/https://github.com/... (only needed for the browser!)
}

// NewCore creates an initialized Core using the platform filesystem.
func NewCore() *Core {
	var c Core
	c.resetCore()

	// Build tags select the classic or WebAssembly filesystem implementation.
	var err error
	c.fs, err = filesystem.GetFS()
	if err != nil {
		panic(err)
	}

	return &c
}

// SetCorsProxy configures the CORS proxy used by browser transports.
func (c *Core) SetCorsProxy(proxyURL string) error {
	if proxyURL == "" {
		c.proxyURL = nil
		return nil
	}

	var err error
	trimmed := strings.TrimSuffix(proxyURL, "/") // remove trailing "/"
	c.proxyURL, err = url.ParseRequestURI(trimmed)
	return err
}

// SyncAll tries to synchronize all calendars with its remotes.
func (c *Core) SyncAll() error {
	var wg sync.WaitGroup
	errs := make(chan error, len(c.calendars))

	for _, cal := range c.calendars {
		if cal == nil {
			continue // important to check here; syncCalendar and other do assume this
		}

		wg.Go(func() {
			var err error
			if cal.ICalURL != nil {
				err = c.fetchICalURL(cal.Name, cal.ICalURL)
			} else if cal.repository != nil {
				err = c.syncCalendar(cal)
			}
			if err != nil {
				errs <- fmt.Errorf("%q: sync failed: %w", cal.Name, err)
			}
		})
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
	if err := fetchCalendar(cal, c.proxyURL); err != nil {
		switch {
		case errors.Is(err, gogit.ErrRemoteNotFound):
			return nil // this is ok

		case errors.Is(err, transport.ErrEmptyRemoteRepository):
			// remote exists, but has no commits yet
			// if we have local commits -> initialize by pushing
			localCommit, err := gitmerge.GetLocalCommit(cal.repository, GitBranchName)
			if err != nil {
				return err
			}
			if localCommit == nil {
				return nil // local and remote are both empty
			}
			return pushCalendar(cal, c.proxyURL)

		default:
			return fmt.Errorf("fetch: %w", err)
		}
	}

	localCommit, remoteCommit, err := gitmerge.GetCommits(cal.repository, GitBranchName, GitRemoteName)
	if err != nil {
		return err
	}

	switch {
	case localCommit == nil && remoteCommit == nil:
		return nil

	case localCommit == nil:
		// local has no commits
		return fastForwardCalendar(cal, remoteCommit.Hash)

	case remoteCommit == nil:
		// remote has no commits
		return pushCalendar(cal, c.proxyURL)

	case localCommit.Hash == remoteCommit.Hash:
		return nil // already in sync

	case isAncestor(localCommit, remoteCommit):
		// remote is ahead
		return fastForwardCalendar(cal, remoteCommit.Hash)

	case isAncestor(remoteCommit, localCommit):
		// local is ahead
		return pushCalendar(cal, c.proxyURL)

	default:
		// Diverged histories require the custom merge policy before pushing.

		fmt.Printf("Diverged history detected on %q, trying to merge...\n", cal.Name)
		if err := mergeOriginMain(cal.repository, cal.Name, cal.EncryptionKey); err != nil {
			return fmt.Errorf("failed to merge: %w", err)
		}
		fmt.Printf("Custom merge successful for %q\n", cal.Name)

		return pushCalendar(cal, c.proxyURL)
	}
}

// ExportZip exports all persisted data or one calendar repository as a ZIP archive.
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
		if cal.repository == nil {
			return nil, errors.New("URL calendars cannot be exported")
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

	return buf.Bytes(), nil
}

// RestoreZip replaces all persisted data with a full backup.
func (c *Core) RestoreZip(data []byte) error {
	if err := export.ValidateZip(data); err != nil {
		return err
	}
	if err := c.clearPersistedData(); err != nil {
		return fmt.Errorf("clear existing data: %w", err)
	}
	c.resetCore()

	if err := export.Unzip(c.fs, data); err != nil {
		if cleanupErr := c.clearPersistedData(); cleanupErr != nil {
			return errors.Join(err, fmt.Errorf("cleanup after failed restore: %w", cleanupErr))
		}
		return err
	}
	if err := c.LoadCalendars(); err != nil {
		c.resetCore()
		if cleanupErr := c.clearPersistedData(); cleanupErr != nil {
			return errors.Join(err, fmt.Errorf("cleanup after failed restore: %w", cleanupErr))
		}
		return fmt.Errorf("load restored data: %w", err)
	}
	return nil
}

// ------------------------------------------------ Helpers -------------------------------------------------

func (c *Core) clearPersistedData() error {
	entries, err := c.fs.ReadDir(".")
	if err != nil {
		return err
	}
	var result error
	for _, entry := range entries {
		if err := gogitutil.RemoveAll(c.fs, entry.Name()); err != nil {
			result = errors.Join(result, err)
		}
	}
	return result
}

// resetCore clears and reinitializes the in-memory indexes.
func (c *Core) resetCore() {
	c.intervalTree = NewIntervalTree()
	c.events = make(map[uuid.UUID]*Event)
	c.calendars = make(map[string]*Calendar)
}

// initCalendarRepo opens or initializes the named calendar repository.
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

	// create initial commit if the repository has no commits yet
	if err := ensureInitialCommit(repo); err != nil {
		return nil, fmt.Errorf("ensure initial commit failed: %w", err)
	}

	return repo, nil
}

// ensureInitialCommit creates an empty initial commit only if none exist.
func ensureInitialCommit(repo *gogit.Repository) error {
	_, err := repo.Head()
	if err == nil {
		// HEAD exists, so the repository already has commits
		return nil
	}
	if !errors.Is(err, plumbing.ErrReferenceNotFound) {
		return fmt.Errorf("check head: %w", err)
	}

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
