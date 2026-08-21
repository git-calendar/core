package core

import (
	"errors"
	"fmt"
	"net/url"
	"path"
	"time"

	"github.com/git-calendar/core/pkg/errcode"
	"github.com/git-calendar/core/pkg/gitmerge"
	gogit "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/go-git/go-git/v5/plumbing/transport"
)

func pushCalendar(cal *Calendar, proxyURL *url.URL) error {
	repoURL, err := repoURLFromCalendar(cal)
	if err != nil {
		return errcode.Wrap(errcode.Validation, err)
	}

	fmt.Println("pushing", cal.Name)

	finalURL, auth := prepareRepoURL(repoURL, proxyURL)
	return wrapGitRemoteError(cal.repository.Push(&gogit.PushOptions{
		RemoteName: GitRemoteName,
		RemoteURL:  finalURL.String(),
		Auth:       auth,
	}))
}

func fetchCalendar(cal *Calendar, proxyURL *url.URL) error {
	repoURL, err := repoURLFromCalendar(cal)
	if err != nil {
		return errcode.Wrap(errcode.Validation, err)
	}

	fmt.Println("fetching", cal.Name)

	finalURL, auth := prepareRepoURL(repoURL, proxyURL)
	return wrapGitRemoteError(cal.repository.Fetch(&gogit.FetchOptions{
		RemoteName: GitRemoteName,
		RemoteURL:  finalURL.String(),
		Auth:       auth,
	}))
}

func fastForwardCalendar(cal *Calendar, hash plumbing.Hash) error {
	fmt.Println("fast-forward", cal.Name)

	wt, err := cal.repository.Worktree()
	if err != nil {
		return err
	}

	if err := wt.Checkout(&gogit.CheckoutOptions{
		Branch: plumbing.NewBranchReferenceName(GitBranchName),
	}); err != nil {
		return fmt.Errorf("failed to checkout %s: %w", GitBranchName, err)
	}

	if err := wt.Reset(&gogit.ResetOptions{
		Commit: hash,
		Mode:   gogit.HardReset,
	}); err != nil {
		return fmt.Errorf("failed to fast-forward %s to %s: %w", GitBranchName, hash, err)
	}

	return nil
}

func wrapGitRemoteError(err error) error {
	err = ignoreUpToDate(err)
	if err == nil {
		return nil
	}

	code := errcode.Network
	switch {
	case errors.Is(err, transport.ErrAuthenticationRequired):
		code = errcode.Auth
	case errors.Is(err, transport.ErrAuthorizationFailed):
		code = errcode.Forbidden
	}
	return errcode.Wrap(code, err)
}

// ignoreUpToDate swallows the benign "already up to date" error from push/fetch.
func ignoreUpToDate(err error) error {
	if errors.Is(err, gogit.NoErrAlreadyUpToDate) {
		return nil
	}
	return err
}

// isAncestor reports whether a is an ancestor of (or equal to) b.
func isAncestor(a, b *object.Commit) bool {
	if a.Hash == b.Hash {
		return true
	}
	ok, err := a.IsAncestor(b)
	if err != nil {
		fmt.Printf("WARN: isAncestor %s -> %s: %v\n", a.Hash, b.Hash, err)
		return false
	}
	return ok
}

// mergeOriginMain is an "adapter" for gitmerge.MergeRemoteIntoBranch.
func mergeOriginMain(repo *gogit.Repository, calendar string, encryptionKey []byte) error {
	return gitmerge.MergeRemoteIntoBranch(repo, gitmerge.Options{
		BranchName: GitBranchName,
		RemoteName: GitRemoteName,
		AuthorName: GitAuthorName,
		IncludePath: func(gitPath string) bool {
			dir := path.Dir(gitPath)
			return (dir == EventsDirName || dir == TagsDirName) && path.Ext(gitPath) == ".json"
		},
		UpdatedAt: func(gitPath string, data []byte) (time.Time, error) {
			dir := path.Dir(gitPath)
			base := path.Base(gitPath)

			switch dir {
			case EventsDirName:
				var ev Event
				if err := ev.LoadFromBytes(data, base, calendar, encryptionKey); err != nil {
					return time.Time{}, err
				}
				return ev.UpdatedAt, nil

			case TagsDirName:
				var tg Tag
				if err := tg.LoadFromBytes(data, base, encryptionKey); err != nil {
					return time.Time{}, err
				}
				return tg.UpdatedAt, nil

			default:
				return time.Time{}, fmt.Errorf("unsupported directory: %s", dir)
			}
		},
	})
}
