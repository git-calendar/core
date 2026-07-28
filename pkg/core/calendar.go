package core

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path"
	"strings"

	gogit "github.com/go-git/go-git/v5"
)

type Calendar struct {
	Name          string
	Tags          []Tag
	EncryptionKey []byte
	Readonly      bool
	ICalURL       *url.URL

	repository *gogit.Repository
}

func (cal *Calendar) Validate() error {
	if cal == nil {
		return nil
	}
	if cal.Name == "" {
		return errors.New("calendar name cannot be empty")
	}
	for _, t := range cal.Tags {
		if err := t.Validate(); err != nil {
			return err
		}
	}
	return nil
}

func (cal *Calendar) IsEncrypted() bool {
	return len(cal.EncryptionKey) != 0
}

func (cal *Calendar) RemoteURL() (string, error) {
	if cal.repository == nil {
		return "", nil
	}

	remote, err := cal.repository.Remote(GitRemoteName)
	if err != nil {
		return "", nil
	}

	cfg := remote.Config()
	if cfg == nil {
		return "", fmt.Errorf("remote for calendar %q has no config", cal.Name)
	}

	if len(cfg.URLs) != 1 || cfg.URLs[0] == "" {
		return "", fmt.Errorf("remote for calendar %q must have exactly one non-empty URL, got %d", cal.Name, len(cfg.URLs))
	}

	return cfg.URLs[0], nil
}

func (cal *Calendar) MarshalJSON() ([]byte, error) {
	type calendarJSON struct {
		Name      string `json:"name"`
		Tags      []Tag  `json:"tags"`
		RemoteURL string `json:"remote_url"`
		ICalUrl   string `json:"ical_url"`
		Encrypted bool   `json:"encrypted"`
		Readonly  bool   `json:"readonly"`
	}

	remoteURL, err := cal.RemoteURL()
	if err != nil {
		return nil, err
	}

	var icalURL string
	if cal.ICalURL != nil {
		icalURL = cal.ICalURL.String()
	}

	return json.Marshal(calendarJSON{
		Name:      cal.Name,
		Tags:      cal.Tags,
		RemoteURL: remoteURL,
		ICalUrl:   icalURL,
		Encrypted: cal.IsEncrypted(),
		Readonly:  cal.Readonly,
	})
}

func (cal *Calendar) LoadTags() error {
	wt, err := cal.repository.Worktree()
	if err != nil {
		return fmt.Errorf("failed to get worktree: %w", err)
	}

	tags, err := readTagFiles(wt, cal.EncryptionKey)
	if err != nil {
		return fmt.Errorf("failed to load tags: %w", err)
	}

	cal.Tags = tags

	return nil
}

func readTagFiles(wt *gogit.Worktree, key []byte) ([]Tag, error) {
	entries, err := wt.Filesystem.ReadDir(TagsDirName)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to read tags dir %q: %w", TagsDirName, err)
	}

	var tags []Tag

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}

		gitPath := path.Join(TagsDirName, entry.Name())
		file, err := wt.Filesystem.Open(gitPath)
		if err != nil {
			return nil, fmt.Errorf("failed to open tag %q: %w", gitPath, err)
		}
		defer file.Close()

		var tag Tag
		if err := tag.LoadFromFile(file, key); err != nil {
			return nil, fmt.Errorf("failed to load tag %q: %w", gitPath, err)
		}

		tags = append(tags, tag)
	}

	return tags, nil
}
