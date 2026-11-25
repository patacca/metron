package vcs

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

// problems builds an expected result from path and kind pairs.
func problems(pathKind ...string) []TreeProblem {
	out := []TreeProblem{}
	for i := 0; i+1 < len(pathKind); i += 2 {
		out = append(out, TreeProblem{Path: pathKind[i], Kind: pathKind[i+1]})
	}
	return out
}

// addLink commits a symbolic link through the index, so that the test needs
// no permission to create links. Checking it out makes a link where the
// platform supports them and a file holding the target elsewhere; either way
// the tree is clean afterwards.
func addLink(t *testing.T, root, link, target string) {
	t.Helper()
	file := filepath.Join(t.TempDir(), "target")
	if err := os.WriteFile(file, []byte(target), 0o666); err != nil {
		t.Fatal(err)
	}
	object := run(t, root, "hash-object", "-w", "--no-filters", file)
	run(t, root, "update-index", "--add", "--cacheinfo", "120000,"+object+","+link)
	run(t, root, "checkout", "--", link)
	run(t, root, "commit", "-q", "-m", "link "+link)
}

func TestTreeProblems(t *testing.T) {
	type change func(t *testing.T, root string)
	link := func(path, target string) change {
		return func(t *testing.T, root string) { addLink(t, root, path, target) }
	}
	tests := []struct {
		name   string
		change change
		want   []TreeProblem
	}{
		{"clean", func(t *testing.T, root string) {}, problems()},

		// What git status reports.
		{"tracked change under .metron", func(t *testing.T, root string) { write(t, root, ".metron/contract.yaml", "x\n") }, problems()},
		{"untracked under .metron", func(t *testing.T, root string) { write(t, root, ".metron/state/ledger.jsonl", "{}\n") }, problems()},
		{"staged under .metron", func(t *testing.T, root string) {
			write(t, root, ".metron/state/new", "x\n")
			run(t, root, "add", ".metron")
		}, problems()},
		{"untracked file", func(t *testing.T, root string) { write(t, root, "untracked.txt", "x\n") }, problems("untracked.txt", TreeUntracked)},
		{"untracked file in a new directory", func(t *testing.T, root string) { write(t, root, "new/deep/f.txt", "x\n") }, problems("new/deep/f.txt", TreeUntracked)},
		{"modified file", func(t *testing.T, root string) { write(t, root, "a.txt", "x\n") }, problems("a.txt", TreeModified)},
		{"deleted file", func(t *testing.T, root string) { os.Remove(filepath.Join(root, "dir", "b.txt")) }, problems("dir/b.txt", TreeModified)},
		{"staged file", func(t *testing.T, root string) {
			write(t, root, "s.txt", "x\n")
			run(t, root, "add", "s.txt")
		}, problems("s.txt", TreeModified)},
		{"staged rename", func(t *testing.T, root string) { run(t, root, "mv", "a.txt", "moved.txt") }, problems("a.txt", TreeModified, "moved.txt", TreeModified)},
		{"sibling named like the excluded directory", func(t *testing.T, root string) { write(t, root, ".metron-other", "x\n") }, problems(".metron-other", TreeUntracked)},
		{"nested .metron directory", func(t *testing.T, root string) { write(t, root, "sub/.metron/x", "x\n") }, problems("sub/.metron/x", TreeUntracked)},
		{"unusual characters", func(t *testing.T, root string) {
			write(t, root, "dir with space/naïve é.txt", "x\n")
			write(t, root, "tab\tand \"quote\".txt", "x\n")
		}, problems("dir with space/naïve é.txt", TreeUntracked, "tab\tand \"quote\".txt", TreeUntracked)},
		{"ignored file", func(t *testing.T, root string) {
			write(t, root, ".gitignore", "*.log\n")
			run(t, root, "add", ".gitignore")
			run(t, root, "commit", "-q", "-m", "ignore")
			write(t, root, "debug.log", "x\n")
		}, problems()},

		// Index flags that hide modifications from git status.
		{"skip-worktree on a modified file", func(t *testing.T, root string) {
			run(t, root, "update-index", "--skip-worktree", "a.txt")
			write(t, root, "a.txt", "hidden change\n")
		}, problems("a.txt", TreeHidden)},
		{"assume-unchanged on a modified file", func(t *testing.T, root string) {
			run(t, root, "update-index", "--assume-unchanged", "dir/b.txt")
			write(t, root, "dir/b.txt", "hidden change\n")
		}, problems("dir/b.txt", TreeHidden)},
		{"flag on an unchanged file", func(t *testing.T, root string) { run(t, root, "update-index", "--skip-worktree", "a.txt") }, problems("a.txt", TreeHidden)},
		{"flag and a staged change", func(t *testing.T, root string) {
			write(t, root, "a.txt", "staged\n")
			run(t, root, "add", "a.txt")
			run(t, root, "update-index", "--assume-unchanged", "a.txt")
		}, problems("a.txt", TreeHidden, "a.txt", TreeModified)},
		{"flag under .metron", func(t *testing.T, root string) {
			run(t, root, "update-index", "--skip-worktree", ".metron/contract.yaml")
		}, problems()},
		{"sparse checkout", func(t *testing.T, root string) {
			run(t, root, "sparse-checkout", "set", "--no-cone", "/a.txt")
		}, problems("dir/b.txt", TreeHidden)},
		{"sparse checkout leaving out only .metron", func(t *testing.T, root string) {
			run(t, root, "sparse-checkout", "set", "--cone", "dir")
			if tag := run(t, root, "ls-files", "-v", ".metron/contract.yaml"); tag != "S .metron/contract.yaml" {
				t.Fatalf("the sparse checkout did not leave out .metron: %q", tag)
			}
		}, problems()},

		// Symbolic links, resolved lexically from their directory.
		{"link to .metron/x", link("l", ".metron/x"), problems("l", TreeLink)},
		{"link to ../.metron/x from a subdirectory", link("sub/l", "../.metron/x"), problems("sub/l", TreeLink)},
		{"link to the .metron directory", link("l", ".metron"), problems("l", TreeLink)},
		{"link through another directory", link("l", "dir/../.metron/x"), problems("l", TreeLink)},
		{"link with backslashes", link("sub/l", `..\.metron\x`), problems("sub/l", TreeLink)},
		{"link with another case", link("l", ".METRON/x"), problems("l", TreeLink)},
		{"absolute link into .metron", func(t *testing.T, root string) {
			r, err := Open(root)
			if err != nil {
				t.Fatal(err)
			}
			addLink(t, root, "l", filepath.Join(r.Root(), ".metron", "x"))
		}, problems("l", TreeLink)},
		{"link to sub/.metron/x", link("l", "sub/.metron/x"), problems()},
		{"link to a .metron sibling", link("l", ".metron-other/x"), problems()},
		{"link to an ordinary file", link("dir/l", "../a.txt"), problems()},
		{"link outside the repository", link("l", "../outside/.metron/x"), problems()},
		{"absolute link outside the repository", func(t *testing.T, root string) {
			addLink(t, root, "l", filepath.Join(t.TempDir(), ".metron", "x"))
		}, problems()},
		{"link under .metron", link(".metron/l", "x"), problems()},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root, r := baseRepo(t)
			if tt.name == "unusual characters" && !canCreate(root, "tab\tand \"quote\".txt") {
				tt.want = tt.want[:1]
				tt.change = func(t *testing.T, root string) { write(t, root, "dir with space/naïve é.txt", "x\n") }
			}
			tt.change(t, root)
			got, err := r.TreeProblems()
			if err != nil {
				t.Fatalf("TreeProblems: %v", err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("TreeProblems = %q, want %q", got, tt.want)
			}
		})
	}
}

// canCreate reports whether the file system accepts a file name; Windows
// rejects tabs and quotes.
func canCreate(dir, name string) bool {
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, nil, 0o666); err != nil {
		return false
	}
	os.Remove(path)
	return true
}

// The status configuration a user may set does not change the result.
func TestTreeProblemsIgnoresConfiguration(t *testing.T) {
	root, r := baseRepo(t)
	run(t, root, "config", "status.showUntrackedFiles", "no")
	run(t, root, "config", "status.branch", "true")
	run(t, root, "config", "status.renames", "true")
	run(t, root, "config", "core.quotePath", "true")
	write(t, root, "naïve/new.txt", "x\n")
	run(t, root, "mv", "a.txt", "é.txt")
	got, err := r.TreeProblems()
	if err != nil {
		t.Fatalf("TreeProblems: %v", err)
	}
	if want := problems("a.txt", TreeModified, "naïve/new.txt", TreeUntracked, "é.txt", TreeModified); !reflect.DeepEqual(got, want) {
		t.Fatalf("TreeProblems = %q, want %q", got, want)
	}
}
