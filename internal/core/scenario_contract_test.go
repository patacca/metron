package core

import (
	"slices"
	"testing"
)

// Scenarios about pins: re-pinning, stage changes, and what carries over.

func TestScenarioRepin(t *testing.T) {
	h := newHarness(t)
	h.checks(true)
	h.accept(h.packet(), "rev-1", green(h.contract))
	h.expect(StepNone)

	amended := testContract()
	amended.Authorizes = "a pilot with external users"
	h.contract = amended
	h.expect(StepPinContract)
	if h.gate().Open {
		t.Fatal("the gate is open while the contract differs from the pinned one")
	}
	eqTypes(t, h.must(Pin(h.state, h.facts(), "widen the authorization", "gov")), EventContractPinned)
	g := h.gate()
	if g.Open || g.Conditions[4].Satisfied {
		t.Fatalf("a GREEN verdict under the old contract still counts: %+v", g)
	}
	// The check command is unchanged, so its evidence still counts.
	h.expect(StepReview)
	stale := h.packet()

	again := amended
	again.Authorizes = "a pilot with external users, read-only"
	h.contract = again
	eqTypes(t, h.must(Pin(h.state, h.facts(), "narrow it", "gov")), EventPacketVoided, EventContractPinned)
	if rs := h.state.StageRounds(); len(rs) != 2 || !rs[1].Voided {
		t.Fatalf("rounds %+v", rs)
	}
	h.refused(stale, "rev-1", green(h.contract), LayerTransition)
	if _, err := Pin(h.state, h.facts(), "again", "gov"); err == nil {
		t.Fatal("the same contract digest was pinned twice")
	}
	rp := h.packet()
	if rp.Round != 3 {
		t.Fatalf("round %d", rp.Round)
	}
	h.accept(rp, "rev-1", green(h.contract))
	h.expect(StepNone)
	if h.state.FailedStreak() != 0 {
		t.Fatalf("a voided round counted as a failure")
	}
}

func TestScenarioStageChange(t *testing.T) {
	h := newHarness(t)
	h.checks(true)
	h.accept(h.packet(), "rev-1", red(h.contract, "crash"))
	h.accept(h.packet(), "impl-1", ImplementerBody{Fixes: []Fix{fix("F-0001", h.commit(true))}})
	h.accept(h.verifying(), "ver-1", verify(map[string]string{"F-0001": VerificationConfirmed}, "F-0001"))
	h.checks(true)
	reviewed := h.candidate()
	h.accept(h.packet(), "rev-1", red(h.contract, "crash again"))
	if h.state.FailedStreak() != 2 || len(h.state.StageRounds()) != 2 {
		t.Fatalf("streak %d, rounds %d", h.state.FailedStreak(), len(h.state.StageRounds()))
	}

	h.contract = stageTwoContract()
	h.must(Pin(h.state, h.facts(), "enter controlled validation", "gov"))
	// The gate looks only at the rounds of the new stage, but the stage
	// change is not a new campaign: the failed steps stay counted.
	if h.state.Stage() != StageControlledValidation || len(h.state.StageRounds()) != 0 || h.state.FailedStreak() != 2 {
		t.Fatalf("stage %s, rounds %d, streak %d", h.state.Stage(), len(h.state.StageRounds()), h.state.FailedStreak())
	}
	// Findings carry over.
	h.expect(StepFix)
	fp := h.packet()
	if fp.Round != 2 || *fp.Base != reviewed || !slices.Equal(fp.Subjects, []string{"F-0002"}) {
		t.Fatalf("fix packet after the stage change %+v", fp)
	}
	h.accept(fp, "impl-1", ImplementerBody{Fixes: []Fix{fix("F-0002", h.commit(true))}})
	h.accept(h.verifying(), "ver-1", verify(map[string]string{"F-0002": VerificationConfirmed}, "F-0002"))
	h.checks(true)
	rp := h.packet()
	if rp.Round != 3 || !slices.Contains(rp.ApplicableRequirements, "REQ-4") || len(rp.History) != 0 {
		t.Fatalf("first review of the new stage %+v", rp)
	}
	h.accept(rp, "rev-1", green(h.contract))
	if g := h.gate(); !g.Open || g.Stage != StageControlledValidation {
		t.Fatalf("gate %+v", g)
	}
	if n := len(h.state.rounds); n != 3 {
		t.Fatalf("%d rounds across stages, want 3", n)
	}
}

func TestScenarioHistoryRoundsAcrossStages(t *testing.T) {
	h := newHarness(t)
	h.checks(true)
	h.accept(h.packet(), "rev-1", red(h.contract, "crash"))
	h.accept(h.packet(), "impl-1", ImplementerBody{Fixes: []Fix{fix("F-0001", h.commit(true))}})
	h.accept(h.verifying(), "ver-1", verify(map[string]string{"F-0001": VerificationConfirmed}, "F-0001"))
	h.checks(true)
	h.accept(h.packet(), "rev-1", red(h.contract, "crash again"))

	h.contract = stageTwoContract()
	h.must(Pin(h.state, h.facts(), "enter controlled validation", "gov"))
	before := map[string]int{}
	for _, f := range h.state.Findings() {
		before[f.ID] = len(f.History)
	}
	h.accept(h.packet(), "impl-1", ImplementerBody{Fixes: []Fix{fix("F-0002", h.commit(true))}})
	h.accept(h.verifying(), "ver-1", verify(map[string]string{"F-0002": VerificationConfirmed}, "F-0002"))
	h.checks(true)
	b := review(h.contract, VerdictRed, map[string]string{"REQ-1": RequirementViolated, "REQ-4": RequirementViolated}, blocker("loss", "REQ-4"))
	b.Reopen = []Reopen{{Finding: "F-0001", Reason: ReopenRegression, Justification: "it is back"}}
	rp := h.packet()
	if rp.Round != 3 {
		t.Fatalf("the first review of the second stage is round %d, want 3", rp.Round)
	}
	h.accept(rp, "rev-1", b)

	// Steps between rounds happen in round 2, the latest; the review steps in
	// round 3.
	want := map[string][]int{"F-0001": {3}, "F-0002": {2, 2}, "F-0003": {3}}
	for _, f := range h.state.Findings() {
		var got []int
		for _, e := range f.History[before[f.ID]:] {
			got = append(got, e.Round)
			if p, _ := h.state.Packet(e.Packet); p.Action == StepReview && e.Round != p.Round {
				t.Fatalf("%s %s is recorded in round %d, its review packet in round %d", f.ID, e.Event, e.Round, p.Round)
			}
		}
		if !slices.Equal(got, want[f.ID]) {
			t.Fatalf("%s rounds in the second stage %v, want %v", f.ID, got, want[f.ID])
		}
	}
}

// A verdict counts whenever the pinned digest is the one it was issued under.
func TestScenarioPinBackToAnEarlierContract(t *testing.T) {
	other := testContract()
	other.Authorizes = "a pilot with external users"
	t.Run("no round under the other contract", func(t *testing.T) {
		h := newHarness(t)
		h.checks(true)
		h.accept(h.packet(), "rev-1", green(h.contract))
		h.contract = other
		h.must(Pin(h.state, h.facts(), "widen", "gov"))
		if h.gate().Open {
			t.Fatal("the gate is open under a contract the verdict was not issued under")
		}
		h.contract = testContract()
		h.must(Pin(h.state, h.facts(), "narrow again", "gov"))
		if !h.gate().Open {
			t.Fatalf("the GREEN verdict does not count again: %+v", h.gate().Conditions)
		}
		h.expect(StepNone)
	})
	t.Run("a pin in between changed the stage", func(t *testing.T) {
		h := newHarness(t)
		h.checks(true)
		h.accept(h.packet(), "rev-1", green(h.contract))
		h.contract = stageTwoContract()
		h.must(Pin(h.state, h.facts(), "controlled validation", "gov"))
		h.contract = testContract()
		h.must(Pin(h.state, h.facts(), "back to the prototype", "gov"))
		g := h.gate()
		if g.Open || g.Conditions[4].Satisfied || !slices.Contains(g.Conditions[4].Reasons, "no round of stage prototype has a verdict") {
			t.Fatalf("gate %+v", g.Conditions[4])
		}
		h.expect(StepReview)
	})
	t.Run("a RED round under the other contract", func(t *testing.T) {
		h := newHarness(t)
		h.checks(true)
		h.accept(h.packet(), "rev-1", green(h.contract))
		h.contract = other
		h.must(Pin(h.state, h.facts(), "widen", "gov"))
		h.accept(h.packet(), "rev-1", red(h.contract, "crash"))
		h.contract = testContract()
		h.must(Pin(h.state, h.facts(), "narrow again", "gov"))
		g := h.gate()
		if g.Open || g.Conditions[4].Satisfied || !slices.Contains(g.Conditions[4].Reasons, "round 2 ended RED") {
			t.Fatalf("gate %+v", g)
		}
	})
}

// A finding upheld once can be disputed again after a pin that drops what
// it cites.
func TestScenarioDisputeAgainAfterPin(t *testing.T) {
	h := newHarness(t)
	h.checks(true)
	h.accept(h.packet(), "rev-1", red(h.contract, "crash"))
	h.accept(h.packet(), "impl-1", ImplementerBody{Disputes: []Dispute{{Finding: "F-0001", Rationale: "not a defect"}}})
	ap := h.packet()
	h.accept(ap, "aud-1", AuditorBody{Resolutions: resolutions(ap, "it stands"),
		Adjudications: []Adjudication{{Finding: "F-0001", Decision: DecisionUphold, Rationale: "reproducible"}}})
	fp := h.packet()
	again := ImplementerBody{Disputes: []Dispute{{Finding: "F-0001", Rationale: "REQ-1 no longer exists"}}}
	h.refused(fp, "impl-1", again, LayerTransition)

	amended := testContract()
	amended.Requirements = append([]Requirement{{ID: "REQ-5", Domain: DomainFunctionalCorrectness,
		Statement: "valid and empty input parse", Criterion: "the tests pass", Command: "go test ./..."}}, amended.Requirements[1:]...)
	h.contract = amended
	h.must(Pin(h.state, h.facts(), "replace REQ-1", "gov"))
	if f, _ := lookup(h.state, "F-0001"); f.Disputed || f.Status != StatusOpen {
		t.Fatalf("finding after the pin %+v", f)
	}
	h.expect(StepFix)
	h.accept(h.packet(), "impl-1", again)
	ap = h.packet()
	h.accept(ap, "aud-1", AuditorBody{Resolutions: resolutions(ap, "the requirement is gone"),
		Adjudications: []Adjudication{{Finding: "F-0001", Decision: DecisionWithdraw, Rationale: "cites nothing current"}}})
	if f, _ := lookup(h.state, "F-0001"); f.Status != StatusWithdrawn {
		t.Fatalf("finding %+v", f)
	}
}
