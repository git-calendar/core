package core

import (
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
	Remotes   []*url.URL `json:"remotes,omitempty"`
}

type Tag struct {
	Name  string `json:"name,omitzero"`
	Color string `json:"color,omitzero"`
}
