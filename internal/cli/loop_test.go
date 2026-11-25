package cli

import (
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/patacca/metron/internal/core"
)

// commitOnly commits the given paths alone, so that the state directory's
// content, which depends on the rendered briefs, never reaches a commit hash
// that a golden file shows.
func (r *repo) commitOnly(msg string, paths ...string) string {
	r.t.Helper()
	r.git(append([]string{"add", "--"}, paths...)...)
	r.git(append([]string{"commit", "-q", "-m", msg, "--"}, paths...)...)
	return r.git("rev-parse", "HEAD")
}

// snapshot compares the views of the loop with golden files.
func snapshot(t *testing.T, r *repo, point string) {
	t.Helper()
	for _, v := range []struct {
		file string
		args []string
	}{
		{"next.json", []string{"next", "--json"}},
		{"next.txt", []string{"next"}},
		{"gate.json", []string{"gate", "--json"}},
		{"gate.txt", []string{"gate"}},
		{"status.json", []string{"status", "--json"}},
		{"status.txt", []string{"status"}},
		{"findings.json", []string{"findings", "--all", "--json"}},
	} {
		res := r.run(v.args...)
		if res.code > exitNegative {
			t.Fatalf("metron %s: exit %d: %s", strings.Join(v.args, " "), res.code, res.err)
		}
		golden(t, point+"."+v.file, res.out)
	}
}

func TestWholeLoop(t *testing.T) {
	r := newRepo(t)
	if a := r.expectNext(core.StepRepairLedger); a.Command != "metron init" {
		t.Fatalf("an uninitialized repository is repaired with %q, want metron init", a.Command)
	}
	r.must("init")
	r.expectNext(core.StepPinContract)
	r.write(".metron/contract.yaml", testContract)
	contains(t, "contract output", r.expect(exitNegative, "contract").out, "valid, not pinned", `metron contract pin -m "<reason>"`)
	r.expectNext(core.StepPinContract)
	r.must("contract", "pin", "-m", "first contract")
	r.must("contract")
	r.commit("pin the contract")

	if a := r.expectNext(core.StepRunChecks); !slices.Equal(a.Requirements, []string{"REQ-1"}) || a.Command != "metron check" {
		t.Fatalf("run-checks action %+v", a)
	}
	snapshot(t, r, "pinned")
	contains(t, "check output", r.must("check").out, "REQ-1    passed")
	r.expectNext(core.StepReview)

	p, sk := r.packet()
	if p.ID != "P-0001" || p.Action != core.StepReview || p.Round != 1 || !p.ReadOnly {
		t.Fatalf("review packet %s %s round %d", p.ID, p.Action, p.Round)
	}
	if a := r.expectNext(core.StepSubmitReport); a.Packet != "P-0001" || a.Owner != core.RoleReviewer {
		t.Fatalf("submit-report action %+v", a)
	}
	res := r.ingest(exitOK, review(t, sk, "rev-1", map[string]string{"REQ-2": core.RequirementViolated}, misspelled()))
	contains(t, "ingest output", res.out, "Accepted the reviewer report for packet P-0001", "F-0001 (T1, objective-alignment)", "round 1: RED by rev-1")
	if a := r.expectNext(core.StepFix); !slices.Equal(a.Findings, []string{"F-0001"}) {
		t.Fatalf("fix action %+v", a)
	}
	snapshot(t, r, "red")

	p, sk = r.packet()
	if p.Action != core.StepFix || p.Base == nil || !slices.Equal(p.Subjects, []string{"F-0001"}) || p.ReadOnly {
		t.Fatalf("fix packet %+v", p)
	}
	r.write("greeting.txt", "hello\n")
	// A fix packet is not read-only: the implementer's work in progress
	// leaves it valid.
	r.expectNext(core.StepSubmitReport)
	commit := r.commitOnly("fix the greeting", "greeting.txt")
	contains(t, "ingest output", r.ingest(exitOK, fix(t, sk, "impl-1", commit)).out, "finding.fix-claimed", "F-0001")
	r.expectNext(core.StepVerify)

	p, sk = r.packet()
	if p.Action != core.StepVerify || p.Base == nil || p.Candidate.Commit != commit {
		t.Fatalf("verify packet %+v", p)
	}
	r.ingest(exitOK, verify(t, sk, "ver-1", core.VerificationConfirmed))
	r.expectNext(core.StepRunChecks)
	r.must("check")
	r.expectNext(core.StepReview)

	p, sk = r.packet()
	if p.Round != 2 || len(p.History) != 1 || p.History[0].Verdict != core.VerdictRed {
		t.Fatalf("second review packet round %d history %+v", p.Round, p.History)
	}
	contains(t, "ingest output", r.ingest(exitOK, review(t, sk, "rev-1", nil)).out, "round 2: GREEN by rev-1", "Next step:    none, the gate is open")
	r.expectNext(core.StepNone)
	contains(t, "gate output", r.must("gate").out, "Gate: OPEN for stage prototype")
	snapshot(t, r, "green")
	contains(t, "fsck output", r.must("fsck").out, "No problem found.")
	contains(t, "log output", r.must("log").out, "verdict.recorded", "finding.fix-confirmed")
}

func TestGreenInOneRound(t *testing.T) {
	r := pinned(t)
	r.write("greeting.txt", "hello\n")
	r.commit("spell it right")
	r.must("check")
	_, sk := r.packet()
	r.ingest(exitOK, review(t, sk, "rev-1", nil))
	r.expectNext(core.StepNone)
	r.must("gate")
	var g gateDoc
	unmarshal(t, r.must("gate", "--json").out, &g)
	if !g.Open || g.Schema != "metron.gate/v1" || len(g.Conditions) != 5 || g.Authorizes != "An internal demonstration" {
		t.Fatalf("gate %+v", g)
	}
}

// green runs a single GREEN round on a corrected greeting.
func green(t *testing.T) *repo {
	t.Helper()
	r := pinned(t)
	r.write("greeting.txt", "hello\n")
	r.commit("spell it right")
	r.must("check")
	_, sk := r.packet()
	r.ingest(exitOK, review(t, sk, "rev-1", nil))
	r.must("gate")
	return r
}

func TestGateAfterCommits(t *testing.T) {
	r := green(t)
	// Committing the state directory does not change the candidate.
	r.commit("record the review")
	r.must("gate")
	r.write(".metron/notes.txt", "governance notes\n")
	r.commit("add notes under .metron")
	r.must("gate")
	r.expectNext(core.StepNone)

	r.write("greeting.txt", "hello, world\n")
	// Uncommitted, the change makes the tree dirty.
	res := r.expect(exitNegative, "gate")
	contains(t, "gate output", res.out, "Gate: CLOSED", "the working tree has changes outside .metron/")
	r.expectNext(core.StepCommitChanges)
	r.commit("change the greeting")
	res = r.expect(exitNegative, "gate")
	contains(t, "gate output", res.out, "[FAIL] I", "differs from", "[FAIL] E", "[FAIL] V")
	r.expectNext(core.StepRunChecks)
}

func TestReadOnlyRoleChangedTree(t *testing.T) {
	for _, tc := range []struct {
		name   string
		change func(r *repo)
		want   string
	}{
		{"dirty", func(r *repo) { r.write("scratch.txt", "left behind\n") }, "the working tree is not clean"},
		{"committed", func(r *repo) { r.write("greeting.txt", "hi\n"); r.commit("reviewer edits") }, "the candidate changed"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := pinned(t)
			r.must("check")
			_, sk := r.packet()
			tc.change(r)
			res := r.ingest(exitNegative, review(t, sk, "rev-1", nil))
			contains(t, "ingest error", res.err, "rejected by the transition layer", "/packet", tc.want, "nothing was changed")
		})
	}
}

func TestGovernanceEscalation(t *testing.T) {
	r := pinned(t)
	r.write("greeting.txt", "hello\n")
	r.commit("spell it right")
	r.must("check")
	_, sk := r.packet()
	b := reviewBody(t, sk, nil)
	b.ContractGaps = []core.ContractGap{{Title: "No language", Description: "The contract does not say which language greets"}}
	res := r.ingest(exitOK, complete(t, sk, "rev-1", b))
	contains(t, "ingest output", res.out, "E-0001 (contract-gap, owned by governance)")
	a := r.expectNext(core.StepAdjudicate)
	if !slices.Equal(a.Escalations, []string{"E-0001"}) || a.Command != `metron resolve E-0001 -m "<decision>"` {
		t.Fatalf("adjudicate action %+v", a)
	}
	r.expect(exitNegative, "gate")
	r.expect(exitUsage, "resolve", "E-0001")
	r.expect(exitNegative, "resolve", "E-0009", "-m", "no such escalation")
	res = r.must("resolve", "E-0001", "-m", "English is implied by the objectives")
	contains(t, "resolve output", res.out, "E-0001 by governance (Grace Governance <grace@example.com>): English is implied")
	r.expectNext(core.StepNone)
	r.must("gate")
	r.expect(exitNegative, "resolve", "E-0001", "-m", "again")
}

func TestStateDirectoryElsewhere(t *testing.T) {
	for _, how := range []string{"flag", "environment"} {
		t.Run(how, func(t *testing.T) {
			r := newRepo(t)
			state := t.TempDir()
			var extra []string
			if how == "flag" {
				extra = []string{"--state-dir", state}
			} else {
				r.env[envStateDir] = state
			}
			run := func(args ...string) result { return r.must(append(args, extra...)...) }
			run("init")
			r.write(".metron/contract.yaml", testContract)
			run("contract", "pin", "-m", "first contract")
			if _, err := os.Stat(r.path(".metron/state")); !os.IsNotExist(err) {
				t.Fatalf(".metron/state exists although the state directory is %s: %v", state, err)
			}
			if _, err := os.Stat(state + "/ledger.jsonl"); err != nil {
				t.Fatal(err)
			}
			var a actionDoc
			unmarshal(t, run("next", "--json").out, &a)
			want := "metron check"
			if how == "flag" {
				want += " --state-dir " + quoteArg(state)
			}
			if a.Step != core.StepRunChecks || a.Command != want {
				t.Fatalf("next %+v, want run-checks with %q", a, want)
			}
			run("check")
			run("packet")
			if _, err := os.Stat(state + "/packets/P-0001.json"); err != nil {
				t.Fatal(err)
			}
			// Without the flag or the variable, the default state directory
			// is not initialized.
			delete(r.env, envStateDir)
			if a := r.next(); a.Step != core.StepRepairLedger {
				t.Fatalf("next without the state directory is %+v", a)
			}
		})
	}
}

func TestDisputeAudited(t *testing.T) {
	r := pinned(t)
	r.must("check")
	_, sk := r.packet()
	r.ingest(exitOK, review(t, sk, "rev-1", map[string]string{"REQ-2": core.RequirementViolated}, misspelled()))
	_, sk = r.packet()
	res := r.ingest(exitOK, complete(t, sk, "impl-1", core.ImplementerBody{Fixes: []core.Fix{},
		Disputes: []core.Dispute{{Finding: "F-0001", Rationale: "helo is the house style"}}}))
	contains(t, "ingest output", res.out, "E-0001 (dispute, owned by the auditor)")
	if a := r.expectNext(core.StepAudit); !slices.Equal(a.Escalations, []string{"E-0001"}) {
		t.Fatalf("audit action %+v", a)
	}
	p, sk := r.packet()
	if p.Action != core.StepAudit || !slices.Equal(p.Adjudicable, []string{"F-0001"}) {
		t.Fatalf("audit packet %+v", p)
	}
	var b core.AuditorBody
	unmarshal(t, string(sk.Body), &b)
	b.Resolutions[0].Analysis, b.Resolutions[0].Conclusion = "the contract fixes no spelling", "the finding does not hold"
	b.Adjudications[0].Decision, b.Adjudications[0].Rationale = core.DecisionWithdraw, "no requirement fixes the spelling"
	b.Directives = []string{"Cite the house style before reporting spelling"}
	res = r.ingest(exitOK, complete(t, sk, "aud-1", b))
	contains(t, "ingest output", res.out, "finding.withdrawn", "escalation.resolved    E-0001 by the auditor (aud-1)", "directive.issued")
	r.expectNext(core.StepReview)
	q, _ := r.packet()
	if len(q.Decisions) != 2 {
		t.Fatalf("the review packet carries decisions %+v", q.Decisions)
	}
}
