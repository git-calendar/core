package core

import (
	"errors"
	"fmt"
	"io"
	"maps"
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

// ListCalendars returns calendar metadata.
func (c *Core) ListCalendars() ([]Calendar, error) {
	calendars := slices.Collect(maps.Values(c.calendars))

	slices.SortFunc(calendars, func(a, b *Calendar) int {
		return strings.Compare(a.Name, b.Name)
	})

	result := make([]Calendar, 0, len(calendars))

	for _, cal := range calendars {
		slices.SortFunc(cal.Tags, func(a, b Tag) int {
			return strings.Compare(a.Name, b.Name)
		})

		result = append(result, Calendar{
			Name:          cal.Name,
			Tags:          slices.Clone(cal.Tags),
			EncryptionKey: slices.Clone(cal.EncryptionKey),
			repository:    cal.repository,
			Readonly:      cal.Readonly,
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

	icalURLs := make(map[string]*url.URL)
	for _, entry := range entries {
		name := entry.Name()
		if !entry.IsDir() {
			sourceURL, err := c.readICalURL(name)
			if err != nil {
				continue
			}
			c.calendars[name] = &Calendar{Name: name, Readonly: true}
			icalURLs[name] = sourceURL
			continue
		}

		repo, err := c.initCalendarRepo(name)
		if err != nil {
			fmt.Printf("failed to init/load %q repository: %v\n", name, err)
			continue
		}

		// load key file
		var key []byte = nil
		keyFile, err := c.fs.Open(fmt.Sprintf("%s.key", name))
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

	// load tree + events
	// TODO do not load files, but build tree from index.json
	for _, cal := range c.calendars {
		if sourceURL, ok := icalURLs[cal.Name]; ok {
			if err := c.loadICalURL(cal.Name, sourceURL); err != nil {
				fmt.Printf("WARN: failed to load iCalendar URL %q: %v\n", cal.Name, err)
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
func (c *Core) CloneCalendar(repoUrl *url.URL, password string, readonly bool) error {
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

	var key []byte = nil
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

// Removes and deletes the whole calendar.
func (c *Core) RemoveCalendar(name string) error {
	// remove dir from filesystem
	if err := gogitutil.RemoveAll(c.fs, name); err != nil {
		return fmt.Errorf("failed to remove repo directory: %w", err)
	}

	// try to remove encryption key
	_ = c.fs.Remove(fmt.Sprintf("%s.key", name))

	// remove from map
	delete(c.calendars, name)

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
		return fmt.Errorf("failed to rename calendar: %w", err)
	}
	if calendar.repository == nil {
		return c.LoadCalendars()
	}
	if len(calendar.EncryptionKey) != 0 { // TODO: maybe check c.fs.Stat() instead?
		if err := c.fs.Rename(fmt.Sprintf("%s.key", oldName), fmt.Sprintf("%s.key", newName)); err != nil {
			_ = c.fs.Rename(newName, oldName) // try to rename repo back
			return fmt.Errorf("failed to rename the encryption key file: %w", err)
		}
	}
	if c.isCalendarReadonly(oldName) {
		if err := c.fs.Rename(fmt.Sprintf("%s.readonly", oldName), fmt.Sprintf("%s.readonly", newName)); err != nil {
			_ = c.fs.Rename(newName, oldName)                                               // try to rename repo back
			_ = c.fs.Rename(fmt.Sprintf("%s.key", newName), fmt.Sprintf("%s.key", oldName)) // try to rename key back (maybe it didn't exist in the first place, mehh)
			return fmt.Errorf("failed to rename the encryption key file: %w", err)
		}
	}

	newRepo, err := c.initCalendarRepo(newName)
	if err != nil {
		_ = c.fs.Rename(newName, oldName)                                                         // try to rename repo back
		_ = c.fs.Rename(fmt.Sprintf("%s.key", newName), fmt.Sprintf("%s.key", oldName))           // try to rename key back (maybe it didn't exist in the first place, mehh)
		_ = c.fs.Rename(fmt.Sprintf("%s.readonly", newName), fmt.Sprintf("%s.readonly", oldName)) // try to rename readonly sign back (maybe it didn't exist in the first place, mehh)
		return fmt.Errorf("failed to load new repo dir: %w", err)
	}
	calendar.Name = newName
	calendar.repository = newRepo

	delete(c.calendars, oldName)
	c.calendars[newName] = calendar

	return nil
}

func (c *Core) UpdateRemote(calendar string, remoteURL *url.URL, readonly bool) error {
	cal, ok := c.calendars[calendar]
	if !ok || cal == nil {
		return fmt.Errorf("calendar not found: %s", calendar)
	}

	if cal.repository == nil {
		return fmt.Errorf("calendar %q has no repository", calendar)
	}

	if remoteURL == nil {
		if err := cal.repository.DeleteRemote(GitRemoteName); err != nil {
			if errors.Is(err, gogit.ErrRemoteNotFound) {
				return nil
			}
			return fmt.Errorf("failed to delete remote: %w", err)
		}
		return nil
	}

	if !strings.HasSuffix(remoteURL.Path, ".git") {
		return errors.New(`remote URL has to end with ".git"`)
	}

	cfg, _ := cal.repository.Config()
	cfg.Remotes[GitRemoteName] = &config.RemoteConfig{
		Name: GitRemoteName,
		URLs: []string{remoteURL.String()},
	}
	if err := cal.repository.SetConfig(cfg); err != nil {
		return fmt.Errorf("failed to set remote: %w", err)
	}

	if err := c.updateReadonlyFile(calendar, readonly); err != nil {
		fmt.Printf("WARN: failed to update calendar read-only file: %v", err)
	}

	return nil
}

// ------------------------------------------------ Helpers -------------------------------------------------

func (c *Core) createKeyFile(calendarName, password string) ([]byte, error) {
	key := encryption.DeriveKey(password, []byte(calendarName))

	keyFile, err := c.fs.Create(fmt.Sprintf("%s.key", calendarName))
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
	path := fmt.Sprintf("%s.readonly", calendarName)

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
	path := fmt.Sprintf("%s.readonly", calendarName)
	_, err := c.fs.Stat(path)
	return err == nil
}
