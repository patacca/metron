package core

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"
)

// These tests hold every recurring path of the loop to one rule: it
// progresses, it reaches governance within a bound the thresholds set, or it
// ends without approval. No number of steps ever opens the gate.

// audits discharges the auditor's open escalations, upholding every finding,
// and returns the kinds the audit report raised.
func (h *harness) audits() []EscalationKind {
	h.tb.Helper()
	var raised []EscalationKind
	for h.next().Step == StepAudit {
		p := h.packet()
		var adj []Adjudication
		for _, id := range p.Adjudicable {
			adj = append(adj, Adjudication{Finding: id, Decision: DecisionUphold, Rationale: "it stands"})
		}
		raised = append(raised, raisedKinds(h.accept(p, "aud-1", AuditorBody{Resolutions: resolutions(p, "carry on"), Adjudications: adj}))...)
	}
	return raised
}

// suspended asserts that the campaign is suspended: the gate is closed, the
// next action is governance's, and neither a packet nor a check recording is
// accepted. It returns the suspension.
func (h *harness) suspended() Escalation {
	h.tb.Helper()
	e, ok := h.state.Suspension()
	if !ok {
		h.tb.Fatalf("the campaign is not suspended; next is %+v", h.next())
	}
	a := h.expect(StepAdjudicate)
	if a.Owner != RoleGovernance || !slices.Contains(a.Escalations, e.ID) || !strings.Contains(a.Reason, "suspended") {
		h.tb.Fatalf("action %+v", a)
	}
	if h.gate().Open {
		h.tb.Fatal("the gate is open while the campaign is suspended")
	}
	var r *Rejection
	if _, err := PlanPacket(h.state, h.facts()); !errors.As(err, &r) {
		h.tb.Fatalf("a packet was planned while the campaign is suspended: %v", err)
	}
	f := h.facts()
	if _, err := RecordEvidence(h.state, f, []Evidence{{Requirement: "REQ-1", Command: "go test ./...", Candidate: f.Candidate.Digest,
		Passed: true, OutputDigest: digestOf("out")}}); !errors.As(err, &r) || !strings.Contains(err.Error(), "suspended") {
		h.tb.Fatalf("a check was recorded while the campaign is suspended: %v", err)
	}
	return e
}

// resume resolves the suspension as governance and checks that the campaign
// starts over with nothing counted against it.
func (h *harness) resume() {
	h.tb.Helper()
	e := h.suspended()
	h.must(Resolve(h.state, h.facts(), e.ID, "resume: the cause is understood", "gov"))
	if h.state.FailedStreak() != 0 || h.state.CampaignSteps() != 0 {
		h.tb.Fatalf("streak %d and steps %d after the resumption", h.state.FailedStreak(), h.state.CampaignSteps())
	}
	if _, still := h.state.Suspension(); still {
		h.tb.Fatal("the campaign is still suspended")
	}
}

// loose is a contract whose failure thresholds are out of reach, so that only
// the campaign budget can stop a loop.
func loose(budget int) Contract {
	c := testContract()
	c.Convergence = &Convergence{RootCauseAfter: 1000, ConvergenceAuditAfter: 1000, ProcessAuditAfter: 1001, SuspendAfter: budget}
	return c
}

// R2. Fixes rejected again and again inside one round used to raise nothing.
func TestRejectedFixesReachGovernance(t *testing.T) {
	h := newHarness(t)
	h.checks(true)
	h.accept(h.packet(), "rev-1", red(h.contract, "crash"))
	var raised []EscalationKind
	rejections := 0
	for {
		if _, ok := h.state.Suspension(); ok {
			break
		}
		if rejections > 10 {
			t.Fatalf("%d rejected fixes and no suspension; raised %v", rejections, raised)
		}
		fx := fix("F-0001", h.commit(true))
		fx.RootCause = "analysed"
		h.accept(h.packet(), "impl-1", ImplementerBody{Fixes: []Fix{fx}})
		h.checks(true)
		raised = append(raised, raisedKinds(h.accept(h.verifying(), "ver-1", verify(map[string]string{"F-0001": VerificationRejected}, "F-0001")))...)
		rejections++
		raised = append(raised, h.audits()...)
	}
	// One RED round and four rejections are five failed steps.
	if want := []EscalationKind{EscalationNonConvergence, EscalationProcessAudit, EscalationSuspension}; rejections != 4 || !slices.Equal(raised, want) {
		t.Fatalf("%d rejections raised %v, want 4 raising %v", rejections, raised, want)
	}
	if len(h.state.rounds) != 1 {
		t.Fatalf("%d rounds", len(h.state.rounds))
	}
	h.resume()
	h.expect(StepFix)
}

// R1. A reviewer and an auditor that disagree about a tier, round after round
// on one unchanged candidate, reach governance instead of looping.
func TestTierDisagreementReachesGovernance(t *testing.T) {
	h := newHarness(t)
	h.checks(true)
	start := h.candidate()
	rounds := 0
	for {
		if _, ok := h.state.Suspension(); ok {
			break
		}
		if rounds > 10 {
			t.Fatalf("%d rounds and no suspension", rounds)
		}
		h.accept(h.packet(), "rev-1", red(h.contract, fmt.Sprint("k", rounds)))
		rounds++
		h.audits()
		if _, ok := h.state.Suspension(); ok {
			break
		}
		id := seqID("F", rounds)
		h.accept(h.packet(), "impl-1", ImplementerBody{Disputes: []Dispute{{Finding: id, Rationale: "an improvement, not a blocker"}}})
		ap := h.packet()
		var res []EscalationResolution
		for _, e := range ap.Escalations {
			res = append(res, EscalationResolution{Escalation: e.ID, Analysis: "a", Conclusion: "reclassified"})
		}
		h.accept(ap, "aud-1", AuditorBody{Resolutions: res,
			Adjudications: []Adjudication{{Finding: id, Decision: DecisionReclassify, Tier: TierImprovement, Rationale: "no requirement is violated"}}})
	}
	if rounds != 5 || h.candidate() != start {
		t.Fatalf("suspended after %d rounds on candidate %v, want 5 on the first one", rounds, h.candidate())
	}
	h.suspended()
}

// R1. A mandatory check that can never pass used to leave run-checks as the
// next action for ever, with no packet and no escalation.
func TestFailingChecksReachGovernance(t *testing.T) {
	h := newHarness(t)
	var raised [][]EscalationKind
	for i := 0; i < 5; i++ {
		h.expect(StepRunChecks)
		f := h.facts()
		d := h.must(RecordEvidence(h.state, f, []Evidence{{Requirement: "REQ-1", Command: "go test ./...", Candidate: f.Candidate.Digest,
			ExitCode: 1, OutputDigest: digestOf("out")}}))
		raised = append(raised, raisedKinds(d))
		if i == 2 {
			// No round has a verdict yet: the audit names round 0.
			h.expect(StepAudit)
			if p, _, err := h.planned(); err != nil || p.Round != 0 || len(p.Escalations) != 1 || p.Escalations[0].Kind != EscalationNonConvergence {
				t.Fatalf("audit before any round: %+v, %v", p, err)
			}
		}
		if k := h.audits(); (i == 4) != slices.Equal(k, []EscalationKind{EscalationSuspension}) {
			t.Fatalf("after failure %d the audits raised %v", i+1, k)
		}
		h.commit(true)
	}
	want := [][]EscalationKind{nil, nil, {EscalationNonConvergence}, nil, {EscalationProcessAudit}}
	for i := range want {
		if !slices.Equal(raised[i], want[i]) {
			t.Fatalf("failure %d raised %v, want %v", i+1, raised[i], want[i])
		}
	}
	if len(h.state.rounds) != 0 {
		t.Fatalf("%d rounds", len(h.state.rounds))
	}
	h.resume()
	h.expect(StepRunChecks)
}

// Steps that succeed in between do not end the streak: only a GREEN verdict
// or governance does.
func TestSuccessfulStepsDoNotEndTheStreak(t *testing.T) {
	h := newHarness(t)
	h.checks(true)
	for round := 1; round <= 5; round++ {
		if _, ok := h.state.Suspension(); ok {
			t.Fatalf("suspended before round %d", round)
		}
		h.accept(h.packet(), "rev-1", red(h.contract, fmt.Sprint("k", round)))
		if h.state.FailedStreak() != round {
			t.Fatalf("streak %d after round %d", h.state.FailedStreak(), round)
		}
		if k := h.audits(); (round == 5) != slices.Contains(k, EscalationSuspension) {
			t.Fatalf("round %d: audits raised %v", round, k)
		}
		if round == 5 {
			break
		}
		// A confirmed fix and passing checks are progress on a finding, not
		// on the campaign.
		id := seqID("F", round)
		h.accept(h.packet(), "impl-1", ImplementerBody{Fixes: []Fix{fix(id, h.commit(true), fmt.Sprint("component-", round))}})
		h.checks(true)
		h.accept(h.verifying(), "ver-1", verify(map[string]string{id: VerificationConfirmed}, id))
		if h.state.FailedStreak() != round {
			t.Fatalf("a confirmed fix changed the streak to %d", h.state.FailedStreak())
		}
	}
	h.suspended()
}

// Pins, stage changes and candidate changes do not end the streak.
func TestStreakSurvivesContractAndCandidateChanges(t *testing.T) {
	h := newHarness(t)
	h.checks(true)
	indeterminate := func() []EscalationKind {
		t.Helper()
		d := h.accept(h.packet(), "rev-1", review(h.contract, VerdictIndeterminate, map[string]string{"REQ-2": RequirementUnverified}))
		h.must(Resolve(h.state, h.facts(), h.expect(StepAdjudicate).Escalations[0], "evidence supplied", "gov"))
		return raisedKinds(d)
	}
	indeterminate()
	h.commit(true) // a new candidate
	h.checks(true)
	indeterminate()
	h.contract = stageTwoContract() // a new stage
	h.must(Pin(h.state, h.facts(), "stage two", "gov"))
	h.contract = testContract() // and back, under the first contract again
	h.must(Pin(h.state, h.facts(), "back", "gov"))
	h.checks(true)
	if h.state.FailedStreak() != 2 {
		t.Fatalf("streak %d after two pins, want 2", h.state.FailedStreak())
	}
	if k := indeterminate(); !slices.Contains(k, EscalationNonConvergence) {
		t.Fatalf("the third failed step raised %v", k)
	}
}

// The campaign budget counts every packet, so a loop whose steps keep
// succeeding still reaches governance.
func TestCampaignBudget(t *testing.T) {
	h := newHarness(t)
	h.contract = loose(6)
	h.must(Pin(h.state, h.facts(), "a budget of six packets", "gov"))
	h.checks(true)
	packets := 0
	issue := func() Packet {
		t.Helper()
		before := len(h.state.escalations)
		p := h.packet()
		packets++
		if raisedNow := len(h.state.escalations) > before; raisedNow != (packets == 6) {
			t.Fatalf("packet %d: suspension raised %t", packets, raisedNow)
		}
		return p
	}
	for round := 1; round <= 2; round++ {
		h.accept(issue(), "rev-1", red(h.contract, fmt.Sprint("k", round)))
		id := seqID("F", round)
		h.accept(issue(), "impl-1", ImplementerBody{Fixes: []Fix{fix(id, h.commit(true), fmt.Sprint("component-", round))}})
		h.checks(true)
		vp := issue()
		if round == 2 {
			// The sixth packet uses up the budget. Its role still answers it.
			if a := h.expect(StepSubmitReport); a.Packet != vp.ID {
				t.Fatalf("action %+v", a)
			}
		}
		h.accept(vp, "ver-1", verify(map[string]string{id: VerificationConfirmed}, id))
	}
	if e := h.suspended(); e.Round != 2 || !strings.Contains(e.Reason, "P-0006") {
		t.Fatalf("suspension %+v", e)
	}
	h.resume()
	h.checks(true)
	h.accept(h.packet(), "rev-1", green(h.contract))
	h.expect(StepNone)
	if h.state.CampaignSteps() != 1 {
		t.Fatalf("steps %d after the resumption and one review", h.state.CampaignSteps())
	}
	// An approval ends the campaign: the next one starts with a full budget.
	h.commit(true)
	h.checks(true)
	h.accept(h.packet(), "rev-1", green(h.contract))
	if h.state.CampaignSteps() != 1 {
		t.Fatalf("steps %d in the campaign after an approval", h.state.CampaignSteps())
	}
	h.expect(StepNone)
}

// Packets that are voided before anyone answers them use up the budget too.
func TestVoidedPacketsUseTheBudget(t *testing.T) {
	h := newHarness(t)
	h.contract = loose(4)
	h.must(Pin(h.state, h.facts(), "a budget of four packets", "gov"))
	for i := 1; i <= 4; i++ {
		h.checks(true)
		if _, ok := h.state.Suspension(); ok {
			t.Fatalf("suspended before packet %d", i)
		}
		h.packet()
		h.commit(true) // the candidate moves under the reviewer: the packet is void
	}
	h.suspended()
	if len(h.state.rounds) != 4 || h.state.FailedStreak() != 0 {
		t.Fatalf("%d rounds, streak %d", len(h.state.rounds), h.state.FailedStreak())
	}
}

// A GREEN verdict does not approve a suspended campaign: governance decides
// first.
func TestGreenUnderSuspensionWaitsForGovernance(t *testing.T) {
	h := newHarness(t)
	h.contract = loose(1)
	h.must(Pin(h.state, h.facts(), "a budget of one packet", "gov"))
	h.checks(true)
	h.accept(h.packet(), "rev-1", green(h.contract))
	e := h.suspended()
	h.must(Resolve(h.state, h.facts(), e.ID, "resume", "gov"))
	h.expect(StepNone)
}

// An auditor can ask for the campaign to be suspended.
func TestAuditorRequestsSuspension(t *testing.T) {
	h := newHarness(t)
	h.checks(true)
	h.accept(h.packet(), "rev-1", red(h.contract, "crash"))
	h.accept(h.packet(), "impl-1", ImplementerBody{Disputes: []Dispute{{Finding: "F-0001", Rationale: "no"}}})
	ap := h.packet()
	body := AuditorBody{Resolutions: resolutions(ap, "upheld"),
		Adjudications: []Adjudication{{Finding: "F-0001", Decision: DecisionUphold, Rationale: "it stands"}}, Suspend: " "}
	if ps := h.refused(ap, "aud-1", body, LayerSemantic); !hasProblem(ps, "/body/suspend") {
		t.Fatalf("problems %+v", ps)
	}
	body.Suspend = "the parser has to be redesigned before more patches"
	d := h.accept(ap, "aud-1", body)
	if k := raisedKinds(d); !slices.Equal(k, []EscalationKind{EscalationSuspension}) {
		t.Fatalf("raised %v", k)
	}
	if e := h.suspended(); !strings.Contains(e.Reason, "redesigned") {
		t.Fatalf("suspension %+v", e)
	}
	h.resume()
	h.expect(StepFix)
}

// However long roles fail, the gate stays closed, and a governance-owned
// action arrives within the bound the thresholds set.
func TestNoApprovalThroughExhaustion(t *testing.T) {
	for _, budget := range []int{3, 7, 20, 60} {
		t.Run(fmt.Sprint("budget ", budget), func(t *testing.T) {
			h := newHarness(t)
			c := testContract()
			c.Convergence = &Convergence{RootCauseAfter: 2, ConvergenceAuditAfter: 3, ProcessAuditAfter: 5, SuspendAfter: budget}
			h.contract = c
			h.must(Pin(h.state, h.facts(), "budget", "gov"))
			for campaign := 1; campaign <= 3; campaign++ {
				steps := 0
				for h.next().Owner != RoleGovernance {
					if steps++; steps > 4*budget+40 {
						t.Fatalf("campaign %d: %d steps without a governance-owned action; next is %+v", campaign, steps, h.next())
					}
					if h.gate().Open {
						t.Fatalf("campaign %d: the gate opened after %d steps of failing roles", campaign, steps)
					}
					h.failing()
				}
				if h.state.CampaignSteps() > budget {
					t.Fatalf("campaign %d issued %d packets with a budget of %d", campaign, h.state.CampaignSteps(), budget)
				}
				for h.next().Owner == RoleGovernance {
					h.must(Resolve(h.state, h.facts(), h.next().Escalations[0], "resume", "gov"))
				}
			}
		})
	}
}

// failing performs the next action the way roles that never converge do:
// every review is RED, every fix is rejected, every audit carries on.
func (h *harness) failing() {
	h.tb.Helper()
	switch a := h.next(); a.Step {
	case StepRunChecks:
		h.checks(true)
	case StepCommitChanges:
		h.commit(true)
	case StepReview:
		h.accept(h.packet(), "rev-1", red(h.contract, "k"))
	case StepFix:
		p := h.packet()
		commit := h.commit(true)
		var b ImplementerBody
		for _, id := range p.Subjects {
			fx := fix(id, commit)
			fx.RootCause = "analysed"
			b.Fixes = append(b.Fixes, fx)
		}
		h.accept(p, "impl-1", b)
	case StepVerify:
		p := h.packet()
		results := map[string]string{}
		for _, id := range p.Subjects {
			results[id] = VerificationRejected
		}
		h.accept(p, "ver-1", verify(results, p.Subjects...))
	case StepAudit:
		p := h.packet()
		var adj []Adjudication
		for _, id := range p.Adjudicable {
			adj = append(adj, Adjudication{Finding: id, Decision: DecisionUphold, Rationale: "it stands"})
		}
		h.accept(p, "aud-1", AuditorBody{Resolutions: resolutions(p, "carry on"), Adjudications: adj})
	default:
		h.tb.Fatalf("failing roles cannot perform %+v", a)
	}
}
