package core

import (
	"errors"
	"fmt"
	"os"
	"path"
	"slices"
	"strings"
	"time"

	gogit "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/google/uuid"
)

// CreateTag saves one tag into a calendar repository.
func (c *Core) CreateTag(calendar string, tag Tag) error {
	if err := tag.Validate(); err != nil {
		return fmt.Errorf("invalid tag: %w", err)
	}

	cal, wt, err := c.tagWorktree(calendar)
	if err != nil {
		return err
	}

	if slices.ContainsFunc(cal.Tags, func(t Tag) bool {
		return t.Id == tag.Id
	}) {
		return fmt.Errorf("tag already exists: %w", err)
	}

	gitPath := tag.getPath()
	if err := writeTagFile(wt, cal.EncryptionKey, tag); err != nil {
		return err
	}
	if _, err := wt.Add(gitPath); err != nil {
		return fmt.Errorf("failed to git add %q: %w", gitPath, err)
	}

	cal.Tags = append(cal.Tags, tag)

	return commitWorktree(wt, fmt.Sprintf("Updated tag %s", tag.Id))
}

// LoadTags loads tags for selected calendars. If none are passed, all calendars are loaded.
func (c *Core) LoadTags(calendars ...string) error {
	if len(calendars) == 0 {
		for calendar := range c.calendars {
			calendars = append(calendars, calendar)
		}
	}

	for _, calendar := range calendars {
		cal, wt, err := c.tagWorktree(calendar)
		if err != nil {
			return err
		}

		tags, err := readTagFiles(wt, cal.EncryptionKey)
		if err != nil {
			return fmt.Errorf("failed to load tags for calendar %q: %w", calendar, err)
		}

		cal.Tags = tags
	}

	return nil
}

// DeleteTag deletes one tag from a calendar repository.
func (c *Core) DeleteTag(calendar string, id uuid.UUID) error {
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
