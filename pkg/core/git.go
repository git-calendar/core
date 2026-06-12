package core

import (
	"errors"
	"fmt"
	"net/url"

	gogit "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
)

func pushCalendar(cal *calendar, proxyUrl *url.URL) error {
	fmt.Println("pushing", cal.Name)

	repoUrl, err := repoUrlFromCalendar(cal)
	if err != nil {
		return err
	}

	finalUrl, auth := prepareRepoUrl(repoUrl, proxyUrl)
	return ignoreUpToDate(cal.repository.Push(&gogit.PushOptions{
		RemoteName: GitRemoteName,
		RemoteURL:  finalUrl.String(),
		Auth:       auth,
	}))
}

func fetchCalendar(cal *calendar, proxyUrl *url.URL) error {
	fmt.Println("fetching", cal.Name)

	repoUrl, err := repoUrlFromCalendar(cal)
	if err != nil {
		return err
	}

	finalUrl, auth := prepareRepoUrl(repoUrl, proxyUrl)
	return ignoreUpToDate(cal.repository.Fetch(&gogit.FetchOptions{
		RemoteName: GitRemoteName,
		RemoteURL:  finalUrl.String(),
		Auth:       auth,
	}))
}

func fastForwardCalendar(cal *calendar, hash plumbing.Hash) error {
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
