package cli

import (
	"slices"
	"strings"
	"testing"

	"github.com/patacca/metron/internal/core"
)

// End-to-end campaigns in which git returns to a tree the ledger has seen. A
// candidate is its content, not its place in history: a revert is the
// earlier candidate again, and what the ledger recorded on it still holds.

// corrected is pinned after a RED round on the misspelled greeting, whose
// candidate it returns, and the fix of F-0001 confirmed on a corrected
// greeting: the next action is the review of the correction.
func corrected(t *testing.T) (*repo, string) {
	t.Helper()
	r := pinned(t)
	r.must("check")
	reviewed := r.status().Candidate.Digest
	_, sk := r.packet()
	r.ingest(exitOK, red(t, sk, "rev-1", misspelled()))
	_, sk = r.packet()
	r.ingest(exitOK, claim(t, sk, "impl-1", "", r.change("spell the greeting correctly", "greeting.txt", "hello\n")))
	r.must("check")
	_, sk = r.packet()
	r.ingest(exitOK, verify(t, sk, "ver-1", core.VerificationConfirmed))
	r.expectAction(core.StepReview, core.RoleReviewer)
	return r, reviewed
}

// TestCampaignRevertToRedCandidate reverts a verified fix: the candidate is
// the one the RED verdict was given on, which carries F-0001, so only a new
// change helps (D3).
func TestCampaignRevertToRedCandidate(t *testing.T) {
	r, reviewed := corrected(t)
	r.revert()
	if got := r.status().Candidate.Digest; got != reviewed {
		t.Fatalf("the revert gives candidate %s, want the reviewed %s", got, reviewed)
	}
	a := r.expectAction(core.StepCommitChanges, core.RoleImplementer)
	if !slices.Equal(a.Findings, []string{"F-0001"}) || a.Command != "" || a.Usage != "" || !strings.Contains(a.Reason, "F-0001") {
		t.Fatalf("next %+v", a)
	}
	g := r.gateDoc(exitNegative)
	if !slices.Contains(failing(g), core.ConditionBlockers) || !strings.Contains(reasons(g, core.ConditionBlockers), "F-0001") {
		t.Fatalf("gate fails %v; B says %q", failing(g), reasons(g, core.ConditionBlockers))
	}
	contains(t, "packet error", r.expect(exitNegative, "packet").err, "the next action is commit-changes, which takes no packet")
}

// TestCampaignRevertAfterReclassification records, in a GREEN round, a
// non-blocking finding that meets every rule of a blocking one and declares
// that it reverses F-0001's resolution. An audit makes it T1, its fix is
// confirmed on another commit, and git reverts that commit: the GREEN verdict
// of the round that found it does not approve its candidate (D1).
func TestCampaignRevertAfterReclassification(t *testing.T) {
	r, _ := corrected(t)
	greened := r.status().Candidate.Digest
	_, sk := r.packet()
	d := r.ingestDoc(exitOK, review(t, sk, "rev-2", nil, eligible("tone", "F-0001")))
	if d.Verdict != core.VerdictGreen || !slices.Equal(d.FindingsOpened, []string{"F-0002"}) || !slices.Equal(d.EscalationsRaised, []string{"E-0001"}) {
		t.Fatalf("ingest document %+v", d)
	}
	r.expectAction(core.StepAudit, core.RoleAuditor)
	ap, sk := r.packet()
	if !slices.Equal(ap.Adjudicable, []string{"F-0002"}) || ap.Escalations[0].Kind != core.EscalationContradiction {
		t.Fatalf("audit packet adjudicable %v, escalations %+v", ap.Adjudicable, ap.Escalations)
	}
	b := auditBody(t, sk)
	b.Adjudications[0] = core.Adjudication{Finding: "F-0002", Decision: core.DecisionReclassify, Tier: core.TierMaterial,
		Rationale: "REQ-2 asks for the agreed greeting"}
	r.ingest(exitOK, complete(t, sk, "aud-1", b))
	if a := r.expectAction(core.StepFix, core.RoleImplementer); !slices.Equal(a.Findings, []string{"F-0002"}) {
		t.Fatalf("next %+v", a)
	}
	_, sk = r.packet()
	r.ingest(exitOK, claim(t, sk, "impl-1", "", r.change("punctuate the greeting", "greeting.txt", "hello.\n")))
	r.must("check")
	_, sk = r.packet()
	r.ingest(exitOK, verify(t, sk, "ver-1", core.VerificationConfirmed))
	r.expectAction(core.StepReview, core.RoleReviewer)

	r.revert()
	if got := r.status().Candidate.Digest; got != greened {
		t.Fatalf("the revert gives candidate %s, want %s", got, greened)
	}
	if a := r.expectAction(core.StepCommitChanges, core.RoleImplementer); !slices.Equal(a.Findings, []string{"F-0002"}) {
		t.Fatalf("next %+v", a)
	}
	// The round that found F-0002 was GREEN, on this candidate, under this
	// contract: only the carried finding keeps the gate closed.
	g := r.gateDoc(exitNegative)
	if !slices.Equal(failing(g), []string{core.ConditionBlockers}) || !strings.Contains(reasons(g, core.ConditionBlockers), "F-0002") {
		t.Fatalf("gate fails %v; B says %q", failing(g), reasons(g, core.ConditionBlockers))
	}
	r.expect(exitNegative, "packet")
}

// TestCampaignReturnToApprovedCandidate goes back to an approved candidate
// after a later round was RED: the earlier approval is not revived. Going
// back is a change like any other, claimed, verified and reviewed again.
func TestCampaignReturnToApprovedCandidate(t *testing.T) {
	r := green(t)
	approved := r.status().Candidate.Digest
	r.change("greet the world", "greeting.txt", "hello, world\n")
	r.must("check")
	_, sk := r.packet()
	r.ingest(exitOK, red(t, sk, "rev-1", misspelled()))
	back := r.revert()
	if got := r.status().Candidate.Digest; got != approved {
		t.Fatalf("the revert gives candidate %s, want the approved %s", got, approved)
	}
	if a := r.expectAction(core.StepFix, core.RoleImplementer); !slices.Equal(a.Findings, []string{"F-0001"}) {
		t.Fatalf("next %+v", a)
	}
	if g := r.gateDoc(exitNegative); !slices.Contains(failing(g), core.ConditionIndependent) {
		t.Fatalf("gate fails %v, want V among them: the latest round is RED", failing(g))
	}
	_, sk = r.packet()
	r.ingest(exitOK, claim(t, sk, "impl-1", "", back))
	// The approved candidate's checks passed before: nothing to run again.
	r.expectAction(core.StepVerify, core.RoleVerifier)
	_, sk = r.packet()
	r.ingest(exitOK, verify(t, sk, "ver-1", core.VerificationConfirmed))
	r.expectAction(core.StepReview, core.RoleReviewer)
	_, sk = r.packet()
	r.ingest(exitOK, review(t, sk, "rev-1", nil))
	r.expectNext(core.StepNone)
	r.must("gate")
}

// TestCampaignUpwardReclassificationRefused asks an audit to make blocking a
// finding that cites nothing: the report is refused and nothing is recorded,
// and upholding the finding leaves the gate open on the GREEN round (D2).
func TestCampaignUpwardReclassificationRefused(t *testing.T) {
	r, _ := corrected(t)
	bare := improvement("tone")
	bare.Contradicts = "F-0001"
	_, sk := r.packet()
	r.ingest(exitOK, review(t, sk, "rev-2", nil, bare))
	r.expectAction(core.StepAudit, core.RoleAuditor)
	_, sk = r.packet()
	b := auditBody(t, sk)
	b.Adjudications[0] = core.Adjudication{Finding: "F-0002", Decision: core.DecisionReclassify, Tier: core.TierMaterial, Rationale: "it is material"}
	events := r.ledgerEvents()
	r.rejected(complete(t, sk, "aud-1", b), core.LayerSemantic, "/body/adjudications/0/tier", "finding F-0002 cannot become T1")
	if r.ledgerEvents() != events {
		t.Fatal("the refused audit recorded something")
	}
	r.expectAction(core.StepSubmitReport, core.RoleAuditor)
	r.ingest(exitOK, upheld(t, sk, "aud-1"))
	r.expectNext(core.StepNone)
	r.must("gate")
	if f := r.findings("--all")[1]; f.ID != "F-0002" || f.Tier != core.TierImprovement || f.Status != core.StatusOpen {
		t.Fatalf("finding %s is %s and %s", f.ID, f.Tier, f.Status)
	}
}

// TestCampaignAlternatingRejectedCandidates rejects fixes on two trees, then
// moves between them with git and back to the reviewed tree: no claim is
// accepted on a tree on which F-0001 was recorded, and a genuinely new tree
// is accepted (D4).
func TestCampaignAlternatingRejectedCandidates(t *testing.T) {
	r := pinned(t)
	r.must("check")
	reviewed := r.status().Candidate.Digest
	_, sk := r.packet()
	r.ingest(exitOK, red(t, sk, "rev-1", misspelled()))
	rejectFix := func(greeting string) string {
		t.Helper()
		_, sk := r.packet()
		r.ingest(exitOK, claim(t, sk, "impl-1", "", r.change("try "+greeting, "greeting.txt", greeting+"\n")))
		r.must("check")
		_, sk = r.packet()
		r.ingest(exitOK, verify(t, sk, "ver-1", core.VerificationRejected))
		return r.status().Candidate.Digest
	}
	first, second := rejectFix("hullo"), rejectFix("hallo")
	// The third failed step called for a convergence analysis.
	r.expectAction(core.StepAudit, core.RoleAuditor)
	_, sk = r.packet()
	r.ingest(exitOK, upheld(t, sk, "aud-1"))
	r.expectAction(core.StepFix, core.RoleImplementer)
	fp, sk := r.packet()
	if !slices.Equal(fp.RootCauseRequired, []string{"F-0001"}) {
		t.Fatalf("fix packet root cause required %v", fp.RootCauseRequired)
	}

	var listed []string
	for _, back := range []struct {
		name, digest, by string
		commit           func() string
	}{
		{"the first rejected tree", first, "finding.fix-rejected", r.revert},
		{"the second rejected tree", second, "finding.fix-rejected", r.revert},
		{"the reviewed tree", reviewed, "finding.opened", func() string {
			return r.change("restore the reviewed greeting", "greeting.txt", "helo\n")
		}},
	} {
		listed = append(listed, back.commit())
		if got := r.status().Candidate.Digest; got != back.digest {
			t.Fatalf("%s: the candidate is %s, want %s", back.name, got, back.digest)
		}
		r.rejected(claim(t, sk, "impl-1", "the greeting was typed from memory", listed...), core.LayerTransition, "/body/fixes/0",
			"on which F-0001 was recorded as present by "+back.by)
	}
	listed = append(listed, r.change("spell the greeting correctly", "greeting.txt", "hello\n"))
	r.ingest(exitOK, claim(t, sk, "impl-1", "the greeting was typed from memory", listed...))
	r.expectAction(core.StepRunChecks, core.RoleImplementer)
}
