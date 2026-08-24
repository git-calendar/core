package gitmerge

import (
	"encoding/json/v2"
	"errors"
	"io"
	"testing"
	"time"

	"github.com/go-git/go-billy/v5/util"
	gogit "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
)

// Caution!
// AI generated tests...

const (
	testBranch = "main"
	testRemote = "origin"
	testFile   = "events/event.json"
)

type testEvent struct {
	UpdatedAt time.Time `json:"updatedAt"`
}

func TestMergeRemoteIntoBranch_StrategyMatrix(t *testing.T) {
	t0 := ts(10)
	t1 := ts(11)
	t2 := ts(12)

	tests := []struct {
		name   string
		base   *time.Time
		local  *time.Time
		remote *time.Time
		want   *time.Time
	}{
		{
			name:  "local added",
			local: t1,
			want:  t1,
		},
		{
			name:   "remote added",
			remote: t1,
			want:   t1,
		},
		{
			name: "both deleted",
			base: t0,
			want: nil,
		},
		{
			name:   "both added, remote newer",
			local:  t1,
			remote: t2,
			want:   t2,
		},
		{
			name:   "both added, local newer",
			local:  t2,
			remote: t1,
			want:   t2,
		},
		{
			name:   "both edited, remote newer",
			base:   t0,
			local:  t1,
			remote: t2,
			want:   t2,
		},
		{
			name:   "both edited, local newer",
			base:   t0,
			local:  t2,
			remote: t1,
			want:   t2,
		},
		{
			name:   "local deleted, remote unchanged",
			base:   t0,
			local:  nil,
			remote: t0,
			want:   nil,
		},
		{
			name:   "local unchanged, remote deleted",
			base:   t0,
			local:  t0,
			remote: nil,
			want:   nil,
		},
		{
			name:   "local deleted, remote edited",
			base:   t0,
			local:  nil,
			remote: t1,
			want:   nil,
		},
		{
			name:   "local edited, remote deleted",
			base:   t0,
			local:  t1,
			remote: nil,
			want:   nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := makeMergeRepo(t, tt.base, tt.local, tt.remote)

			if err := MergeRemoteIntoBranch(repo, testOptions()); err != nil {
				t.Fatalf("MergeRemoteIntoBranch() error = %v", err)
			}

			assertHeadEvent(t, repo, tt.want)
		})
	}
}

func TestMergeRemoteIntoBranch_AlreadyUpToDateDoesNotCommit(t *testing.T) {
	when := ts(10)
	repo := makeMergeRepo(t, when, when, when)

	before := headHash(t, repo)

	if err := MergeRemoteIntoBranch(repo, testOptions()); err != nil {
		t.Fatalf("MergeRemoteIntoBranch() error = %v", err)
	}

	after := headHash(t, repo)
	if after != before {
		t.Fatalf("HEAD changed: before=%s after=%s", before, after)
	}
}

func TestMergeRemoteIntoBranch_UsesConfiguredTime(t *testing.T) {
	repo := makeMergeRepo(t, ts(10), ts(11), ts(12))
	opts := testOptions()
	want := opts.Now()

	if err := MergeRemoteIntoBranch(repo, opts); err != nil {
		t.Fatalf("MergeRemoteIntoBranch() error = %v", err)
	}

	commit, err := repo.CommitObject(headHash(t, repo))
	if err != nil {
		t.Fatalf("CommitObject: %v", err)
	}
	if !commit.Author.When.Equal(want) {
		t.Fatalf("merge time = %s, want %s", commit.Author.When, want)
	}
}

func testOptions() Options {
	return Options{
		BranchName:  testBranch,
		RemoteName:  testRemote,
		AuthorName:  "Test",
		AuthorEmail: "test@example.com",

		IncludePath: func(p string) bool {
			return p == testFile
		},

		UpdatedAt: func(_ string, data []byte) (time.Time, error) {
			var ev testEvent
			if err := json.Unmarshal(data, &ev); err != nil {
				return time.Time{}, err
			}
			return ev.UpdatedAt, nil
		},

		Now: func() time.Time {
			return time.Date(2026, 1, 1, 13, 0, 0, 0, time.UTC)
		},
	}
}

// -------------------------------------- Repo setup ---------------------------------------

func makeMergeRepo(t *testing.T, base, local, remote *time.Time) *gogit.Repository {
	t.Helper()

	repo, err := gogit.PlainInit(t.TempDir(), false)
	if err != nil {
		t.Fatalf("PlainInit: %v", err)
	}

	mainRef := plumbing.NewBranchReferenceName(testBranch)
	if err := repo.Storer.SetReference(plumbing.NewSymbolicReference(plumbing.HEAD, mainRef)); err != nil {
		t.Fatalf("set HEAD: %v", err)
	}

	wt, err := repo.Worktree()
	if err != nil {
		t.Fatalf("Worktree: %v", err)
	}

	if err := util.WriteFile(wt.Filesystem, "root.txt", []byte("root\n"), 0o644); err != nil {
		t.Fatalf("write root: %v", err)
	}
	if _, err := wt.Add("root.txt"); err != nil {
		t.Fatalf("add root: %v", err)
	}

	applyEvent(t, wt, nil, base)
	baseHash := commit(t, wt, "base")

	localHash := baseHash
	if !sameTime(base, local) {
		applyEvent(t, wt, base, local)
		localHash = commit(t, wt, "local")
	}

	remoteHash := baseHash
	if !sameTime(base, remote) {
		tmpRef := plumbing.NewBranchReferenceName("tmp-remote-build")

		if err := repo.Storer.SetReference(plumbing.NewHashReference(tmpRef, baseHash)); err != nil {
			t.Fatalf("create tmp remote branch: %v", err)
		}

		if err := wt.Checkout(&gogit.CheckoutOptions{
			Branch: tmpRef,
			Force:  true,
		}); err != nil {
			t.Fatalf("checkout tmp remote branch: %v", err)
		}

		applyEvent(t, wt, base, remote)
		remoteHash = commit(t, wt, "remote")
	}

	remoteRef := plumbing.NewRemoteReferenceName(testRemote, testBranch)

	if err := repo.Storer.SetReference(plumbing.NewHashReference(remoteRef, remoteHash)); err != nil {
		t.Fatalf("set remote ref: %v", err)
	}

	if err := repo.Storer.SetReference(plumbing.NewHashReference(mainRef, localHash)); err != nil {
		t.Fatalf("set local ref: %v", err)
	}

	if err := wt.Checkout(&gogit.CheckoutOptions{
		Branch: mainRef,
		Force:  true,
	}); err != nil {
		t.Fatalf("checkout local branch: %v", err)
	}

	return repo
}

func applyEvent(t *testing.T, wt *gogit.Worktree, from, to *time.Time) {
	t.Helper()

	switch {
	case to == nil:
		if from != nil {
			if _, err := wt.Remove(testFile); err != nil {
				t.Fatalf("remove event: %v", err)
			}
		}

	case from != nil && to.Equal(*from):
		return

	default:
		writeEvent(t, wt, *to)
	}
}

func writeEvent(t *testing.T, wt *gogit.Worktree, when time.Time) {
	t.Helper()

	data, err := json.Marshal(testEvent{UpdatedAt: when}, json.Deterministic(true))
	if err != nil {
		t.Fatalf("marshal event: %v", err)
	}

	if err := wt.Filesystem.MkdirAll("events", 0o755); err != nil {
		t.Fatalf("mkdir events: %v", err)
	}

	if err := util.WriteFile(wt.Filesystem, testFile, data, 0o644); err != nil {
		t.Fatalf("write event: %v", err)
	}

	if _, err := wt.Add(testFile); err != nil {
		t.Fatalf("add event: %v", err)
	}
}

func commit(t *testing.T, wt *gogit.Worktree, msg string) plumbing.Hash {
	t.Helper()

	hash, err := wt.Commit(msg, &gogit.CommitOptions{
		Author: &object.Signature{
			Name:  "Test",
			Email: "test@example.com",
			When:  time.Date(2026, 1, 1, 9, 0, 0, 0, time.UTC),
		},
	})
	if err != nil {
		t.Fatalf("commit %q: %v", msg, err)
	}

	return hash
}

// ------------------------------------ Assertions -----------------------------------------

func assertHeadEvent(t *testing.T, repo *gogit.Repository, want *time.Time) {
	t.Helper()

	tree := headTree(t, repo)

	f, err := tree.File(testFile)
	if want == nil {
		if errors.Is(err, object.ErrFileNotFound) {
			return
		}
		if err != nil {
			t.Fatalf("lookup event: %v", err)
		}
		t.Fatalf("HEAD has %s, want deleted", f.Name)
	}

	if err != nil {
		t.Fatalf("lookup event: %v", err)
	}

	r, err := f.Reader()
	if err != nil {
		t.Fatalf("open event blob: %v", err)
	}
	defer r.Close()

	data, err := io.ReadAll(r)
	if err != nil {
		t.Fatalf("read event blob: %v", err)
	}

	var got testEvent
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal event: %v", err)
	}

	if !got.UpdatedAt.Equal(*want) {
		t.Fatalf(
			"UpdatedAt = %s, want %s",
			got.UpdatedAt.Format(time.RFC3339Nano),
			want.Format(time.RFC3339Nano),
		)
	}
}

func headHash(t *testing.T, repo *gogit.Repository) plumbing.Hash {
	t.Helper()

	ref, err := repo.Head()
	if err != nil {
		t.Fatalf("Head: %v", err)
	}

	return ref.Hash()
}

func headTree(t *testing.T, repo *gogit.Repository) *object.Tree {
	t.Helper()

	ref, err := repo.Head()
	if err != nil {
		t.Fatalf("Head: %v", err)
	}

	commit, err := repo.CommitObject(ref.Hash())
	if err != nil {
		t.Fatalf("CommitObject: %v", err)
	}

	tree, err := commit.Tree()
	if err != nil {
		t.Fatalf("Tree: %v", err)
	}

	return tree
}

func ts(hour int) *time.Time {
	t := time.Date(2026, 1, 1, hour, 0, 0, 0, time.UTC)
	return &t
}

func sameTime(a, b *time.Time) bool {
	if a == nil || b == nil {
		return a == b
	}
	return a.Equal(*b)
}
