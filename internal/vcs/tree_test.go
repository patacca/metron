package vcs

import (
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"testing"
	"time"
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
		{"deleted file", func(t *testing.T, root string) { os.Remove(filepath.Join(root, "dir", "b.txt")) }, problems("dir/b.txt", TreeDeleted)},
		{"staged deletion", func(t *testing.T, root string) { run(t, root, "rm", "-q", "dir/b.txt") }, problems("dir/b.txt", TreeDeleted)},
		{"removed from the index only", func(t *testing.T, root string) { run(t, root, "rm", "-q", "--cached", "a.txt") }, problems("a.txt", TreeDeleted, "a.txt", TreeUntracked)},
		{"staged file", func(t *testing.T, root string) {
			write(t, root, "s.txt", "x\n")
			run(t, root, "add", "s.txt")
		}, problems("s.txt", TreeModified)},
		{"staged rename", func(t *testing.T, root string) { run(t, root, "mv", "a.txt", "moved.txt") }, problems("a.txt", TreeDeleted, "moved.txt", TreeModified)},
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
		{"link that leaves the repository and re-enters .metron", func(t *testing.T, root string) {
			addLink(t, root, "cfg2.yaml", "../"+filepath.Base(root)+"/.metron/contract.yaml")
		}, problems("cfg2.yaml", TreeLink)},
		{"link that leaves and re-enters .metron from a subdirectory", func(t *testing.T, root string) {
			addLink(t, root, "sub/cfg2.yaml", "../../"+filepath.Base(root)+"/.metron/contract.yaml")
		}, problems("sub/cfg2.yaml", TreeLink)},
		{"link that leaves and re-enters at an ordinary file", func(t *testing.T, root string) {
			addLink(t, root, "sub/l", "../../"+filepath.Base(root)+"/a.txt")
		}, problems()},
		{"link that leaves for a sibling with a longer name", func(t *testing.T, root string) {
			addLink(t, root, "l", "../"+filepath.Base(root)+"-other/.metron/x")
		}, problems()},
		{"link to sub/.metron/x", link("l", "sub/.metron/x"), problems()},
		{"link to a .metron sibling", link("l", ".metron-other/x"), problems()},
		{"link to an ordinary file", link("dir/l", "../a.txt"), problems()},
		{"link outside the repository", link("l", "../outside/.metron/x"), problems()},
		{"absolute link outside the repository", func(t *testing.T, root string) {
			addLink(t, root, "l", filepath.Join(t.TempDir(), ".metron", "x"))
		}, problems()},
		{"link under .metron", link(".metron/l", "x"), problems()},
		{"link into .git", link("l", ".git/config"), problems("l", TreeLink)},
		{"link to the git directory", link("sub/l", "../.git"), problems("sub/l", TreeLink)},
		{"dangling link into .metron", link("l", ".metron/missing"), problems("l", TreeLink)},
		{"link loop", func(t *testing.T, root string) {
			addLink(t, root, "l1", "l2")
			addLink(t, root, "l2", "l1")
		}, problems()},
		{"link loop through .metron", func(t *testing.T, root string) {
			addLink(t, root, ".metron/x", "../l")
			addLink(t, root, "l", ".metron/x")
		}, problems("l", TreeLink)},
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
	if want := problems("a.txt", TreeDeleted, "naïve/new.txt", TreeUntracked, "é.txt", TreeModified); !reflect.DeepEqual(got, want) {
		t.Fatalf("TreeProblems = %q, want %q", got, want)
	}
}

// isLink reports whether path is a symbolic link in the file system.
func isLink(path string) bool {
	info, err := os.Lstat(path)
	return err == nil && info.Mode()&os.ModeSymlink != 0
}

// Links are resolved through the file system, following every link on the
// way. Where git checks links out as plain files (core.symlinks=false, the
// Windows default without the privilege to create links), there is nothing
// to resolve: those cases are skipped there, and the lexical fallback is
// covered by TestTreeProblems.
func TestTreeProblemsResolvesLinks(t *testing.T) {
	outside := t.TempDir()
	if err := os.WriteFile(filepath.Join(outside, "file"), []byte("x\n"), 0o666); err != nil {
		t.Fatal(err)
	}
	ignoredLink := func(t *testing.T, root, name, target string) {
		t.Helper()
		write(t, root, ".gitignore", name+"\n")
		run(t, root, "add", ".gitignore")
		run(t, root, "commit", "-q", "-m", "ignore")
		if err := os.Symlink(target, filepath.Join(root, name)); err != nil {
			t.Skipf("this platform cannot create symbolic links without privilege: %v", err)
		}
	}
	tests := []struct {
		name   string
		link   string // the tracked link whose resolution is tested
		change func(t *testing.T, root string)
		want   []TreeProblem
	}{
		{"two steps through a tracked link", "app.txt", func(t *testing.T, root string) {
			write(t, root, ".metron/payload", "x\n")
			addLink(t, root, "d", ".")
			addLink(t, root, "app.txt", "d/.metron/payload")
		}, problems("app.txt", TreeLink)},
		{"two steps through an ignored link", "app.txt", func(t *testing.T, root string) {
			write(t, root, ".metron/payload", "x\n")
			ignoredLink(t, root, "ign", ".metron")
			addLink(t, root, "app.txt", "ign/payload")
		}, problems("app.txt", TreeLink)},
		{"two steps into .git", "l", func(t *testing.T, root string) {
			addLink(t, root, "d", ".")
			addLink(t, root, "l", "d/.git/HEAD")
		}, problems("l", TreeLink)},
		{"leaves the repository and re-enters .metron", "sub/cfg2.yaml", func(t *testing.T, root string) {
			addLink(t, root, "sub/cfg2.yaml", "../../"+filepath.Base(root)+"/.metron/contract.yaml")
		}, problems("sub/cfg2.yaml", TreeLink)},
		{"two steps to outside the repository", "l", func(t *testing.T, root string) {
			ignoredLink(t, root, "ign", outside)
			addLink(t, root, "l", "ign/file")
		}, problems()},
		{"two steps to an ordinary file", "l", func(t *testing.T, root string) {
			addLink(t, root, "d", "dir")
			addLink(t, root, "l", "d/b.txt")
		}, problems()},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root, r := baseRepo(t)
			tt.change(t, root)
			if !isLink(filepath.Join(root, filepath.FromSlash(tt.link))) {
				t.Skip("git checks out symbolic links as plain files on this platform (core.symlinks=false), so there is nothing to resolve")
			}
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

// The repository's file system monitor and stat settings cannot make a
// changed file look unchanged.
func TestTreeProblemsStrictStat(t *testing.T) {
	t.Run("file system monitor that reports no change", func(t *testing.T) {
		root, r := baseRepo(t)
		hook := filepath.Join(t.TempDir(), "fsmonitor.sh")
		if err := os.WriteFile(hook, []byte("#!/bin/sh\nprintf 'token\\0'\n"), 0o755); err != nil {
			t.Fatal(err)
		}
		run(t, root, "config", "core.fsmonitor", filepath.ToSlash(hook))
		run(t, root, "config", "core.fsmonitorHookVersion", "2")
		run(t, root, "update-index", "--fsmonitor")
		run(t, root, "status", "--porcelain") // records the token, trusts every entry
		write(t, root, "a.txt", "changed behind the monitor's back\n")
		if out := run(t, root, "status", "--porcelain"); out != "" {
			t.Fatalf("setup: the monitor did not hide the change from git status: %q", out)
		}
		got, err := r.TreeProblems()
		if err != nil {
			t.Fatalf("TreeProblems: %v", err)
		}
		if want := problems("a.txt", TreeModified); !reflect.DeepEqual(got, want) {
			t.Fatalf("TreeProblems = %q, want %q", got, want)
		}
	})
	t.Run("minimal stat comparison", func(t *testing.T) {
		if runtime.GOOS == "windows" {
			t.Skip("Windows records neither a change time nor an inode number, so no git setting can see an edit that keeps the size and the modification time")
		}
		root, r := baseRepo(t)
		path := filepath.Join(root, "a.txt")
		past := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
		if err := os.Chtimes(path, past, past); err != nil {
			t.Fatal(err)
		}
		run(t, root, "update-index", "--refresh")
		run(t, root, "config", "core.checkStat", "minimal")
		// Same size, same modification time, new content.
		tmp := path + ".new"
		if err := os.WriteFile(tmp, []byte("ALPHA\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.Chtimes(tmp, past, past); err != nil {
			t.Fatal(err)
		}
		if err := os.Rename(tmp, path); err != nil {
			t.Fatal(err)
		}
		if out := run(t, root, "status", "--porcelain"); out != "" {
			t.Fatalf("setup: core.checkStat=minimal did not hide the change from git status: %q", out)
		}
		got, err := r.TreeProblems()
		if err != nil {
			t.Fatalf("TreeProblems: %v", err)
		}
		if want := problems("a.txt", TreeModified); !reflect.DeepEqual(got, want) {
			t.Fatalf("TreeProblems = %q, want %q", got, want)
		}
	})
}

// A flag set inside a submodule is reported with its full path.
func TestTreeProblemsSubmoduleFlags(t *testing.T) {
	subRoot, _ := newRepo(t, "library")
	write(t, subRoot, "s.txt", "s\n")
	commit(t, subRoot, "library")
	root, r := baseRepo(t)
	run(t, root, "-c", "protocol.file.allow=always", "submodule", "add", "-q", subRoot, "lib")
	run(t, root, "commit", "-q", "-m", "submodule")
	sub := filepath.Join(root, "lib")
	run(t, sub, "update-index", "--skip-worktree", "s.txt")
	write(t, sub, "s.txt", "hidden change\n")
	if out := run(t, root, "status", "--porcelain", "--ignore-submodules=none"); out != "" {
		t.Fatalf("setup: git status sees the change: %q", out)
	}
	got, err := r.TreeProblems()
	if err != nil {
		t.Fatalf("TreeProblems: %v", err)
	}
	if want := problems("lib/s.txt", TreeHidden); !reflect.DeepEqual(got, want) {
		t.Fatalf("TreeProblems = %q, want %q", got, want)
	}
}

func TestSparseCheckoutAndIgnoreStat(t *testing.T) {
	root, r := baseRepo(t)
	check := func(sparse, ignoreStat bool) {
		t.Helper()
		if got, err := r.SparseCheckout(); err != nil || got != sparse {
			t.Errorf("SparseCheckout = %v, %v, want %v", got, err, sparse)
		}
		if got, err := r.IgnoreStat(); err != nil || got != ignoreStat {
			t.Errorf("IgnoreStat = %v, %v, want %v", got, err, ignoreStat)
		}
	}
	check(false, false)
	run(t, root, "sparse-checkout", "set", "--cone", "dir")
	run(t, root, "config", "core.ignoreStat", "yes")
	check(true, true)
	run(t, root, "sparse-checkout", "disable")
	run(t, root, "config", "core.ignoreStat", "false")
	check(false, false)
	run(t, root, "config", "core.ignoreStat", "sometimes")
	if _, err := r.IgnoreStat(); err == nil {
		t.Error("IgnoreStat with an invalid value succeeded")
	}
}
