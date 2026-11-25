package cli

import (
	"path/filepath"
	"testing"
)

// TestRepositoryThroughAlias reaches the repository, and gives the state
// directory, through another spelling of its path than the one git reports:
// what metron shows is relative to the repository either way.
func TestRepositoryThroughAlias(t *testing.T) {
	r := newRepo(t)
	real := realPath(r.dir)
	r.dir = alias(t, real)
	contains(t, "init output", r.must("init").out, "Created .metron/state/ledger.jsonl", "Commit .metron/, state included")
	contains(t, "fsck output", r.must("fsck").out, "Ledger:  .metron/state/ledger.jsonl")
	for _, tc := range []struct {
		name, base string
		flag       bool
	}{
		{"alias flag", r.dir, true},
		{"alias environment", r.dir, false},
		{"real flag", real, true},
		{"real environment", real, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			run := func(code int, state string, args ...string) result {
				t.Helper()
				if tc.flag {
					return r.expect(code, append(args, "--state-dir", state)...)
				}
				r.env[envStateDir] = state
				defer delete(r.env, envStateDir)
				return r.expect(code, args...)
			}
			state := filepath.Join(tc.base, ".metron", "state")
			contains(t, "init output", run(exitOK, state, "init").out,
				"Found .metron/state/ledger.jsonl with 1 event", "Commit .metron/, state included")
			var st statusDoc
			unmarshal(t, run(exitOK, state, "status", "--json").out, &st)
			if st.StateDir != ".metron/state" {
				t.Fatalf("status shows the state directory as %q", st.StateDir)
			}
			contains(t, "fsck output", run(exitOK, state, "fsck").out, "Ledger:  .metron/state/ledger.jsonl")
			refused := map[bool]int{true: exitUsage, false: exitEnv}[tc.flag]
			for _, other := range []string{filepath.Join(tc.base, ".metron", "other"), filepath.Join(tc.base, "sub", "state")} {
				contains(t, "refusal", run(refused, other, "next").err, "lies inside the repository")
			}
		})
	}
}
