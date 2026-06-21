package core

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"

	gogit "github.com/go-git/go-git/v5"
)

type metadata struct {
	Tags      []Tag `json:"tags"`
	Encrypted bool  `json:"encrypted"`
}

func (m *metadata) Load(repo *gogit.Repository) error {
	if repo == nil {
		return errors.New("repo can't be nil")
	}

	wt, err := repo.Worktree()
	if err != nil {
		return fmt.Errorf("failed to get the worktree: %w\n", err)
	}

	metaFile, err := wt.Filesystem.Open(MetadataFileName)
	if err != nil {
		return fmt.Errorf("failed to open metadata file: %w", err)
	}

	data, err := io.ReadAll(metaFile)
	if err != nil {
		return fmt.Errorf("failed to read metadata file: %w", err)
	}

	return json.Unmarshal(data, m)
}

func (m metadata) Save(repo *gogit.Repository) error {
	if repo == nil {
		return errors.New("repo can't be nil")
	}

	wt, err := repo.Worktree()
	if err != nil {
		return fmt.Errorf("failed to get the worktree: %w\n", err)
	}

	metaFile, err := wt.Filesystem.Create(MetadataFileName)
	if err != nil {
		return fmt.Errorf("failed to create metadata file: %w", err)
	}

	data, err := json.Marshal(m)
	if err != nil {
		return err
	}

	if _, err := metaFile.Write(data); err != nil {
		return fmt.Errorf("failed to write to metadata file: %w", err)
	}

	return nil
}
