package vcs

import (
	"cmp"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"unicode"
)

// Kinds of TreeProblem.
const (
	// TreeModified is a tracked path that git status reports as changed,
	// staged or not, other than a deletion.
	TreeModified = "modified"
	// TreeDeleted is a tracked path that git status reports as deleted,
	// staged or not.
	TreeDeleted = "deleted"
	// TreeUntracked is a file git status reports as untracked.
	TreeUntracked = "untracked"
	// TreeHidden is a tracked file with the assume-unchanged or skip-worktree
	// flag, under which git reports a modified file as unchanged, in the
	// repository or in one of its submodules. A sparse checkout sets
	// skip-worktree on the files it leaves out.
	TreeHidden = "hidden"
	// TreeLink is a tracked symbolic link that resolves into .metron/ or the
	// git directory, whose content is not part of the candidate.
	TreeLink = "link"
)

// TreeProblem is one reason the working tree is not clean.
type TreeProblem struct {
	// Path is relative to the root and slash-separated.
	Path string
	// Kind is TreeModified, TreeDeleted, TreeUntracked, TreeHidden or
	// TreeLink.
	Kind string
}

// TreeProblems returns the reasons the working tree is not clean, as
// docs/architecture.md section 5 defines it, ordered by path and then kind.
// Paths under .metron/ are not examined. The tree is clean when there are
// none.
func (r *Repo) TreeProblems() ([]TreeProblem, error) {
	problems, err := r.statusProblems()
	if err != nil {
		return nil, err
	}
	index, err := r.indexProblems()
	if err != nil {
		return nil, err
	}
	problems = append(problems, index...)
	slices.SortFunc(problems, func(a, b TreeProblem) int {
		return cmp.Or(strings.Compare(a.Path, b.Path), strings.Compare(a.Kind, b.Kind))
	})
	return slices.Compact(problems), nil
}

// strictStat keeps a repository's configuration from making git skip a
// changed file: no file system monitor, and git's default, strictest
// comparison of file times. core.fileMode and core.ignoreCase are left as
// configured, because forcing them breaks honest setups on Windows and on
// case-insensitive file systems.
var strictStat = []string{"-c", "core.fsmonitor=false", "-c", "core.checkStat=default", "-c", "core.trustctime=true"}

// statusProblems returns what `git status --porcelain` reports.
func (r *Repo) statusProblems() ([]TreeProblem, error) {
	// The flags pin down what configuration could otherwise change:
	// untracked files are listed one by one, submodules are not ignored,
	// renames are reported as a deletion and an addition, and there is no
	// branch header.
	out, err := git(r.root, append(slices.Clone(strictStat), "status", "--porcelain", "-z",
		"--untracked-files=all", "--ignore-submodules=none", "--no-renames", "--no-branch")...)
	if err != nil {
		return nil, err
	}
	problems := []TreeProblem{}
	for rest := string(out); rest != ""; {
		entry, after, ok := strings.Cut(rest, "\x00")
		if !ok || len(entry) < 4 || entry[2] != ' ' {
			return nil, fmt.Errorf("unexpected git status entry %q", entry)
		}
		rest = after
		kind := statusKind(entry[:2])
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
				problems = append(problems, TreeProblem{Path: p, Kind: kind})
			}
		}
	}
	return problems, nil
}

// statusKind classifies a porcelain status code. Unmerged paths are
// modified, whatever each side did.
func statusKind(xy string) string {
	switch {
	case xy == "??":
		return TreeUntracked
	case xy == "DD" || xy == "AA" || strings.Contains(xy, "U"):
		return TreeModified
	case strings.Contains(xy, "D"):
		return TreeDeleted
	default:
		return TreeModified
	}
}

// indexProblems returns the hidden files and the links into .metron/ or the
// git directory that the index records, submodules included.
func (r *Repo) indexProblems() ([]TreeProblem, error) {
	// Each entry is "<tag> <mode> <object> <stage>\t<path>". The tag is
	// lower case for assume-unchanged and S for skip-worktree. Entries of
	// submodules carry their full path.
	out, err := git(r.root, append(slices.Clone(strictStat), "ls-files", "-v", "-s", "-z", "--recurse-submodules")...)
	if err != nil {
		return nil, err
	}
	var problems []TreeProblem
	var links []string
	for rest := string(out); rest != ""; {
		entry, after, ok := strings.Cut(rest, "\x00")
		rest = after
		meta, p, tab := strings.Cut(entry, "\t")
		fields := strings.Fields(meta)
		if !ok || !tab || len(fields) != 4 || len(fields[0]) != 1 || !isHash(fields[2]) {
			return nil, fmt.Errorf("unexpected git ls-files entry %q", entry)
		}
		if strings.HasPrefix(p, ExcludedDir) {
			continue
		}
		if tag := fields[0][0]; tag == 'S' || ('a' <= tag && tag <= 'z') {
			problems = append(problems, TreeProblem{Path: p, Kind: TreeHidden})
		}
		if fields[1] == "120000" {
			links = append(links, p)
		}
	}
	if len(links) == 0 {
		return problems, nil
	}
	scope, err := r.excluded()
	if err != nil {
		return nil, err
	}
	for _, link := range links {
		into, err := r.linkInto(link, scope)
		if err != nil {
			return nil, err
		}
		if into {
			problems = append(problems, TreeProblem{Path: link, Kind: TreeLink})
		}
	}
	return problems, nil
}

// excludedDirs holds the directories no tracked link may reach: the roots
// under which .metron/ lies, as git spells the root and fully resolved, and
// the git directories.
type excludedDirs struct {
	roots, gitDirs []string
}

func (r *Repo) excluded() (excludedDirs, error) {
	out, err := git(r.root, "rev-parse", "--path-format=absolute", "--git-dir", "--git-common-dir")
	if err != nil {
		return excludedDirs{}, err
	}
	withResolved := func(dirs ...string) []string {
		var all []string
		for _, d := range dirs {
			all = append(all, d)
			if real, err := filepath.EvalSymlinks(d); err == nil && real != d {
				all = append(all, real)
			}
		}
		return all
	}
	var gitDirs []string
	for _, line := range strings.Split(strings.TrimSuffix(string(out), "\n"), "\n") {
		gitDirs = append(gitDirs, filepath.FromSlash(line))
	}
	return excludedDirs{roots: withResolved(r.root), gitDirs: withResolved(gitDirs...)}, nil
}

// contains reports whether the absolute path p is .metron/ at the root, lies
// under it, or lies in a git directory. The name .metron is compared without
// regard to case, which errs towards reporting a problem.
func (x excludedDirs) contains(p string) bool {
	for _, root := range x.roots {
		if rel, err := filepath.Rel(root, p); err == nil {
			first, _, _ := strings.Cut(filepath.ToSlash(rel), "/")
			if strings.EqualFold(first, strings.TrimSuffix(ExcludedDir, "/")) {
				return true
			}
		}
	}
	for _, d := range x.gitDirs {
		if rel, err := filepath.Rel(d, p); err == nil && rel != ".." && !strings.HasPrefix(filepath.ToSlash(rel), "../") {
			return true
		}
	}
	return false
}

// linkInto reports whether a tracked link reaches an excluded directory. The
// link is resolved through the file system, following every link on the
// way. When it does not resolve, because it dangles or loops, or when the
// platform checked it out as a file holding its target, its target is
// resolved lexically instead. A link missing from the working tree is
// reported by git status.
func (r *Repo) linkInto(link string, x excludedDirs) (bool, error) {
	full := filepath.Join(r.root, filepath.FromSlash(link))
	info, err := os.Lstat(full)
	if errors.Is(err, fs.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	var target string
	switch {
	case info.Mode()&fs.ModeSymlink != 0:
		if resolved, err := filepath.EvalSymlinks(full); err == nil {
			return x.contains(resolved), nil
		}
		if target, err = os.Readlink(full); err != nil {
			return false, err
		}
	case info.Mode().IsRegular():
		content, err := os.ReadFile(full)
		if err != nil {
			return false, err
		}
		target = string(content)
	default:
		return false, nil
	}
	// Lexically, from the link's directory. Backslashes count as separators,
	// so that the answer is the same on every platform.
	target = strings.ReplaceAll(target, `\`, "/")
	native := filepath.FromSlash(target)
	if !path.IsAbs(target) && !filepath.IsAbs(native) {
		native = filepath.Join(r.root, filepath.FromSlash(path.Dir(link)), native)
	}
	return x.contains(filepath.Clean(native)), nil
}

// SparseCheckout reports whether the working tree is a sparse checkout. The
// files a sparse checkout leaves out carry skip-worktree, so its tree is never
// clean.
func (r *Repo) SparseCheckout() (bool, error) {
	return r.configBool("core.sparseCheckout")
}

// IgnoreStat reports whether core.ignoreStat is set. Under it git marks the
// files it writes assume-unchanged, so the tree is not clean.
func (r *Repo) IgnoreStat() (bool, error) {
	return r.configBool("core.ignoreStat")
}

// IgnoreCaseMismatch reports whether git is set to ignore case
// (core.ignoreCase) while the file system distinguishes it, as after copying a
// repository from a case-insensitive system. Git then overlooks a file whose
// name differs from a tracked one only by case, so the tree is not clean. The
// file system is probed, without writing, only when the setting is true.
func (r *Repo) IgnoreCaseMismatch() (bool, error) {
	ignore, err := r.configBool("core.ignoreCase")
	if err != nil || !ignore {
		return false, err
	}
	// The .git entry of the work tree lies on the work tree's file system;
	// without one, the git directory itself is probed.
	probe := filepath.Join(r.root, ".git")
	if _, err := os.Lstat(probe); err != nil {
		out, err := git(r.root, "rev-parse", "--absolute-git-dir")
		if err != nil {
			return false, err
		}
		probe = filepath.FromSlash(strings.TrimSuffix(string(out), "\n"))
	}
	return distinguishesCase(probe)
}

// distinguishesCase reports whether the file system distinguishes case at an
// existing path: whether the last component spelled in the other case fails
// to name the same file. A component without letters cannot tell, so its
// nearest ancestor that has some is probed instead.
func distinguishesCase(p string) (bool, error) {
	for {
		base := filepath.Base(p)
		swapped := strings.Map(func(c rune) rune {
			if unicode.IsUpper(c) {
				return unicode.ToLower(c)
			}
			return unicode.ToUpper(c)
		}, base)
		if swapped == base {
			parent := filepath.Dir(p)
			if parent == p {
				return false, fmt.Errorf("cannot tell whether the file system distinguishes case: no component of %s has letters", p)
			}
			p = parent
			continue
		}
		info, err := os.Lstat(p)
		if err != nil {
			return false, err
		}
		other, err := os.Lstat(filepath.Join(filepath.Dir(p), swapped))
		if errors.Is(err, fs.ErrNotExist) {
			return true, nil
		}
		if err != nil {
			return false, err
		}
		return !os.SameFile(info, other), nil
	}
}

// configBool reads a boolean setting, false when unset. A value git cannot
// read as a boolean is an error.
func (r *Repo) configBool(key string) (bool, error) {
	out, err := git(r.root, "config", "-z", "--type=bool", "--get", key)
	var ge *Error
	if errors.As(err, &ge) && ge.ExitCode == 1 {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return strings.TrimSuffix(string(out), "\x00") == "true", nil
}
