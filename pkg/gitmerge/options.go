package gitmerge

import (
	"errors"
	"time"
)

type UpdatedAtFunc func(gitPath string, data []byte) (time.Time, error)

type IncludePathFunc func(gitPath string) bool

type Options struct {
	BranchName string
	RemoteName string

	AuthorName  string
	AuthorEmail string

	IncludePath IncludePathFunc
	UpdatedAt   UpdatedAtFunc

	Now func() time.Time
}

func (o Options) validate() error {
	if o.BranchName == "" {
		return errors.New("branch name is required")
	}
	if o.RemoteName == "" {
		return errors.New("remote name is required")
	}
	if o.IncludePath == nil {
		return errors.New("include path function is required")
	}
	if o.UpdatedAt == nil {
		return errors.New("updated-at function is required")
	}
	return nil
}
