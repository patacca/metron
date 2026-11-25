package vcs

import (
	"bytes"
	"cmp"
	"fmt"
	"path"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
)

// Kinds of TreeProblem.
const (
	// TreeModified is a tracked path that git status reports as changed,
	// staged or not.
	TreeModified = "modified"
	// TreeUntracked is a file git status reports as untracked.
	TreeUntracked = "untracked"
	// TreeHidden is a tracked file with the assume-unchanged or skip-worktree
	// flag, under which git reports a modified file as unchanged. A sparse
	// checkout sets skip-worktree on the files it leaves out.
	TreeHidden = "hidden"
	// TreeLink is a tracked symbolic link that points into .metron/, whose
	// content is not part of the candidate.
	TreeLink = "link"
)

// TreeProblem is one reason the working tree is not clean.
type TreeProblem struct {
	// Path is relative to the root and slash-separated.
	Path string
	// Kind is TreeModified, TreeUntracked, TreeHidden or TreeLink.
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

// statusProblems returns what `git status --porcelain` reports.
func (r *Repo) statusProblems() ([]TreeProblem, error) {
	// The flags pin down what configuration could otherwise change:
	// untracked files are listed one by one, submodules are not ignored,
	// renames are reported as a deletion and an addition, and there is no
	// branch header.
	out, err := git(r.root, "status", "--porcelain", "-z", "--untracked-files=all",
		"--ignore-submodules=none", "--no-renames", "--no-branch")
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
		kind := TreeModified
		if entry[:2] == "??" {
			kind = TreeUntracked
		}
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

// indexProblems returns the hidden files and the links into .metron/ that
// the index records.
func (r *Repo) indexProblems() ([]TreeProblem, error) {
	// Each entry is "<tag> <mode> <object> <stage>\t<path>". The tag is
	// lower case for assume-unchanged and S for skip-worktree.
	out, err := git(r.root, "ls-files", "-v", "-s", "-z")
	if err != nil {
		return nil, err
	}
	var problems []TreeProblem
	var links, objects []string
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
			objects = append(objects, fields[2])
		}
	}
	targets, err := r.blobs(objects)
	if err != nil {
		return nil, err
	}
	for i, link := range links {
		if r.pointsIntoMetron(link, targets[i]) {
			problems = append(problems, TreeProblem{Path: link, Kind: TreeLink})
		}
	}
	return problems, nil
}

// blobs returns the contents of blobs, given by object name, in order.
func (r *Repo) blobs(objects []string) ([]string, error) {
	if len(objects) == 0 {
		return nil, nil
	}
	out, err := gitInput(r.root, strings.Join(objects, "\n")+"\n", "cat-file", "--batch")
	if err != nil {
		return nil, err
	}
	// Each answer is "<object> <type> <size>\n<content>\n".
	contents := make([]string, 0, len(objects))
	for range objects {
		header, rest, ok := bytes.Cut(out, []byte("\n"))
		fields := strings.Fields(string(header))
		if !ok || len(fields) != 3 || fields[1] != "blob" {
			return nil, fmt.Errorf("unexpected git cat-file answer %q", header)
		}
		size, err := strconv.Atoi(fields[2])
		if err != nil || size < 0 || size+1 > len(rest) || rest[size] != '\n' {
			return nil, fmt.Errorf("unexpected git cat-file answer %q", header)
		}
		contents = append(contents, string(rest[:size]))
		out = rest[size+1:]
	}
	return contents, nil
}

// pointsIntoMetron reports whether a link's target, resolved lexically, is
// .metron/ at the root or lies under it. A relative target is resolved from
// the link's directory as an absolute path, so that one which leaves the
// repository and comes back in is resolved like any other. Backslashes count
// as separators and the directory name is compared without regard to case,
// so that the answer is the same on every platform and errs towards
// reporting a problem.
func (r *Repo) pointsIntoMetron(link, target string) bool {
	target = strings.ReplaceAll(target, `\`, "/")
	native := filepath.FromSlash(target)
	if !path.IsAbs(target) && !filepath.IsAbs(native) {
		native = filepath.Join(r.root, filepath.FromSlash(path.Dir(link)), native)
	}
	rel, err := filepath.Rel(r.root, filepath.Clean(native))
	if err != nil {
		return false
	}
	first, _, _ := strings.Cut(filepath.ToSlash(rel), "/")
	return strings.EqualFold(first, strings.TrimSuffix(ExcludedDir, "/"))
}
