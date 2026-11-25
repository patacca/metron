package cli

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/patacca/metron/internal/core"
	"github.com/patacca/metron/internal/store"
)

// pinnedWith is pinned with REQ-1's check command replaced.
func pinnedWith(t *testing.T, command string) *repo {
	t.Helper()
	r := newRepo(t)
	r.must("init")
	quoted := "'" + strings.ReplaceAll(command, "'", "''") + "'"
	r.write(".metron/contract.yaml", strings.Replace(testContract, `"git ls-files --error-unmatch greeting.txt"`, quoted, 1))
	r.must("contract", "pin", "-m", "first contract")
	r.commit("pin the contract")
	return r
}

func TestFailingCheck(t *testing.T) {
	r := pinnedWith(t, "git ls-files --error-unmatch farewell.txt")
	res := r.expect(exitNegative, "check")
	contains(t, "check output", res.out, "REQ-1    FAILED  exit 1", "         | error: pathspec 'farewell.txt' did not match",
		"Recorded 1 result", "Next step:    run-checks")
	a := r.expectNext(core.StepRunChecks)
	contains(t, "next reason", a.Reason, "REQ-1 failed on this candidate (exit 1)")
	contains(t, "packet error", r.expect(exitNegative, "packet").err, "the next action is run-checks", "Run: metron check")
	contains(t, "gate", r.expect(exitNegative, "gate").out, "[FAIL] E", "REQ-1 failed on this candidate (exit 1)")
	var d checkDoc
	unmarshal(t, r.expect(exitNegative, "check", "--json").out, &d)
	if d.Schema != "metron.check/v1" || d.Passed || !d.Recorded || len(d.Results) != 1 || d.Results[0].OutputTail == "" {
		t.Fatalf("check document %+v", d)
	}
	contains(t, "log", r.must("log").out, "REQ-1 FAILED (exit 1)")

	r.write("farewell.txt", "bye\n")
	r.commit("add the farewell")
	contains(t, "check output", r.must("check").out, "REQ-1    passed")
	r.expectNext(core.StepReview)
}

// TestPOSIXCheckCommand runs a check that only a POSIX shell understands, so
// that the Windows CI job exercises the sh that ships with Git.
func TestPOSIXCheckCommand(t *testing.T) {
	r := pinnedWith(t, `name=$(git ls-files greeting.txt) && [ "$name" = greeting.txt ] && test -n "${name%.txt}"`)
	contains(t, "check output", r.must("check").out, "REQ-1    passed")
	r.expectNext(core.StepReview)
}

func TestCheckThatDirtiesTheTree(t *testing.T) {
	r := pinnedWith(t, "echo checked> check-output.txt")
	before := r.read(".metron/state/ledger.jsonl")
	res := r.expect(exitNegative, "check")
	contains(t, "check output", res.out, "REQ-1    passed", "The checks left the working tree unclean",
		"check-output.txt is untracked: delete it", "nothing was recorded", "Do:           remove what the checks left behind, then run metron check again")
	if strings.Contains(res.out, "git add") || strings.Contains(res.out, "commit it") {
		t.Fatalf("check suggests committing what it left behind:\n%s", res.out)
	}
	if !bytes.Equal(before, r.read(".metron/state/ledger.jsonl")) {
		t.Fatal("a check that dirtied the tree recorded something")
	}
	// The tree is dirty now: check refuses to start.
	contains(t, "check output", r.expect(exitNegative, "check").out, "The working tree is not clean", "check-output.txt is untracked: commit it, delete it, or ignore it")
	r.expectNext(core.StepCommitChanges)
}

func TestLockHeld(t *testing.T) {
	r := pinned(t)
	r.must("check")
	_, sk := r.packet()
	w, err := store.New(r.path(".metron/state")).Lock()
	if err != nil {
		t.Fatal(err)
	}
	defer w.Close()
	for _, args := range [][]string{
		{"packet"}, {"check"}, {"contract", "pin", "-m", "x"}, {"resolve", "E-0001", "-m", "x"}, {"init"},
	} {
		res := r.expect(exitEnv, args...)
		contains(t, "metron "+strings.Join(args, " "), res.err, "is locked: another metron process is running; wait for it to finish and retry")
	}
	contains(t, "ingest error", r.ingest(exitEnv, review(t, sk, "rev-1", nil)).err, "is locked")
	// Reading needs no lock.
	r.expectNext(core.StepSubmitReport)
	r.must("status")
	r.expect(exitNegative, "gate")
}

func TestTamperedLedger(t *testing.T) {
	r := green(t)
	ledger := r.read(".metron/state/ledger.jsonl")
	r.write(".metron/state/ledger.jsonl", strings.Replace(string(ledger), `"reviewer":"rev-1"`, `"reviewer":"rev-2"`, 1))
	contains(t, "fsck", r.expect(exitNegative, "fsck").out, "1 problem found", "hash does not match the content of the line", "restore the ledger from version control")
	// The gate cannot be evaluated, which is not the same as closed.
	res := r.expect(exitEnv, "gate")
	contains(t, "gate", res.err, "the gate cannot be evaluated: the ledger fails verification", "Run metron fsck.")
	if res.out != "" {
		t.Fatalf("gate printed an evaluation it could not make: %q", res.out)
	}
	var e errorDoc
	unmarshal(t, r.expect(exitEnv, "gate", "--json").out, &e)
	if e.Schema != "metron.error/v1" || e.Status != exitEnv || !strings.Contains(e.Message, "the gate cannot be evaluated") {
		t.Fatalf("gate --json error document %+v", e)
	}
	if a := r.expectNext(core.StepRepairLedger); a.Command != "metron fsck" {
		t.Fatalf("next %+v", a)
	}
	for _, args := range [][]string{{"packet"}, {"check"}, {"log"}, {"findings"}, {"resolve", "E-0001", "-m", "x"}} {
		contains(t, "metron "+args[0], r.expect(exitEnv, args...).err, "the ledger fails verification")
	}
	contains(t, "status", r.must("status").out, "hash does not match", "Next step:    repair-ledger")
	contains(t, "init", r.expect(exitEnv, "init").err, "fails verification")
}

func TestNotInitialized(t *testing.T) {
	r := newRepo(t)
	if a := r.expectNext(core.StepRepairLedger); a.Command != "metron init" {
		t.Fatalf("next %+v", a)
	}
	contains(t, "gate", r.expect(exitEnv, "gate").err, "the gate cannot be evaluated: metron is not initialized here", "Run metron init.")
	contains(t, "fsck", r.expect(exitNegative, "fsck").out, ".metron/state/ledger.jsonl does not exist: run metron init")
	r.expect(exitEnv, "packet")
	r.must("status")
}

// TestEventSchema checks that fsck holds every event to the published schema,
// beyond what replaying it requires.
func TestEventSchema(t *testing.T) {
	r := pinned(t)
	st := store.New(r.path(".metron/state"))
	events, err := st.Load()
	if err != nil {
		t.Fatal(err)
	}
	digest := "sha256:" + strings.Repeat("0", 64)
	sealed, err := store.Seal(&events[len(events)-1], "2026-01-02T03:04:05Z", []core.Draft{{Type: core.EventEvidenceRecorded,
		Data: core.Evidence{Requirement: "lint", Command: "true", Candidate: digest, Passed: true, OutputDigest: digest}}})
	if err != nil {
		t.Fatal(err)
	}
	w, err := st.Lock()
	if err != nil {
		t.Fatal(err)
	}
	err = w.Append(sealed)
	w.Close()
	if err != nil {
		t.Fatal(err)
	}
	contains(t, "fsck", r.expect(exitNegative, "fsck").out,
		"ledger event 3 (evidence.recorded) does not satisfy the event schema: /data/requirement: must be a requirement identifier")
}

func TestNotARepository(t *testing.T) {
	dir := t.TempDir()
	var out, errb bytes.Buffer
	code := Run([]string{"next"}, Env{Stdout: &out, Stderr: &errb, Dir: dir, Getenv: func(string) string { return "" }, Now: func() time.Time { return testClock }})
	if code != exitEnv || !strings.Contains(errb.String(), "metron works inside a git repository") {
		t.Fatalf("exit %d: %s", code, errb.String())
	}
}

func TestRepositoryWithoutCommits(t *testing.T) {
	r := &repo{t: t, dir: t.TempDir(), env: map[string]string{}}
	r.git("init", "-q")
	r.git("config", "user.name", "Grace Governance")
	r.git("config", "user.email", "grace@example.com")
	r.must("init")
	r.write(".metron/contract.yaml", testContract)
	r.must("contract", "pin", "-m", "first contract")
	a := r.expectNext(core.StepCommitChanges)
	contains(t, "next reason", a.Reason, "the repository has no commits")
	contains(t, "packet error", r.expect(exitNegative, "packet").err, "the next action is commit-changes", "the repository has no commits")
	contains(t, "gate error", r.expect(exitEnv, "gate").err, "commit the work to review first")
	r.expect(exitEnv, "check")
	contains(t, "status", r.must("status").out, "Candidate:    none: the repository has no commits")
}

func TestUsageErrors(t *testing.T) {
	r := newRepo(t)
	for _, args := range [][]string{
		{},
		{"frobnicate"},
		{"--state-dir", "x", "next"},
		{"next", "extra"},
		{"next", "--bogus"},
		{"ingest"},
		{"ingest", "a.json", "b.json"},
		{"contract", "pin"},
		{"contract", "unpin"},
		{"resolve", "-m", "x"},
		{"findings", "--all", "--backlog"},
		{"packet", "--format", "yaml"},
		{"schema", "nope"},
		{"help", "nope"},
	} {
		res := r.expect(exitUsage, args...)
		if res.err == "" || res.out != "" {
			t.Errorf("metron %s: stdout %q, stderr %q", strings.Join(args, " "), res.out, res.err)
		}
	}
	contains(t, "flag first", r.expect(exitUsage, "--state-dir", "x", "next").err, "flags go after the command name")
	contains(t, "unknown flag", r.expect(exitUsage, "next", "--bogus").err, "flag provided but not defined: -bogus", "Run 'metron help next' for usage.")
	contains(t, "pin without reason", r.expect(exitUsage, "contract", "pin").err, `missing -m "<reason>"`, "Run 'metron help contract'")
}

func TestHelp(t *testing.T) {
	r := newRepo(t)
	overview := r.must("help").out
	if r.must("-h").out != overview {
		t.Fatal("metron -h differs from metron help")
	}
	for _, c := range commands {
		contains(t, "overview", overview, "  "+c.name+" ")
		res := r.must(c.name, "-h")
		if !strings.HasPrefix(res.out, "Usage: metron "+c.name) || res.err != "" {
			t.Errorf("metron %s -h printed %q, %q", c.name, res.out, res.err)
		}
		if res.out != r.must("help", c.name).out {
			t.Errorf("metron help %s differs from metron %s -h", c.name, c.name)
		}
		for _, l := range strings.Split(res.out, "\n") {
			if len(l) > 80 {
				t.Errorf("metron %s -h has a line of %d characters: %q", c.name, len(l), l)
			}
		}
	}
	contains(t, "pin help", r.must("contract", "pin", "-h").out, "Usage: metron contract pin -m <reason>", "-m string")
	contains(t, "ingest help", r.must("ingest", "-h").out, "-json", "-state-dir directory")
}

func TestVersionSchemaProtocol(t *testing.T) {
	r := newRepo(t)
	contains(t, "version", r.must("version").out, "metron ", "protocol "+core.ProtocolVersion)
	var v versionDoc
	unmarshal(t, r.must("version", "--json").out, &v)
	if v.Schema != "metron.version/v1" || v.Protocol != core.ProtocolVersion || v.Version == "" {
		t.Fatalf("version %+v", v)
	}
	contains(t, "schema list", r.must("schema").out, "contract\n", "report\n")
	contains(t, "report schema", r.must("schema", "report").out, `"$id": "https://github.com/patacca/metron/schemas/v1/report.schema.json"`)
	contains(t, "protocol", r.must("protocol").out, "Metron")
}

// TestOutputIsASCII checks the text views for characters outside ASCII.
func TestOutputIsASCII(t *testing.T) {
	r := green(t)
	for _, args := range [][]string{{"next"}, {"gate"}, {"status"}, {"findings", "--all"}, {"log"}, {"fsck"}, {"contract"}, {"help"}} {
		out := r.run(args...).out
		for i, c := range out {
			if c > 0x7e || (c < 0x20 && c != '\n') {
				t.Errorf("metron %s prints %q at byte %d", strings.Join(args, " "), c, i)
				break
			}
		}
	}
}
