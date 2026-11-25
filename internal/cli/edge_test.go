package cli

import (
	"strings"
	"testing"

	"github.com/patacca/metron/internal/core"
)

// TestTruncatedBatch deletes the last line of the ledger after a command that
// committed several events: the batch marker shows the cut.
func TestTruncatedBatch(t *testing.T) {
	r := pinned(t)
	r.must("check")
	_, sk := r.packet()
	r.ingest(exitOK, review(t, sk, "rev-1", map[string]string{"REQ-2": core.RequirementViolated}, misspelled()))
	ledger := string(r.read(".metron/state/ledger.jsonl"))
	lines := strings.SplitAfter(strings.TrimSuffix(ledger, "\n"), "\n")
	r.write(".metron/state/ledger.jsonl", strings.Join(lines[:len(lines)-1], ""))
	out := r.expect(exitNegative, "fsck").out
	contains(t, "fsck", out, "1 problem found")
	if n := strings.Count(out, "restore the ledger from version control"); n != 1 {
		t.Fatalf("fsck gives the advice %d times:\n%s", n, out)
	}
	if res := r.expect(exitEnv, "init"); strings.Count(res.err, "version control") != 1 {
		t.Fatalf("init gives the advice other than once:\n%s", res.err)
	}
	r.expect(exitEnv, "gate")
	contains(t, "log", r.expect(exitEnv, "log").err, "the ledger fails verification")
	r.expectNext(core.StepRepairLedger)
}

func TestEmptyLedger(t *testing.T) {
	r := newRepo(t)
	r.must("init")
	r.write(".metron/state/ledger.jsonl", "")
	if a := r.expectNext(core.StepRepairLedger); a.Command != "metron init" || !strings.Contains(a.Reason, "not initialized") {
		t.Fatalf("next %+v", a)
	}
	contains(t, "fsck", r.expect(exitNegative, "fsck").out, ".metron/state/ledger.jsonl is empty, so metron is not initialized: run metron init")
	contains(t, "gate", r.expect(exitEnv, "gate").err, "the ledger in .metron/state is empty", "Run metron init.")
	contains(t, "packet", r.expect(exitEnv, "packet").err, "is empty", "Run metron init.")
	contains(t, "init", r.must("init").out, "Initialized the empty .metron/state/ledger.jsonl")
	r.must("fsck")
	r.expectNext(core.StepPinContract)
}

func TestTreeProblems(t *testing.T) {
	for _, tc := range []struct {
		name    string
		make    func(r *repo)
		want    string
		command string
	}{
		{"skip-worktree", func(r *repo) {
			r.git("update-index", "--skip-worktree", "--", "greeting.txt")
			r.write("greeting.txt", "hello\n")
		}, "greeting.txt is hidden by an assume-unchanged or skip-worktree flag: clear the flag with git update-index --no-skip-worktree -- greeting.txt",
			"git update-index --no-skip-worktree -- greeting.txt && git update-index --no-assume-unchanged -- greeting.txt"},
		{"link into .metron", func(r *repo) {
			// The link is made through the index, with the working tree file
			// holding its target as git writes it where links are
			// unavailable, so that the test runs on every platform.
			r.git("config", "core.symlinks", "false")
			r.write("contract-link.yaml", ".metron/contract.yaml")
			blob := r.git("hash-object", "-w", "contract-link.yaml")
			r.git("update-index", "--add", "--cacheinfo", "120000,"+blob+",contract-link.yaml")
			r.git("commit", "-q", "-m", "link the contract")
		}, "contract-link.yaml is a symbolic link into .metron/: replace the link with what it points to, or remove it, and commit", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := pinned(t)
			tc.make(r)
			a := r.expectNext(core.StepCommitChanges)
			if a.Command != tc.command {
				t.Fatalf("command %q, want %q", a.Command, tc.command)
			}
			out := r.must("next").out
			contains(t, "next", out, "Tree:         "+tc.want)
			if hidden := strings.Contains(tc.want, "skip-worktree"); hidden != strings.Contains(out, "disable the sparse checkout with git sparse-checkout disable, and set git config core.ignoreStat false") {
				t.Errorf("the advice about sparse checkouts and core.ignoreStat appears = %t:\n%s", !hidden, out)
			}
			contains(t, "check", r.expect(exitNegative, "check").out, tc.want)
			var st statusDoc
			unmarshal(t, r.must("status", "--json").out, &st)
			if st.Clean || len(st.Dirty) != 1 {
				t.Fatalf("status tree %+v", st.Dirty)
			}
		})
	}
}

// TestUnchangedCandidate reverts a fix after a RED verdict: the tree is clean,
// so no git command helps; the candidate itself must change.
func TestUnchangedCandidate(t *testing.T) {
	r := pinned(t)
	r.must("check")
	_, sk := r.packet()
	r.ingest(exitOK, review(t, sk, "rev-1", map[string]string{"REQ-2": core.RequirementViolated}, misspelled()))
	_, sk = r.packet()
	r.write("greeting.txt", "hello\n")
	r.ingest(exitOK, fix(t, sk, "impl-1", r.commit("fix the greeting")))
	_, sk = r.packet()
	r.ingest(exitOK, verify(t, sk, "ver-1", core.VerificationConfirmed))
	r.write("greeting.txt", "helo\n")
	r.commit("undo the fix")
	a := r.expectNext(core.StepCommitChanges)
	if a.Command != "" {
		t.Fatalf("command %q for an unchanged candidate", a.Command)
	}
	out := r.must("next").out
	contains(t, "next", out, "Do:           the candidate itself must change: commit a change to something outside .metron/")
	if strings.Contains(out, "git add") {
		t.Fatalf("next suggests a git command:\n%s", out)
	}
}

func TestJSONErrors(t *testing.T) {
	r := newRepo(t)
	for _, tc := range []struct {
		args   []string
		status int
		want   string
	}{
		{[]string{"next", "--json", "--bogus"}, exitUsage, "flag provided but not defined: -bogus"},
		{[]string{"next", "--bogus", "--json"}, exitUsage, "flag provided but not defined: -bogus"},
		{[]string{"gate", "--json"}, exitEnv, "the gate cannot be evaluated"},
		{[]string{"findings", "--json"}, exitEnv, "metron is not initialized here"},
		{[]string{"ingest", "--json", "missing.json"}, exitEnv, "cannot read the report"},
		{[]string{"contract", "--json=true", "extra"}, exitUsage, "unexpected argument"},
	} {
		res := r.expect(tc.status, tc.args...)
		var e errorDoc
		unmarshal(t, res.out, &e)
		if e.Schema != "metron.error/v1" || e.Status != tc.status || !strings.Contains(e.Message, tc.want) || !strings.Contains(res.err, e.Message) {
			t.Errorf("metron %s: document %+v, stderr %q", strings.Join(tc.args, " "), e, res.err)
		}
	}
	// Without --json, nothing reaches standard output.
	if res := r.expect(exitEnv, "gate"); res.out != "" {
		t.Fatalf("gate printed %q", res.out)
	}
	if res := r.expect(exitUsage, "next", "--json=false", "--bogus"); res.out != "" {
		t.Fatalf("next --json=false printed %q", res.out)
	}
}

func TestCheckOutputBounded(t *testing.T) {
	r := pinnedWith(t, `i=0; while [ $i -lt 100 ]; do echo "line $i"; i=$((i+1)); done; printf '\033[31mred\n'; exit 1`)
	res := r.expect(exitNegative, "check")
	contains(t, "check output", res.out, "[earlier output omitted: only the last 40 lines and 4096 bytes are shown]",
		"| line 99", `| \x1b[31mred`)
	if strings.Contains(res.out, "line 60\n") || strings.Contains(res.out, "\x1b") {
		t.Fatalf("check printed too much, or a raw escape:\n%s", res.out)
	}
	var d checkDoc
	unmarshal(t, r.expect(exitNegative, "check", "--json").out, &d)
	if got := d.Results[0]; !got.OutputTruncated || strings.Count(got.OutputTail, "\n") != 39 || !strings.HasSuffix(got.OutputTail, "\x1b[31mred") {
		t.Fatalf("check result %+v", got)
	}
}

func TestTail(t *testing.T) {
	for _, tc := range []struct {
		name      string
		output    string
		want      string
		truncated bool
	}{
		{"short", "a\r\nb\r\n\n", "a\nb", false},
		{"lines", strings.Repeat("x\n", 50), strings.TrimSuffix(strings.Repeat("x\n", 40), "\n"), true},
		{"bytes", strings.Repeat("é", 3000), strings.Repeat("é", 2048), true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, truncated := tail([]byte(tc.output))
			if got != tc.want || truncated != tc.truncated {
				t.Fatalf("tail = %q, %t", got, truncated)
			}
		})
	}
	if got := printable("a\tb\n\x1b[2J\r\u009b"); got != `a`+"\t"+`b`+"\n"+`\x1b[2J\x0d\u009b` {
		t.Fatalf("printable = %q", got)
	}
}

func TestResolveIdentifiers(t *testing.T) {
	r := pinned(t)
	contains(t, "malformed", r.expect(exitUsage, "resolve", "E-1", "-m", "x").err, `"E-1" is not an escalation identifier`)
	r.expect(exitUsage, "resolve", "F-0001", "-m", "x")
	contains(t, "unknown", r.expect(exitNegative, "resolve", "E-0001", "-m", "x").err, `escalation "E-0001" does not exist`)
}

func TestReadOnlyCommandsOnCorruptLedger(t *testing.T) {
	r := pinned(t)
	r.write(".metron/state/ledger.jsonl", strings.Replace(string(r.read(".metron/state/ledger.jsonl")), "first contract", "other contract", 1))
	for _, args := range [][]string{{"findings"}, {"log"}} {
		res := r.expect(exitEnv, args...)
		contains(t, "metron "+args[0], res.err, "the ledger fails verification")
		if strings.Contains(res.err, "nothing was changed") {
			t.Errorf("metron %s speaks of a change: %s", args[0], res.err)
		}
	}
}
