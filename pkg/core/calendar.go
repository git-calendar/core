package core

import (
	gogit "github.com/go-git/go-git/v5"
)

type Calendar struct {
	Repository    *gogit.Repository
	Tags          []string
	EncryptionKey []byte
}

func (cal *Calendar) IsEncrypted() bool {
	return len(cal.EncryptionKey) != 0
}
