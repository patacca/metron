// Package vcs observes a git repository through the git executable: the
// candidate commit and its digest, working-tree cleanliness, history between
// candidates, and the configured identity.
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
	"strings"

	"github.com/patacca/metron/internal/canon"
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

// DirtyPaths returns the paths, relative to the root and slash-separated,
// that `git status --porcelain` reports outside .metron/, untracked files
// included. The working tree is clean when there are none.
func (r *Repo) DirtyPaths() ([]string, error) {
	// The flags pin down what configuration could otherwise change:
	// untracked files are listed one by one, submodules are not ignored,
	// renames are reported as a deletion and an addition, and there is no
	// branch header.
	out, err := git(r.root, "status", "--porcelain", "-z", "--untracked-files=all",
		"--ignore-submodules=none", "--no-renames", "--no-branch")
	if err != nil {
		return nil, err
	}
	paths := []string{}
	rest := string(out)
	for rest != "" {
		entry, after, ok := strings.Cut(rest, "\x00")
		if !ok || len(entry) < 4 || entry[2] != ' ' {
			return nil, fmt.Errorf("unexpected git status entry %q", entry)
		}
		rest = after
		changed := []string{entry[3:]}
		if strings.ContainsAny(entry[:2], "RC") {
			// The original path follows as a field of its own.
			from, after, ok := strings.Cut(rest, "\x00")
			if !ok {
				return nil, fmt.Errorf("git status entry %q lacks its original path", entry)
			}
			rest = after
			changed = append(changed, from)
		}
		for _, p := range changed {
			if !strings.HasPrefix(p, ExcludedDir) {
				paths = append(paths, p)
			}
		}
	}
	return paths, nil
}

// CommitsSince returns the full hashes of the commits reachable from HEAD and
// not from base, newest first.
func (r *Repo) CommitsSince(base string) ([]string, error) {
	if !isHash(base) {
		return nil, fmt.Errorf("invalid base commit %q: expected a full hexadecimal commit hash", base)
	}
	out, err := git(r.root, "rev-list", "HEAD", "^"+base+"^{commit}")
	if err != nil {
		return nil, err
	}
	commits := strings.Fields(string(out))
	for _, c := range commits {
		if !isHash(c) {
			return nil, fmt.Errorf("git rev-list returned %q, not a commit hash", c)
		}
	}
	return commits, nil
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
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
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
