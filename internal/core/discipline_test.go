package core

import (
	"slices"
	"strings"
	"testing"
)

// A correction cycle changes what its blocking findings need and nothing
// else: every commit in it belongs to a fix, the checks pass before anyone
// verifies it, it is verified as one batch on one candidate, and the review
// that follows is told which candidate that was.

// A claim is about a candidate. When the candidate changes before the
// verification, the claims are made again, so that no commit reaches a
// verifier without a fix that accounts for it.
func TestClaimsLapseWhenTheCandidateChanges(t *testing.T) {
	h := newHarness(t)
	h.checks(true)
	h.accept(h.packet(), "rev-1", red(h.contract, "crash"))
	h.accept(h.packet(), "impl-1", ImplementerBody{Fixes: []Fix{fix("F-0001", h.commit(true))}})
	claimed := h.candidate().Digest

	// The checks fail on the claimed candidate, and the implementer commits
	// a correction.
	h.expect(StepRunChecks)
	h.checks(false)
	h.expect(StepRunChecks)
	extra := h.commit(true)
	a := h.expect(StepFix)
	if !slices.Equal(a.Findings, []string{"F-0001"}) || !strings.Contains(a.Reason, claimed) || !strings.Contains(a.Reason, "again") {
		t.Fatalf("action %+v", a)
	}
	fp := h.packet()
	if !slices.Equal(fp.Subjects, []string{"F-0001"}) || fp.Findings[0].Status != StatusClaimed {
		t.Fatalf("fix packet %+v", fp)
	}
	// A claimed fix is claimed again, not disputed, and with every commit.
	if ps := h.refused(fp, "impl-1", ImplementerBody{Disputes: []Dispute{{Finding: "F-0001", Rationale: "no"}}}, LayerTransition); !hasProblem(ps, "/body/disputes/0/finding") {
		t.Fatalf("problems %+v", ps)
	}
	if ps := h.refused(fp, "impl-1", ImplementerBody{}, LayerTransition); !hasProblem(ps, "/body/fixes") {
		t.Fatalf("problems %+v", ps)
	}
	again := fix("F-0001", extra)
	again.Commits = append(h.state.findings[0].Fix.Commits, extra)
	eqTypes(t, h.accept(fp, "impl-1", ImplementerBody{Fixes: []Fix{again}}), EventReportAccepted, EventFindingFixClaimed)

	f, _ := lookup(h.state, "F-0001")
	if f.Status != StatusClaimed || len(f.Fix.Commits) != 2 || f.History[len(f.History)-1].Candidate != h.candidate().Digest {
		t.Fatalf("finding %+v", f)
	}
	h.expect(StepRunChecks)
	h.checks(true)
	vp := h.packet()
	if vp.Action != StepVerify || vp.Candidate != h.candidate() {
		t.Fatalf("verify packet %+v", vp)
	}

	// The candidate changes under the verifier: the packet is void and the
	// claim has lapsed again.
	h.commit(true)
	if a := h.expect(StepFix); !strings.Contains(a.Reason, "can no longer be answered") {
		t.Fatalf("action %+v", a)
	}
}

// A correction is verified as one batch on one candidate: a finding that an
// audit upholds is fixed before the fixes already claimed are verified.
func TestCorrectionIsVerifiedAsOneBatch(t *testing.T) {
	h := newHarness(t)
	h.checks(true)
	h.accept(h.packet(), "rev-1", review(h.contract, VerdictRed, map[string]string{"REQ-1": RequirementViolated},
		blocker("a", "REQ-1"), blocker("b", "REQ-1")))
	h.accept(h.packet(), "impl-1", ImplementerBody{Fixes: []Fix{fix("F-0001", h.commit(true))},
		Disputes: []Dispute{{Finding: "F-0002", Rationale: "not a defect"}}})
	ap := h.packet()
	h.accept(ap, "aud-1", AuditorBody{Resolutions: resolutions(ap, "it stands"),
		Adjudications: []Adjudication{{Finding: "F-0002", Decision: DecisionUphold, Rationale: "reproducible"}}})
	fp := h.packet()
	if fp.Action != StepFix || !slices.Equal(fp.Subjects, []string{"F-0001", "F-0002"}) {
		t.Fatalf("packet after the audit: %s for %v", fp.Action, fp.Subjects)
	}
	// Fixing the upheld finding changes the candidate, so the fix claimed
	// before is claimed again with it.
	again := h.restate("F-0001")
	second := fix("F-0002", h.commit(true))
	if ps := h.refused(fp, "impl-1", ImplementerBody{Fixes: []Fix{second}}, LayerTransition); !hasProblem(ps, "/body/fixes") {
		t.Fatalf("problems %+v", ps)
	}
	h.accept(fp, "impl-1", ImplementerBody{Fixes: []Fix{again, second}})
	vp := h.verifying()
	if !slices.Equal(vp.Subjects, []string{"F-0001", "F-0002"}) || vp.Candidate != h.candidate() {
		t.Fatalf("verify packet %+v", vp)
	}
	h.accept(vp, "ver-1", verify(map[string]string{"F-0001": VerificationConfirmed, "F-0002": VerificationConfirmed}, "F-0001", "F-0002"))
	h.expect(StepReview)
}

// The verifier judges whether a fix changes only what its finding needs. A
// fix that does more is rejected for that alone.
func TestVerifierJudgesTheScopeOfAFix(t *testing.T) {
	setup := func(t *testing.T) (*harness, Packet) {
		h := newHarness(t)
		h.checks(true)
		h.accept(h.packet(), "rev-1", red(h.contract, "crash"))
		h.accept(h.packet(), "impl-1", ImplementerBody{Fixes: []Fix{fix("F-0001", h.commit(true))}})
		return h, h.verifying()
	}
	body := func(result, scope, evidence string) VerifierBody {
		return VerifierBody{Verifications: []Verification{{Finding: "F-0001", Result: result, Evidence: "the repro passes",
			Scope: scope, ScopeEvidence: evidence}}}
	}
	for _, tc := range []struct {
		name string
		body VerifierBody
		path string
	}{
		{"excessive but confirmed", body(VerificationConfirmed, ScopeExcessive, "it also rewrites the lexer"), "/body/verifications/0/result"},
		{"no scope", body(VerificationConfirmed, "", "only the guard"), "/body/verifications/0/scope"},
		{"unknown scope", body(VerificationRejected, "minimal", "only the guard"), "/body/verifications/0/scope"},
		{"no scope evidence", body(VerificationConfirmed, ScopeProportionate, " "), "/body/verifications/0/scope_evidence"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h, vp := setup(t)
			if ps := h.refused(vp, "ver-1", tc.body, LayerSemantic); !hasProblem(ps, tc.path) {
				t.Fatalf("problems %+v", ps)
			}
		})
	}
	t.Run("excessive and rejected", func(t *testing.T) {
		h, vp := setup(t)
		d := h.accept(vp, "ver-1", body(VerificationRejected, ScopeExcessive, "it also rewrites the lexer, which the finding does not need"))
		eqTypes(t, d, EventReportAccepted, EventFindingFixRejected)
		if e := d[1].Data.(FindingFixRejected); e.Scope != ScopeExcessive || !strings.Contains(e.ScopeEvidence, "lexer") {
			t.Fatalf("event %+v", e)
		}
		f, _ := lookup(h.state, "F-0001")
		if f.Status != StatusOpen || f.Recurrences != 1 || !strings.Contains(f.History[len(f.History)-1].Note, "more than the finding needs") {
			t.Fatalf("finding %+v", f)
		}
		if h.state.FailedStreak() != 2 {
			t.Fatalf("streak %d: a fix rejected for its scope is a failed step", h.state.FailedStreak())
		}
		h.expect(StepFix)
	})
	t.Run("proportionate and confirmed", func(t *testing.T) {
		h, vp := setup(t)
		d := h.accept(vp, "ver-1", body(VerificationConfirmed, ScopeProportionate, "the guard and its test"))
		if e := d[1].Data.(FindingFixConfirmed); e.Scope != ScopeProportionate || e.ScopeEvidence != "the guard and its test" {
			t.Fatalf("event %+v", e)
		}
	})
}

// The review after a correction is told which candidate the fixes were
// verified on, so that changes made after the verification are not taken for
// verified ones.
func TestReviewPacketNamesTheVerifiedCandidate(t *testing.T) {
	h := newHarness(t)
	h.checks(true)
	if rp := h.packet(); rp.Verified != nil {
		t.Fatalf("the first review names a verified candidate: %+v", rp.Verified)
	}
	h.accept(h.packet(), "rev-1", red(h.contract, "crash"))
	h.accept(h.packet(), "impl-1", ImplementerBody{Fixes: []Fix{fix("F-0001", h.commit(true))}})
	vp := h.verifying()
	h.accept(vp, "ver-1", verify(map[string]string{"F-0001": VerificationConfirmed}, "F-0001"))
	// Work done after the verification is not part of the correction. It is
	// allowed, and the reviewer is told that nobody verified it.
	h.commit(true)
	h.checks(true)
	rp := h.packet()
	if rp.Verified == nil || *rp.Verified != vp.Candidate || *rp.Verified == rp.Candidate {
		t.Fatalf("review packet verified %+v, candidate %+v, verification on %+v", rp.Verified, rp.Candidate, vp.Candidate)
	}
	if f := rp.Findings[0]; f.History[len(f.History)-1].Event != EventFindingFixConfirmed || f.History[len(f.History)-1].Candidate != vp.Candidate.Digest {
		t.Fatalf("history %+v", f.History)
	}
	h.accept(rp, "rev-1", green(h.contract))
	h.commit(true)
	h.checks(true)
	if rp := h.packet(); rp.Verified != nil {
		t.Fatalf("a review with no correction before it names a verified candidate: %+v", rp.Verified)
	}
}

// Non-blocking findings are no part of a correction. They are addressed by
// ordinary work, and a later review closes them with evidence.
func TestBacklogIsClosedByALaterReview(t *testing.T) {
	h := newHarness(t)
	h.checks(true)
	h.accept(h.packet(), "rev-1", review(h.contract, VerdictRed, map[string]string{"REQ-1": RequirementViolated},
		blocker("crash", "REQ-1"), improvement("long"), improvement("slow")))
	fp := h.packet()
	if !slices.Equal(fp.Subjects, []string{"F-0001"}) || len(fp.Findings) != 1 {
		t.Fatalf("the fix packet offers more than the blocking finding: %+v", fp.Findings)
	}
	h.accept(fp, "impl-1", ImplementerBody{Fixes: []Fix{fix("F-0001", h.commit(true))}})
	h.accept(h.verifying(), "ver-1", verify(map[string]string{"F-0001": VerificationConfirmed}, "F-0001"))
	h.accept(h.packet(), "rev-1", green(h.contract))
	h.expect(StepNone)
	if b := h.state.Backlog(); len(b) != 2 {
		t.Fatalf("backlog %+v", b)
	}

	// The long function is split in ordinary work, outside any correction.
	h.commit(true)
	h.checks(true)
	rp := h.packet()
	closing := func(entries ...Addressed) ReviewerBody {
		b := green(h.contract)
		b.Addressed = entries
		return b
	}
	for _, tc := range []struct {
		name  string
		body  ReviewerBody
		layer Layer
		path  string
	}{
		{"unknown finding", closing(Addressed{Finding: "F-0042", Evidence: "e"}), LayerSemantic, "/body/addressed/0/finding"},
		{"twice", closing(Addressed{Finding: "F-0002", Evidence: "e"}, Addressed{Finding: "F-0002", Evidence: "e"}), LayerSemantic, "/body/addressed/1/finding"},
		{"no evidence", closing(Addressed{Finding: "F-0002", Evidence: " "}), LayerSemantic, "/body/addressed/0/evidence"},
		{"a resolved blocking finding", closing(Addressed{Finding: "F-0001", Evidence: "e"}), LayerTransition, "/body/addressed/0/finding"},
	} {
		if ps := h.refused(rp, "rev-2", tc.body, tc.layer); !hasProblem(ps, tc.path) {
			t.Fatalf("%s: problems %+v", tc.name, ps)
		}
	}
	d := h.accept(rp, "rev-2", closing(Addressed{Finding: "F-0002", Evidence: "parse() is now three functions of under 80 lines each"}))
	eqTypes(t, d, EventReportAccepted, EventFindingAddressed, EventVerdictRecorded)
	f, _ := lookup(h.state, "F-0002")
	last := f.History[len(f.History)-1]
	if f.Status != StatusResolved || last.Event != EventFindingAddressed || last.Candidate != rp.Candidate.Digest || last.Packet != rp.ID {
		t.Fatalf("finding %+v", f)
	}
	if b := h.state.Backlog(); len(b) != 1 || b[0].ID != "F-0003" {
		t.Fatalf("backlog %+v", b)
	}
	h.expect(StepNone)

	// It is closed like any resolved finding: it cannot be closed twice, and
	// a later review can reopen it with a reason.
	h.commit(true)
	h.checks(true)
	rp = h.packet()
	if ps := h.refused(rp, "rev-1", closing(Addressed{Finding: "F-0002", Evidence: "e"}), LayerTransition); !hasProblem(ps, "/body/addressed/0/finding") {
		t.Fatalf("problems %+v", ps)
	}
	b := green(h.contract)
	b.Reopen = []Reopen{{Finding: "F-0002", Reason: ReopenRegression, Justification: "the function grew back"}}
	h.accept(rp, "rev-1", b)
	if f, _ := lookup(h.state, "F-0002"); f.Status != StatusOpen || f.Recurrences != 1 {
		t.Fatalf("finding %+v", f)
	}
	h.expect(StepNone)
}

// An open blocking finding is never closed by a review.
func TestBlockingFindingIsNotAddressedByAReview(t *testing.T) {
	h := newHarness(t)
	h.checks(true)
	h.accept(h.packet(), "rev-1", red(h.contract, "crash"))
	h.accept(h.packet(), "impl-1", ImplementerBody{Disputes: []Dispute{{Finding: "F-0001", Rationale: "no"}}})
	ap := h.packet()
	h.accept(ap, "aud-1", AuditorBody{Resolutions: resolutions(ap, "withdrawn"),
		Adjudications: []Adjudication{{Finding: "F-0001", Decision: DecisionWithdraw, Rationale: "outside the envelope"}}})
	rp := h.packet()
	second := red(h.contract, "again")
	h.accept(rp, "rev-1", second)
	// F-0002 is open and blocking: the implementer fixes it.
	h.accept(h.packet(), "impl-1", ImplementerBody{Fixes: []Fix{fix("F-0002", h.commit(true))}})
	h.accept(h.verifying(), "ver-1", verify(map[string]string{"F-0002": VerificationRejected}, "F-0002"))
	h.drain()
	h.accept(h.packet(), "impl-1", ImplementerBody{Disputes: []Dispute{{Finding: "F-0002", Rationale: "no"}}})
	ap = h.packet()
	h.accept(ap, "aud-1", AuditorBody{Resolutions: resolutions(ap, "withdrawn"),
		Adjudications: []Adjudication{{Finding: "F-0002", Decision: DecisionReclassify, Tier: TierImprovement, Rationale: "an improvement"}}})
	h.checks(true)
	rp = h.packet()
	b := green(h.contract)
	b.Addressed = []Addressed{{Finding: "F-0001", Evidence: "e"}}
	if ps := h.refused(rp, "rev-1", b, LayerTransition); !hasProblem(ps, "/body/addressed/0/finding") {
		t.Fatalf("a withdrawn finding was marked addressed: %+v", ps)
	}
	b.Addressed = []Addressed{{Finding: "F-0002", Evidence: "the candidate handles empty input"}}
	h.accept(rp, "rev-1", b)
	if f, _ := lookup(h.state, "F-0002"); f.Status != StatusResolved || f.Tier != TierImprovement {
		t.Fatalf("finding %+v", f)
	}
}
