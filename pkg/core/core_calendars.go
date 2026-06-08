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
	gogitutil "github.com/go-git/go-billy/v5/util"
	gogit "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing/cache"
	gogitfs "github.com/go-git/go-git/v5/storage/filesystem"
)

// Creates a new calendar.
func (c *Core) CreateCalendar(name, password string) error {
	if _, exists := c.calendars[name]; exists {
		return fmt.Errorf("calendar named %s already exist", name)
	}

	repo, err := c.initCalendarRepo(name)
	if err != nil {
		return fmt.Errorf("failed to init calendar repo: %w", err)
	}

	var key []byte = nil
	if len(password) != 0 {
		key = encryption.DeriveKey(password, []byte(name))

		keyFile, err := c.fs.Create(fmt.Sprintf("%s.key", name))
		if err != nil {
			return fmt.Errorf("failed to create key file: %w", err)
		}
		defer keyFile.Close()

		if _, err = keyFile.Write(key); err != nil {
			return fmt.Errorf("failed to write key to key file: %w", err)
		}
	}

	cal := calendar{
		Name:          name,
		Tags:          []Tag{},
		EncryptionKey: key,
		repository:    repo,
	}
	if err := cal.Validate(); err != nil {
		_ = gogitutil.RemoveAll(c.fs, name) // cleanup
		_ = c.fs.Remove(fmt.Sprintf("%s.key", name))
		return fmt.Errorf("calendar invalid: %w", err)
	}

	c.calendars[name] = &cal
	return nil
}

// ListCalendars returns calendar metadata.
func (c *Core) ListCalendars() ([]CalendarInfo, error) {
	calendars := slices.Collect(maps.Values(c.calendars))
	slices.SortFunc(calendars, func(a, b *calendar) int {
		return strings.Compare(a.Name, b.Name)
	})

	result := make([]CalendarInfo, 0, len(calendars))

	for _, cal := range calendars {
		var remoteUrl string
		if cal.repository != nil {
			remote, err := cal.repository.Remote("origin")
			if err == nil {
				cfg := remote.Config()
				if cfg == nil {
					return nil, fmt.Errorf("remote for calendar %q has no config", cal.Name)
				}
				if len(cfg.URLs) != 1 || cfg.URLs[0] == "" {
					return nil, fmt.Errorf("remote for calendar %q must have exactly one non-empty URL, got %d", cal.Name, len(cfg.URLs))
				}
				remoteUrl = cfg.URLs[0]
			}
		}

		result = append(result, CalendarInfo{
			Name:      cal.Name,
			Tags:      slices.Clone(cal.Tags),
			Encrypted: cal.IsEncrypted(),
			RemoteUrl: remoteUrl,
		})
	}

	return result, nil
}

// Tries to load every directory/repo/calendar in the fs root.
func (c *Core) LoadCalendars() error {
	c.resetCore()

	// load repositories
	entries, err := c.fs.ReadDir(".")
	if err != nil {
		return fmt.Errorf("failed to list all directories in root: %w", err)
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		name := entry.Name()

		repo, err := c.initCalendarRepo(name)
		if err != nil {
			fmt.Printf("failed to init/load %q repository: %v", name, err)
			continue
		}

		var key []byte = nil
		keyFile, err := c.fs.Open(fmt.Sprintf("%s.key", name))
		if err == nil {
			key, err = io.ReadAll(keyFile)
			if err != nil {
				fmt.Printf("failed to read encryption key for %q repository: %v", name, err)
			}
			keyFile.Close()
		}

		c.calendars[name] = &calendar{
			Name:          name,
			Tags:          nil, // TODO: load tags
			EncryptionKey: key,
			repository:    repo,
		}
	}

	// load tree + events
	// TODO do not load files, but build tree from index.json
	for _, cal := range c.calendars {
		wt, _ := cal.repository.Worktree()
		eventsDir, _ := wt.Filesystem.Chroot(EventsDirName)
		eventEntries, _ := eventsDir.ReadDir("/")
		for _, eventEntry := range eventEntries {
			if eventEntry.IsDir() {
				continue
			}

			file, err := eventsDir.Open(eventEntry.Name())
			if err != nil {
				fmt.Printf("failed to open file %q from cal %s: %v\n", eventEntry.Name(), wt.Filesystem.Root(), err)
				continue
			}
			defer file.Close()

			var event Event
			err = event.LoadFromFile(file, cal.EncryptionKey)
			if err != nil {
				fmt.Printf("failed to load event from file %q from cal %s: %v\n", eventEntry.Name(), wt.Filesystem.Root(), err)
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
				fmt.Printf("failed to insert event %q into index tree: %v\n", event.Id, err)
				continue
			}
		}
	}

	return nil
}

// Clones a repository/calendar from url, using CORS proxy, if specified.
func (c *Core) CloneCalendar(repoUrl *url.URL, password string) error {
	calendarName := calendarNameFromUrl(repoUrl)
	if cal, ok := c.calendars[calendarName]; ok || cal != nil {
		return errors.New("calendar with this name already exists")
	}

	// make sure that the repo dir is created
	if err := c.fs.MkdirAll(calendarName, 0o755); err != nil {
		return fmt.Errorf("create repo dir: %w", err)
	}
	repoFS, err := c.fs.Chroot(calendarName)
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

	var key []byte = nil
	if len(password) != 0 {
		key = encryption.DeriveKey(password, []byte(calendarName))

		keyFile, err := c.fs.Create(fmt.Sprintf("%s.key", calendarName))
		if err != nil {
			return fmt.Errorf("failed to create key file: %w", err)
		}
		defer keyFile.Close()

		if _, err = keyFile.Write(key); err != nil {
			return fmt.Errorf("failed to write key to key file: %w", err)
		}
	}
	c.calendars[calendarName] = &calendar{
		Name:          calendarName,
		Tags:          nil, // TODO: load tags
		EncryptionKey: key,
		repository:    newRepo,
	}

	// repair the remote url (set the pure url with auth, without proxy)
	if err := c.UpdateRemote(calendarName, repoUrl); err != nil {
		return err
	}

	return nil
}

// Removes and deletes the whole calendar.
func (c *Core) RemoveCalendar(name string) error {
	// remove from map
	delete(c.calendars, name)

	// remove dir from filesystem
	if err := gogitutil.RemoveAll(c.fs, name); err != nil {
		return fmt.Errorf("failed to remove repo directory: %w", err)
	}

	// try to remove encryption key
	_ = c.fs.Remove(fmt.Sprintf("%s.key", name))

	// TODO: This is the lazy way.
	// LoadCalendars does full erase and load again for events map and tree. It also deletes all the repos, and reloads them from disk.
	// Better approach would be to only delete the selected events.

	return c.LoadCalendars()
}

func (c *Core) RenameCalendar(oldName, newName string) error {
	if _, exists := c.calendars[oldName]; !exists {
		return fmt.Errorf("calendar %s doesn't exist", oldName)
	}
	if oldName == newName {
		return nil
	}
	if _, exists := c.calendars[newName]; exists {
		return fmt.Errorf("calendar named %s already exists", newName)
	}

	calendar := c.calendars[oldName]
	if err := c.fs.Rename(oldName, newName); err != nil {
		return fmt.Errorf("failed to rename the repository directory: %w", err)
	}
	if len(calendar.EncryptionKey) != 0 { // TODO: maybe check c.fs.Stat() instead?
		if err := c.fs.Rename(fmt.Sprintf("%s.key", oldName), fmt.Sprintf("%s.key", newName)); err != nil {
			_ = c.fs.Rename(newName, oldName) // try to rename repo back
			return fmt.Errorf("failed to rename the encryption key file: %w", err)
		}
	}

	newRepo, err := c.initCalendarRepo(newName)
	if err != nil {
		_ = c.fs.Rename(newName, oldName)                                               // try to rename repo back
		_ = c.fs.Rename(fmt.Sprintf("%s.key", newName), fmt.Sprintf("%s.key", oldName)) // try to rename key back (maybe it didn't exist in the first place, mehh)
		return fmt.Errorf("failed to load new repo dir: %w", err)
	}
	calendar.Name = newName
	calendar.repository = newRepo

	delete(c.calendars, oldName)
	c.calendars[newName] = calendar

	return nil
}

func (c *Core) UpdateRemote(calendar string, remoteURL *url.URL) error {
	cal, ok := c.calendars[calendar]
	if !ok || cal == nil {
		return fmt.Errorf("calendar not found: %s", calendar)
	}

	if cal.repository == nil {
		return fmt.Errorf("calendar %q has no repository", calendar)
	}

	if remoteURL == nil {
		if err := cal.repository.DeleteRemote("origin"); err != nil {
			return fmt.Errorf("failed to delete remote: %w", err)
		}
		return nil
	}

	if !strings.HasSuffix(remoteURL.Path, ".git") {
		return errors.New(`remote URL has to end with ".git"`)
	}

	if _, err := cal.repository.Remote("origin"); err == nil {
		if err := cal.repository.DeleteRemote("origin"); err != nil {
			return fmt.Errorf("failed to delete remote: %w", err)
		}
	}

	if _, err := cal.repository.CreateRemote(&config.RemoteConfig{
		Name: "origin",
		URLs: []string{remoteURL.String()},
	}); err != nil {
		return fmt.Errorf("failed to create remote: %w", err)
	}

	return nil
}
