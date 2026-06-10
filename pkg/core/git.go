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
	err = cal.repository.Push(&gogit.PushOptions{
		RemoteName: GitRemoteName,
		RemoteURL:  finalUrl.String(),
		Auth:       auth,
	})
	if err != nil {
		if errors.Is(err, gogit.NoErrAlreadyUpToDate) {
			return nil // this is ok
		}
		return err
	}

	return nil
}

func fetchCalendar(cal *calendar, proxyUrl *url.URL) error {
	fmt.Println("fetching", cal.Name)

	repoUrl, err := repoUrlFromCalendar(cal)
	if err != nil {
		return err
	}

	finalUrl, auth := prepareRepoUrl(repoUrl, proxyUrl)
	err = cal.repository.Fetch(&gogit.FetchOptions{
		RemoteName: GitRemoteName,
		RemoteURL:  finalUrl.String(),
		Auth:       auth,
	})
	if err != nil {
		if errors.Is(err, gogit.NoErrAlreadyUpToDate) {
			return nil // this is ok
		}
		return err
	}

	return nil
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

// ---------------------------------------------------------------------------------------------------

func localMainRef(repo *gogit.Repository) (*plumbing.Reference, error) {
	refName := plumbing.NewBranchReferenceName(GitBranchName)

	ref, err := repo.Reference(refName, true)
	if err != nil {
		return nil, fmt.Errorf("local ref %s: %w", refName, err)
	}

	return ref, nil
}

func remoteMainRef(repo *gogit.Repository) (*plumbing.Reference, error) {
	refName := plumbing.NewRemoteReferenceName(GitRemoteName, GitBranchName)

	ref, err := repo.Reference(refName, true)
	if err != nil {
		return nil, fmt.Errorf("remote ref %s: %w", refName, err)
	}

	return ref, nil
}

func isAncestor(repo *gogit.Repository, ancestorHash, descendantHash plumbing.Hash) bool {
	if ancestorHash == descendantHash {
		return true
	}

	ancestorCommit, err := repo.CommitObject(ancestorHash)
	if err != nil {
		fmt.Println(err)
		return false
	}

	descendantCommit, err := repo.CommitObject(descendantHash)
	if err != nil {
		fmt.Println(err)
		return false
	}

	ok, err := ancestorCommit.IsAncestor(descendantCommit)
	if err != nil {
		fmt.Println(err)
		return false
	}

	return ok
}
