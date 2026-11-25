package vcs

import (
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
)

// intervalCommit is a commit that Interval must list, with the paths it must
// give for it.
type intervalCommit struct {
	hash  string
	paths []string
}

func TestInterval(t *testing.T) {
	tests := []struct {
		name string
		// build makes a repository and returns it, the base to list the
		// interval from, whether the interval is known, and the commits it
		// holds.
		build func(t *testing.T) (r *Repo, base string, known bool, want []intervalCommit)
	}{
		{"base is HEAD", func(t *testing.T) (*Repo, string, bool, []intervalCommit) {
			_, r := baseRepo(t)
			return r, head(t, r), true, nil
		}},
		{"linear history", func(t *testing.T) (*Repo, string, bool, []intervalCommit) {
			root, r := baseRepo(t)
			base := head(t, r)
			write(t, root, "a.txt", "fixed\n")
			fix := commit(t, root, "fix")
			write(t, root, ".metron/state/ledger.jsonl", "{}\n")
			metron := commit(t, root, "only .metron")
			write(t, root, "dir/b.txt", "both\n")
			write(t, root, ".metron/state/ledger.jsonl", "{}\n{}\n")
			both := commit(t, root, "both")
			write(t, root, unusualName, "x\n")
			unusual := commit(t, root, "unusual name")
			return r, base, true, []intervalCommit{
				{fix, []string{"a.txt"}},
				{metron, nil},
				{both, []string{"dir/b.txt"}},
				{unusual, []string{unusualName}},
			}
		}},
		{"merge of a side branch", func(t *testing.T) (*Repo, string, bool, []intervalCommit) {
			root, r := baseRepo(t)
			base := head(t, r)
			run(t, root, "checkout", "-q", "-b", "side")
			write(t, root, "side.txt", "s\n")
			side := commit(t, root, "side")
			write(t, root, ".metron/only", "x\n")
			sideMetron := commit(t, root, "side, only .metron")
			run(t, root, "checkout", "-q", "main")
			write(t, root, "main.txt", "m\n")
			main := commit(t, root, "main")
			run(t, root, "merge", "-q", "--no-ff", "--no-edit", "side")
			return r, base, true, []intervalCommit{
				{main, []string{"main.txt"}},
				{side, []string{"side.txt"}},
				{sideMetron, nil},
				{head(t, r), []string{"side.txt"}}, // relative to main, its first parent
			}
		}},
		{"base rewritten out of the history of HEAD", func(t *testing.T) (*Repo, string, bool, []intervalCommit) {
			root, r := baseRepo(t)
			write(t, root, "a.txt", "fixed\n")
			base := commit(t, root, "fix")
			write(t, root, "a.txt", "fixed again\n")
			run(t, root, "add", "-A")
			run(t, root, "commit", "-q", "--amend", "-m", "fix, amended")
			amended := head(t, r)
			write(t, root, "dir/b.txt", "next\n")
			next := commit(t, root, "next")
			return r, base, true, []intervalCommit{{amended, []string{"a.txt"}}, {next, []string{"dir/b.txt"}}}
		}},
		{"shallow boundary after base", func(t *testing.T) (*Repo, string, bool, []intervalCommit) {
			root, r := baseRepo(t)
			write(t, root, "a.txt", "fixed\n")
			base := commit(t, root, "before the boundary")
			write(t, root, "dir/b.txt", "changed\n")
			boundary := commit(t, root, "boundary")
			write(t, root, "new.txt", "new\n")
			after := commit(t, root, "after the boundary")
			// A commit listed in the shallow file has no parent, as at the
			// boundary of the history of a shallow clone.
			if err := os.WriteFile(filepath.Join(root, ".git", "shallow"), []byte(boundary+"\n"), 0o666); err != nil {
				t.Fatal(err)
			}
			return r, base, true, []intervalCommit{
				{boundary, []string{"a.txt", "dir/b.txt"}}, // its whole tree, as a root commit
				{after, []string{"new.txt"}},
			}
		}},
		{"base does not exist", func(t *testing.T) (*Repo, string, bool, []intervalCommit) {
			_, r := baseRepo(t)
			return r, strings.Repeat("d", 40), false, nil
		}},
		{"base is a tree", func(t *testing.T) (*Repo, string, bool, []intervalCommit) {
			root, r := baseRepo(t)
			return r, run(t, root, "rev-parse", "HEAD^{tree}"), false, nil
		}},
		{"HEAD on a branch without commits", func(t *testing.T) (*Repo, string, bool, []intervalCommit) {
			root, r := baseRepo(t)
			base := head(t, r)
			run(t, root, "checkout", "-q", "--orphan", "fresh")
			return r, base, false, nil
		}},
		{"repository without commits", func(t *testing.T) (*Repo, string, bool, []intervalCommit) {
			_, r := newRepo(t, "empty")
			return r, strings.Repeat("a", 40), false, nil
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r, base, wantKnown, want := tt.build(t)
			got, known, err := r.Interval(base)
			if err != nil {
				t.Fatalf("Interval: %v", err)
			}
			if known != wantKnown || len(got) != len(want) || !known && got != nil {
				t.Fatalf("Interval = %+v, known %v; want %d commits, known %v", got, known, len(want), wantKnown)
			}
			position := map[string]int{}
			for i, c := range got {
				position[c.Hash] = i
			}
			for _, w := range want {
				i, ok := position[w.hash]
				if !ok {
					t.Errorf("commit %s is missing from %+v", w.hash, got)
					continue
				}
				if c := got[i]; !c.Reachable || c.InBase || c.Substantive != (len(w.paths) > 0) || !slices.Equal(c.Paths, w.paths) {
					t.Errorf("got %+v, want reachable, not in base, paths %q", c, w.paths)
				}
				// Oldest first: no commit comes before one of its parents.
				for _, p := range strings.Fields(run(t, r.Root(), "rev-list", "--no-walk", "--parents", w.hash))[1:] {
					if j, ok := position[p]; ok && j > i {
						t.Errorf("commit %s comes before its parent %s", w.hash, p)
					}
				}
			}
			for _, c := range got {
				if c.Substantive != (len(c.Paths) > 0) {
					t.Errorf("%s: Substantive is %v with paths %q", c.Hash, c.Substantive, c.Paths)
				}
			}
			if again, _, err := r.Interval(base); err != nil || !reflect.DeepEqual(again, got) {
				t.Errorf("Interval again = %+v, %v; first %+v", again, err, got)
			}
			if len(got) > 0 {
				var hashes []string
				for _, c := range got {
					hashes = append(hashes, c.Hash)
				}
				if described, err := r.Describe(base, hashes); err != nil || !reflect.DeepEqual(described, got) {
					t.Errorf("Describe = %+v, %v; Interval %+v", described, err, got)
				}
			}
		})
	}
}

func TestIntervalRejectsMalformedBase(t *testing.T) {
	_, r := baseRepo(t)
	h := head(t, r)
	for _, bad := range []string{"", "HEAD", "--all", strings.ToUpper(h), h[:12], h + "..HEAD", "^" + h} {
		if got, known, err := r.Interval(bad); err == nil || known || got != nil {
			t.Errorf("Interval(%q) = %+v, %v, %v; want an error", bad, got, known, err)
		}
	}
}
