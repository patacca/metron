package cli

import (
	"bytes"
	"os"
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
		want    []string
		command string
		usage   string
	}{
		{"skip-worktree", func(r *repo) {
			r.git("update-index", "--skip-worktree", "--", "greeting.txt")
			r.write("greeting.txt", "hello\n")
		}, []string{"Tree:         greeting.txt: hidden by an assume-unchanged or skip-worktree flag",
			"Fix:          clear the flags of each hidden file with git update-index --no-skip-worktree -- <path> and git update-index --no-assume-unchanged -- <path>"},
			"", "git update-index --no-skip-worktree -- <path> && git update-index --no-assume-unchanged -- <path>"},
		{"sparse checkout", func(r *repo) {
			r.write("docs/guide.md", "read me\n")
			r.commit("add a guide")
			r.git("sparse-checkout", "set", ".metron")
		}, []string{"Tree:         docs/guide.md: hidden by an assume-unchanged or skip-worktree flag",
			"Fix:          this is a sparse checkout, which sets the flag on every file it leaves out, so it is never clean: end it with git sparse-checkout disable"},
			"git sparse-checkout disable", ""},
		{"core.ignoreStat", func(r *repo) {
			r.git("config", "core.ignoreStat", "true")
			r.write("farewell.txt", "bye\n")
			r.commit("add a farewell")
		}, []string{"Tree:         farewell.txt: hidden by an assume-unchanged or skip-worktree flag",
			"Fix:          core.ignoreStat is true, under which git flags every file it writes: set it to false with git config core.ignoreStat false"},
			"git config core.ignoreStat false", ""},
		{"deleted", func(r *repo) {
			if err := os.Remove(r.path("greeting.txt")); err != nil {
				t.Fatal(err)
			}
		}, []string{"Tree:         greeting.txt: deleted",
			"Fix:          commit each deletion with git rm -- <path>, or restore the file with git checkout -- <path>"},
			"", "git add -A && git commit -m <message>"},
		{"link into .metron", func(r *repo) {
			// The link is made through the index, with the working tree file
			// holding its target as git writes it where links are
			// unavailable, so that the test runs on every platform.
			r.git("config", "core.symlinks", "false")
			r.write("contract-link.yaml", ".metron/contract.yaml")
			blob := r.git("hash-object", "-w", "contract-link.yaml")
			r.git("update-index", "--add", "--cacheinfo", "120000,"+blob+",contract-link.yaml")
			r.git("commit", "-q", "-m", "link the contract")
		}, []string{"Tree:         contract-link.yaml: a symbolic link into .metron/ or .git/",
			"Fix:          replace each such link with what it points to, or remove it, and commit",
			"Do:           make the working tree clean as listed, then run metron next"}, "", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := pinned(t)
			tc.make(r)
			a := r.expectNext(core.StepCommitChanges)
			if a.Command != tc.command || a.Usage != tc.usage || len(a.Tree) == 0 {
				t.Fatalf("command %q, usage %q, tree %+v; want %q and %q", a.Command, a.Usage, a.Tree, tc.command, tc.usage)
			}
			contains(t, "next", r.must("next").out, tc.want...)
			contains(t, "check", r.expect(exitNegative, "check").out, tc.want[:2]...)
			var st statusDoc
			unmarshal(t, r.must("status", "--json").out, &st)
			if st.Clean || len(st.Dirty) != len(a.Tree) {
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
	r.must("check")
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
	if got := printable("a\tb\n\x1b[2J\r\u009b\u202egnp.exe\U000e0041"); got != `a`+"\t"+`b`+"\n"+`\x1b[2J\x0d\u009b\u202egnp.exe\U000e0041` {
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

// TestRepositoryWideFilter puts a filter on every JSON file of the
// repository: the state directory must keep its bytes in a clone.
func TestRepositoryWideFilter(t *testing.T) {
	r := pinned(t)
	r.git("config", "filter.shout.clean", "tr a-z A-Z")
	r.git("config", "filter.shout.smudge", "cat")
	r.write(".gitattributes", "*.json filter=shout\n*.jsonl filter=shout\n")
	r.write("data.json", "{\"quiet\":true}\n")
	r.write("greeting.txt", "hello\n")
	r.commit("spell it right")
	r.must("check")
	_, sk := r.packet()
	r.ingest(exitOK, review(t, sk, "rev-1", nil))
	r.commit("record the state")
	c := &repo{t: t, dir: t.TempDir(), env: map[string]string{}}
	r.git("clone", "-q", r.dir, c.dir)
	if got := string(c.read("data.json")); got != "{\"QUIET\":TRUE}\n" {
		t.Fatalf("the filter did not apply to data.json (%q), so the test proves nothing", got)
	}
	for _, f := range []string{".metron/state/ledger.jsonl", ".metron/state/reports/P-0001.json", ".metron/state/packets/P-0001.json"} {
		if !bytes.Equal(c.read(f), r.read(f)) {
			t.Errorf("the clone's %s differs from the original", f)
		}
	}
	c.must("fsck")
	c.must("gate")
}

// TestIgnoreCase sets core.ignoreCase. On a file system that distinguishes
// case, which is what the test probes for, the tree is then not clean; on one
// that does not, the setting is right and changes nothing.
func TestIgnoreCase(t *testing.T) {
	r := pinned(t)
	r.must("check")
	r.git("config", "core.ignoreCase", "true")
	if _, err := os.Stat(r.path("GREETING.TXT")); err == nil || !os.IsNotExist(err) {
		// Case-insensitive: GREETING.TXT names greeting.txt.
		r.expectNext(core.StepReview)
		return
	}
	a := r.expectNext(core.StepCommitChanges)
	if a.Command != "git config core.ignoreCase false" || !strings.Contains(a.Reason, "git is set to ignore case (core.ignoreCase) on a file system that distinguishes it") {
		t.Fatalf("next %+v", a)
	}
	contains(t, "next", r.must("next").out, "Tree:         core.ignoreCase is true on a file system that distinguishes case", "Run:          git config core.ignoreCase false")
	contains(t, "check", r.expect(exitNegative, "check").out, "The working tree is not clean", "git config core.ignoreCase false")
	contains(t, "gate", r.expect(exitNegative, "gate").out, "[FAIL] I", "git is set to ignore case")
	var st statusDoc
	unmarshal(t, r.must("status", "--json").out, &st)
	if st.Clean || !st.IgnoreCase {
		t.Fatalf("status clean %t, ignore case %t", st.Clean, st.IgnoreCase)
	}
	r.git("config", "core.ignoreCase", "false")
	r.expectNext(core.StepReview)
}
