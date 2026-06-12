package core

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"path"
	"sort"
	"strings"
	"time"

	"github.com/go-git/go-billy/v5"
	"github.com/go-git/go-billy/v5/util"
	gogit "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
)

// mergeOriginMain performs a 3-way last-write-wins merge of origin/main -> main.
func mergeOriginMain(repo *gogit.Repository) error {
	if repo == nil {
		return errors.New("repository is nil")
	}

	// get worktree with main branch
	wt, err := ensureBranch(repo, GitBranchName)
	if err != nil {
		return err
	}

	localCommit, remoteCommit, err := getCommits(repo)
	if err != nil {
		return err
	}
	if localCommit.Hash == remoteCommit.Hash {
		return nil // already up to date
	}

	// prepare trees
	baseCommit, err := mergeBase(localCommit, remoteCommit)
	if err != nil {
		return err
	}
	baseTree, err := baseCommit.Tree()
	if err != nil {
		return fmt.Errorf("load base tree: %w", err)
	}
	localTree, err := localCommit.Tree()
	if err != nil {
		return fmt.Errorf("load local tree: %w", err)
	}
	remoteTree, err := remoteCommit.Tree()
	if err != nil {
		return fmt.Errorf("load remote tree: %w", err)
	}

	paths, err := collectEventPaths(baseTree, localTree, remoteTree)
	if err != nil {
		return err
	}
	for _, p := range paths {
		baseVer, err := readEventVersion(baseTree, p)
		if err != nil {
			return err
		}
		localVer, err := readEventVersion(localTree, p)
		if err != nil {
			return err
		}
		remoteVer, err := readEventVersion(remoteTree, p)
		if err != nil {
			return err
		}
		if err := applyLWW(wt, p, baseVer, localVer, remoteVer); err != nil {
			return err
		}
	}

	commitMsg := fmt.Sprintf("Merge '%s/%s' (LWW)", GitRemoteName, GitBranchName)
	_, err = wt.Commit(commitMsg, &gogit.CommitOptions{
		Parents: []plumbing.Hash{localCommit.Hash, remoteCommit.Hash},
		Author: &object.Signature{
			Name: GitAuthorName,
			When: time.Now(),
		},
		AllowEmptyCommits: true,
	})
	if err != nil {
		return fmt.Errorf("failed to commit merge: %w", err)
	}

	return nil
}

// applyLWW applies last-write-wins strategy to an event file from three versions (base, local, remote).
func applyLWW(wt *gogit.Worktree, gitPath string, base, local, remote eventVersion) error {
	switch {
	case base.exists && !remote.exists: // remote deleted -> delete wins
		if local.exists {
			if _, err := wt.Remove(gitPath); err != nil {
				return fmt.Errorf("%s: failed to remove an event which was deleted on remote: %w", gitPath, err)
			}
		}

	case base.exists && !local.exists: // local deleted -> delete wins; nothing to do

	case !remote.exists: // remote has nothing new; local-only add or both absent

	case !local.exists: // remote added a new file -> take it
		return writeEvent(wt, gitPath, remote)

	case local.hash == remote.hash: // identical blobs; no-op

	default: // both sides modified -> last write wins
		if remote.event.UpdatedAt.After(local.event.UpdatedAt) { // if same, local wins i guess?
			return writeEvent(wt, gitPath, remote)
		}
	}

	return nil
}

// writeEvent writes a remote event blob to the worktree filesystem and stages it.
func writeEvent(wt *gogit.Worktree, gitPath string, ver eventVersion) error {
	fs := wt.Filesystem
	localPath := billyPath(fs, gitPath)

	if dir := path.Dir(gitPath); dir != "." {
		if err := fs.MkdirAll(billyPath(fs, dir), 0o755); err != nil {
			return fmt.Errorf("%s: failed to create parent dir: %w", gitPath, err)
		}
	}
	if err := util.WriteFile(fs, localPath, ver.data, 0o644); err != nil {
		return fmt.Errorf("%s: failed to write event: %w", gitPath, err)
	}
	if _, err := wt.Add(gitPath); err != nil {
		return fmt.Errorf("%s: failed to stage event: %w", gitPath, err)
	}
	return nil
}

// ------------------------------------ Helpers ------------------------------------

// ensureBranch checks out branch if HEAD is elsewhere, then returns the worktree.
func ensureBranch(repo *gogit.Repository, branch string) (*gogit.Worktree, error) {
	wt, err := repo.Worktree()
	if err != nil {
		return nil, err
	}
	head, err := repo.Head()
	if err != nil {
		return nil, err
	}
	ref := plumbing.NewBranchReferenceName(branch)
	if head.Name() == ref {
		return wt, nil
	}
	if err := wt.Checkout(&gogit.CheckoutOptions{Branch: ref}); err != nil {
		return nil, fmt.Errorf("failed to checkout %s: %w", branch, err)
	}
	return wt, nil
}

func localMainRef(repo *gogit.Repository) (*plumbing.Reference, error) {
	name := plumbing.NewBranchReferenceName(GitBranchName)
	ref, err := repo.Reference(name, true)
	if err != nil {
		return nil, fmt.Errorf("failed to get local ref %s: %w", name, err)
	}
	return ref, nil
}

func remoteMainRef(repo *gogit.Repository) (*plumbing.Reference, error) {
	name := plumbing.NewRemoteReferenceName(GitRemoteName, GitBranchName)
	ref, err := repo.Reference(name, true)
	if err != nil {
		return nil, fmt.Errorf("failed to get remote ref %s: %w", name, err)
	}
	return ref, nil
}

// getCommits returns the local and remote HEAD commits for the main branch.
func getCommits(repo *gogit.Repository) (local, remote *object.Commit, err error) {
	localRef, err := localMainRef(repo)
	if err != nil {
		return nil, nil, err
	}
	remoteRef, err := remoteMainRef(repo)
	if err != nil {
		return nil, nil, err
	}

	local, err = repo.CommitObject(localRef.Hash())
	if err != nil {
		return nil, nil, fmt.Errorf("failed to load local commit: %w", err)
	}
	remote, err = repo.CommitObject(remoteRef.Hash())
	if err != nil {
		return nil, nil, fmt.Errorf("failed to load remote commit: %w", err)
	}

	return local, remote, nil
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

// mergeBase returns the best common ancestor of a and b.
func mergeBase(a, b *object.Commit) (*object.Commit, error) {
	bases, err := a.MergeBase(b)
	if err != nil {
		return nil, fmt.Errorf("merge base %s / %s: %w", a.Hash, b.Hash, err)
	}
	if len(bases) == 0 {
		return nil, fmt.Errorf("no merge base between %s and %s", a.Hash, b.Hash)
	}
	return bases[0], nil
}

// ------------------------------------------------------------------------------

type eventVersion struct {
	exists bool
	hash   plumbing.Hash
	data   []byte
	event  Event
}

// readEventVersion reads and parses an event at gitPath from tree.
func readEventVersion(tree *object.Tree, gitPath string) (eventVersion, error) {
	f, err := tree.File(gitPath)
	if errors.Is(err, object.ErrFileNotFound) {
		return eventVersion{}, nil
	}
	if err != nil {
		return eventVersion{}, fmt.Errorf("%s: failed to lookup in tree: %w", gitPath, err)
	}

	r, err := f.Reader()
	if err != nil {
		return eventVersion{}, fmt.Errorf("%s: failed to open blob: %w", gitPath, err)
	}
	defer r.Close()

	data, err := io.ReadAll(r)
	if err != nil {
		return eventVersion{}, fmt.Errorf("%s: failed to read blob: %w", gitPath, err)
	}

	var ev Event
	if err := json.Unmarshal(data, &ev); err != nil {
		return eventVersion{}, fmt.Errorf("%s: failed to parse event JSON: %w", gitPath, err)
	}

	return eventVersion{exists: true, hash: f.Hash, data: data, event: ev}, nil
}

// collectEventPaths returns the sorted union of event paths across all trees.
func collectEventPaths(trees ...*object.Tree) ([]string, error) {
	seen := make(map[string]struct{})
	for _, tree := range trees {
		if tree == nil {
			continue
		}

		err := tree.Files().ForEach(
			func(f *object.File) error {
				if p := path.Clean(f.Name); isEventPath(p) {
					seen[p] = struct{}{}
				}
				return nil
			},
		)
		if err != nil {
			return nil, fmt.Errorf("failed to collect event paths: %w", err)
		}
	}

	paths := make([]string, 0, len(seen))
	for p := range seen {
		paths = append(paths, p)
	}

	sort.Strings(paths)
	return paths, nil
}

func isEventPath(gitPath string) bool {
	return path.Dir(gitPath) == EventsDirName && path.Ext(gitPath) == ".json"
}

// billyPath converts a slash-delimited git path to a native filesystem path just to make sure everything is ok.
func billyPath(fs billy.Filesystem, gitPath string) string {
	return fs.Join(strings.Split(path.Clean(gitPath), "/")...)
}
