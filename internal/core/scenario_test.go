package core

import (
	"errors"
	"fmt"
	"reflect"
	"slices"
	"strings"
	"testing"
)

// drain discharges open escalations the way governance and an auditor would,
// until the next action is something else.
func (h *harness) drain() {
	h.tb.Helper()
	for {
		switch a := h.next(); a.Step {
		case StepAdjudicate:
			h.must(Resolve(h.state, h.facts(), a.Escalations[0], "dismissed after review", "gov"))
		case StepAudit:
			p := h.packet()
			var adj []Adjudication
			for _, id := range p.Adjudicable {
				adj = append(adj, Adjudication{Finding: id, Decision: DecisionUphold, Rationale: "it stands"})
			}
			h.accept(p, "aud-1", AuditorBody{Resolutions: resolutions(p, "carry on"), Adjudications: adj})
		default:
			return
		}
	}
}

// checkout commits the content of an earlier tree version again, as git
// revert does: the candidate is one the ledger has already seen.
func (h *harness) checkout(tree int) string {
	h.tree = tree
	c := Commit{Hash: commitHash(len(h.history)), Substantive: true}
	h.history = append(h.history, c)
	h.clean = true
	return c.Hash
}

func TestScenarioGreenInOneRound(t *testing.T) {
	h := newHarness(t)
	h.expect(StepRunChecks)
	if h.gate().Open {
		t.Fatal("the gate is open before any review")
	}
	h.checks(true)
	h.expect(StepReview)
	p := h.packet()
	if p.Round != 1 || p.Action != StepReview || !p.ReadOnly || p.Base != nil {
		t.Fatalf("review packet %+v", p)
	}
	if want := []string{"REQ-1", "REQ-2", "REQ-3"}; !slices.Equal(p.ApplicableRequirements, want) {
		t.Fatalf("applicable %v, want %v", p.ApplicableRequirements, want)
	}
	if len(p.Evidence) != 1 || p.Evidence[0].Requirement != "REQ-1" || !p.Evidence[0].Passed {
		t.Fatalf("packet evidence %+v", p.Evidence)
	}
	if a := h.expect(StepSubmitReport); a.Packet != p.ID || a.Owner != RoleReviewer {
		t.Fatalf("next %+v", a)
	}
	if again := h.packet(); !reflect.DeepEqual(again, p) {
		t.Fatal("planning while the packet is open did not return the same packet")
	}
	eqTypes(t, h.accept(p, "rev-1", green(h.contract)), EventReportAccepted, EventVerdictRecorded)
	h.expect(StepNone)
	g := h.gate()
	if !g.Open || g.Stage != StagePrototype || g.Authorizes != "internal demonstration" {
		t.Fatalf("gate %+v", g)
	}
	if _, err := PlanPacket(h.state, h.facts()); err == nil {
		t.Fatal("a packet was planned while the gate is open")
	}
}

func TestScenarioRedFixVerifyGreen(t *testing.T) {
	h := newHarness(t)
	h.checks(true)
	reviewed := h.candidate()
	eqTypes(t, h.accept(h.packet(), "rev-1", red(h.contract, "crash")),
		EventReportAccepted, EventFindingOpened, EventVerdictRecorded)
	if a := h.expect(StepFix); !slices.Equal(a.Findings, []string{"F-0001"}) {
		t.Fatalf("fix action %+v", a)
	}
	g := h.gate()
	for i, name := range []string{ConditionCriteria, ConditionBlockers, ConditionIndependent} {
		c := g.Conditions[slices.IndexFunc(g.Conditions, func(c Condition) bool { return c.Name == name })]
		if c.Satisfied || len(c.Reasons) == 0 {
			t.Fatalf("condition %d %s satisfied after RED: %+v", i, name, c)
		}
	}
	fp := h.packet()
	if fp.Action != StepFix || fp.ReadOnly || fp.Round != 1 || *fp.Base != reviewed || !slices.Equal(fp.Subjects, []string{"F-0001"}) {
		t.Fatalf("fix packet %+v", fp)
	}
	if rec, _ := h.state.Packet(fp.ID); rec.Base == nil || *rec.Base != reviewed {
		t.Fatalf("the ledger does not record the base: %+v", rec.Base)
	}
	eqTypes(t, h.accept(fp, "impl-1", ImplementerBody{Fixes: []Fix{fix("F-0001", h.commit(true))}}),
		EventReportAccepted, EventFindingFixClaimed)
	h.expect(StepVerify)
	vp := h.packet()
	if vp.Action != StepVerify || !vp.ReadOnly || *vp.Base != reviewed || vp.Candidate == reviewed {
		t.Fatalf("verify packet %+v", vp)
	}
	eqTypes(t, h.accept(vp, "ver-1", verify(map[string]string{"F-0001": VerificationConfirmed}, "F-0001")),
		EventReportAccepted, EventFindingFixConfirmed)
	h.expect(StepRunChecks)
	h.checks(true)
	rp := h.packet()
	if rp.Round != 2 || len(rp.History) != 1 || rp.History[0].Verdict != VerdictRed || len(rp.Findings) != 1 {
		t.Fatalf("second review packet %+v", rp)
	}
	h.accept(rp, "rev-1", green(h.contract))
	h.expect(StepNone)
	if f, _ := lookup(h.state, "F-0001"); f.Status != StatusResolved || f.Fix == nil {
		t.Fatalf("finding %+v", f)
	}
}

func TestScenarioRootCauseRequired(t *testing.T) {
	h := newHarness(t)
	h.checks(true)
	h.accept(h.packet(), "rev-1", red(h.contract, "crash"))
	for attempt := 1; attempt <= 2; attempt++ {
		fp := h.packet()
		if len(fp.RootCauseRequired) != 0 {
			t.Fatalf("attempt %d: root cause required too early", attempt)
		}
		h.accept(fp, "impl-1", ImplementerBody{Fixes: []Fix{fix("F-0001", h.commit(true))}})
		d := h.accept(h.packet(), "ver-1", verify(map[string]string{"F-0001": VerificationRejected}, "F-0001"))
		if got := d[1].Data.(FindingFixRejected).RootCauseRequired; got != (attempt == 2) {
			t.Fatalf("attempt %d: root_cause_required %t", attempt, got)
		}
	}
	if f, _ := lookup(h.state, "F-0001"); f.Recurrences != 2 || !f.RootCauseRequired || f.Status != StatusOpen {
		t.Fatalf("finding %+v", f)
	}
	fp := h.packet()
	if !slices.Equal(fp.RootCauseRequired, []string{"F-0001"}) {
		t.Fatalf("packet root_cause_required %v", fp.RootCauseRequired)
	}
	c := h.commit(true)
	ps := h.refused(fp, "impl-1", ImplementerBody{Fixes: []Fix{fix("F-0001", c)}}, LayerSemantic)
	if !hasProblem(ps, "/body/fixes/0/root_cause") {
		t.Fatalf("problems %+v", ps)
	}
	fx := fix("F-0001", c)
	fx.RootCause = "the parser assumed non-empty input everywhere"
	h.accept(fp, "impl-1", ImplementerBody{Fixes: []Fix{fx}})
	h.accept(h.packet(), "ver-1", verify(map[string]string{"F-0001": VerificationConfirmed}, "F-0001"))
	if f, _ := lookup(h.state, "F-0001"); !f.RootCauseRequired || f.Status != StatusResolved {
		t.Fatalf("the flag did not stay set: %+v", f)
	}
}

func TestScenarioDispute(t *testing.T) {
	start := func(t *testing.T) (*harness, Packet) {
		h := newHarness(t)
		h.checks(true)
		h.accept(h.packet(), "rev-1", red(h.contract, "crash"))
		d := h.accept(h.packet(), "impl-1", ImplementerBody{Disputes: []Dispute{{Finding: "F-0001", Rationale: "the demo never reads empty files"}}})
		if k := raisedKinds(d); !slices.Equal(k, []EscalationKind{EscalationDispute}) {
			t.Fatalf("raised %v", k)
		}
		if f, _ := lookup(h.state, "F-0001"); !f.Disputed || f.Status != StatusOpen {
			t.Fatalf("finding %+v", f)
		}
		if a := h.expect(StepAudit); !slices.Equal(a.Escalations, []string{"E-0001"}) {
			t.Fatalf("audit action %+v", a)
		}
		ap := h.packet()
		if !slices.Equal(ap.Adjudicable, []string{"F-0001"}) || len(ap.Escalations) != 1 || ap.Escalations[0].ID != "E-0001" {
			t.Fatalf("audit packet %+v", ap)
		}
		return h, ap
	}

	t.Run("upheld", func(t *testing.T) {
		h, ap := start(t)
		eqTypes(t, h.accept(ap, "aud-1", AuditorBody{Resolutions: resolutions(ap, "the finding stands"),
			Adjudications: []Adjudication{{Finding: "F-0001", Decision: DecisionUphold, Rationale: "reproducible"}}}),
			EventReportAccepted, EventEscalationResolved)
		h.expect(StepFix)
		fp := h.packet()
		ps := h.refused(fp, "impl-1", ImplementerBody{Disputes: []Dispute{{Finding: "F-0001", Rationale: "still disagree"}}}, LayerTransition)
		if !hasProblem(ps, "/body/disputes/0/finding") {
			t.Fatalf("problems %+v", ps)
		}
		h.accept(fp, "impl-1", ImplementerBody{Fixes: []Fix{fix("F-0001", h.commit(true))}})
		h.expect(StepVerify)
	})

	t.Run("withdrawn", func(t *testing.T) {
		h, ap := start(t)
		eqTypes(t, h.accept(ap, "aud-1", AuditorBody{Resolutions: resolutions(ap, "withdrawn"),
			Adjudications: []Adjudication{{Finding: "F-0001", Decision: DecisionWithdraw, Rationale: "not in the envelope"}}}),
			EventReportAccepted, EventFindingWithdrawn, EventEscalationResolved)
		if f, _ := lookup(h.state, "F-0001"); len(f.History) != 4 || f.History[2] != (FindingEvent{Event: EventFindingWithdrawn, Round: 1, Packet: ap.ID, Candidate: ap.Candidate.Digest, Note: "not in the envelope"}) {
			t.Fatalf("history %+v", f.History)
		}
		// The same candidate may be reviewed again: the finding its RED
		// verdict rested on no longer stands.
		h.expect(StepReview)
		rp := h.packet()
		if rp.Round != 2 {
			t.Fatalf("round %d", rp.Round)
		}
		h.accept(rp, "rev-1", green(h.contract))
		h.expect(StepNone)
	})
}

func TestScenarioIndeterminate(t *testing.T) {
	h := newHarness(t)
	h.checks(true)
	d := h.accept(h.packet(), "rev-1", review(h.contract, VerdictIndeterminate, map[string]string{"REQ-2": RequirementUnverified}))
	eqTypes(t, d, EventReportAccepted, EventVerdictRecorded, EventEscalationRaised)
	e := d[2].Data.(EscalationRaised)
	if e.Kind != EscalationEvidenceGap || e.Owner != RoleGovernance || !slices.Equal(e.Requirements, []string{"REQ-2"}) {
		t.Fatalf("escalation %+v", e)
	}
	if a := h.expect(StepAdjudicate); !slices.Equal(a.Escalations, []string{"E-0001"}) {
		t.Fatalf("action %+v", a)
	}
	var r *Rejection
	if _, err := PlanPacket(h.state, h.facts()); !errors.As(err, &r) || r.Layer != LayerTransition {
		t.Fatalf("plan during adjudication: %v", err)
	}
	if _, err := Resolve(h.state, h.facts(), "E-0001", " ", "gov"); !errors.As(err, &r) || r.Layer != LayerSemantic {
		t.Fatalf("blank decision: %v", err)
	}
	h.must(Resolve(h.state, h.facts(), "E-0001", "REQ-2 confirmed by the product owner", "gov"))
	// Re-review of the same candidate is permitted after a resolution.
	h.expect(StepReview)
	rp := h.packet()
	if len(rp.Decisions) != 1 || rp.Decisions[0] != (Decision{Source: DecisionResolution, Escalation: "E-0001", By: RoleGovernance, Text: "REQ-2 confirmed by the product owner"}) {
		t.Fatalf("decisions %+v", rp.Decisions)
	}
	h.accept(rp, "rev-1", green(h.contract))
	h.expect(StepNone)
}

func TestScenarioUnchangedCandidateResubmission(t *testing.T) {
	h := newHarness(t)
	h.checks(true)
	reviewed := h.tree
	h.accept(h.packet(), "rev-1", red(h.contract, "crash"))
	h.accept(h.packet(), "impl-1", ImplementerBody{Fixes: []Fix{fix("F-0001", h.commit(true))}})
	h.accept(h.packet(), "ver-1", verify(map[string]string{"F-0001": VerificationConfirmed}, "F-0001"))
	h.checkout(reviewed)
	a := h.expect(StepCommitChanges)
	if !slices.Equal(a.Findings, []string{"F-0001"}) || !strings.Contains(a.Reason, "neither approved nor reviewed again") {
		t.Fatalf("action %+v", a)
	}
	if _, err := PlanPacket(h.state, h.facts()); err == nil {
		t.Fatal("a review packet was planned for an unchanged candidate")
	}
	// Re-running the checks is not a change.
	h.checks(true)
	h.expect(StepCommitChanges)
}

func TestScenarioFixCommittedBeforePacket(t *testing.T) {
	h := newHarness(t)
	h.checks(true)
	h.accept(h.packet(), "rev-1", red(h.contract, "crash"))
	early := h.commit(true) // the implementer fixed it before asking for the packet
	fp := h.packet()
	if fp.Candidate.Digest != h.candidate().Digest {
		t.Fatal("the fix packet is not bound to the current candidate")
	}
	h.accept(fp, "impl-1", ImplementerBody{Fixes: []Fix{fix("F-0001", early)}})
	h.accept(h.packet(), "ver-1", verify(map[string]string{"F-0001": VerificationRejected}, "F-0001"))
	// Claiming the rejected fix again without a new change is refused.
	fp = h.packet()
	if ps := h.refused(fp, "impl-1", ImplementerBody{Fixes: []Fix{fix("F-0001", early)}}, LayerTransition); !hasProblem(ps, "/body/fixes/0") {
		t.Fatalf("problems %+v", ps)
	}
	h.accept(fp, "impl-1", ImplementerBody{Fixes: []Fix{fix("F-0001", h.commit(true))}})
	h.expect(StepVerify)
}

func TestScenarioNonConvergenceBeyondThreshold(t *testing.T) {
	h := newHarness(t)
	lenient := testContract()
	lenient.Convergence = &Convergence{RootCauseAfter: 2, ConvergenceAuditAfter: 4, ProcessAuditAfter: 5}
	h.contract = lenient
	h.must(Pin(h.state, h.facts(), "lenient", "gov"))
	h.checks(true)
	round := func(v Verdict) []EscalationKind {
		statuses := map[string]string{}
		if v == VerdictIndeterminate {
			statuses["REQ-2"] = RequirementUnverified
		}
		d := h.accept(h.packet(), "rev-1", review(h.contract, v, statuses))
		h.drain()
		var kinds []EscalationKind
		for _, k := range raisedKinds(d) {
			if k != EscalationEvidenceGap {
				kinds = append(kinds, k)
			}
		}
		return kinds
	}
	for range 3 {
		if k := round(VerdictIndeterminate); len(k) != 0 {
			t.Fatalf("raised %v below the threshold", k)
		}
	}
	strict := testContract()
	strict.Convergence = &Convergence{RootCauseAfter: 2, ConvergenceAuditAfter: 2, ProcessAuditAfter: 5}
	h.contract = strict
	h.must(Pin(h.state, h.facts(), "strict", "gov"))
	for i, want := range [][]EscalationKind{
		{EscalationNonConvergence}, // streak 4, already beyond the new threshold
		{EscalationProcessAudit},   // streak 5: not raised twice in one streak
	} {
		if k := round(VerdictIndeterminate); !slices.Equal(k, want) {
			t.Fatalf("round %d raised %v, want %v", i+4, k, want)
		}
	}
	round(VerdictGreen)
	h.commit(true)
	h.checks(true)
	if k := round(VerdictIndeterminate); len(k) != 0 {
		t.Fatalf("a new streak of 1 raised %v", k)
	}
	if k := round(VerdictIndeterminate); !slices.Equal(k, []EscalationKind{EscalationNonConvergence}) {
		t.Fatalf("the second streak raised %v", k)
	}
}

func TestScenarioNonConvergenceAndProcessAudit(t *testing.T) {
	h := newHarness(t)
	h.checks(true)
	for round := 1; round <= 6; round++ {
		rp := h.packet()
		if rp.Round != round {
			t.Fatalf("round %d, want %d", rp.Round, round)
		}
		d := h.accept(rp, "rev-1", review(h.contract, VerdictIndeterminate, map[string]string{"REQ-2": RequirementUnverified}))
		want := []EscalationKind{EscalationEvidenceGap}
		switch round {
		case 3:
			want = []EscalationKind{EscalationNonConvergence, EscalationEvidenceGap}
		case 5:
			want = []EscalationKind{EscalationProcessAudit, EscalationEvidenceGap}
		}
		if got := raisedKinds(d); !slices.Equal(got, want) {
			t.Fatalf("round %d raised %v, want %v", round, got, want)
		}
		if h.state.FailedStreak() != round {
			t.Fatalf("streak %d after round %d", h.state.FailedStreak(), round)
		}
		h.drain()
	}
	h.accept(h.packet(), "rev-1", green(h.contract))
	if h.state.FailedStreak() != 0 {
		t.Fatalf("streak %d after GREEN", h.state.FailedStreak())
	}
	h.expect(StepNone)
}

func TestScenarioComponentChurn(t *testing.T) {
	h := newHarness(t)
	h.checks(true)
	for round := 1; round <= 4; round++ {
		h.accept(h.packet(), "rev-1", red(h.contract, "crash"))
		h.drain()
		id := fmt.Sprintf("F-%04d", round)
		d := h.accept(h.packet(), "impl-1", ImplementerBody{Fixes: []Fix{fix(id, h.commit(true), "parser", "lexer")}})
		var want []EscalationKind
		if round == 3 {
			want = []EscalationKind{EscalationComponentChurn, EscalationComponentChurn}
			lexer, parser := d[2].Data.(EscalationRaised), d[3].Data.(EscalationRaised)
			if !slices.Equal(lexer.Components, []string{"lexer"}) || !slices.Equal(parser.Components, []string{"parser"}) ||
				!slices.Equal(parser.Findings, []string{id}) {
				t.Fatalf("churn escalations %+v %+v", lexer, parser)
			}
		}
		if got := raisedKinds(d); !slices.Equal(got, want) {
			t.Fatalf("round %d raised %v, want %v", round, got, want)
		}
		h.drain()
		h.accept(h.packet(), "ver-1", verify(map[string]string{id: VerificationConfirmed}, id))
		h.checks(true)
	}
}

func TestScenarioRegressionSpiral(t *testing.T) {
	for _, tc := range []struct {
		name     string
		findings int
		spiral   bool
	}{
		{"one confirmed, one blocking regression", 1, true},
		{"two confirmed, one blocking regression", 2, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := newHarness(t)
			h.checks(true)
			var findings []FindingInput
			for i := range tc.findings {
				findings = append(findings, blocker(fmt.Sprint("k", i), "REQ-1"))
			}
			h.accept(h.packet(), "rev-1", review(h.contract, VerdictRed, map[string]string{"REQ-1": RequirementViolated}, findings...))
			fp := h.packet()
			c := h.commit(true)
			var fixes []Fix
			results := map[string]string{}
			var ids []string
			for i := range tc.findings {
				id := fmt.Sprintf("F-%04d", i+1)
				fixes = append(fixes, fix(id, c))
				results[id] = VerificationConfirmed
				ids = append(ids, id)
			}
			h.accept(fp, "impl-1", ImplementerBody{Fixes: fixes})
			b := verify(results, ids...)
			reg := blocker("leak", "REQ-1")
			reg.IntroducedBy = "F-0001"
			b.Regressions = []FindingInput{reg}
			d := h.accept(h.packet(), "ver-1", b)
			var want []EscalationKind
			if tc.spiral {
				want = []EscalationKind{EscalationRegressionSpiral}
				if r := d[len(d)-1].Data.(EscalationRaised).Reason; r != "verification P-0003 found 1 blocking regression and confirmed 1 fix" {
					t.Fatalf("reason %q", r)
				}
			}
			if got := raisedKinds(d); !slices.Equal(got, want) {
				t.Fatalf("raised %v, want %v", got, want)
			}
			regID := fmt.Sprintf("F-%04d", tc.findings+1)
			if f, _ := lookup(h.state, regID); f.Source != RoleVerifier || f.Status != StatusOpen {
				t.Fatalf("regression %+v", f)
			}
		})
	}
}

func TestScenarioCandidateChangeAfterGreen(t *testing.T) {
	h := newHarness(t)
	h.checks(true)
	h.accept(h.packet(), "rev-1", green(h.contract))
	h.commit(false) // a ledger-only commit leaves the candidate unchanged
	h.expect(StepNone)
	h.clean = false
	if h.gate().Open {
		t.Fatal("the gate is open on a dirty tree")
	}
	h.expect(StepCommitChanges)
	h.commit(true)
	g := h.gate()
	if g.Open || g.Conditions[3].Satisfied || g.Conditions[4].Satisfied || g.Conditions[2].Satisfied {
		t.Fatalf("gate after a candidate change %+v", g)
	}
	h.expect(StepRunChecks)
	h.checks(true)
	h.expect(StepReview)
}

func TestScenarioRoleSeparation(t *testing.T) {
	h := newHarness(t)
	h.checks(true)
	h.accept(h.packet(), "rev-1", red(h.contract, "crash"))
	fp := h.packet()
	c := h.commit(true)
	body := ImplementerBody{Fixes: []Fix{fix("F-0001", c)}}
	if ps := h.refused(fp, "rev-1", body, LayerTransition); !hasProblem(ps, "/author/id") {
		t.Fatalf("problems %+v", ps)
	}
	h.accept(fp, "impl-1", body)
	vp := h.packet()
	confirmed := verify(map[string]string{"F-0001": VerificationConfirmed}, "F-0001")
	h.refused(vp, "impl-1", confirmed, LayerTransition)
	h.accept(vp, "ver-1", confirmed)
	h.checks(true)
	rp := h.packet()
	h.refused(rp, "impl-1", red(h.contract, "again"), LayerTransition)
	h.accept(rp, "rev-1", red(h.contract, "again"))
	h.accept(h.packet(), "impl-1", ImplementerBody{Disputes: []Dispute{{Finding: "F-0002", Rationale: "out of scope"}}})
	ap := h.packet()
	audit := AuditorBody{Resolutions: resolutions(ap, "c"), Adjudications: []Adjudication{{Finding: "F-0002", Decision: DecisionUphold, Rationale: "r"}}}
	for _, author := range []string{"rev-1", "ver-1", "impl-1"} {
		h.refused(ap, author, audit, LayerTransition)
	}
	h.accept(ap, "aud-1", audit)
	h.accept(h.packet(), "impl-1", ImplementerBody{Fixes: []Fix{fix("F-0002", h.commit(true))}})
	vp = h.packet()
	h.refused(vp, "aud-1", verify(map[string]string{"F-0002": VerificationConfirmed}, "F-0002"), LayerTransition)
}

func TestScenarioReplayAndIdempotentIngest(t *testing.T) {
	h := newHarness(t)
	h.checks(true)
	h.accept(h.packet(), "rev-1", red(h.contract, "crash"))
	h.accept(h.packet(), "impl-1", ImplementerBody{Fixes: []Fix{fix("F-0001", h.commit(true))}})
	h.accept(h.packet(), "ver-1", verify(map[string]string{"F-0001": VerificationConfirmed}, "F-0001"))
	h.checks(true)
	h.accept(h.packet(), "rev-1", green(h.contract))

	s, err := Replay(h.events)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(s, h.state) {
		t.Fatal("replay differs from the folded state")
	}
	for id, r := range h.accepted {
		drafts, dup, err := h.ingest(r)
		if err != nil || !dup || drafts != nil {
			t.Fatalf("re-ingesting the report for %s: %d drafts, duplicate %t, %v", id, len(drafts), dup, err)
		}
	}
	other := h.accepted["P-0001"]
	other.Author.Session = "another run"
	drafts, dup, err := h.ingest(other)
	var r *Rejection
	if !errors.As(err, &r) || r.Layer != LayerTransition || dup || drafts != nil || !hasProblem(r.Problems, "/packet") {
		t.Fatalf("a different report for a consumed packet: %v", err)
	}
}
