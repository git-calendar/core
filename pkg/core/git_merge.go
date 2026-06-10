package core

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path"
	"strings"
	"time"

	"github.com/go-git/go-billy/v5"
	"github.com/go-git/go-billy/v5/util"
	gogit "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
)

func customMerge(cal *calendar, proxyUrl *url.URL) error {
	if cal == nil {
		return errors.New("cal is nil")
	}

	wt, err := cal.repository.Worktree()
	if err != nil {
		return err
	}

	localBranch := plumbing.NewBranchReferenceName(GitBranchName)
	head, err := cal.repository.Head()
	if err != nil {
		return err
	}

	// checkout main (just to be sure)
	if head.Name() != localBranch {
		if err := wt.Checkout(&gogit.CheckoutOptions{
			Branch: localBranch,
		}); err != nil {
			return fmt.Errorf("checkout %s: %w", localBranch, err)
		}
	}

	// get local HEAD and remote Ref
	localHead, err := cal.repository.Head()
	if err != nil {
		return err
	}
	remoteRef, err := cal.repository.Reference(plumbing.NewRemoteReferenceName(GitRemoteName, GitBranchName), true)
	if err != nil {
		return fmt.Errorf("could not find remote ref: %w", err)
	}

	if localHead.Hash() == remoteRef.Hash() { // if same hash, no need to merge
		return nil
	}

	remoteCommit, err := cal.repository.CommitObject(remoteRef.Hash())
	if err != nil {
		return err
	}
	remoteTree, err := remoteCommit.Tree()
	if err != nil {
		return err
	}

	// process changes file by file
	err = remoteTree.Files().ForEach(func(f *object.File) error {
		remotePath := path.Clean(f.Name)

		// merge based on file type (parent dir)
		switch path.Dir(remotePath) {
		case EventsDirName:
			return mergeEventFile(wt.Filesystem, remotePath, f)
		default:
			fmt.Printf("skipping %q merge...\n", f.Name)
			return nil // TODO: merge index.json, ... ?
		}
	})
	if err != nil {
		return fmt.Errorf("failed during tree traversal: %w", err)
	}

	// commit the merge
	if _, err = wt.Add(EventsDirName); err != nil { // TODO: stage only touched files
		return fmt.Errorf("failed to stage merged events: %w", err)
	}

	commitMsg := fmt.Sprintf("Merge remote-tracking branch '%s/%s' (LWW resolution)", GitRemoteName, GitBranchName)
	_, err = wt.Commit(commitMsg, &gogit.CommitOptions{
		Parents: []plumbing.Hash{localHead.Hash(), remoteRef.Hash()},
		Author: &object.Signature{
			Name:  GitAuthorName,
			Email: "",
			When:  time.Now(),
		},
		AllowEmptyCommits: true,
	})

	return err
}

func mergeEventFile(repoFs billy.Filesystem, remotePath string, f *object.File) error {
	if path.Dir(remotePath) != EventsDirName {
		return nil
	}
	if path.Ext(remotePath) != ".json" {
		return nil
	}
	localFilePath := repoFs.Join(strings.Split(remotePath, "/")...) // let fs decide on the path format just in case

	// read contents
	remoteReader, err := f.Reader()
	if err != nil {
		return fmt.Errorf("%s: failed to open remote blob as reader: %w", remotePath, err)
	}
	defer remoteReader.Close()

	remoteData, err := io.ReadAll(remoteReader)
	if err != nil {
		return fmt.Errorf("%s: failed to read remote blob: %w", remotePath, err)
	}

	// if file doesn't exist locally, take the remote one
	if _, err := repoFs.Stat(localFilePath); os.IsNotExist(err) {
		if os.IsNotExist(err) {
			// make sure events/ exists
			if err := repoFs.MkdirAll(EventsDirName, 0o755); err != nil {
				return fmt.Errorf("%s: failed to create events dir: %w", remotePath, err)
			}
			if err := util.WriteFile(repoFs, localFilePath, remoteData, 0o644); err != nil {
				return fmt.Errorf("%s: failed to write remote event: %w", remotePath, err)
			}
			return nil
		}

		return fmt.Errorf("%s: failed to stat local event: %w", remotePath, err)
	}

	// else collision
	localData, err := util.ReadFile(repoFs, localFilePath)
	if err != nil {
		return fmt.Errorf("%s: failed to read local event: %w", remotePath, err)
	}

	// parse json events
	var localEvent, remoteEvent Event
	if err := json.Unmarshal(localData, &localEvent); err != nil {
		return fmt.Errorf("%s: failed to parse local event: %w", remotePath, err)
	}
	if err := json.Unmarshal(remoteData, &remoteEvent); err != nil {
		return fmt.Errorf("%s: failed to parse remote event: %w", remotePath, err)
	}

	// latest update wins
	if remoteEvent.UpdatedAt.After(localEvent.UpdatedAt) {
		if err := util.WriteFile(repoFs, localFilePath, remoteData, 0o644); err != nil {
			return fmt.Errorf("%s: failed to write newer remote event: %w", remotePath, err)
		}
	}

	return nil
}
