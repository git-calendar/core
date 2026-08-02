package core

import (
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"slices"
	"strings"

	"github.com/git-calendar/core/pkg/encryption"
	gogitutil "github.com/go-git/go-billy/v5/util"
	gogit "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing/cache"
	"github.com/go-git/go-git/v5/plumbing/transport"
	gogitfs "github.com/go-git/go-git/v5/storage/filesystem"
)

// CreateCalendar creates a calendar backed by a local Git repository.
func (c *Core) CreateCalendar(name, password string) error {
	if _, exists := c.calendars[name]; exists {
		return fmt.Errorf("calendar named %s already exists", name)
	}

	repo, err := c.initCalendarRepo(name)
	if err != nil {
		return fmt.Errorf("failed to init calendar repo: %w", err)
	}

	var key []byte
	if len(password) != 0 {
		if key, err = c.createKeyFile(name, password); err != nil {
			return err
		}
	}

	cal := Calendar{
		Name:          name,
		Tags:          []Tag{},
		EncryptionKey: key,
		repository:    repo,
		Readonly:      false,
	}
	if err := cal.Validate(); err != nil {
		c.RemoveCalendar(name) // cleanup
		return fmt.Errorf("calendar invalid: %w", err)
	}

	c.calendars[name] = &cal
	return nil
}

// ListCalendars returns calendar metadata sorted by calendar and tag name.
func (c *Core) ListCalendars() ([]Calendar, error) {
	result := make([]Calendar, 0, len(c.calendars))
	for _, calendar := range c.calendars {
		cal := *calendar
		cal.Tags = slices.Clone(calendar.Tags)
		slices.SortFunc(cal.Tags, func(a, b Tag) int {
			return strings.Compare(a.Name, b.Name)
		})
		cal.EncryptionKey = slices.Clone(calendar.EncryptionKey)
		result = append(result, cal)
	}

	slices.SortFunc(result, func(a, b Calendar) int {
		return strings.Compare(a.Name, b.Name)
	})
	return result, nil
}

// LoadCalendars rebuilds in-memory calendar and event state from storage.
func (c *Core) LoadCalendars() error {
	c.resetCore()

	// discover URL calendars and load repository-backed calendars
	entries, err := c.fs.ReadDir(".")
	if err != nil {
		return fmt.Errorf("failed to list all directories in root: %w", err)
	}

	icalURLs := make(map[string]*url.URL)
	for _, entry := range entries {
		name := entry.Name()
		if !entry.IsDir() {
			if !strings.HasSuffix(name, ICalURLFileSuffix) {
				continue
			}

			calendarName := strings.TrimSuffix(name, ICalURLFileSuffix)
			if err := validateICalName(calendarName); err != nil {
				continue
			}
			sourceURL, err := c.readICalURL(name)
			if err != nil {
				continue
			}
			icalURLs[calendarName] = sourceURL
			continue
		}

		repo, err := c.initCalendarRepo(name)
		if err != nil {
			fmt.Printf("failed to init/load %q repository: %v\n", name, err)
			continue
		}

		// load key file
		var key []byte
		keyFile, err := c.fs.Open(name + KeyFileSuffix)
		if err == nil {
			if key, err = io.ReadAll(keyFile); err != nil {
				fmt.Printf("failed to read encryption key for %q calendar: %v\n", name, err)
			}
			keyFile.Close()
		}

		cal := &Calendar{
			Name:          name,
			EncryptionKey: key,
			repository:    repo,
			Readonly:      c.isCalendarReadonly(name),
		}

		// load tags
		if err := cal.LoadTags(); err != nil {
			fmt.Printf("WARN: failed to load tags for %q calendar: %v\n", cal.Name, err)
		}

		c.calendars[name] = cal
	}

	for name, icalURL := range icalURLs {
		c.calendars[name] = &Calendar{Name: name, Readonly: true, ICalURL: icalURL}
	}

	// load events into the map and interval tree
	// TODO: do not load files, but build tree from index.json
	for _, cal := range c.calendars {
		if _, ok := icalURLs[cal.Name]; ok {
			if err := c.loadICalFile(cal.Name); err != nil {
				fmt.Printf("WARN: failed to load cached iCalendar %q: %v\n", cal.Name, err)
			}
			continue
		}

		wt, _ := cal.repository.Worktree()
		eventsDir, _ := wt.Filesystem.Chroot(EventsDirName)
		eventEntries, _ := eventsDir.ReadDir("/")
		for _, eventEntry := range eventEntries {
			if eventEntry.IsDir() || !strings.HasSuffix(eventEntry.Name(), ".json") {
				continue
			}

			file, err := eventsDir.Open(eventEntry.Name())
			if err != nil {
				fmt.Printf("failed to open file %q from cal %s: %v\n", eventEntry.Name(), wt.Filesystem.Root(), err)
				continue
			}
			defer file.Close()

			var event Event
			err = event.LoadFromFile(file, cal.Name, cal.EncryptionKey)
			if err != nil {
				fmt.Printf("failed to load event from file %q from cal %s: %v\n", eventEntry.Name(), wt.Filesystem.Root(), err)
				continue
			}

			if err := event.Validate(); err != nil {
				fmt.Printf("invalid event: %v\n", err)
				continue
			}

			c.events[event.ID] = &event

			if err := c.intervalTree.InsertEvent(event); err != nil {
				fmt.Printf("failed to insert event %q into index tree: %v\n", event.ID, err)
				continue
			}
		}
	}

	return nil
}

// CloneCalendar clones a remote Git calendar, using CORS proxy, if set.
func (c *Core) CloneCalendar(repoUrl *url.URL, password string, readonly bool) error {
	if !strings.HasSuffix(repoUrl.Path, ".git") {
		return errors.New(`remote URL must end with ".git"`)
	}
	calendarName := calendarNameFromUrl(repoUrl)
	if _, ok := c.calendars[calendarName]; ok {
		return errors.New("calendar with this name already exists")
	}

	// ensure the repo dir exists before cloning
	if err := c.fs.MkdirAll(calendarName, 0o755); err != nil {
		return fmt.Errorf("create repo dir: %w", err)
	}
	repoFS, err := c.fs.Chroot(calendarName)
	if err != nil {
		return fmt.Errorf("chroot repo dir: %w", err)
	}

	// ensure the Git metadata dir exists
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
	repo, err := gogit.Clone(storage, repoFS, &gogit.CloneOptions{
		RemoteName: GitRemoteName,
		URL:        finalUrl.String(),
		Auth:       auth,
	})
	if err != nil {
		c.RemoveCalendar(calendarName) // even on error, clone might create a directory, so let's delete it
		if errors.Is(err, transport.ErrEmptyRemoteRepository) {
			return fmt.Errorf("git clone failed: %w: maybe you wanted to init instead?", err)
		}
		return fmt.Errorf("git clone failed: %w", err)
	}

	var key []byte
	if len(password) != 0 {
		if key, err = c.createKeyFile(calendarName, password); err != nil {
			return err
		}
	}

	cal := &Calendar{
		Name:          calendarName,
		EncryptionKey: key,
		repository:    repo,
		Readonly:      readonly,
	}

	// load tags
	if err := cal.LoadTags(); err != nil {
		fmt.Printf("WARN: failed to load tags for %q calendar: %v\n", cal.Name, err)
	}

	if err := c.updateReadonlyFile(calendarName, readonly); err != nil {
		fmt.Printf("WARN: failed to update calendar read-only file: %v\n", err)
	}

	c.calendars[calendarName] = cal

	if c.proxyUrl != nil {
		// repair the remote url (set the pure url with auth, without proxy)
		if err := c.UpdateRemote(calendarName, repoUrl, readonly); err != nil {
			return err
		}
	}

	return nil
}

// RemoveCalendar deletes a calendar and reloads the remaining state.
func (c *Core) RemoveCalendar(name string) error {
	calendar := c.calendars[name]
	if calendar != nil && calendar.repository == nil {
		if err := c.fs.Remove(name + ICalURLFileSuffix); err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("failed to remove iCalendar URL file: %w", err)
		}
		if err := c.fs.Remove(name + ICalFileSuffix); err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("failed to remove cached iCalendar file: %w", err)
		}
	} else {
		// remove dir from filesystem
		if err := gogitutil.RemoveAll(c.fs, name); err != nil {
			return fmt.Errorf("failed to remove repo directory: %w", err)
		}

		// try to remove other related files
		_ = c.fs.Remove(name + KeyFileSuffix)
		_ = c.fs.Remove(name + ReadonlyFileSuffix)
	}

	// remove from map
	delete(c.calendars, name)

	// TODO: This is the lazy way.
	// LoadCalendars does full erase and load again for events map and tree. It also deletes all the repos, and reloads them from disk.
	// Better approach would be to only delete the selected events.

	return c.LoadCalendars()
}

// RenameCalendar changes a calendar's name and associated storage paths.
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
	if calendar.repository == nil {
		if err := validateICalName(newName); err != nil {
			return err
		}
		if err := c.fs.Rename(oldName+ICalURLFileSuffix, newName+ICalURLFileSuffix); err != nil {
			return fmt.Errorf("failed to rename iCalendar URL file: %w", err)
		}
		if err := c.fs.Rename(oldName+ICalFileSuffix, newName+ICalFileSuffix); err != nil && !errors.Is(err, os.ErrNotExist) {
			_ = c.fs.Rename(newName+ICalURLFileSuffix, oldName+ICalURLFileSuffix)
			return fmt.Errorf("failed to rename cached iCalendar file: %w", err)
		}
		return c.LoadCalendars()
	}
	if err := c.fs.Rename(oldName, newName); err != nil {
		return fmt.Errorf("failed to rename calendar: %w", err)
	}
	if len(calendar.EncryptionKey) != 0 { // TODO: maybe check c.fs.Stat() instead?
		if err := c.fs.Rename(oldName+KeyFileSuffix, newName+KeyFileSuffix); err != nil {
			_ = c.fs.Rename(newName, oldName) // try to rename repo back
			return fmt.Errorf("failed to rename the encryption key file: %w", err)
		}
	}
	if c.isCalendarReadonly(oldName) {
		if err := c.fs.Rename(oldName+ReadonlyFileSuffix, newName+ReadonlyFileSuffix); err != nil {
			_ = c.fs.Rename(newName, oldName)                             // try to rename repo back
			_ = c.fs.Rename(newName+KeyFileSuffix, oldName+KeyFileSuffix) // try to rename key back (maybe it didn't exist in the first place, mehh)
			return fmt.Errorf("failed to rename the encryption key file: %w", err)
		}
	}

	newRepo, err := c.initCalendarRepo(newName)
	if err != nil {
		_ = c.fs.Rename(newName, oldName)                                       // try to rename repo back
		_ = c.fs.Rename(newName+KeyFileSuffix, oldName+KeyFileSuffix)           // try to rename key back (maybe it didn't exist in the first place, mehh)
		_ = c.fs.Rename(newName+ReadonlyFileSuffix, oldName+ReadonlyFileSuffix) // try to rename readonly sign back (maybe it didn't exist in the first place, mehh)
		return fmt.Errorf("failed to load new repo dir: %w", err)
	}
	calendar.Name = newName
	calendar.repository = newRepo

	delete(c.calendars, oldName)
	c.calendars[newName] = calendar
	for _, event := range c.events {
		if event != nil && event.Calendar == oldName {
			event.Calendar = newName
		}
	}

	return nil
}

// UpdateRemote sets or removes a calendar's Git remote and read-only marker.
func (c *Core) UpdateRemote(calendar string, remoteURL *url.URL, readonly bool) error {
	cal, ok := c.calendars[calendar]
	if !ok || cal == nil {
		return fmt.Errorf("calendar not found: %s", calendar)
	}

	if cal.repository == nil {
		return fmt.Errorf("calendar %q has no repository", calendar)
	}

	if remoteURL == nil {
		if err := cal.repository.DeleteRemote(GitRemoteName); err != nil && !errors.Is(err, gogit.ErrRemoteNotFound) {
			return fmt.Errorf("failed to delete remote: %w", err)
		}
	} else {
		if !strings.HasSuffix(remoteURL.Path, ".git") {
			return errors.New(`remote URL must end with ".git"`)
		}

		cfg, _ := cal.repository.Config()
		cfg.Remotes[GitRemoteName] = &config.RemoteConfig{
			Name: GitRemoteName,
			URLs: []string{remoteURL.String()},
		}
		if err := cal.repository.SetConfig(cfg); err != nil {
			return fmt.Errorf("failed to set remote: %w", err)
		}
	}

	if err := c.updateReadonlyFile(calendar, readonly); err != nil {
		return fmt.Errorf("failed to update calendar read-only state: %w", err)
	}
	cal.Readonly = readonly

	return nil
}

// ------------------------------------------------ Helpers -------------------------------------------------

func (c *Core) createKeyFile(calendarName, password string) ([]byte, error) {
	key := encryption.DeriveKey(password, []byte(calendarName))

	keyFile, err := c.fs.Create(calendarName + KeyFileSuffix)
	if err != nil {
		return nil, fmt.Errorf("failed to create key file: %w", err)
	}
	defer keyFile.Close()

	if _, err = keyFile.Write(key); err != nil {
		return nil, fmt.Errorf("failed to write key to key file: %w", err)
	}

	return key, nil
}

func (c *Core) updateReadonlyFile(calendarName string, readonly bool) error {
	path := calendarName + ReadonlyFileSuffix

	if readonly {
		file, err := c.fs.Create(path)
		if err != nil {
			return fmt.Errorf("failed to create readonly file: %w", err)
		}

		if err := file.Close(); err != nil {
			return fmt.Errorf("failed to close readonly file: %w", err)
		}

		return nil
	}

	if err := c.fs.Remove(path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to remove readonly file: %w", err)
	}

	return nil
}

func (c *Core) isCalendarReadonly(calendarName string) bool {
	_, err := c.fs.Stat(calendarName + ReadonlyFileSuffix)
	return err == nil
}
