package gitmerge

import (
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

// MergeRemoteIntoBranch performs a 3-way last-write-wins merge of origin/main -> main.
func MergeRemoteIntoBranch(repo *gogit.Repository, opts Options) error {
	if repo == nil {
		return errors.New("repository is nil")
	}
	if err := opts.validate(); err != nil {
		return err
	}

	wt, err := ensureBranch(repo, opts.BranchName)
	if err != nil {
		return err
	}

	localCommit, remoteCommit, err := GetCommits(repo, opts.BranchName, opts.RemoteName)
	if err != nil {
		return err
	}
	if localCommit.Hash == remoteCommit.Hash {
		return nil
	}

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

	paths, err := collectPaths(opts.IncludePath, baseTree, localTree, remoteTree)
	if err != nil {
		return err
	}

	for _, p := range paths {
		baseVer, err := readFileVersion(baseTree, p, opts.UpdatedAt)
		if err != nil {
			return err
		}

		localVer, err := readFileVersion(localTree, p, opts.UpdatedAt)
		if err != nil {
			return err
		}

		remoteVer, err := readFileVersion(remoteTree, p, opts.UpdatedAt)
		if err != nil {
			return err
		}

		if err := applyLWW(wt, p, baseVer, localVer, remoteVer); err != nil {
			return err
		}
	}

	commitMsg := fmt.Sprintf("Merge '%s/%s' (LWW)", opts.RemoteName, opts.BranchName)

	_, err = wt.Commit(commitMsg, &gogit.CommitOptions{
		Parents: []plumbing.Hash{localCommit.Hash, remoteCommit.Hash},
		Author: &object.Signature{
			Name:  opts.AuthorName,
			Email: opts.AuthorEmail,
			When:  time.Now(),
		},
		AllowEmptyCommits: true,
	})
	if err != nil {
		return fmt.Errorf("failed to commit merge: %w", err)
	}

	return nil
}

// applyLWW applies last-write-wins strategy to an event file from three versions (base, local, remote).
func applyLWW(wt *gogit.Worktree, gitPath string, base, local, remote fileVersion) error {
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
		return writeFile(wt, gitPath, remote)

	case local.hash == remote.hash: // identical blobs; no-op

	default: // both sides modified -> last write wins
		if remote.updatedAt.After(local.updatedAt) { // if same, local wins i guess?
			return writeFile(wt, gitPath, remote)
		}
	}

	return nil
}

// writeFile writes a remote blob to the worktree filesystem and stages it.
func writeFile(wt *gogit.Worktree, gitPath string, ver fileVersion) error {
	fs := wt.Filesystem
	localPath := billyPath(fs, gitPath)

	if dir := path.Dir(gitPath); dir != "." {
		if err := fs.MkdirAll(billyPath(fs, dir), 0o755); err != nil {
			return fmt.Errorf("%s: failed to create parent dir: %w", gitPath, err)
		}
	}

	if err := util.WriteFile(fs, localPath, ver.data, 0o644); err != nil {
		return fmt.Errorf("%s: failed to write file: %w", gitPath, err)
	}

	if _, err := wt.Add(gitPath); err != nil {
		return fmt.Errorf("%s: failed to stage file: %w", gitPath, err)
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

// GetCommits returns the local and remote HEAD commits for the main branch.
func GetCommits(
	repo *gogit.Repository,
	branchName string,
	remoteName string,
) (local, remote *object.Commit, err error) {
	localRef, err := localBranchRef(repo, branchName)
	if err != nil {
		return nil, nil, err
	}

	remoteRef, err := remoteBranchRef(repo, remoteName, branchName)
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

func localBranchRef(repo *gogit.Repository, branchName string) (*plumbing.Reference, error) {
	name := plumbing.NewBranchReferenceName(branchName)

	ref, err := repo.Reference(name, true)
	if err != nil {
		return nil, fmt.Errorf("failed to get local ref %s: %w", name, err)
	}

	return ref, nil
}

func remoteBranchRef(
	repo *gogit.Repository,
	remoteName string,
	branchName string,
) (*plumbing.Reference, error) {
	name := plumbing.NewRemoteReferenceName(remoteName, branchName)

	ref, err := repo.Reference(name, true)
	if err != nil {
		return nil, fmt.Errorf("failed to get remote ref %s: %w", name, err)
	}

	return ref, nil
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

type fileVersion struct {
	exists    bool
	hash      plumbing.Hash
	data      []byte
	updatedAt time.Time
}

func readFileVersion(
	tree *object.Tree,
	gitPath string,
	updatedAtFunc UpdatedAtFunc,
) (fileVersion, error) {
	f, err := tree.File(gitPath)
	if errors.Is(err, object.ErrFileNotFound) {
		return fileVersion{}, nil
	}
	if err != nil {
		return fileVersion{}, fmt.Errorf("%s: failed to lookup in tree: %w", gitPath, err)
	}

	r, err := f.Reader()
	if err != nil {
		return fileVersion{}, fmt.Errorf("%s: failed to open blob: %w", gitPath, err)
	}
	defer r.Close()

	data, err := io.ReadAll(r)
	if err != nil {
		return fileVersion{}, fmt.Errorf("%s: failed to read blob: %w", gitPath, err)
	}

	updatedAt, err := updatedAtFunc(gitPath, data)
	if err != nil {
		return fileVersion{}, err
	}

	return fileVersion{
		exists:    true,
		hash:      f.Hash,
		data:      data,
		updatedAt: updatedAt,
	}, nil
}

// collectPaths returns the sorted union of file paths across all trees.
func collectPaths(include IncludePathFunc, trees ...*object.Tree) ([]string, error) {
	seen := make(map[string]struct{})

	for _, tree := range trees {
		if tree == nil {
			continue
		}

		err := tree.Files().ForEach(func(f *object.File) error {
			p := path.Clean(f.Name)
			if include(p) {
				seen[p] = struct{}{}
			}
			return nil
		})
		if err != nil {
			return nil, fmt.Errorf("failed to collect paths: %w", err)
		}
	}

	paths := make([]string, 0, len(seen))
	for p := range seen {
		paths = append(paths, p)
	}

	sort.Strings(paths)
	return paths, nil
}

// billyPath converts a slash-delimited git path to a native filesystem path just to make sure everything is ok.
func billyPath(fs billy.Filesystem, gitPath string) string {
	return fs.Join(strings.Split(path.Clean(gitPath), "/")...)
}
