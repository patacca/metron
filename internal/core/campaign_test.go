package core

import (
	"slices"
	"testing"
)

// Packets issued after an approval but before the next round count against
// the approved campaign's budget, so a new campaign is suspended early.
func TestBudgetStartsOverAfterAnApproval(t *testing.T) {
	h := newHarness(t)
	c := testContract()
	c.Convergence = &Convergence{RootCauseAfter: 2, ConvergenceAuditAfter: 3, ProcessAuditAfter: 5, SuspendAfter: 5}
	h.contract = c
	h.must(Pin(h.state, h.facts(), "budget 5", "gov"))
	h.checks(true)
	h.accept(h.packet(), "rev-1", red(h.contract, "crash"))                           // P-0001
	h.accept(h.packet(), "impl-1", ImplementerBody{Fixes: []Fix{h.repair("F-0001")}}) // P-0002
	vp := h.verifying()                                                               // P-0003
	h.accept(vp, "ver-1", verify(map[string]string{"F-0001": VerificationConfirmed}, "F-0001"))
	h.checks(true)
	h.accept(h.packet(), "rev-1", green(h.contract)) // P-0004
	h.expect(StepNone)
	t.Logf("approved; campaign steps %d", h.state.CampaignSteps())
	// Ordinary development after the approval; the checks fail three times.
	h.commit(true)
	h.checks(false)
	h.checks(false)
	h.checks(false)
	a := h.expect(StepAudit)
	t.Logf("next %s %v", a.Step, a.Escalations)
	ap := h.packet() // the first packet since the approval
	if e, ok := h.state.Suspension(); ok {
		t.Fatalf("the first packet after an approval suspended the campaign: %s (%s)", e.ID, e.Reason)
	}
	_ = ap
}

// A lapsed claim can be made again listing only the new commits: the
// verifier then sees a fix without the commits it was first claimed with.
func TestClaimMadeAgainKeepsItsCommits(t *testing.T) {
	h := newHarness(t)
	h.checks(true)
	h.accept(h.packet(), "rev-1", red(h.contract, "crash"))
	fx := h.repair("F-0001")
	extra := h.commit(true)
	fx.Commits = append(fx.Commits, extra)
	first := slices.Clone(fx.Commits)
	h.accept(h.packet(), "impl-1", ImplementerBody{Fixes: []Fix{fx}})
	h.commit(true) // the candidate changes: the claim lapses
	fp := h.packet()
	if fp.Action != StepFix {
		t.Fatalf("packet %s", fp.Action)
	}
	// Listing only the new commit would leave the first ones in no fix the
	// verifier sees.
	again := fix("F-0001", h.history[len(h.history)-1].Hash)
	if ps := h.refused(fp, "impl-1", ImplementerBody{Fixes: []Fix{again}}, LayerTransition); !hasProblem(ps, "/body/fixes/0/commits") {
		t.Fatalf("problems %+v", ps)
	}
	h.accept(fp, "impl-1", ImplementerBody{Fixes: []Fix{h.restate("F-0001")}})
	vp := h.verifying()
	got := vp.Findings[0].Fix.Commits
	for _, c := range first {
		if !slices.Contains(got, c) {
			t.Fatalf("the verify packet's fix of F-0001 lists %v; commit %s, claimed first, is in no fix the verifier sees", got, c)
		}
	}
}

// approved() takes a GREEN round as an approval even when the gate never
// opened: here an open contract gap kept it closed until governance pinned an
// amended contract, and the next round resets the budget although "not a pin"
// begins a campaign.
func TestBudgetIsNotResetWithoutAnApproval(t *testing.T) {
	h := newHarness(t)
	h.checks(true)
	h.accept(h.packet(), "rev-1", red(h.contract, "crash"))
	h.accept(h.packet(), "impl-1", ImplementerBody{Fixes: []Fix{h.repair("F-0001")}})
	h.accept(h.verifying(), "ver-1", verify(map[string]string{"F-0001": VerificationConfirmed}, "F-0001"))
	h.checks(true)
	b := green(h.contract)
	b.ContractGaps = []ContractGap{{Title: "no limit on input size", Description: "large files are not covered"}}
	h.accept(h.packet(), "rev-1", b)
	if h.gate().Open {
		t.Fatal("gate open with a contract gap")
	}
	before := h.state.CampaignSteps()
	c := h.contract
	c.Risks.Unacceptable = append(c.Risks.Unacceptable, Risk{ID: "RISK-2", Statement: "unbounded input"})
	h.contract = c
	h.must(Pin(h.state, h.facts(), "cover the gap", "gov"))
	h.must(Resolve(h.state, h.facts(), "E-0001", "amended and pinned", "gov"))
	if h.gate().Open {
		t.Fatal("gate open under the amended contract without a review")
	}
	h.expect(StepReview)
	h.packet()
	t.Logf("steps before the pin %d, after the next round opened %d", before, h.state.CampaignSteps())
	if h.state.CampaignSteps() != before+1 {
		t.Fatalf("the budget was reset by the round after a pin, though the gate never opened")
	}
}

// A claim survives a later claim on another candidate (claimedOn is only the
// latest), so a fix claimed on T is verified on W, a tree on which the
// finding was recorded by a rejected fix, without the "never recorded on"
// check ever applying to W.
func TestEveryClaimStandsOnTheVerifiedCandidate(t *testing.T) {
	h := newHarness(t)
	h.checks(true)
	h.accept(h.packet(), "rev-1", red(h.contract, "crash")) // F-0001 on X
	h.accept(h.packet(), "impl-1", ImplementerBody{Fixes: []Fix{h.repair("F-0001")}})
	w := h.tree
	h.accept(h.verifying(), "ver-1", verify(map[string]string{"F-0001": VerificationRejected}, "F-0001")) // recorded on W
	h.accept(h.packet(), "impl-1", ImplementerBody{Fixes: []Fix{h.repair("F-0001")}})
	h.accept(h.verifying(), "ver-1", verify(map[string]string{"F-0001": VerificationConfirmed}, "F-0001"))
	h.checks(true)
	b := review(h.contract, VerdictRed, map[string]string{"REQ-1": RequirementViolated}, blocker("b", "REQ-1"))
	b.Reopen = []Reopen{{Finding: "F-0001", Reason: ReopenRegression, Justification: "the crash is back"}}
	h.accept(h.packet(), "rev-1", b) // F-0001 reopened and F-0002 opened on V
	h.audits()
	fx := h.repair("F-0001")
	fx.RootCause = "analysed"
	h.accept(h.packet(), "impl-1", ImplementerBody{Fixes: []Fix{fx}, Disputes: []Dispute{{Finding: "F-0002", Rationale: "not a defect"}}})
	h.audits()
	// The correction covers the whole batch: the upheld finding and the fix
	// already claimed.
	fp := h.packet()
	if !slices.Equal(fp.Subjects, []string{"F-0001", "F-0002"}) {
		t.Fatalf("fix packet subjects %v", fp.Subjects)
	}
	h.touch("src/back.go")
	h.tree = w // the fix of F-0002 returns the tree to W
	f2 := fix("F-0002", "")
	f2.Commits = h.unlisted()
	// The candidate changed, so the fix of F-0001 must be claimed again...
	if ps := h.refused(fp, "impl-1", ImplementerBody{Fixes: []Fix{f2}}, LayerTransition); !hasProblem(ps, "/body/fixes") {
		t.Fatalf("problems %+v", ps)
	}
	// ...and it cannot be claimed on W, where a fix for it was rejected.
	f1 := h.restate("F-0001")
	f1.RootCause = "analysed"
	if ps := h.refused(fp, "impl-1", ImplementerBody{Fixes: []Fix{f1, f2}}, LayerTransition); !hasProblem(ps, "/body/fixes/0") {
		t.Fatalf("problems %+v", ps)
	}
	h.commit(true)
	f1, f2 = h.restate("F-0001"), fix("F-0002", "")
	f1.RootCause, f2.Commits = "analysed", f1.Commits
	h.accept(fp, "impl-1", ImplementerBody{Fixes: []Fix{f1, f2}})
	vp := h.verifying()
	x, _ := lookup(h.state, "F-0001")
	if !slices.Equal(vp.Subjects, []string{"F-0001", "F-0002"}) || slices.Contains(x.RecordedOn(), vp.Candidate.Digest) {
		t.Fatalf("verify packet subjects %v on %s; F-0001 recorded on %v", vp.Subjects, vp.Candidate.Digest, x.RecordedOn())
	}
}

// A fix that awaits verification keeps the gate closed whatever the tier of
// its finding, as Next says there is a verification to do. Only a ledger that
// did not come from these commands holds a claim on a non-blocking finding.
func TestClaimedFixOfAnyTierKeepsTheGateClosed(t *testing.T) {
	h := newHarness(t)
	h.checks(true)
	h.accept(h.packet(), "rev-1", review(h.contract, VerdictRed, map[string]string{"REQ-1": RequirementViolated},
		blocker("crash", "REQ-1"), improvement("long")))
	fp := h.packet()
	h.accept(fp, "impl-1", ImplementerBody{Fixes: []Fix{h.repair("F-0001")}})
	h.accept(h.verifying(), "ver-1", verify(map[string]string{"F-0001": VerificationConfirmed}, "F-0001"))
	h.accept(h.packet(), "rev-1", green(h.contract))
	h.expect(StepNone)
	h.seal([]Draft{{Type: EventFindingFixClaimed, Data: FindingFixClaimed{ID: "F-0002", Round: 1, Packet: fp.ID,
		Candidate: h.candidate().Digest, Fix: fix("F-0002", h.history[len(h.history)-1].Hash), Paths: []string{"src/v1.go"}}}})
	if a, g := h.next(), h.gate(); a.Step != StepVerify || g.Open {
		t.Fatalf("next %s with the gate open %t", a.Step, g.Open)
	}
}

// Check results are recorded in contract order, whatever order they come in.
func TestEvidenceIsRecordedInContractOrder(t *testing.T) {
	h := newHarness(t)
	c := testContract()
	c.Requirements = slices.Clone(c.Requirements)
	c.Requirements[1].Command = "go vet ./..."
	h.contract = c
	h.must(Pin(h.state, h.facts(), "two checks", "gov"))
	f := h.facts()
	ev := func(req, cmd string) Evidence {
		return Evidence{Requirement: req, Command: cmd, Candidate: f.Candidate.Digest, Passed: true, OutputDigest: digestOf("out")}
	}
	d := h.must(RecordEvidence(h.state, f, []Evidence{ev("REQ-2", "go vet ./..."), ev("REQ-1", "go test ./...")}))
	if len(d) != 2 || d[0].Data.(Evidence).Requirement != "REQ-1" || d[1].Data.(Evidence).Requirement != "REQ-2" {
		t.Fatalf("drafts %+v", d)
	}
}
