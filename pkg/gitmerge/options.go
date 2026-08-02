package gitmerge

import (
	"errors"
	"time"
)

// UpdatedAtFunc extracts a file's logical update time for merge comparison.
type UpdatedAtFunc func(gitPath string, data []byte) (time.Time, error)

// IncludePathFunc reports whether a Git path participates in the merge.
type IncludePathFunc func(gitPath string) bool

// Options configures a last-write-wins merge.
type Options struct {
	// BranchName identifies the local branch to merge into.
	BranchName string
	// RemoteName identifies the remote-tracking branch namespace.
	RemoteName string

	// AuthorName is recorded on the merge commit.
	AuthorName string
	// AuthorEmail is recorded on the merge commit.
	AuthorEmail string

	// IncludePath selects paths that participate in content merging.
	IncludePath IncludePathFunc
	// UpdatedAt extracts timestamps used to resolve concurrent changes.
	UpdatedAt UpdatedAtFunc

	// Now returns the timestamp recorded on merge commits. It defaults to time.Now.
	Now func() time.Time
}

func (o Options) now() time.Time {
	if o.Now != nil {
		return o.Now()
	}
	return time.Now()
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
