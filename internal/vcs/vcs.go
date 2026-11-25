// Package vcs observes a git repository through the git executable: the
// candidate commit and its digest, working-tree cleanliness, the commits a
// report lists, and the configured identity.
//
// docs/architecture.md, section 5, defines the candidate digest and
// cleanliness.
package vcs

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"

	"github.com/patacca/metron/internal/canon"
	"github.com/patacca/metron/internal/core"
)

// ExcludedDir is the directory, relative to the repository root, that the
// candidate digest and cleanliness ignore.
const ExcludedDir = ".metron/"

// ErrNoCommits is returned by Head for a repository without commits.
var ErrNoCommits = errors.New("the repository has no commits; commit the work to review first")

// Repo is a git work tree.
type Repo struct {
	root string
}

// Open locates the root of the git work tree that contains dir.
func Open(dir string) (*Repo, error) {
	out, err := git(dir, "rev-parse", "--show-toplevel")
	if err != nil {
		return nil, fmt.Errorf("%s is not inside a git work tree: %w", dir, err)
	}
	root := strings.TrimSuffix(string(out), "\n")
	if root == "" {
		return nil, fmt.Errorf("%s is not inside a git work tree", dir)
	}
	return &Repo{root: filepath.FromSlash(root)}, nil
}

// Root returns the repository's root directory.
func (r *Repo) Root() string { return r.root }

// Head returns the full hash of the commit HEAD points to. A repository
// without commits gives an error that wraps ErrNoCommits.
func (r *Repo) Head() (string, error) {
	out, err := git(r.root, "rev-parse", "--verify", "--quiet", "HEAD^{commit}")
	if err != nil {
		var ge *Error
		if errors.As(err, &ge) && ge.ExitCode == 1 && len(out) == 0 {
			return "", ErrNoCommits
		}
		return "", err
	}
	hash := strings.TrimSuffix(string(out), "\n")
	if !isHash(hash) {
		return "", fmt.Errorf("git rev-parse returned %q, not a commit hash", hash)
	}
	return hash, nil
}

// CandidateDigest returns the candidate digest of a commit, given as a full
// hash: the SHA-256 of the output of `git ls-tree -r -z --full-tree <commit>`
// without the entries whose path lies under .metron/, written
// "sha256:<hex>".
func (r *Repo) CandidateDigest(commit string) (string, error) {
	if !isHash(commit) {
		return "", fmt.Errorf("invalid commit %q: expected a full hexadecimal commit hash", commit)
	}
	out, err := git(r.root, "ls-tree", "-r", "-z", "--full-tree", commit+"^{commit}")
	if err != nil {
		return "", err
	}
	var kept []byte
	for len(out) > 0 {
		end := bytes.IndexByte(out, 0)
		if end < 0 {
			return "", errors.New("git ls-tree output ends without a NUL terminator")
		}
		entry := out[:end+1]
		out = out[end+1:]
		tab := bytes.IndexByte(entry, '\t')
		if tab < 0 {
			return "", fmt.Errorf("unexpected git ls-tree entry %q", entry)
		}
		if bytes.HasPrefix(entry[tab+1:], []byte(ExcludedDir)) {
			continue
		}
		kept = append(kept, entry...)
	}
	return canon.Digest(kept), nil
}

// Describe describes the commits a report lists, one entry per hash in input
// order, relative to HEAD and to base, the commit the round reviewed:
//
//   - Reachable: the hash names a commit that is HEAD or an ancestor of it.
//   - InBase: the hash names a commit that is base or an ancestor of it. It is
//     false when base no longer exists, for example after history was
//     rewritten and garbage-collected.
//   - Substantive: the hash names a commit whose candidate digest differs
//     from its first parent's, merges included, or a root commit whose tree
//     has an entry outside .metron/.
//
// A hash that is unknown, malformed or names another kind of object is
// described with every flag false. Only a failure to run git is an error.
func (r *Repo) Describe(base string, commits []string) ([]core.Commit, error) {
	if !isHash(base) {
		return nil, fmt.Errorf("invalid base commit %q: expected a full hexadecimal commit hash", base)
	}
	var listed []string
	for _, c := range commits {
		if isHash(c) && !slices.Contains(listed, c) {
			listed = append(listed, c)
		}
	}
	// One query says which names are commits: the listed hashes, base and
	// HEAD, which does not exist before the first commit.
	names := append(slices.Clone(listed), base, "HEAD")
	out, err := gitInput(r.root, strings.Join(names, "\n")+"\n", "cat-file", "--batch-check=%(objectname) %(objecttype)")
	if err != nil {
		return nil, err
	}
	lines := strings.Split(strings.TrimSuffix(string(out), "\n"), "\n")
	if len(lines) != len(names) {
		return nil, fmt.Errorf("git cat-file answered %d of %d queries", len(lines), len(names))
	}
	isCommit := make([]bool, len(names))
	for i, line := range lines {
		isCommit[i] = strings.HasSuffix(line, " commit")
	}
	baseExists, headExists := isCommit[len(names)-2], isCommit[len(names)-1]

	var existing []string
	for i, c := range listed {
		if isCommit[i] {
			existing = append(existing, c)
		}
	}
	parents, err := r.parents(existing)
	if err != nil {
		return nil, err
	}
	described := map[string]core.Commit{}
	for _, c := range existing {
		d := core.Commit{Hash: c}
		if headExists {
			if d.Reachable, err = r.isAncestor(c, "HEAD"); err != nil {
				return nil, err
			}
		}
		if baseExists {
			if d.InBase, err = r.isAncestor(c, base); err != nil {
				return nil, err
			}
		}
		if d.Substantive, err = r.substantive(c, parents[c]); err != nil {
			return nil, err
		}
		described[c] = d
	}
	result := make([]core.Commit, len(commits))
	for i, c := range commits {
		if d, ok := described[c]; ok {
			result[i] = d
		} else {
			result[i] = core.Commit{Hash: c}
		}
	}
	return result, nil
}

// parents returns the parents of existing commits, first parent first.
func (r *Repo) parents(commits []string) (map[string][]string, error) {
	parents := map[string][]string{}
	if len(commits) == 0 {
		return parents, nil
	}
	out, err := git(r.root, append([]string{"rev-list", "--no-walk=unsorted", "--parents"}, commits...)...)
	if err != nil {
		return nil, err
	}
	for _, line := range strings.Split(strings.TrimSuffix(string(out), "\n"), "\n") {
		hashes := strings.Fields(line)
		for _, h := range hashes {
			if !isHash(h) {
				return nil, fmt.Errorf("git rev-list returned %q, not a commit hash", h)
			}
		}
		if len(hashes) > 0 {
			parents[hashes[0]] = hashes[1:]
		}
	}
	return parents, nil
}

// isAncestor reports whether commit a is b or an ancestor of b.
func (r *Repo) isAncestor(a, b string) (bool, error) {
	_, err := git(r.root, "merge-base", "--is-ancestor", a, b)
	var ge *Error
	switch {
	case err == nil:
		return true, nil
	case errors.As(err, &ge) && ge.ExitCode == 1:
		return false, nil
	default:
		return false, err
	}
}

// substantive reports whether a commit changes an entry outside .metron/
// relative to its first parent, or, for a root commit, has one. Comparing
// the two trees path by path is equivalent to comparing their candidate
// digests, and costs only the size of the difference.
func (r *Repo) substantive(commit string, parents []string) (bool, error) {
	if len(parents) == 0 {
		d, err := r.CandidateDigest(commit)
		return d != canon.Digest(nil), err
	}
	out, err := git(r.root, "diff-tree", "-r", "-z", "--name-only", "--no-renames", "--ignore-submodules=none", parents[0], commit)
	if err != nil {
		return false, err
	}
	for _, path := range strings.Split(string(out), "\x00") {
		if path != "" && !strings.HasPrefix(path, ExcludedDir) {
			return true, nil
		}
	}
	return false, nil
}

// Identity returns the user.name and user.email configured for the
// repository.
func (r *Repo) Identity() (name, email string, err error) {
	if name, err = r.config("user.name"); err != nil {
		return "", "", err
	}
	if email, err = r.config("user.email"); err != nil {
		return "", "", err
	}
	return name, email, nil
}

func (r *Repo) config(key string) (string, error) {
	out, err := git(r.root, "config", "-z", "--get", key)
	var ge *Error
	if errors.As(err, &ge) && ge.ExitCode == 1 {
		return "", fmt.Errorf("git %s is not set; set it with: git config %s <value>", key, key)
	}
	if err != nil {
		return "", err
	}
	value := strings.TrimSuffix(string(out), "\x00")
	if strings.TrimSpace(value) == "" {
		return "", fmt.Errorf("git %s is empty; set it with: git config %s <value>", key, key)
	}
	return value, nil
}

// Error is a git command that exited with a non-zero status.
type Error struct {
	Args     []string
	ExitCode int
	Stderr   string
}

func (e *Error) Error() string {
	msg := fmt.Sprintf("git %s exited with status %d", strings.Join(e.Args, " "), e.ExitCode)
	if e.Stderr != "" {
		msg += ": " + e.Stderr
	}
	return msg
}

// git runs git in dir and returns its standard output. Messages are in the
// C locale, and status takes no optional locks, so that observing a
// repository never competes with the user's git commands. Replace refs are
// ignored so that a commit always means its own tree.
func git(dir string, args ...string) ([]byte, error) {
	return gitInput(dir, "", args...)
}

// gitInput is git with input on its standard input.
func gitInput(dir, input string, args ...string) ([]byte, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Stdin = strings.NewReader(input)
	cmd.Env = append(os.Environ(), "LC_ALL=C", "GIT_OPTIONAL_LOCKS=0", "GIT_NO_REPLACE_OBJECTS=1")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			return out, &Error{Args: args, ExitCode: ee.ExitCode(), Stderr: strings.TrimSpace(stderr.String())}
		}
		if errors.Is(err, exec.ErrNotFound) {
			return nil, fmt.Errorf("git is not installed or not on PATH; Metron requires it: %w", err)
		}
		return nil, fmt.Errorf("run git %s: %w", strings.Join(args, " "), err)
	}
	return out, nil
}

// isHash reports whether s is a full SHA-1 or SHA-256 object name in
// lowercase hex. Such a string can never be read as an option.
func isHash(s string) bool {
	if len(s) != 40 && len(s) != 64 {
		return false
	}
	for i := 0; i < len(s); i++ {
		if c := s[i]; !('0' <= c && c <= '9' || 'a' <= c && c <= 'f') {
			return false
		}
	}
	return true
}
