package core

import (
	"errors"
	"fmt"
	"io"
	"maps"
	"net/url"
	"slices"
	"strings"

	"github.com/git-calendar/core/pkg/encryption"
	"github.com/git-calendar/core/pkg/filesystem"
	gogitutil "github.com/go-git/go-billy/v5/util"
	gogit "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing/cache"
	gogitfs "github.com/go-git/go-git/v5/storage/filesystem"
)

// Creates a new calendar.
func (c *Core) CreateCalendar(name, password string) error {
	repo, err := c.initCalendarRepo(name)
	if err != nil {
		return fmt.Errorf("failed to init calendar repo: %w", err)
	}

	var key []byte = nil
	if len(password) != 0 {
		key = encryption.DeriveKey(password, []byte(name))

		wt, err := repo.Worktree()
		if err != nil {
			return fmt.Errorf("failed get worktree from repo: %w", err)
		}

		keyFile, err := wt.Filesystem.Create(EncryptionKeyFileName)
		if err != nil {
			return fmt.Errorf("failed to create key file: %w", err)
		}
		defer keyFile.Close()

		if _, err = keyFile.Write(key); err != nil {
			return fmt.Errorf("failed to write key to key file: %w", err)
		}
	}

	c.calendars[name] = &Calendar{
		Repository:    repo,
		Tags:          []string{},
		EncryptionKey: key,
	}
	return nil
}

// Returns a list of calendar names loaded.
func (c *Core) ListCalendars() []string {
	// TODO list tags too
	calendars := slices.Collect(maps.Keys(c.calendars))
	slices.Sort(calendars)
	return calendars
}

// Tries to load every directory/repo/calendar in the fs root.
func (c *Core) LoadCalendars() error {
	c.resetCore()

	// load repositories
	entries, err := c.fs.ReadDir(filesystem.DirName)
	if err != nil {
		return fmt.Errorf("failed to list all directories in root: %w", err)
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		repo, err := c.initCalendarRepo(entry.Name())
		if err != nil {
			fmt.Printf("failed to init/load '%s' repository: %v", entry.Name(), err)
			continue
		}

		var key []byte = nil
		keyFile, err := c.fs.Open(c.fs.Join(filesystem.DirName, entry.Name(), EncryptionKeyFileName))
		if err == nil {
			key, err = io.ReadAll(keyFile)
			if err != nil {
				fmt.Printf("failed to read encryption key for '%s' repository: %v", entry.Name(), err)
			}
		}

		c.calendars[entry.Name()] = &Calendar{
			Repository:    repo,
			Tags:          nil, // TODO: load tags
			EncryptionKey: key,
		}
	}

	// load tree + events
	// TODO do not load files, but build tree from index.json
	for _, cal := range c.calendars {
		wt, _ := cal.Repository.Worktree()
		eventsDir, _ := wt.Filesystem.Chroot(EventsDirName)
		eventEntries, _ := eventsDir.ReadDir("/")
		for _, eventEntry := range eventEntries {
			if eventEntry.IsDir() {
				continue
			}

			file, err := eventsDir.Open(eventEntry.Name())
			if err != nil {
				fmt.Printf("failed to open file '%s' from cal %s: %v\n", eventEntry.Name(), wt.Filesystem.Root(), err)
				continue
			}
			defer file.Close()

			var event Event
			err = event.LoadFromFile(file, cal.EncryptionKey)
			if err != nil {
				fmt.Printf("failed to load event from file '%s' from cal %s: %v\n", eventEntry.Name(), wt.Filesystem.Root(), err)
				continue
			}

			err = event.Validate()
			if err != nil {
				fmt.Printf("invalid event: %v\n", err)
				continue
			}

			c.events[event.Id] = &event

			err = c.intervalTree.InsertEvent(event)
			if err != nil {
				fmt.Printf("failed to insert event '%s' into index tree: %v\n", event.Id, err)
				continue
			}
		}
	}

	return nil
}

// Clones a repository/calendar from url, using CORS proxy, if specified.
func (c *Core) CloneCalendar(repoUrl url.URL, password string) error {
	calendarName := calendarNameFromUrl(repoUrl)
	if cal, ok := c.calendars[calendarName]; ok || cal != nil {
		return errors.New("calendar with this name already exists")
	}

	// make sure that the repo dir is created
	repoPath := c.fs.Join(filesystem.DirName, calendarName)
	if err := c.fs.MkdirAll(repoPath, 0o755); err != nil {
		return fmt.Errorf("create repo dir: %w", err)
	}
	repoFS, err := c.fs.Chroot(repoPath)
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
	finalUrl, auth := prepareRepoUrl(repoUrl, c.proxyUrl)
	// clone now
	newRepo, err := gogit.Clone(storage, repoFS, &gogit.CloneOptions{
		RemoteName: "origin",
		URL:        finalUrl.String(),
		Auth:       auth,
	})
	if err != nil {
		c.RemoveCalendar(calendarName) // even on error, clone might create a directory, so let's delete it
		return fmt.Errorf("git clone failed: %w", err)
	}

	// repair the remote url (set the pure url with auth, without proxy)
	err = newRepo.DeleteRemote("origin")
	c.AddRemote(calendarName, "origin", repoUrl.String())

	c.calendars[calendarName] = &Calendar{
		Repository:    newRepo,
		Tags:          nil, // TODO: load tags
		EncryptionKey: encryption.DeriveKey(password, []byte(calendarName)),
	}

	return err
}

// Removes and deletes the whole calendar.
func (c *Core) RemoveCalendar(name string) error {
	// remove from map
	delete(c.calendars, name)

	// remove from filesystem
	err := gogitutil.RemoveAll(c.fs, c.fs.Join(filesystem.DirName, name))
	if err != nil {
		return fmt.Errorf("failed to remove repo directory: %w", err)
	}

	// TODO: This is the lazy way.
	// LoadCalendars does full erase and load again for events map and tree. It also deletes all the repos, and reloads them from disk.
	// Better approach would be to only delete the selected events.

	return c.LoadCalendars()
}

// Adds a new remote to the specified calendar repository.
func (c *Core) AddRemote(calendar, remoteName, remoteUrl string) error {
	var validUrl string
	{
		// validate URL (git doesn't do that when adding a remote, it fails afterwards with e.g. git fetch)
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

	_, err := c.calendars[calendar].Repository.CreateRemote(&config.RemoteConfig{
		Name: remoteName,
		URLs: []string{validUrl},
	})
	if err != nil {
		return fmt.Errorf("failed to create a remote: %w", err)
	}

	return nil
}
