package core

import (
	"encoding/json"
	"errors"
	"fmt"
	"slices"

	gogit "github.com/go-git/go-git/v5"
)

type Calendar struct {
	Name          string
	Tags          []Tag
	EncryptionKey []byte
	Readonly      bool

	repository *gogit.Repository
}

type Tag struct {
	Name  string `json:"name,omitzero"`
	Color string `json:"color,omitzero"`
}

func (cal *Calendar) Validate() error {
	if cal == nil {
		return nil
	}
	if cal.Name == "" {
		return errors.New("name cannot be empty")
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
		Tags      []Tag  `json:"tags,omitempty"`
		RemoteURL string `json:"remote_url,omitempty"`
		Encrypted bool   `json:"encrypted"`
		Readonly  bool   `json:"readonly"`
	}

	remoteURL, err := cal.RemoteURL()
	if err != nil {
		return nil, err
	}

	return json.Marshal(calendarJSON{
		Name:      cal.Name,
		Tags:      slices.Clone(cal.Tags),
		RemoteURL: remoteURL,
		Encrypted: cal.IsEncrypted(),
		Readonly:  cal.Readonly,
	})
}
