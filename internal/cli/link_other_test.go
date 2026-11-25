//go:build !windows

package cli

import (
	"os"
	"strings"
	"testing"
)

// TestStateDirectoryThroughLink makes .metron/state, then .metron itself, a
// symbolic link into the working tree: the placement rule applies to where
// the state directory really is, for the default as well.
func TestStateDirectoryThroughLink(t *testing.T) {
	choices := "the state directory is either .metron/state, the default, or a directory outside the repository"
	for _, tc := range []struct {
		name string
		link func(r *repo)
	}{
		{".metron/state", func(r *repo) {
			r.write("elsewhere/keep", "")
			if err := os.MkdirAll(r.path(".metron"), 0o777); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(r.path("elsewhere"), r.path(".metron/state")); err != nil {
				t.Fatal(err)
			}
		}},
		{".metron", func(r *repo) {
			r.write("meta/keep", "")
			if err := os.Symlink(r.path("meta"), r.path(".metron")); err != nil {
				t.Fatal(err)
			}
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := newRepo(t)
			tc.link(r)
			res := r.expect(exitEnv, "init")
			contains(t, "init error", res.err, "set by default", "which resolves to", choices)
			r.expect(exitEnv, "next")
			r.expect(exitEnv, "gate")
			r.env[envStateDir] = r.path(".metron/state")
			contains(t, "next error", r.expect(exitEnv, "next").err, choices)
		})
	}

	// A link to a directory outside the repository is outside the repository.
	r := newRepo(t)
	outside := t.TempDir()
	if err := os.MkdirAll(r.path(".metron"), 0o777); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, r.path(".metron/state")); err != nil {
		t.Fatal(err)
	}
	out := r.must("init").out
	contains(t, "init output", out, "ledger.jsonl")
	if strings.Contains(out, "Commit .metron/, state included") {
		t.Fatalf("init advises committing a state directory outside the repository:\n%s", out)
	}
}
