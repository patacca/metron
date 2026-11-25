package core

import (
	"slices"
	"strings"
	"testing"
)

// A candidate is a tree, so undoing a change returns to a candidate the ledger
// has already seen. These tests hold every finding step to the candidate it
// was recorded on: a fix confirmed on one candidate says nothing about a
// candidate on which the finding was recorded as present.

// eligible is a non-blocking finding that already meets every rule of a
// blocking one, so that an audit may reclassify it to a blocking tier.
func eligible(key, contradicts string) FindingInput {
	x := improvement(key)
	x.Domain = DomainFunctionalCorrectness
	x.Requirement, x.BlockingRationale, x.Verification = "REQ-1", "the demo feeds such files", "a regression test"
	x.Contradicts = contradicts
	return x
}

// fixAndVerify resolves the open finding id with a new commit.
func (h *harness) fixAndVerify(id string) {
	h.tb.Helper()
	h.accept(h.packet(), "impl-1", ImplementerBody{Fixes: []Fix{fix(id, h.commit(true))}})
	if h.next().Step == StepRunChecks {
		h.checks(true)
	}
	h.accept(h.verifying(), "ver-1", verify(map[string]string{id: VerificationConfirmed}, id))
}

func blockersOf(g GateResult) []string {
	return g.Conditions[slices.IndexFunc(g.Conditions, func(c Condition) bool { return c.Name == ConditionBlockers })].Reasons
}

// D1. A GREEN round records a non-blocking finding; an audit makes it
// blocking; its fix is confirmed on another candidate; the tree goes back.
// The old verdict must not approve the candidate the finding was found on.
func TestGateClosedOnTheCandidateOfAResolvedBlockingFinding(t *testing.T) {
	h := newHarness(t)
	h.checks(true)
	h.accept(h.packet(), "rev-1", red(h.contract, "crash"))
	h.fixAndVerify("F-0001")
	h.checks(true)
	x, tree := h.candidate(), h.tree
	h.accept(h.packet(), "rev-2", review(h.contract, VerdictGreen, nil, eligible("back", "F-0001")))
	ap := h.packet()
	h.accept(ap, "aud-1", AuditorBody{Resolutions: resolutions(ap, "REQ-1 is violated"),
		Adjudications: []Adjudication{{Finding: "F-0002", Decision: DecisionReclassify, Tier: TierMaterial, Rationale: "it loses input"}}})
	if h.gate().Open {
		t.Fatal("the gate is open with F-0002 open and blocking")
	}
	h.fixAndVerify("F-0002")
	h.checkout(tree)
	if h.candidate().Digest != x.Digest {
		t.Fatal("the revert did not return to the reviewed candidate")
	}
	h.checks(true)
	g := h.gate()
	if g.Open {
		t.Fatalf("the gate is open on the candidate F-0002 was found on: %+v", g.Conditions)
	}
	if rs := blockersOf(g); len(rs) != 1 || !strings.Contains(rs[0], "F-0002") {
		t.Fatalf("blockers %v, want F-0002", rs)
	}
	a := h.expect(StepCommitChanges)
	if !slices.Equal(a.Findings, []string{"F-0002"}) || !strings.Contains(a.Reason, "F-0002") {
		t.Fatalf("action %+v", a)
	}
	if _, err := PlanPacket(h.state, h.facts()); err == nil {
		t.Fatal("a packet was planned for a candidate that carries a resolved blocking finding")
	}
}

// D3. A resolution that upheld a finding does not unlock another review of
// the candidate it was found on; one that removed the verdict's basis does.
func TestReviewOfACandidateAfterAdjudication(t *testing.T) {
	for _, tc := range []struct {
		name string
		adj  Adjudication
		want Step
	}{
		{"upheld", Adjudication{Decision: DecisionUphold, Rationale: "it stands"}, StepCommitChanges},
		{"withdrawn", Adjudication{Decision: DecisionWithdraw, Rationale: "outside the envelope"}, StepReview},
		{"reclassified down", Adjudication{Decision: DecisionReclassify, Tier: TierImprovement, Rationale: "no requirement is violated"}, StepReview},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := newHarness(t)
			h.checks(true)
			tree := h.tree
			h.accept(h.packet(), "rev-1", red(h.contract, "crash"))
			h.accept(h.packet(), "impl-1", ImplementerBody{Disputes: []Dispute{{Finding: "F-0001", Rationale: "no"}}})
			ap := h.packet()
			tc.adj.Finding = "F-0001"
			h.accept(ap, "aud-1", AuditorBody{Resolutions: resolutions(ap, "decided"), Adjudications: []Adjudication{tc.adj}})
			if tc.adj.Decision == DecisionUphold {
				h.fixAndVerify("F-0001")
				h.checkout(tree)
				h.checks(true)
			}
			a := h.expect(tc.want)
			if tc.want == StepCommitChanges && !strings.Contains(a.Reason, "F-0001") {
				t.Fatalf("reason %q does not name the finding", a.Reason)
			}
			if h.gate().Open {
				t.Fatal("the gate is open")
			}
		})
	}
}

// D4. A fix is refused on every candidate on which the finding was recorded,
// not only on the latest one, so alternating between two rejected trees
// claims nothing.
func TestRejectedFixCannotBeClaimedOnAnEarlierRejectedCandidate(t *testing.T) {
	h := newHarness(t)
	h.checks(true)
	opened := h.tree
	h.accept(h.packet(), "rev-1", red(h.contract, "crash"))
	reject := func(commit string) {
		t.Helper()
		fx := fix("F-0001", commit)
		fx.RootCause = "analysed"
		h.accept(h.packet(), "impl-1", ImplementerBody{Fixes: []Fix{fx}})
		if h.next().Step == StepRunChecks {
			h.checks(true)
		}
		h.accept(h.verifying(), "ver-1", verify(map[string]string{"F-0001": VerificationRejected}, "F-0001"))
		h.drain()
	}
	reject(h.commit(true))
	first := h.tree
	reject(h.commit(true))
	second := h.tree
	for _, tree := range []int{first, second, opened} {
		fx := fix("F-0001", h.checkout(tree))
		fx.Commits, fx.RootCause = h.unlisted(), "analysed"
		ps := h.refused(h.packet(), "impl-1", ImplementerBody{Fixes: []Fix{fx}}, LayerTransition)
		if len(ps) != 1 || !hasProblem(ps, "/body/fixes/0") {
			t.Fatalf("tree %d: problems %+v", tree, ps)
		}
	}
	fx := h.repair("F-0001")
	fx.RootCause = "analysed"
	h.accept(h.packet(), "impl-1", ImplementerBody{Fixes: []Fix{fx}})
}

// Returning to a candidate that was approved earlier does not revive its
// verdict once a later round has been held: the gate reads the latest round.
func TestEarlierApprovalIsNotRevivedByAReturnToItsCandidate(t *testing.T) {
	h := newHarness(t)
	h.checks(true)
	approved := h.tree
	h.accept(h.packet(), "rev-1", green(h.contract))
	h.expect(StepNone)
	h.commit(true)
	h.checks(true)
	h.accept(h.packet(), "rev-1", red(h.contract, "crash"))
	h.checkout(approved)
	h.checks(true)
	if h.gate().Open {
		t.Fatal("the gate is open on a verdict older than the latest round")
	}
	// Going back is a change like any other: it is claimed as the fix,
	// verified, and reviewed again.
	a := h.expect(StepFix)
	if !slices.Equal(a.Findings, []string{"F-0001"}) {
		t.Fatalf("action %+v", a)
	}
	h.accept(h.packet(), "impl-1", ImplementerBody{Fixes: []Fix{fix("F-0001", h.history[len(h.history)-1].Hash)}})
	h.accept(h.verifying(), "ver-1", verify(map[string]string{"F-0001": VerificationConfirmed}, "F-0001"))
	h.expect(StepReview)
	h.accept(h.packet(), "rev-1", green(h.contract))
	h.expect(StepNone)
}

// Every step of a finding records the candidate it was made on.
func TestFindingHistoryRecordsCandidates(t *testing.T) {
	h := newHarness(t)
	h.checks(true)
	x := h.candidate().Digest
	h.accept(h.packet(), "rev-1", red(h.contract, "crash"))
	h.accept(h.packet(), "impl-1", ImplementerBody{Fixes: []Fix{fix("F-0001", h.commit(true))}})
	y := h.candidate().Digest
	if h.next().Step == StepRunChecks {
		h.checks(true)
	}
	h.accept(h.verifying(), "ver-1", verify(map[string]string{"F-0001": VerificationRejected}, "F-0001"))
	f, _ := lookup(h.state, "F-0001")
	var got []string
	for _, e := range f.History {
		got = append(got, string(e.Event)+" "+e.Candidate)
	}
	want := []string{
		string(EventFindingOpened) + " " + x,
		string(EventFindingFixClaimed) + " " + y,
		string(EventFindingFixRejected) + " " + y,
	}
	if !slices.Equal(got, want) {
		t.Fatalf("history %v, want %v", got, want)
	}
	if on := f.RecordedOn(); !slices.Equal(on, []string{x, y}) {
		t.Fatalf("recorded on %v, want %v", on, []string{x, y})
	}
}

// D2. An audit can make a finding blocking only if the finding, as recorded,
// already meets every rule of a blocking finding under the pinned contract.
func TestReclassificationToBlockingNeedsWhatABlockingFindingNeeds(t *testing.T) {
	bare := improvement("bare")
	bare.Contradicts = "F-0001"
	uncited := eligible("uncited", "F-0001")
	uncited.Requirement = ""
	noRationale := eligible("no rationale", "F-0001")
	noRationale.BlockingRationale = ""
	noVerification := eligible("no verification", "F-0001")
	noVerification.Verification = ""
	disproportionate := eligible("disproportionate", "F-0001")
	disproportionate.Domain = DomainMaintainability
	proportionate := disproportionate
	proportionate.Key = "proportionate"
	proportionate.Proportionality = &Proportionality{ProposedChange: "split it", RegressionRisk: "none", DeferralCost: "grows"}
	risk := eligible("risk", "F-0001")
	risk.Requirement, risk.Risk = "", "RISK-1"

	for _, tc := range []struct {
		name    string
		finding FindingInput
		tier    Tier
		refused bool
	}{
		{"cites nothing", bare, TierMaterial, true},
		{"no requirement and no risk", uncited, TierMaterial, true},
		{"no blocking rationale", noRationale, TierCritical, true},
		{"no verification", noVerification, TierMaterial, true},
		{"no proportionality in an improvement-prone domain", disproportionate, TierMaterial, true},
		{"meets every rule", eligible("ok", "F-0001"), TierMaterial, false},
		{"meets every rule, with proportionality", proportionate, TierCritical, false},
		{"cites an unacceptable risk", risk, TierMaterial, false},
		{"to another non-blocking tier", bare, TierPreference, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := newHarness(t)
			h.checks(true)
			h.accept(h.packet(), "rev-1", red(h.contract, "crash"))
			h.fixAndVerify("F-0001")
			h.checks(true)
			h.accept(h.packet(), "rev-2", review(h.contract, VerdictGreen, nil, tc.finding))
			ap := h.packet()
			body := AuditorBody{Resolutions: resolutions(ap, "decided"),
				Adjudications: []Adjudication{{Finding: "F-0002", Decision: DecisionReclassify, Tier: tc.tier, Rationale: "it is material"}}}
			if !tc.refused {
				h.accept(ap, "aud-1", body)
				if f, _ := lookup(h.state, "F-0002"); f.Tier != tc.tier {
					t.Fatalf("tier %s, want %s", f.Tier, tc.tier)
				}
				return
			}
			if ps := h.refused(ap, "aud-1", body, LayerSemantic); !hasProblem(ps, "/body/adjudications/0/tier") {
				t.Fatalf("problems %+v", ps)
			}
			// The auditor can still uphold it; it then stays where it is.
			body.Adjudications[0] = Adjudication{Finding: "F-0002", Decision: DecisionUphold, Rationale: "real, but nothing in the contract makes it mandatory"}
			h.accept(ap, "aud-1", body)
			if f, _ := lookup(h.state, "F-0002"); f.Tier.Blocking() {
				t.Fatalf("F-0002 became %s", f.Tier)
			}
			h.expect(StepNone)
		})
	}

	// A blocking finding whose requirement the pinned contract no longer has
	// cannot be moved between blocking tiers either.
	t.Run("citation no longer in the contract", func(t *testing.T) {
		h := newHarness(t)
		h.checks(true)
		h.accept(h.packet(), "rev-1", review(h.contract, VerdictRed, map[string]string{"REQ-3": RequirementViolated}, blocker("layers", "REQ-3")))
		c := testContract()
		c.Requirements = append(c.Requirements[:2:2], c.Requirements[3])
		c.NotApplicable = []Waiver{{Domain: DomainArchitecture, Rationale: "the layering is accepted as it is"}}
		h.contract = c
		h.must(Pin(h.state, h.facts(), "REQ-3 dropped", "gov"))
		h.accept(h.packet(), "impl-1", ImplementerBody{Disputes: []Dispute{{Finding: "F-0001", Rationale: "the requirement is gone"}}})
		ap := h.packet()
		body := AuditorBody{Resolutions: resolutions(ap, "decided"),
			Adjudications: []Adjudication{{Finding: "F-0001", Decision: DecisionReclassify, Tier: TierCritical, Rationale: "worse than thought"}}}
		if ps := h.refused(ap, "aud-1", body, LayerSemantic); !hasProblem(ps, "/body/adjudications/0/tier") {
			t.Fatalf("problems %+v", ps)
		}
	})
}

// D5. The regression spiral compares blocking regressions with the confirmed
// fixes of blocking findings. A confirmed fix of a non-blocking finding
// repairs nothing the gate depends on, so it must not dilute the comparison.
// Fix packets no longer offer non-blocking findings, so the claim on one is
// written here as a ledger from before that change holds it.
func TestRegressionSpiralCountsOnlyBlockingConfirmations(t *testing.T) {
	h := newHarness(t)
	h.checks(true)
	h.accept(h.packet(), "rev-1", review(h.contract, VerdictRed, map[string]string{"REQ-1": RequirementViolated},
		blocker("crash", "REQ-1"), improvement("long")))
	fp := h.packet()
	commit := h.commit(true)
	h.accept(fp, "impl-1", ImplementerBody{Fixes: []Fix{fix("F-0001", commit)}})
	h.seal([]Draft{{Type: EventFindingFixClaimed, Data: FindingFixClaimed{
		ID: "F-0002", Round: 1, Packet: fp.ID, Candidate: h.candidate().Digest, Fix: fix("F-0002", commit)}}})
	if h.next().Step == StepRunChecks {
		h.checks(true)
	}
	vp := h.packet()
	if !slices.Equal(vp.Subjects, []string{"F-0001", "F-0002"}) {
		t.Fatalf("verify subjects %v", vp.Subjects)
	}
	b := verify(map[string]string{"F-0001": VerificationConfirmed, "F-0002": VerificationConfirmed}, "F-0001", "F-0002")
	reg := blocker("leak", "REQ-1")
	reg.IntroducedBy = "F-0002"
	b.Regressions = []FindingInput{reg}
	d := h.accept(vp, "ver-1", b)
	if k := raisedKinds(d); !slices.Contains(k, EscalationRegressionSpiral) {
		t.Fatalf("one blocking regression against one confirmed blocking fix raised %v, want a regression spiral", k)
	}
}
