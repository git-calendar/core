package core

import (
	"errors"
	"fmt"
	"os"
	"slices"
	"time"

	gogit "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/google/uuid"
)

// CreateTag saves one tag into a calendar repository.
func (c *Core) CreateTag(calendar string, tag Tag) (*Tag, error) {
	if err := tag.Validate(); err != nil {
		return nil, fmt.Errorf("invalid tag: %w", err)
	}

	cal, wt, err := c.tagWorktree(calendar)
	if err != nil {
		return nil, err
	}

	if slices.ContainsFunc(cal.Tags, func(t Tag) bool {
		return t.Id == tag.Id
	}) {
		return nil, fmt.Errorf("tag already exists")
	}

	gitPath := tag.getPath()
	if err := writeTagFile(wt, cal.EncryptionKey, tag); err != nil {
		return nil, err
	}
	if _, err := wt.Add(gitPath); err != nil {
		return nil, fmt.Errorf("failed to git add %q: %w", gitPath, err)
	}

	cal.Tags = append(cal.Tags, tag)

	return &tag, commitWorktree(wt, fmt.Sprintf("Created tag %s", tag.Id))
}

// RemoveTag deletes one tag from a calendar repository.
func (c *Core) RemoveTag(calendar string, id uuid.UUID) error {
	if id == uuid.Nil {
		return errors.New("invalid tag id")
	}

	cal, wt, err := c.tagWorktree(calendar)
	if err != nil {
		return err
	}

	gitPath := Tag{Id: id}.getPath()
	if _, err := wt.Remove(gitPath); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("failed to git remove %q: %w", gitPath, err)
	}

	cal.Tags = slices.DeleteFunc(cal.Tags, func(tag Tag) bool {
		return tag.Id == id
	})

	return commitWorktree(wt, fmt.Sprintf("Deleted tag %s", id))
}

// ------------------------------------------------ Helpers -------------------------------------------------

func (c *Core) tagWorktree(calendar string) (*Calendar, *gogit.Worktree, error) {
	cal := c.calendars[calendar]
	if cal == nil {
		return nil, nil, fmt.Errorf("invalid calendar %q", calendar)
	}
	if cal.repository == nil {
		return nil, nil, fmt.Errorf("invalid calendar %q: no repository", calendar)
	}

	wt, err := cal.repository.Worktree()
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get worktree: %w", err)
	}

	return cal, wt, nil
}

func writeTagFile(wt *gogit.Worktree, key []byte, tag Tag) error {
	if err := wt.Filesystem.MkdirAll(TagsDirName, 0o755); err != nil {
		return fmt.Errorf("failed to create tags dir %q: %w", TagsDirName, err)
	}

	gitPath := tag.getPath()
	file, err := wt.Filesystem.Create(gitPath)
	if err != nil {
		return fmt.Errorf("failed to create tag %q: %w", gitPath, err)
	}
	defer file.Close()

	if err := tag.WriteToFile(file, key); err != nil {
		return fmt.Errorf("failed to write tag %q: %w", gitPath, err)
	}

	return nil
}

func commitWorktree(wt *gogit.Worktree, message string) error {
	_, err := wt.Commit(message, &gogit.CommitOptions{
		Author: &object.Signature{
			Name:  GitAuthorName,
			Email: "",
			When:  time.Now(),
		},
	})
	if err != nil && !errors.Is(err, gogit.ErrEmptyCommit) {
		return fmt.Errorf("failed to git commit: %w", err)
	}

	return nil
}
