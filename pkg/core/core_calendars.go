package core

import (
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"net/url"
	"slices"
	"strings"

	"github.com/firu11/git-calendar-core/pkg/filesystem"
	gogitutil "github.com/go-git/go-billy/v5/util"
	gogit "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing/cache"
	gogitfs "github.com/go-git/go-git/v5/storage/filesystem"
)

// Creates a new calendar.
func (c *Core) CreateCalendar(name string) error {
	repo, err := c.initCalendarRepo(name)
	if err != nil {
		return fmt.Errorf("failed to init calendar repo: %w", err)
	}
	c.repos[name] = repo
	return nil
}

// Returns a list of calendar names loaded.
func (c *Core) ListCalendars() []string {
	// TODO list tags too
	calendars := slices.Collect(maps.Keys(c.repos))
	slices.Sort(calendars)
	return calendars
}

// Tries to load every directory/repo/calendar in the fs root.
func (c *Core) LoadCalendars() error {
	c.eraseAndAlloc()

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
		c.repos[entry.Name()] = repo
	}

	// load tree + events
	// TODO do not load files, but build tree from index.json
	for _, repo := range c.repos {
		wt, _ := repo.Worktree()
		entries, _ := wt.Filesystem.ReadDir(EventsDirName)
		for _, entry := range entries {
			if entry.IsDir() {
				continue
			}

			fileName := wt.Filesystem.Join(EventsDirName, entry.Name())
			file, err := wt.Filesystem.Open(fileName)
			if err != nil {
				fmt.Printf("failed to open file '%s': %v", fileName, err)
				continue
			}
			defer file.Close()

			var event Event
			err = json.NewDecoder(file).Decode(&event)
			if err != nil {
				fmt.Printf("failed to decode event from file '%s': %v", fileName, err)
				continue
			}

			c.events[event.Id] = &event

			err = insertEventIntoTree(c.tree, event)
			if err != nil {
				fmt.Printf("failed to insert event '%s' into index tree: %v", event.Id, err)
				continue
			}
		}
	}

	return nil
}

// Clones a repository/calendar from url, using CORS proxy, if specified.
func (c *Core) CloneCalendar(repoUrl url.URL) error {
	calendarName := calendarNameFromUrl(repoUrl)
	if _, ok := c.repos[calendarName]; ok {
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
	c.repos[calendarName], err = gogit.Clone(storage, repoFS, &gogit.CloneOptions{
		RemoteName: "origin",
		URL:        finalUrl.String(),
		Auth:       auth,
	})
	if err != nil {
		return fmt.Errorf("git clone failed: %w", err)
	}

	// repair the remote url (set the pure url with auth, without proxy)
	err = c.repos[calendarName].DeleteRemote("origin")
	c.AddRemote(calendarName, "origin", repoUrl.String())

	return err
}

// Removes and deletes the whole calendar.
func (c *Core) RemoveCalendar(name string) error {
	// remove from map
	delete(c.repos, name)

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

	_, err := c.repos[calendar].CreateRemote(&config.RemoteConfig{
		Name: remoteName,
		URLs: []string{validUrl},
	})
	if err != nil {
		return fmt.Errorf("failed to create a remote: %w", err)
	}

	return nil
}
