package core

import (
	"encoding/json"
	"errors"
	"net/url"

	gogit "github.com/go-git/go-git/v5"
)

type calendar struct {
	Name          string
	Tags          []Tag
	EncryptionKey []byte

	repository *gogit.Repository
}

func (cal *calendar) Validate() error {
	if cal == nil {
		return nil
	}
	if cal.Name == "" {
		return errors.New("name cannot be empty")
	}
	return nil
}

func (cal *calendar) IsEncrypted() bool {
	return len(cal.EncryptionKey) != 0
}

// A DTO like calendar struct.
type CalendarInfo struct {
	Name      string     `json:"name"`
	Tags      []Tag      `json:"tags,omitempty"`
	Encrypted bool       `json:"encrypted"`
	Remotes   RemoteUrls `json:"remotes,omitempty"`
}

type Tag struct {
	Name  string `json:"name,omitzero"`
	Color string `json:"color,omitzero"`
}

type RemoteUrls []*url.URL // custom type that ensures urls are strings in json, not objects

func (r RemoteUrls) MarshalJSON() ([]byte, error) {
	remotes := make([]string, 0, len(r))

	for _, remote := range r {
		if remote == nil {
			continue
		}
		remotes = append(remotes, remote.String())
	}

	return json.Marshal(remotes)
}
