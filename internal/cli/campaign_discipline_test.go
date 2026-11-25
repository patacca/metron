package cli

import (
	"slices"
	"strings"
	"testing"

	"github.com/patacca/metron/internal/core"
)

// End-to-end campaigns that hold a correction to what its blocking findings
// need, through git: every commit belongs to a fix, the checks pass before a
// verification, the verifier judges the scope, and the review is told which
// candidate was verified. And the decisions recorded on the way bind where
// they were made.

// redRound is pinned after a RED round that opened F-0001, with the fix
// packet issued; it returns its report skeleton.
func redRound(t *testing.T, r *repo) core.Report {
	t.Helper()
	r.must("check")
	_, sk := r.packet()
	r.ingest(exitOK, red(t, sk, "rev-1", misspelled()))
	_, sk = r.packet()
	return sk
}

// TestCampaignUnlistedCommit commits an unrelated change during a
// correction: a report that leaves it out is refused, one that lists it is
// accepted, and a commit that touches only .metron/ needs no fix.
func TestCampaignUnlistedCommit(t *testing.T) {
	r := pinned(t)
	sk := redRound(t, r)
	unrelated := r.change("add a farewell", "farewell.txt", "bye\n")
	fixed := r.change("spell the greeting correctly", "greeting.txt", "hello\n")
	notes := r.change("note the fix for governance", ".metron/notes.txt", "fixed\n")
	msg := r.rejected(claim(t, sk, "impl-1", "", fixed), core.LayerTransition, "/body/fixes", "commit "+unrelated+" changes the candidate")
	if strings.Contains(msg, notes) {
		t.Fatalf("the refusal asks for the commit that touches only .metron/: %s", msg)
	}
	r.ingest(exitOK, claim(t, sk, "impl-1", "", unrelated, fixed))
	r.expectAction(core.StepRunChecks, core.RoleImplementer)
}

// TestCampaignClaimLapses fails the check of a claimed fix and commits a
// correction: the claim lapses, and the fix is claimed again with both
// commits before the checks and the verification.
func TestCampaignClaimLapses(t *testing.T) {
	r := campaign(t, campaignContract(2, 3, 5, 20))
	sk := redRound(t, r)
	r.write("greeting.txt", "hello\n")
	r.write("broken", "yes\n")
	first := r.commitOnly("spell the greeting, and break the check", "greeting.txt", "broken")
	r.ingest(exitOK, claim(t, sk, "impl-1", "", first))
	claimed := r.status().Candidate.Digest
	r.expectAction(core.StepRunChecks, core.RoleImplementer)
	r.expect(exitNegative, "check")
	r.expectAction(core.StepRunChecks, core.RoleImplementer)

	r.remove("broken")
	second := r.commitOnly("repair the check", "broken")
	a := r.expectAction(core.StepFix, core.RoleImplementer)
	if !slices.Equal(a.Findings, []string{"F-0001"}) || !strings.Contains(a.Reason, claimed) || !strings.Contains(a.Reason, "claim it again") {
		t.Fatalf("next %+v", a)
	}
	p, sk := r.packet()
	if !slices.Equal(p.Subjects, []string{"F-0001"}) || p.Findings[0].Status != core.StatusClaimed {
		t.Fatalf("fix packet subjects %v, findings %+v", p.Subjects, p.Findings)
	}
	r.rejected(claim(t, sk, "impl-1", "", first), core.LayerTransition, "/body/fixes", "commit "+second+" changes the candidate")
	r.ingest(exitOK, claim(t, sk, "impl-1", "", first, second))
	r.expectAction(core.StepRunChecks, core.RoleImplementer)
	r.must("check")
	r.expectAction(core.StepVerify, core.RoleVerifier)
	if vp, _ := r.packet(); !slices.Equal(vp.Findings[0].Fix.Commits, []string{first, second}) {
		t.Fatalf("the verifier is given the fix %+v", vp.Findings[0].Fix)
	}
}

// TestCampaignExcessiveFix has the verifier judge a fix excessive: confirming
// it is refused, rejecting it is accepted and opens the finding again.
func TestCampaignExcessiveFix(t *testing.T) {
	r := pinned(t)
	sk := redRound(t, r)
	r.ingest(exitOK, claim(t, sk, "impl-1", "", r.change("spell the greeting correctly", "greeting.txt", "hello\n")))
	r.must("check")
	_, sk = r.packet()
	b := core.VerifierBody{Verifications: []core.Verification{{Finding: "F-0001", Result: core.VerificationConfirmed,
		Evidence: "cat greeting.txt prints hello", Scope: core.ScopeExcessive, ScopeEvidence: "the commit also reflows the file"}}}
	r.rejected(complete(t, sk, "ver-1", b), core.LayerSemantic, "/body/verifications/0/result", "confirmed although its scope is excessive")
	b.Verifications[0].Result = core.VerificationRejected
	r.ingest(exitOK, complete(t, sk, "ver-1", b))
	if f := r.findings("--all")[0]; f.Status != core.StatusOpen || f.Recurrences != 1 {
		t.Fatalf("finding %s is %s with %d recurrences", f.ID, f.Status, f.Recurrences)
	}
	r.expectAction(core.StepFix, core.RoleImplementer)
}

// TestCampaignReviewNamesVerifiedCandidate checks what the review after a
// correction is told: the candidate the fixes were verified on, which is
// still named after a later commit that no verifier saw.
func TestCampaignReviewNamesVerifiedCandidate(t *testing.T) {
	r := pinned(t)
	sk := redRound(t, r)
	r.ingest(exitOK, claim(t, sk, "impl-1", "", r.change("spell the greeting correctly", "greeting.txt", "hello\n")))
	r.must("check")
	vp, sk := r.packet()
	r.ingest(exitOK, verify(t, sk, "ver-1", core.VerificationConfirmed))
	rp, _ := r.packet()
	if rp.Action != core.StepReview || rp.Verified == nil || *rp.Verified != vp.Candidate || rp.Candidate != vp.Candidate {
		t.Fatalf("review packet on %+v names verified %+v; the verification was on %+v", rp.Candidate, rp.Verified, vp.Candidate)
	}
	r.change("greet louder", "greeting.txt", "HELLO\n")
	r.expectAction(core.StepRunChecks, core.RoleImplementer)
	r.must("check")
	later, _ := r.packet()
	if later.ID == rp.ID || later.Verified == nil || *later.Verified != vp.Candidate || later.Candidate == vp.Candidate {
		t.Fatalf("review packet %s on %+v names verified %+v; the verification was on %+v", later.ID, later.Candidate, later.Verified, vp.Candidate)
	}
}

// TestCampaignBacklogAddressed leaves a T2 finding open on an approved
// candidate; ordinary work addresses it, and the next reviewer closes it.
func TestCampaignBacklogAddressed(t *testing.T) {
	r := pinned(t)
	r.change("spell the greeting correctly", "greeting.txt", "hello\n")
	r.must("check")
	_, sk := r.packet()
	r.ingest(exitOK, review(t, sk, "rev-1", nil, improvement("punctuation")))
	r.expectNext(core.StepNone)
	if b := r.findings("--backlog"); len(b) != 1 || b[0].ID != "F-0001" {
		t.Fatalf("backlog %+v", b)
	}
	r.change("punctuate the greeting", "greeting.txt", "hello.\n")
	r.expectAction(core.StepRunChecks, core.RoleImplementer)
	r.must("check")
	_, sk = r.packet()
	b := reviewBody(t, sk, nil)
	b.Addressed = []core.Addressed{{Finding: "F-0001", Evidence: "greeting.txt ends with a full stop"}}
	r.ingest(exitOK, complete(t, sk, "rev-2", b))
	r.expectNext(core.StepNone)
	if b := r.findings("--backlog"); len(b) != 0 {
		t.Fatalf("backlog %+v after the finding was addressed", b)
	}
	f := r.findings("--all")[0]
	if last := f.History[len(f.History)-1]; f.Status != core.StatusResolved || last.Event != core.EventFindingAddressed {
		t.Fatalf("finding %s is %s, last step %s", f.ID, f.Status, last.Event)
	}
}

// expectDecisions checks which decisions a packet carries as binding and
// which as earlier ones.
func expectDecisions(t *testing.T, p core.Packet, binding, earlier string) {
	t.Helper()
	if got := decisionIDs(p.Decisions); got != binding {
		t.Errorf("packet %s (%s) binds decisions %q, want %q", p.ID, p.Action, got, binding)
	}
	if got := decisionIDs(p.EarlierDecisions); got != earlier {
		t.Errorf("packet %s (%s) carries earlier decisions %q, want %q", p.ID, p.Action, got, earlier)
	}
}

// TestCampaignDecisions follows the decisions of a campaign into its packets:
// a directive binds under the contract of its audit and no other; evidence
// governance supplies binds on its candidate only; and a directive an audit
// supersedes no longer binds.
func TestCampaignDecisions(t *testing.T) {
	r := pinned(t)
	sk := redRound(t, r)
	reviewed := r.status().Candidate.Digest
	r.ingest(exitOK, dispute(t, sk, "impl-1"))
	_, sk = r.packet()
	b := auditBody(t, sk)
	b.Adjudications[0].Decision, b.Adjudications[0].Rationale = core.DecisionWithdraw, "no requirement fixes the spelling"
	b.Directives = []string{"Cite the house style before reporting spelling"}
	r.ingest(exitOK, complete(t, sk, "aud-1", b))
	p, _ := r.packet()
	expectDecisions(t, p, "D-0001 D-0002", "")

	// An amended contract of the same stage poses another question.
	r.write(".metron/contract.yaml", strings.Replace(testContract, "An internal demonstration", "A demonstration to the team", 1))
	r.must("contract", "pin", "-m", "widen the audience")
	p, sk = r.packet()
	expectDecisions(t, p, "", "D-0001 D-0002")

	// Evidence supplied for an evidence gap binds on its candidate.
	r.ingest(exitOK, review(t, sk, "rev-1", map[string]string{"REQ-2": core.RequirementUnverified}))
	if a := r.expectAction(core.StepAdjudicate, core.RoleGovernance); !slices.Equal(a.Escalations, []string{"E-0002"}) {
		t.Fatalf("next %+v", a)
	}
	r.must("resolve", "E-0002", "-m", "REQ-2 holds: the product owner read greeting.txt aloud")
	p, _ = r.packet()
	expectDecisions(t, p, "D-0003", "D-0001 D-0002")
	if d := p.Decisions[0]; d.Kind != core.EscalationEvidenceGap || d.By != core.RoleGovernance || d.Candidate != reviewed || !slices.Equal(d.Requirements, []string{"REQ-2"}) {
		t.Fatalf("decision %+v", d)
	}
	r.change("spell the greeting correctly", "greeting.txt", "hello\n")
	r.must("check")
	p, sk = r.packet()
	expectDecisions(t, p, "", "D-0001 D-0002 D-0003")

	// A directive issued under this contract binds until an audit supersedes
	// it. The third failed step calls for the audit that issues it.
	r.ingest(exitOK, red(t, sk, "rev-1", misspelled()))
	r.expectAction(core.StepAudit, core.RoleAuditor)
	_, sk = r.packet()
	b = auditBody(t, sk)
	b.Directives = []string{"Quote greeting.txt byte for byte in evidence"}
	r.ingest(exitOK, complete(t, sk, "aud-1", b))
	p, sk = r.packet()
	expectDecisions(t, p, "D-0004 D-0005", "D-0001 D-0002 D-0003")
	r.ingest(exitOK, dispute(t, sk, "impl-1"))
	ap, sk := r.packet()
	b = auditBody(t, sk)
	b.Supersede = []core.Supersession{{Decision: "D-0004", Rationale: "evidence may paraphrase a one-word file"}}
	r.ingest(exitOK, complete(t, sk, "aud-1", b))
	p, _ = r.packet()
	expectDecisions(t, p, "D-0005 D-0006", "D-0001 D-0002 D-0003 D-0004")
	if d := p.EarlierDecisions[3]; d.SupersededBy != ap.ID {
		t.Fatalf("superseded decision %+v, want superseded by %s", d, ap.ID)
	}
}
