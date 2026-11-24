package vcs

import (
	"crypto/sha1"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// TestMain makes git hermetic: no system or global configuration, a fixed
// author and committer, and no repository selected through the environment.
// The tests fail, rather than skip, without git.
func TestMain(m *testing.M) {
	if _, err := exec.LookPath("git"); err != nil {
		fmt.Fprintln(os.Stderr, "the vcs tests need the git executable on PATH:", err)
		os.Exit(1)
	}
	home, err := os.MkdirTemp("", "metron-vcs-home-")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	global := filepath.Join(home, "gitconfig")
	if err := os.WriteFile(global, nil, 0o666); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	for _, k := range []string{
		"GIT_DIR", "GIT_WORK_TREE", "GIT_INDEX_FILE", "GIT_OBJECT_DIRECTORY", "GIT_ALTERNATE_OBJECT_DIRECTORIES",
		"GIT_COMMON_DIR", "GIT_CEILING_DIRECTORIES", "GIT_CONFIG", "GIT_CONFIG_PARAMETERS", "GIT_CONFIG_COUNT",
		"XDG_CONFIG_HOME",
	} {
		os.Unsetenv(k)
	}
	for k, v := range map[string]string{
		"HOME":                home,
		"GIT_CONFIG_NOSYSTEM": "1",
		"GIT_CONFIG_GLOBAL":   global,
		"GIT_AUTHOR_NAME":     "Test Author",
		"GIT_AUTHOR_EMAIL":    "author@example.com",
		"GIT_AUTHOR_DATE":     "2026-01-01T00:00:00Z",
		"GIT_COMMITTER_NAME":  "Test Committer",
		"GIT_COMMITTER_EMAIL": "committer@example.com",
		"GIT_COMMITTER_DATE":  "2026-01-01T00:00:00Z",
	} {
		os.Setenv(k, v)
	}
	code := m.Run()
	os.RemoveAll(home)
	os.Exit(code)
}

func run(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
	}
	return strings.TrimSpace(string(out))
}

func write(t *testing.T, root, rel, content string) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(path), 0o777); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o666); err != nil {
		t.Fatal(err)
	}
}

// newRepo creates an empty SHA-1 repository in a directory with the given
// name.
func newRepo(t *testing.T, name string) (string, *Repo) {
	t.Helper()
	root := filepath.Join(t.TempDir(), name)
	if err := os.Mkdir(root, 0o777); err != nil {
		t.Fatal(err)
	}
	run(t, root, "init", "-q", "--object-format=sha1", "-b", "main")
	r, err := Open(root)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	return root, r
}

// baseRepo is a repository with one commit holding ordinary files and a
// .metron directory.
func baseRepo(t *testing.T) (string, *Repo) {
	t.Helper()
	root, r := newRepo(t, "repo")
	write(t, root, "a.txt", "alpha\n")
	write(t, root, "dir/b.txt", "beta\n")
	write(t, root, ".metron/contract.yaml", "schema: metron.contract/v1\n")
	commit(t, root, "base")
	return root, r
}

func commit(t *testing.T, root, msg string) string {
	t.Helper()
	run(t, root, "add", "-A")
	run(t, root, "commit", "-q", "--allow-empty", "-m", msg)
	return run(t, root, "rev-parse", "HEAD")
}

func head(t *testing.T, r *Repo) string {
	t.Helper()
	h, err := r.Head()
	if err != nil {
		t.Fatalf("Head: %v", err)
	}
	return h
}

func digest(t *testing.T, r *Repo, commit string) string {
	t.Helper()
	d, err := r.CandidateDigest(commit)
	if err != nil {
		t.Fatalf("CandidateDigest: %v", err)
	}
	return d
}

// The digest is computed from an independent derivation of git's object
// names and ls-tree format.
func TestCandidateDigestExact(t *testing.T) {
	root, r := newRepo(t, "repo")
	write(t, root, "a.txt", "hello\n")
	write(t, root, "z/c.txt", "c\n")
	write(t, root, ".metron/state/ledger.jsonl", "{}\n")
	commit(t, root, "c1")
	blob := func(content string) string {
		sum := sha1.Sum([]byte(fmt.Sprintf("blob %d\x00%s", len(content), content)))
		return hex.EncodeToString(sum[:])
	}
	listing := "100644 blob " + blob("hello\n") + "\ta.txt\x00" + "100644 blob " + blob("c\n") + "\tz/c.txt\x00"
	sum := sha256.Sum256([]byte(listing))
	if got, want := digest(t, r, head(t, r)), "sha256:"+hex.EncodeToString(sum[:]); got != want {
		t.Fatalf("digest = %s, want %s", got, want)
	}
}

func TestCandidateDigestIgnoresMetron(t *testing.T) {
	tests := []struct {
		name   string
		change func(t *testing.T, root string)
	}{
		{"modify contract", func(t *testing.T, root string) { write(t, root, ".metron/contract.yaml", "changed\n") }},
		{"add ledger", func(t *testing.T, root string) { write(t, root, ".metron/state/ledger.jsonl", "{}\n") }},
		{"delete contract", func(t *testing.T, root string) { run(t, root, "rm", "-q", ".metron/contract.yaml") }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root, r := baseRepo(t)
			before := digest(t, r, head(t, r))
			tt.change(t, root)
			after := commit(t, root, tt.name)
			if got := digest(t, r, after); got != before {
				t.Fatalf("digest changed from %s to %s", before, got)
			}
		})
	}
}

func TestCandidateDigestChanges(t *testing.T) {
	tests := []struct {
		name   string
		change func(t *testing.T, root string)
	}{
		{"content", func(t *testing.T, root string) { write(t, root, "a.txt", "changed\n"); run(t, root, "add", "a.txt") }},
		{"mode", func(t *testing.T, root string) { run(t, root, "update-index", "--chmod=+x", "a.txt") }},
		{"rename", func(t *testing.T, root string) { run(t, root, "mv", "a.txt", "renamed.txt") }},
		{"add", func(t *testing.T, root string) { write(t, root, "new.txt", "new\n"); run(t, root, "add", "new.txt") }},
		{"delete", func(t *testing.T, root string) { run(t, root, "rm", "-q", "dir/b.txt") }},
		{"sibling named like the excluded directory", func(t *testing.T, root string) {
			write(t, root, ".metron-other", "x\n")
			run(t, root, "add", ".metron-other")
		}},
		{"nested .metron directory", func(t *testing.T, root string) {
			write(t, root, "sub/.metron/x", "x\n")
			run(t, root, "add", "sub/.metron/x")
		}},
		{"path with spaces and non-ASCII", func(t *testing.T, root string) {
			write(t, root, "dir with space/naïve é.txt", "x\n")
			run(t, root, "add", "dir with space")
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root, r := baseRepo(t)
			base := head(t, r)
			before := digest(t, r, base)
			tt.change(t, root)
			// Commit the index as staged; adding everything again would undo
			// the mode change on file systems that record modes.
			run(t, root, "commit", "-q", "-m", tt.name)
			after := head(t, r)
			if after == base {
				t.Fatal("no commit was made")
			}
			if got := digest(t, r, after); got == before {
				t.Fatalf("digest did not change: %s", got)
			}
			if got := digest(t, r, base); got != before {
				t.Fatalf("digest of the base commit changed: %s", got)
			}
		})
	}
}

func TestCandidateDigestRejects(t *testing.T) {
	_, r := baseRepo(t)
	tree := run(t, r.Root(), "rev-parse", "HEAD^{tree}")
	for _, bad := range []string{"", "HEAD", "--output=x", "-r", strings.ToUpper(head(t, r)), head(t, r)[:12], tree, strings.Repeat("0", 40)} {
		if _, err := r.CandidateDigest(bad); err == nil {
			t.Errorf("CandidateDigest(%q) succeeded", bad)
		}
	}
}

func TestDirtyPaths(t *testing.T) {
	tests := []struct {
		name   string
		change func(t *testing.T, root string)
		want   []string
	}{
		{"clean", func(t *testing.T, root string) {}, []string{}},
		{"tracked change under .metron", func(t *testing.T, root string) { write(t, root, ".metron/contract.yaml", "x\n") }, []string{}},
		{"untracked under .metron", func(t *testing.T, root string) { write(t, root, ".metron/state/ledger.jsonl", "{}\n") }, []string{}},
		{"staged under .metron", func(t *testing.T, root string) {
			write(t, root, ".metron/state/new", "x\n")
			run(t, root, "add", ".metron")
		}, []string{}},
		{"untracked file", func(t *testing.T, root string) { write(t, root, "untracked.txt", "x\n") }, []string{"untracked.txt"}},
		{"untracked file in a new directory", func(t *testing.T, root string) { write(t, root, "new/deep/f.txt", "x\n") }, []string{"new/deep/f.txt"}},
		{"modified file", func(t *testing.T, root string) { write(t, root, "a.txt", "x\n") }, []string{"a.txt"}},
		{"deleted file", func(t *testing.T, root string) { os.Remove(filepath.Join(root, "dir", "b.txt")) }, []string{"dir/b.txt"}},
		{"staged file", func(t *testing.T, root string) {
			write(t, root, "s.txt", "x\n")
			run(t, root, "add", "s.txt")
		}, []string{"s.txt"}},
		{"staged rename", func(t *testing.T, root string) { run(t, root, "mv", "a.txt", "moved.txt") }, []string{"a.txt", "moved.txt"}},
		{"sibling named like the excluded directory", func(t *testing.T, root string) { write(t, root, ".metron-other", "x\n") }, []string{".metron-other"}},
		{"nested .metron directory", func(t *testing.T, root string) { write(t, root, "sub/.metron/x", "x\n") }, []string{"sub/.metron/x"}},
		{"unusual characters", func(t *testing.T, root string) {
			write(t, root, "dir with space/naïve é.txt", "x\n")
			write(t, root, "tab\tand \"quote\".txt", "x\n")
		}, []string{"dir with space/naïve é.txt", "tab\tand \"quote\".txt"}},
		{"ignored file", func(t *testing.T, root string) {
			write(t, root, ".gitignore", "*.log\n")
			run(t, root, "add", ".gitignore")
			run(t, root, "commit", "-q", "-m", "ignore")
			write(t, root, "debug.log", "x\n")
		}, []string{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root, r := baseRepo(t)
			if tt.name == "unusual characters" && !canCreate(root, "tab\tand \"quote\".txt") {
				tt.want = tt.want[:1]
				tt.change = func(t *testing.T, root string) { write(t, root, "dir with space/naïve é.txt", "x\n") }
			}
			tt.change(t, root)
			got, err := r.DirtyPaths()
			if err != nil {
				t.Fatalf("DirtyPaths: %v", err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("DirtyPaths = %q, want %q", got, tt.want)
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
func TestDirtyPathsIgnoresConfiguration(t *testing.T) {
	root, r := baseRepo(t)
	run(t, root, "config", "status.showUntrackedFiles", "no")
	run(t, root, "config", "status.branch", "true")
	run(t, root, "config", "status.renames", "true")
	run(t, root, "config", "core.quotePath", "true")
	write(t, root, "naïve/new.txt", "x\n")
	run(t, root, "mv", "a.txt", "é.txt")
	got, err := r.DirtyPaths()
	if err != nil {
		t.Fatalf("DirtyPaths: %v", err)
	}
	if want := []string{"a.txt", "é.txt", "naïve/new.txt"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("DirtyPaths = %q, want %q", got, want)
	}
}

func TestRepositoryPathWithSpacesAndNonASCII(t *testing.T) {
	root, r := newRepo(t, "my répo ü")
	write(t, root, "f.txt", "x\n")
	c := commit(t, root, "c1")
	sub := filepath.Join(root, "deep dir")
	if err := os.Mkdir(sub, 0o777); err != nil {
		t.Fatal(err)
	}
	fromSub, err := Open(sub)
	if err != nil {
		t.Fatalf("Open from a subdirectory: %v", err)
	}
	same := func(a, b string) bool {
		ra, err1 := filepath.EvalSymlinks(a)
		rb, err2 := filepath.EvalSymlinks(b)
		return err1 == nil && err2 == nil && ra == rb
	}
	if !same(fromSub.Root(), root) || !same(r.Root(), root) {
		t.Fatalf("Root = %q and %q, want %q", fromSub.Root(), r.Root(), root)
	}
	if got := head(t, fromSub); got != c {
		t.Fatalf("Head = %s, want %s", got, c)
	}
	digest(t, fromSub, c)
	if paths, err := fromSub.DirtyPaths(); err != nil || len(paths) != 0 {
		t.Fatalf("DirtyPaths = %q, %v", paths, err)
	}
}

func TestOpenOutsideRepository(t *testing.T) {
	if _, err := Open(t.TempDir()); err == nil {
		t.Fatal("Open outside a repository succeeded")
	}
}

func TestHeadWithoutCommits(t *testing.T) {
	_, r := newRepo(t, "empty")
	if _, err := r.Head(); !errors.Is(err, ErrNoCommits) {
		t.Fatalf("Head = %v, want ErrNoCommits", err)
	}
}

func TestCommitsSince(t *testing.T) {
	root, r := baseRepo(t)
	base := head(t, r)
	if got, err := r.CommitsSince(base); err != nil || len(got) != 0 {
		t.Fatalf("CommitsSince(HEAD) = %q, %v", got, err)
	}
	write(t, root, "a.txt", "1\n")
	c1 := commit(t, root, "c1")
	write(t, root, "a.txt", "2\n")
	c2 := commit(t, root, "c2")
	got, err := r.CommitsSince(base)
	if err != nil {
		t.Fatalf("CommitsSince: %v", err)
	}
	if want := []string{c2, c1}; !reflect.DeepEqual(got, want) {
		t.Fatalf("CommitsSince = %q, want %q", got, want)
	}
	tree := run(t, root, "rev-parse", "HEAD^{tree}")
	for _, bad := range []string{"HEAD", "--all", base[:10], tree, strings.Repeat("0", 40)} {
		if _, err := r.CommitsSince(bad); err == nil {
			t.Errorf("CommitsSince(%q) succeeded", bad)
		}
	}
}

func TestIdentity(t *testing.T) {
	root, r := baseRepo(t)
	if _, _, err := r.Identity(); err == nil || !strings.Contains(err.Error(), "user.name is not set") {
		t.Fatalf("Identity without configuration = %v", err)
	}
	run(t, root, "config", "user.name", "Ada Lovelace")
	if _, _, err := r.Identity(); err == nil || !strings.Contains(err.Error(), "user.email is not set") {
		t.Fatalf("Identity without an email = %v", err)
	}
	run(t, root, "config", "user.email", "ada@example.com")
	name, email, err := r.Identity()
	if err != nil || name != "Ada Lovelace" || email != "ada@example.com" {
		t.Fatalf("Identity = %q, %q, %v", name, email, err)
	}
}
