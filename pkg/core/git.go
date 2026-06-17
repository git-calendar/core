package core

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"path"
	"time"

	"github.com/git-calendar/core/pkg/gitmerge"
	gogit "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
)

func pushCalendar(cal *Calendar, proxyUrl *url.URL) error {
	repoUrl, err := repoUrlFromCalendar(cal)
	if err != nil {
		return err
	}

	fmt.Println("pushing", cal.Name)

	finalUrl, auth := prepareRepoUrl(repoUrl, proxyUrl)
	return ignoreUpToDate(cal.repository.Push(&gogit.PushOptions{
		RemoteName: GitRemoteName,
		RemoteURL:  finalUrl.String(),
		Auth:       auth,
	}))
}

func fetchCalendar(cal *Calendar, proxyUrl *url.URL) error {
	repoUrl, err := repoUrlFromCalendar(cal)
	if err != nil {
		return err
	}

	fmt.Println("fetching", cal.Name)

	finalUrl, auth := prepareRepoUrl(repoUrl, proxyUrl)
	return ignoreUpToDate(cal.repository.Fetch(&gogit.FetchOptions{
		RemoteName: GitRemoteName,
		RemoteURL:  finalUrl.String(),
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
func mergeOriginMain(repo *gogit.Repository) error {
	return gitmerge.MergeRemoteIntoBranch(repo, gitmerge.Options{
		BranchName: GitBranchName,
		RemoteName: GitRemoteName,

		AuthorName: GitAuthorName,

		IncludePath: func(gitPath string) bool {
			return path.Dir(gitPath) == EventsDirName &&
				path.Ext(gitPath) == ".json"
		},

		UpdatedAt: func(gitPath string, data []byte) (time.Time, error) {
			var ev Event
			if err := json.Unmarshal(data, &ev); err != nil {
				return time.Time{}, fmt.Errorf("%s: failed to parse event JSON: %w", gitPath, err)
			}

			return ev.UpdatedAt, nil
		},
	})
}
