package core

import (
	"errors"

	gogit "github.com/go-git/go-git/v5"
)

type Calendar struct {
	Name          string `json:"name"`
	Tags          []Tag  `json:"tags"`
	EncryptionKey []byte `json:"encryption_key"`

	repository *gogit.Repository
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

type Tag struct {
	Name  string `json:"name,omitzero"`
	Color string `json:"color,omitzero"`
}
