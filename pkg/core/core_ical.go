package core

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"strings"
	"time"

	"github.com/go-git/go-billy/v5/util"
	"github.com/google/uuid"
)

// ImportICalFile imports events once and saves them like normal events.
func (c *Core) ImportICalFile(calendar string, tagId *uuid.UUID, r io.Reader) error {
	cal, ok := c.calendars[calendar]
	if !ok {
		return fmt.Errorf("calendar not found: %s", calendar)
	}
	if cal.Readonly {
		return errors.New("the specified calendar is read-only")
	}

	events, err := parseICal(r, calendar, false)
	if err != nil {
		return err
	}

	if tagId != nil && *tagId != uuid.Nil {
		for i := range events {
			events[i].TagId = tagId
		}
	}

	for i, event := range events {
		if _, err := c.CreateEvent(event); err != nil {
			return fmt.Errorf("save imported event %d: %w", i+1, err)
		}
	}

	return nil
}

// ImportICalURL creates a read-only calendar and caches its feed.
func (c *Core) ImportICalURL(name string, sourceURL *url.URL) error {
	if err := validateICalURL(sourceURL); err != nil {
		return err
	}
	if err := validateICalName(name); err != nil {
		return err
	}
	if _, exists := c.calendars[name]; exists {
		return fmt.Errorf("calendar named %s already exists", name)
	}
	if _, err := c.fs.Stat(name); err == nil {
		return fmt.Errorf("file named %s already exists", name)
	} else if !os.IsNotExist(err) {
		return err
	}

	fileName := name + ICalURLFileSuffix
	if _, err := c.fs.Stat(fileName); err == nil {
		return fmt.Errorf("file named %s already exists", fileName)
	} else if !os.IsNotExist(err) {
		return err
	}
	cacheName := name + ICalFileSuffix
	if _, err := c.fs.Stat(cacheName); err == nil {
		return fmt.Errorf("file named %s already exists", cacheName)
	} else if !os.IsNotExist(err) {
		return err
	}

	file, err := c.fs.Create(fileName)
	if err != nil {
		return fmt.Errorf("create iCalendar URL file: %w", err)
	}
	if _, err := file.Write([]byte(sourceURL.String())); err != nil {
		file.Close()
		_ = c.fs.Remove(fileName)
		return fmt.Errorf("write iCalendar URL file: %w", err)
	}
	if err := file.Close(); err != nil {
		_ = c.fs.Remove(fileName)
		return fmt.Errorf("close iCalendar URL file: %w", err)
	}

	c.calendars[name] = &Calendar{Name: name, Readonly: true, ICalURL: sourceURL}
	if err := c.fetchICalURL(name, sourceURL); err != nil {
		fmt.Printf("WARN: failed to fetch iCalendar %q: %v\n", name, err)
		return nil
	}
	if err := c.loadICalFile(name); err != nil {
		fmt.Printf("WARN: failed to load cached iCalendar %q: %v\n", name, err)
	}
	return nil
}

func (c *Core) readICalURL(name string) (*url.URL, error) {
	file, err := c.fs.Open(name)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	data, err := io.ReadAll(file)
	if err != nil {
		return nil, err
	}

	sourceURL, err := url.ParseRequestURI(strings.TrimSpace(string(data)))
	if err != nil {
		return nil, err
	}
	if err := validateICalURL(sourceURL); err != nil {
		return nil, err
	}
	return sourceURL, nil
}

func (c *Core) fetchICalURL(name string, sourceURL *url.URL) error {
	requestURL := sourceURL
	if c.proxyUrl != nil {
		requestURL = useCorsProxy(sourceURL, c.proxyUrl)
	}
	if requestURL == nil {
		return errors.New("invalid proxied iCalendar URL")
	}

	fmt.Println("fetching ical", name)

	client := http.Client{Timeout: 30 * time.Second}
	response, err := client.Get(requestURL.String())
	if err != nil {
		return err
	}
	defer response.Body.Close()

	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return fmt.Errorf("fetch iCalendar: %s", response.Status)
	}

	data, err := io.ReadAll(response.Body)
	if err != nil {
		return err
	}
	if _, err := parseICal(bytes.NewReader(data), name, true); err != nil {
		return err
	}
	if err := util.WriteFile(c.fs, name+ICalFileSuffix, data, 0o644); err != nil {
		return fmt.Errorf("cache iCalendar: %w", err)
	}
	return nil
}

func (c *Core) loadICalFile(name string) error {
	file, err := c.fs.Open(name + ICalFileSuffix)
	if err != nil {
		return err
	}
	defer file.Close()

	events, err := parseICal(file, name, true)
	if err != nil {
		return err
	}

	seen := make(map[uuid.UUID]struct{}, len(events))
	for _, event := range events {
		_, alreadyLoaded := c.events[event.Id]
		_, duplicate := seen[event.Id]
		if alreadyLoaded || duplicate {
			fmt.Printf("WARN: duplicate imported event ID %q\n", event.Id)
			continue
		}
		seen[event.Id] = struct{}{}
	}

	for i := range events {
		event := &events[i]
		if err := c.intervalTree.InsertEvent(*event); err != nil {
			return err
		}
		c.events[event.Id] = event
	}

	return nil
}

func validateICalURL(sourceURL *url.URL) error {
	if sourceURL == nil || sourceURL.Host == "" ||
		(sourceURL.Scheme != "http" && sourceURL.Scheme != "https") {
		return errors.New("iCalendar URL must be an absolute HTTP or HTTPS URL")
	}
	if !strings.HasSuffix(sourceURL.Path, ".ics") {
		return errors.New(`iCalendar URL must end with ".ics"`)
	}
	return nil
}

func validateICalName(name string) error {
	if name == "" || name == "." || path.Base(name) != name {
		return errors.New("iCalendar name must be a single file name")
	}
	return nil
}
