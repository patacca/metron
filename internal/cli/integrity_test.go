package cli

import (
	"bytes"
	"os"
	"os/exec"
	"testing"

	"github.com/patacca/metron/internal/core"
)

// TestReportsAfterRewrittenBase ingests implementer reports after the round's
// candidate commit was amended away and its object removed.
func TestReportsAfterRewrittenBase(t *testing.T) {
	for _, dispute := range []bool{false, true} {
		name := map[bool]string{false: "fix", true: "dispute"}[dispute]
		t.Run(name, func(t *testing.T) {
			r := pinned(t)
			r.must("check")
			_, sk := r.packet()
			r.ingest(exitOK, review(t, sk, "rev-1", map[string]string{"REQ-2": core.RequirementViolated}, misspelled()))
			_, sk = r.packet()
			base := r.git("rev-parse", "HEAD")
			r.git("commit", "-q", "--amend", "-m", "pin the contract, reworded")
			r.git("reflog", "expire", "--expire=now", "--all")
			r.git("gc", "-q", "--prune=now")
			probe := exec.Command("git", "cat-file", "-e", base)
			probe.Dir = r.dir
			if probe.Run() == nil {
				t.Fatalf("the base commit %s still exists", base)
			}
			if dispute {
				r.ingest(exitOK, complete(t, sk, "impl-1", core.ImplementerBody{Fixes: []core.Fix{},
					Disputes: []core.Dispute{{Finding: "F-0001", Rationale: "helo is the house style"}}}))
				r.expectNext(core.StepAudit)
				return
			}
			r.write("greeting.txt", "hello\n")
			r.ingest(exitOK, fix(t, sk, "impl-1", r.commit("fix the greeting")))
			r.expectNext(core.StepVerify)
		})
	}
}

// TestStaleOpenPacketOnGreenCandidate loses the file of an open packet that
// is already invalid, on a candidate with a GREEN verdict: next and gate both
// see an open gate.
func TestStaleOpenPacketOnGreenCandidate(t *testing.T) {
	r := green(t)
	r.write("greeting.txt", "hi\n")
	r.commit("try another greeting")
	r.must("check")
	p, _ := r.packet()
	r.write("greeting.txt", "hello\n")
	r.commit("back to the reviewed greeting")
	if err := os.Remove(r.path(".metron/state/packets/" + p.ID + ".json")); err != nil {
		t.Fatal(err)
	}
	r.expectNext(core.StepNone)
	contains(t, "next", r.must("next").out, "Note:         packet "+p.ID+" (review) is open but no longer valid (its stored file")
	r.expect(exitNegative, "fsck")
}

// TestLostEarlierReport loses the report of an earlier packet: fsck and status
// report it, but it neither blocks the loop nor closes the gate.
func TestLostEarlierReport(t *testing.T) {
	r := pinned(t)
	r.must("check")
	_, sk := r.packet()
	r.ingest(exitOK, review(t, sk, "rev-1", map[string]string{"REQ-2": core.RequirementViolated}, misspelled()))
	if err := os.Remove(r.path(".metron/state/reports/P-0001.json")); err != nil {
		t.Fatal(err)
	}
	contains(t, "fsck", r.expect(exitNegative, "fsck").out, "reports/P-0001.json is missing")
	var st statusDoc
	unmarshal(t, r.must("status", "--json").out, &st)
	if len(st.StoredFiles) != 1 || st.StoredFiles[0].Packet != "P-0001" {
		t.Fatalf("status stored files %+v", st.StoredFiles)
	}
	r.expectNext(core.StepFix)
	_, sk = r.packet()
	r.write("greeting.txt", "hello\n")
	r.ingest(exitOK, fix(t, sk, "impl-1", r.commit("fix the greeting")))
	_, sk = r.packet()
	r.ingest(exitOK, verify(t, sk, "ver-1", core.VerificationConfirmed))
	r.must("check")
	_, sk = r.packet()
	r.ingest(exitOK, review(t, sk, "rev-1", nil))
	r.expectNext(core.StepNone)
	r.must("gate")
	r.expect(exitNegative, "fsck")
}

// TestLostVerdictReport loses the report that carries the GREEN verdict:
// next says repair-ledger and packet refuses with its reason; a new candidate
// leads to a new review.
func TestLostVerdictReport(t *testing.T) {
	r := green(t)
	if err := os.Remove(r.path(".metron/state/reports/P-0001.json")); err != nil {
		t.Fatal(err)
	}
	r.expectNext(core.StepRepairLedger)
	res := r.expect(exitNegative, "packet")
	contains(t, "packet error", res.err, "the next action is repair-ledger, which takes no packet", "reports/P-0001.json is missing",
		"Run: metron ingest <the report accepted for P-0001>")
	r.write("greeting.txt", "hello!\n")
	r.commit("change the candidate")
	r.expectNext(core.StepRunChecks)
	r.must("check")
	_, sk := r.packet()
	r.ingest(exitOK, review(t, sk, "rev-1", nil))
	r.expectNext(core.StepNone)
}

// TestCloneWithAutoCRLF clones a repository whose state is committed under
// core.autocrlf=true: the stored files must keep their bytes.
func TestCloneWithAutoCRLF(t *testing.T) {
	r := green(t)
	r.commit("record the state")
	c := &repo{t: t, dir: t.TempDir(), env: map[string]string{}}
	r.git("clone", "-q", "-c", "core.autocrlf=true", r.dir, c.dir)
	if !bytes.Contains(c.read("greeting.txt"), []byte("\r\n")) {
		t.Fatal("the clone did not convert line endings, so the test proves nothing")
	}
	if report := ".metron/state/reports/P-0001.json"; !bytes.Equal(c.read(report), r.read(report)) {
		t.Fatal("the clone converted the stored report")
	}
	c.must("fsck")
	c.must("gate")
}

func TestStateDirectoryInsideTree(t *testing.T) {
	r := newRepo(t)
	res := r.expect(exitUsage, "init", "--state-dir", "state")
	contains(t, "init error", res.err, "lies inside the working tree but outside .metron/",
		"use a directory outside the repository, or leave the default .metron/state")
	r.env[envStateDir] = r.path("sub/state")
	r.expect(exitEnv, "next")
	r.env[envStateDir] = r.dir
	r.expect(exitEnv, "status")
	// Under .metron/ it is committed together with the work.
	r.env[envStateDir] = r.path(".metron/elsewhere")
	contains(t, "init output", r.must("init").out, "Created .metron/elsewhere/ledger.jsonl", "Commit .metron/, state included")
}
