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
	"slices"
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
	if problems, err := fromSub.TreeProblems(); err != nil || len(problems) != 0 {
		t.Fatalf("TreeProblems = %v, %v", problems, err)
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

// unusualName is a path with a space and non-ASCII letters, which git quotes
// unless asked for NUL-terminated output. Every platform accepts it.
const unusualName = "dir with space/naïve é.txt"

// describeRepo builds a history with one commit of each kind Describe must
// tell apart, and returns their hashes by name. HEAD is the last merge.
func describeRepo(t *testing.T) (string, *Repo, map[string]string) {
	t.Helper()
	root, r := baseRepo(t)
	h := map[string]string{"root": head(t, r)}
	step := func(name string, change func()) {
		change()
		h[name] = commit(t, root, name)
	}
	step("fix", func() { write(t, root, "a.txt", "fixed\n") })
	step("only .metron", func() { write(t, root, ".metron/state/ledger.jsonl", "{}\n") })
	step("empty", func() {})
	// The mode changes on disk too, where the file system records it, so that
	// adding everything for a later commit does not change it back.
	if err := os.Chmod(filepath.Join(root, "a.txt"), 0o755); err != nil {
		t.Fatal(err)
	}
	run(t, root, "update-index", "--chmod=+x", "a.txt")
	run(t, root, "commit", "-q", "-m", "mode")
	h["mode"] = head(t, r)
	step("both", func() {
		write(t, root, "dir/b.txt", "both\n")
		write(t, root, ".metron/state/ledger.jsonl", "{}\n{}\n")
	})
	step("two directories", func() {
		write(t, root, "src/one.go", "package one\n")
		write(t, root, "docs/two.md", "two\n")
	})
	step("unusual name", func() { write(t, root, unusualName, "x\n") })
	step("delete", func() { run(t, root, "rm", "-q", "docs/two.md") })

	run(t, root, "checkout", "-q", "-b", "other")
	step("other branch", func() { write(t, root, "other.txt", "o\n") })
	run(t, root, "checkout", "-q", "main")

	run(t, root, "checkout", "-q", "-b", "side")
	step("side", func() { write(t, root, "side.txt", "s\n") })
	run(t, root, "checkout", "-q", "main")
	step("main, only .metron", func() { write(t, root, ".metron/x", "x\n") })
	run(t, root, "merge", "-q", "--no-ff", "--no-edit", "side")
	h["merge"] = head(t, r)

	run(t, root, "checkout", "-q", "-b", "side2")
	step("side2, only .metron", func() { write(t, root, ".metron/y", "y\n") })
	run(t, root, "checkout", "-q", "main")
	run(t, root, "merge", "-q", "--no-ff", "--no-edit", "side2")
	h["merge of .metron"] = head(t, r)

	run(t, root, "checkout", "-q", "--orphan", "lone")
	run(t, root, "rm", "-rfq", ".")
	step("root with only .metron", func() { write(t, root, ".metron/only", "x\n") })
	run(t, root, "checkout", "-q", "main")
	return root, r, h
}

func TestDescribe(t *testing.T) {
	root, r, h := describeRepo(t)
	missing := strings.Repeat("e", 40) // well formed, no such object
	type flags struct{ reachable, inBase, substantive bool }
	commits := map[string]struct { // with base "fix"
		flags
		paths []string
	}{
		"root":                   {flags{true, true, true}, []string{"a.txt", "dir/b.txt"}},
		"fix":                    {flags{true, true, true}, []string{"a.txt"}},
		"only .metron":           {flags{true, false, false}, nil},
		"empty":                  {flags{true, false, false}, nil},
		"mode":                   {flags{true, false, true}, []string{"a.txt"}},
		"both":                   {flags{true, false, true}, []string{"dir/b.txt"}},
		"two directories":        {flags{true, false, true}, []string{"docs/two.md", "src/one.go"}},
		"unusual name":           {flags{true, false, true}, []string{unusualName}},
		"delete":                 {flags{true, false, true}, []string{"docs/two.md"}},
		"other branch":           {flags{false, false, true}, []string{"other.txt"}},
		"side":                   {flags{true, false, true}, []string{"side.txt"}},
		"main, only .metron":     {flags{true, false, false}, nil},
		"merge":                  {flags{true, false, true}, []string{"side.txt"}}, // not .metron/x, from its second parent
		"side2, only .metron":    {flags{true, false, false}, nil},
		"merge of .metron":       {flags{true, false, false}, nil},
		"root with only .metron": {flags{false, false, false}, nil},
	}
	var names, input []string
	for name := range h {
		names = append(names, name)
	}
	slices.Sort(names)
	for _, name := range names {
		input = append(input, h[name])
	}
	tree := run(t, root, "rev-parse", "HEAD^{tree}")
	invalid := []string{missing, "HEAD", strings.ToUpper(h["fix"]), h["fix"][:12], tree, "", "--all"}
	input = append(input, invalid...)
	input = append(input, h["fix"]) // repeated

	for _, tt := range []struct {
		name     string
		base     string
		baseGone bool
	}{
		{"base is an ancestor of HEAD", h["fix"], false},
		{"base no longer exists", strings.Repeat("d", 40), true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got, err := r.Describe(tt.base, input)
			if err != nil {
				t.Fatalf("Describe: %v", err)
			}
			if len(got) != len(input) {
				t.Fatalf("got %d entries for %d hashes", len(got), len(input))
			}
			for i, c := range got {
				if c.Hash != input[i] {
					t.Fatalf("entry %d is %s, want %s", i, c.Hash, input[i])
				}
			}
			for i, name := range names {
				want := commits[name]
				if tt.baseGone {
					want.inBase = false
				}
				c := got[i]
				if (flags{c.Reachable, c.InBase, c.Substantive}) != want.flags || !slices.Equal(c.Paths, want.paths) {
					t.Errorf("%s: got %+v, want %+v", name, c, want)
				}
			}
			for i, c := range got[len(names) : len(names)+len(invalid)] {
				if c.Reachable || c.InBase || c.Substantive || c.Paths != nil {
					t.Errorf("hash %q: got %+v, want every flag false and no paths", invalid[i], c)
				}
			}
			for _, c := range got {
				if c.Substantive != (len(c.Paths) > 0) {
					t.Errorf("%s: Substantive is %v with paths %q", c.Hash, c.Substantive, c.Paths)
				}
			}
			if repeated, first := got[len(got)-1], got[slices.Index(names, "fix")]; !reflect.DeepEqual(repeated, first) {
				t.Errorf("repeated hash: got %+v, want %+v", repeated, first)
			}
		})
	}

	// Substantive agrees with a comparison of candidate digests.
	for _, name := range names {
		parents := strings.Fields(run(t, root, "rev-list", "--no-walk", "--parents", h[name]))[1:]
		if len(parents) == 0 {
			continue
		}
		changed := digest(t, r, h[name]) != digest(t, r, parents[0])
		if changed != commits[name].substantive {
			t.Errorf("%s: digests differ = %v, expected substantive = %v", name, changed, commits[name].substantive)
		}
	}

	if _, err := r.Describe("HEAD", input); err == nil {
		t.Error("Describe with a malformed base succeeded")
	}
	if got, err := r.Describe(h["fix"], nil); err != nil || len(got) != 0 {
		t.Errorf("Describe of no commits = %+v, %v", got, err)
	}
}

func TestDescribeWithoutCommits(t *testing.T) {
	_, r := newRepo(t, "empty")
	hash := strings.Repeat("a", 40)
	got, err := r.Describe(hash, []string{hash})
	if err != nil || len(got) != 1 || got[0].Reachable || got[0].InBase || got[0].Substantive {
		t.Fatalf("Describe = %+v, %v", got, err)
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
