package core

import (
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"net/url"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/firu11/git-calendar-core/pkg/filesystem"
	"github.com/go-git/go-billy/v5"
	gogitutil "github.com/go-git/go-billy/v5/util"
	gogit "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing/cache"
	"github.com/go-git/go-git/v5/plumbing/object"
	gogitfs "github.com/go-git/go-git/v5/storage/filesystem"
	"github.com/google/uuid"
	"github.com/rdleal/intervalst/interval"
)

// The real API.
//
// Works with raw Go structs, use api.Api to work with JSON.
type Core struct {
	eventTree *interval.SearchTree[[]uuid.UUID, time.Time] // for each interval we can have multiple events ({time.Time, time.Time} -> []uuid.UUID)
	events    map[uuid.UUID]*Event
	repos     map[string]*gogit.Repository
	fs        billy.Filesystem // root "/" for OPFS, "$HOME" for classic FS
	proxyUrl  *url.URL         // cors proxy, that works with "url" query param (like https://cors-proxy.abc/?url=https://github.com/...) (only needed for the browser!)
	// tags      map[string][]string // might not be needed to "cache" it like this
}

// A "constructor" for Core.
func NewCore() *Core {
	var c Core
	c.eraseAndAlloc()

	// get the fs; go tags handle which one (classic/wasm)
	var err error
	c.fs, err = filesystem.GetFS()
	if err != nil {
		panic(err)
	}

	err = c.fs.MkdirAll(filesystem.DirName, 0o755)
	if err != nil {
		panic(err)
	}

	return &c
}

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

			eventEnd := event.To
			if event.Repeat != nil {
				eventEnd = event.Repeat.Until // if repeating, insert interval [From, Repetition.Until]
				if event.Repeat.Count >= 1 /* if repeating on count basis */ {
					eventEnd = addUnit(event.To, event.Repeat.Interval*event.Repeat.Count, event.Repeat.Frequency)
				}
			}
			ids, _ := c.eventTree.Find(event.From, eventEnd) // find existing interval
			updated := append(ids, event.Id)                 // if not found, ids is nil -> append makes [event.Id]

			err = c.eventTree.Insert(event.From, eventEnd, updated)
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
	c.addRemote(calendarName, "origin", repoUrl.String())

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

func (c *Core) addRemote(calendar, remoteName, remoteUrl string) error {
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

// Sets a url for CORS proxy. This is only needed inside a browser.
func (c *Core) SetCorsProxy(proxyUrl string) error {
	var err error
	trimmed := strings.TrimSuffix(proxyUrl, "/") // remove trailing "/"
	c.proxyUrl, err = url.Parse(trimmed)
	return err
}

// Creates a new event.
func (c *Core) CreateEvent(event Event) (*Event, error) {
	if err := event.Validate(); err != nil {
		return nil, fmt.Errorf("invalid event: %w", err)
	}

	// add to all events
	c.events[event.Id] = &event

	// -------- insert into tree --------
	eventEnd := event.To
	if event.Repeat != nil {
		eventEnd = event.Repeat.Until // if repeating, insert interval [From, Repetition.Until]
		if event.Repeat.Count >= 1 /* if repeating on count basis */ {
			eventEnd = addUnit(event.To, event.Repeat.Interval*event.Repeat.Count, event.Repeat.Frequency)
		}
	}
	ids, _ := c.eventTree.Find(event.From, eventEnd) // find existing interval
	updated := append(ids, event.Id)                 // if not found, ids is nil -> append makes [event.Id]

	err := c.eventTree.Insert(event.From, eventEnd, updated)
	if err != nil {
		return nil, fmt.Errorf("failed to insert into index tree: %w", err)
	}

	// -------- create json file and add to git --------
	err = c.saveEventToRepo(&event, fmt.Sprintf("CALENDAR: Added event '%s'", event.Title))
	if err != nil {
		return nil, fmt.Errorf("failed to save event to repo: %w", err)
	}
	return &event, nil
}

// Updates an event based on its id. You can specify an UpdateOption to control the behaviour for updating a repeating event,
func (c *Core) UpdateEvent(event Event, opts ...UpdateOption) (*Event, error) {
	if err := event.Validate(); err != nil {
		return nil, fmt.Errorf("invalid event: %w", err)
	}
	originalEvent, exists := c.events[event.Id]
	if !exists { // generated event
		if !isGeneratedEvent(event) {
			return nil, fmt.Errorf("no event found with id '%s'", event.Id)
		}
		if len(opts) != 1 || !opts[0].IsValid() {
			return nil, fmt.Errorf("invalid update event: incorrect options provided")
		}
		master, ok := c.events[event.MasterId]
		if !ok || master == nil || master.Repeat == nil {
			return nil, fmt.Errorf("invalid update event: no valid master found")
		}
		switch opts[0] {
		case Current:
			exceptionTime := event.OriginalFrom
			if exceptionTime.IsZero() {
				exceptionTime = event.From
			}
			exception := Exception{Id: event.Id, Time: exceptionTime}
			master.Repeat.Exceptions = append(master.Repeat.Exceptions, exception)
			if err := c.saveEventToRepo(master, fmt.Sprintf("CALENDAR: Added exception to master '%s'", master.Title)); err != nil {
				return nil, fmt.Errorf("failed to save master event: %w", err)
			}
			event.Repeat = nil
			c.events[event.Id] = &event
			ids, _ := c.eventTree.Find(event.From, event.To)
			updated := append(ids, event.Id)
			if err := c.eventTree.Insert(event.From, event.To, updated); err != nil {
				return nil, fmt.Errorf("failed to insert into index tree: %w", err)
			}
			if err := c.saveEventToRepo(&event, fmt.Sprintf("CALENDAR: Saved exception event '%s'", event.Title)); err != nil {
				return nil, err
			}
			return &event, nil
		case Following:
			master.Repeat.Until = event.From // cap master at start of change
			master.Repeat.Count = -1         // enforce 'Until' logic over 'Count'
			if err := c.saveEventToRepo(master, fmt.Sprintf("CALENDAR: Capped master event '%s'", master.Title)); err != nil {
				return nil, fmt.Errorf("failed to cap master event: %w", err)
			}
			// make the incoming event new Master event
			event.MasterId = uuid.Nil
			c.events[event.Id] = &event
			// calculate the end of the new series for the tree
			eventEnd := event.To
			if event.Repeat != nil {
				eventEnd = event.Repeat.Until
				if event.Repeat.Count >= 1 {
					eventEnd = addUnit(event.To, event.Repeat.Interval*event.Repeat.Count, event.Repeat.Frequency)
				}
			}
			// insert the new master into the tree
			ids, _ := c.eventTree.Find(event.From, eventEnd)
			updated := append(ids, event.Id)
			if err := c.eventTree.Insert(event.From, eventEnd, updated); err != nil {
				return nil, fmt.Errorf("failed to insert into index tree: %w", err)
			}
			if err := c.saveEventToRepo(&event, fmt.Sprintf("CALENDAR: Created new master event '%s'", event.Title)); err != nil {
				return nil, err
			}
			return &event, nil
		case All:
			fromChanged := event.From != master.From
			toChanged := event.To != master.To
			repeatChanged := event.Repeat != master.Repeat
			if fromChanged { // shift all exceptions by the time difference
				distance := event.From.Sub(master.From)
				for i := range master.Repeat.Exceptions {
					master.Repeat.Exceptions[i].Time = master.Repeat.Exceptions[i].Time.Add(distance)
				}
			}
			if fromChanged || toChanged || repeatChanged {
				err := c.rebuildTreeForEvent(master, &event)
				if err != nil {
					return nil, fmt.Errorf("failed to rebuild tree for event: %w", err)
				}
			}
			master.Title = event.Title
			master.Location = event.Location
			master.Description = event.Description
			master.From = event.From
			master.To = event.To
			master.Tag = event.Tag
			master.Repeat = event.Repeat

			if err := c.saveEventToRepo(master, fmt.Sprintf("CALENDAR: Updated master event '%s'", master.Title)); err != nil {
				return nil, fmt.Errorf("failed to save event to repo: %w", err)
			}
			return master, nil
		}
	} else { // normal event, exception event
		oldEnd := originalEvent.To
		if originalEvent.Repeat != nil {
			oldEnd = originalEvent.Repeat.Until
			if originalEvent.Repeat.Count >= 1 {
				oldEnd = addUnit(originalEvent.To, originalEvent.Repeat.Interval*originalEvent.Repeat.Count, originalEvent.Repeat.Frequency)
			}
		}
		newEnd := event.To
		if event.Repeat != nil {
			newEnd = event.Repeat.Until
			if event.Repeat.Count >= 1 {
				newEnd = addUnit(event.To, event.Repeat.Interval*event.Repeat.Count, event.Repeat.Frequency)
			}
		}
		if originalEvent.From != event.From || oldEnd != newEnd { // update the eventTree
			ids, found := c.eventTree.Find(originalEvent.From, oldEnd)
			if found {
				index := slices.Index(ids, originalEvent.Id)
				if index != -1 {
					updated := slices.Delete(ids, index, index+1)
					if len(updated) == 0 {
						_ = c.eventTree.Delete(originalEvent.From, oldEnd)
					} else {
						_ = c.eventTree.Insert(originalEvent.From, oldEnd, updated)
					}
				}
			}
			newIds, _ := c.eventTree.Find(event.From, newEnd)
			newIds = append(newIds, event.Id)
			if err := c.eventTree.Insert(event.From, newEnd, newIds); err != nil {
				return nil, fmt.Errorf("failed to reinsert event into tree: %w", err)
			}
		}
		c.events[event.Id] = &event
		if err := c.saveEventToRepo(&event, fmt.Sprintf("CALENDAR: Updated event '%s'", event.Title)); err != nil {
			return nil, err
		}
		return &event, nil
	}
	return nil, fmt.Errorf("something went wrong, event was not updated")
}

// Removes an event from the calendar it belongs to.
// TODO update options?
func (c *Core) RemoveEvent(event Event) error {
	if err := event.Validate(); err != nil {
		return fmt.Errorf("invalid event: %w", err)
	}

	// real event, must be deleted entirely
	if event.MasterId == uuid.Nil {
		// find last slave and its To
		eventEnd := event.To
		if event.Repeat != nil {
			eventEnd = event.Repeat.Until
			if event.Repeat.Count >= 1 {
				eventEnd = addUnit(event.To, event.Repeat.Interval*event.Repeat.Count, event.Repeat.Frequency)
			}
		}

		// get the full interval
		ids, found := c.eventTree.Find(event.From, eventEnd)
		if !found {
			return fmt.Errorf("event not found in search tree")
		}

		// find index of our event
		index := slices.Index(ids, event.Id)
		if index == -1 {
			return errors.New("")
		}

		// delete event from interval
		updated := slices.Delete(ids, index, index+1)

		if len(updated) == 0 { // interval now empty -> delete from tree
			if err := c.eventTree.Delete(event.From, eventEnd); err != nil {
				return fmt.Errorf("failed to delete tree node: %w", err)
			}
		} else { // not empty -> overwrite
			if err := c.eventTree.Insert(event.From, eventEnd, updated); err != nil {
				return fmt.Errorf("failed to reinsert node into tree: %w", err)
			}
		}

		// delete file from disk + git
		err := c.deleteEventFromRepo(event.Id, fmt.Sprintf("CALENDAR: Delete event '%s'", event.Title))
		if err != nil {
			return fmt.Errorf("failed to delete event: %w", err)
		}

		delete(c.events, event.Id)
		return nil
	}

	// generated repeating event, must be added to repeat exceptions
	if event.MasterId != uuid.Nil {
		// get master event
		masterEvent := c.events[event.MasterId]
		if masterEvent == nil || masterEvent.Repeat == nil {
			return fmt.Errorf("master event not found")
		}

		// if exception doesn't exist yet
		if !containsTime(masterEvent.Repeat.Exceptions, event.From) {
			// add date to master exceptions
			newException := Exception{
				event.Id,
				event.From,
			}
			masterEvent.Repeat.Exceptions = append(masterEvent.Repeat.Exceptions, newException)

			// update/overwrite the file in repo
			err := c.saveEventToRepo(masterEvent, fmt.Sprintf("CALENDAR: Updated event '%s'", event.Title))
			if err != nil {
				return fmt.Errorf("failed to save event to repo: %w", err)
			}
		}

		delete(c.events, event.Id) // TODO is this needed? It's no-op, but can there be a generated event in events?
		return nil
	}

	return errors.New("something went wrong, event was not removed")
}

// Returns event by id, or an error if it doesn't exist.
func (c *Core) GetEvent(id uuid.UUID) (*Event, error) {
	e, ok := c.events[id]
	if !ok {
		return nil, fmt.Errorf("event with this id doesnt exist")
	}
	return e, nil
}

// Returns an array of events which fall into the specified interval.
func (c *Core) GetEvents(from, to time.Time) []Event {
	// query the interval tree
	intervalsMatched, found := c.eventTree.AllIntersections(from, to)
	if !found {
		return []Event{}
	}

	result := make([]Event, 0, len(intervalsMatched))

	for _, intersection := range intervalsMatched {
		for _, eId := range intersection {
			curEvent := c.events[eId]

			// if it doesn't repeat, just plain append to result
			if curEvent.Repeat == nil {
				result = append(result, *c.events[eId])
				continue
			}

			duration := curEvent.To.Sub(curEvent.From)
			tmpEventTime, index := getFirstCandidate(from, curEvent)

			for tmpEventTime.Before(to) { // while generated event fits in the wanted interval
				if tmpEventTime.Add(duration).Before(from) { // if the generated event ends before our wanted interval -> skip
					tmpEventTime = addUnit(tmpEventTime, 1, curEvent.Repeat.Frequency) // next occurrence
					continue
				}
				// logic when repeating until
				if curEvent.Repeat.Count == -1 && tmpEventTime.After(curEvent.Repeat.Until) {
					break // new event exceeded the repetition end (Until)
				}
				// logic for repeating only N times (count)
				if curEvent.Repeat.Count != -1 && index >= curEvent.Repeat.Count {
					break // new event exceeded the max count of generated events
				}

				index++
				generatedEvent := Event{
					Id:           uuid.New(),
					Title:        curEvent.Title,
					Location:     curEvent.Location,
					Description:  curEvent.Description,
					From:         tmpEventTime,
					OriginalFrom: tmpEventTime,
					To:           tmpEventTime.Add(duration),
					Calendar:     curEvent.Calendar,
					Tag:          curEvent.Tag,
					MasterId:     curEvent.Id,
					Repeat:       curEvent.Repeat, // TODO send the repeat struct
				}
				// ignore exceptions
				if !containsTime(curEvent.Repeat.Exceptions, tmpEventTime) {
					result = append(result, generatedEvent)
				}
				tmpEventTime = addUnit(tmpEventTime, 1, curEvent.Repeat.Frequency) // next occurrence
			}
		}
	}

	return result
}

// Update all repotes for all repositories.
func (c *Core) PushAll() error {
	// TODO idk if it works

	var err error
	for _, repo := range c.repos {
		errx := repo.Push(&gogit.PushOptions{})
		if errx == gogit.NoErrAlreadyUpToDate {
			continue // this is ok
		}
		if errx != nil {
			err = errors.Join(errx)
		}
	}
	return err
}

// Update all repositories from remotes.
func (c *Core) PullAll() error {
	// TODO idk if it works

	var err error
	for _, repo := range c.repos {
		wt, errx := repo.Worktree()
		if errx != nil || wt == nil { // only fails if repo is bare (aka. only .git/ folder exists, no files) which should not happen ever haha
			continue
		}

		errx = wt.Pull(&gogit.PullOptions{})
		if errx == gogit.NoErrAlreadyUpToDate {
			continue // this is ok
		}
		if errx != nil {
			err = errors.Join(errx)
		}
	}
	return err
}

// ------------------------------------------------ Helpers -------------------------------------------------

// Resets the Core internal variables and reallocates them.
func (c *Core) eraseAndAlloc() {
	c.eventTree = interval.NewSearchTree[[]uuid.UUID](
		func(x, y time.Time) int {
			return x.Compare(y)
		},
	)
	c.events = make(map[uuid.UUID]*Event)
	c.repos = make(map[string]*gogit.Repository)
}

// Loads, if exists, or creates new repository with the given name.
func (c *Core) initCalendarRepo(name string) (*gogit.Repository, error) {
	repoPath := c.fs.Join(filesystem.DirName, name)

	if err := c.fs.MkdirAll(repoPath, 0o755); err != nil {
		return nil, fmt.Errorf("create repo dir: %w", err)
	}

	repoFS, err := c.fs.Chroot(repoPath)
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

// Serializes event to JSON, saves to file, stages and commits with given message.
func (c *Core) saveEventToRepo(event *Event, commitMsg string) error {
	// marshal event
	data, err := json.MarshalIndent(event, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal event: %w", err)
	}

	// ensure events directory exists
	dirPath := c.fs.Join(filesystem.DirName, event.Calendar, EventsDirName)
	if err := c.fs.MkdirAll(dirPath, 0o755); err != nil {
		return fmt.Errorf("failed mkdir events: %w", err)
	}

	filename := fmt.Sprintf("%s.json", event.Id)
	filePath := c.fs.Join(dirPath, filename)

	// create truncates/overwrites the file if it exists
	file, err := c.fs.Create(filePath)
	if err != nil {
		return fmt.Errorf("failed to create file: %w", err)
	}

	// write JSON content
	if _, err := file.Write(data); err != nil {
		_ = file.Close()
		return fmt.Errorf("failed to write file: %w", err)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("failed to close file: %w", err)
	}

	if c.repos[event.Calendar] == nil {
		return fmt.Errorf("calendar repo not initialized")
	}

	// -------- add to git repo --------
	w, err := c.repos[event.Calendar].Worktree()
	if err != nil {
		return fmt.Errorf("failed to get worktree: %w", err)
	}

	// stage
	gitPath := filepath.ToSlash(c.fs.Join(EventsDirName, filename))
	if _, err := w.Add(gitPath); err != nil {
		return fmt.Errorf("git add: %w", err)
	}

	// commit
	_, err = w.Commit(commitMsg, &gogit.CommitOptions{
		Author: &object.Signature{
			Name:  "git-calendar",
			Email: "",
			When:  time.Now(),
		},
	})

	if err != nil && !errors.Is(err, gogit.ErrEmptyCommit) {
		return fmt.Errorf("failed to git commit: %w", err)
	}
	return nil
}

// Removes event from filesystem and commits the change.
func (c *Core) deleteEventFromRepo(eventId uuid.UUID, commitMsg string) error {
	event, ok := c.events[eventId]
	if !ok {
		return fmt.Errorf("failed to find event by id")
	}
	filename := fmt.Sprintf("%s.json", eventId)

	// -------- remove from filesystem --------
	filePath := c.fs.Join(filesystem.DirName, event.Calendar, EventsDirName, filename)
	if err := c.fs.Remove(filePath); err != nil {
		// TODO maybe continue, to clean the git from this file
		return fmt.Errorf("failed to remove file from disk: %w", err)
	}

	// -------- remove from git --------
	repo, ok := c.repos[event.Calendar]
	if !ok {
		return fmt.Errorf("calendar repo not initialized")
	}

	w, err := repo.Worktree()
	if err != nil {
		return fmt.Errorf("failed to get worktree: %w", err)
	}

	gitPath := filepath.ToSlash(c.fs.Join(EventsDirName, filename))

	if _, err := w.Remove(gitPath); err != nil {
		return fmt.Errorf("git remove: %w", err)
	}

	_, err = w.Commit(commitMsg, &gogit.CommitOptions{
		Author: &object.Signature{
			Name:  "git-calendar",
			Email: "",
			When:  time.Now(),
		},
	})
	if err != nil {
		return fmt.Errorf("failed to git commit: %w", err)
	}

	return nil
}

// TODO comment what id does
func (c *Core) rebuildTreeForEvent(master, updated *Event) error {
	oldEnd := master.To
	if master.Repeat != nil {
		oldEnd = master.Repeat.Until
		if master.Repeat.Count >= 1 {
			oldEnd = addUnit(master.To, master.Repeat.Interval*master.Repeat.Count, master.Repeat.Frequency)
		}
	}

	ids, found := c.eventTree.Find(master.From, oldEnd)
	if found {
		index := slices.Index(ids, master.Id)
		if index != -1 {
			ids = slices.Delete(ids, index, index+1)
			if len(ids) == 0 {
				_ = c.eventTree.Delete(master.From, oldEnd)
			} else {
				_ = c.eventTree.Insert(master.From, oldEnd, ids)
			}
		}
	}

	newEnd := updated.To
	if updated.Repeat != nil {
		newEnd = updated.Repeat.Until
		if updated.Repeat.Count >= 1 {
			newEnd = addUnit(updated.To, updated.Repeat.Interval*updated.Repeat.Count, updated.Repeat.Frequency)
		}
	}

	newIds, _ := c.eventTree.Find(updated.From, newEnd)
	newIds = append(newIds, master.Id) // add the master id
	if err := c.eventTree.Insert(updated.From, newEnd, newIds); err != nil {
		return fmt.Errorf("failed to reinsert event into tree: %w", err)
	}
	return nil
}
