package core

import (
	"slices"
	"testing"
)

// A decision binds where it was made: under the contract pinned at the time
// and, for evidence governance supplied, on the candidate it is about.
// Elsewhere it is carried as an earlier decision, for context.

func ids(ds []Decision) []string {
	out := []string{}
	for _, d := range ds {
		out = append(out, d.ID)
	}
	return out
}

// directed leaves a ledger in which an audit withdrew F-0001 and issued a
// directive, D-0001, with its resolution D-0002.
func directed(t *testing.T) *harness {
	h := newHarness(t)
	h.checks(true)
	h.accept(h.packet(), "rev-1", red(h.contract, "crash"))
	h.accept(h.packet(), "impl-1", ImplementerBody{Disputes: []Dispute{{Finding: "F-0001", Rationale: "no"}}})
	ap := h.packet()
	h.accept(ap, "aud-1", AuditorBody{Resolutions: resolutions(ap, "withdrawn"),
		Adjudications: []Adjudication{{Finding: "F-0001", Decision: DecisionWithdraw, Rationale: "outside the envelope"}},
		Directives:    []string{"Do not raise findings about empty input: it is outside the envelope."}})
	return h
}

func TestDecisionsBindUnderTheirContractOnly(t *testing.T) {
	h := directed(t)
	first := h.contract
	rp := h.packet()
	if !slices.Equal(ids(rp.Decisions), []string{"D-0001", "D-0002"}) || len(rp.EarlierDecisions) != 0 {
		t.Fatalf("under the contract of the audit: binding %v, earlier %v", ids(rp.Decisions), ids(rp.EarlierDecisions))
	}
	if d := rp.Decisions[0]; d.Stage != StagePrototype || d.Contract != rp.ContractDigest || d.Candidate != rp.Candidate.Digest || d.By != RoleAuditor {
		t.Fatalf("directive %+v", d)
	}

	// The contract changes and the stage does not: empty files are now
	// inside the envelope, so the directive answers a question nobody asks.
	amended := testContract()
	amended.Envelope = []string{"files under 1 MiB, including empty files"}
	h.contract = amended
	h.must(Pin(h.state, h.facts(), "empty files are in the envelope", "gov"))
	h.checks(true)
	rp = h.packet()
	if len(rp.Decisions) != 0 || !slices.Equal(ids(rp.EarlierDecisions), []string{"D-0001", "D-0002"}) {
		t.Fatalf("under an amended contract of the same stage: binding %v, earlier %v", ids(rp.Decisions), ids(rp.EarlierDecisions))
	}

	// Pinning the first contract again makes its decisions bind again, as it
	// makes its verdicts count again.
	h.contract = first
	h.must(Pin(h.state, h.facts(), "back", "gov"))
	if rp = h.packet(); !slices.Equal(ids(rp.Decisions), []string{"D-0001", "D-0002"}) {
		t.Fatalf("under the first contract again: binding %v", ids(rp.Decisions))
	}
}

// Evidence governance supplies for an evidence gap is about the candidate
// the gap was raised on.
func TestSuppliedEvidenceBindsOnItsCandidateOnly(t *testing.T) {
	h := newHarness(t)
	h.checks(true)
	x := h.candidate().Digest
	h.accept(h.packet(), "rev-1", review(h.contract, VerdictIndeterminate, map[string]string{"REQ-2": RequirementUnverified}))
	h.commit(false) // governance works from a later, ledger-only commit
	h.must(Resolve(h.state, h.facts(), "E-0001", "REQ-2 holds: load test report LT-17 passed", "gov"))
	rp := h.packet()
	if len(rp.Decisions) != 1 || rp.Decisions[0].Candidate != x || !slices.Equal(rp.Decisions[0].Requirements, []string{"REQ-2"}) ||
		rp.Decisions[0].Kind != EscalationEvidenceGap {
		t.Fatalf("on the candidate of the gap: %+v", rp.Decisions)
	}
	// The candidate changes before the review: the load test was not run on
	// this one.
	h.commit(true)
	h.checks(true)
	rp = h.packet()
	if len(rp.Decisions) != 0 || !slices.Equal(ids(rp.EarlierDecisions), []string{"D-0001"}) {
		t.Fatalf("on another candidate: binding %v, earlier %v", ids(rp.Decisions), ids(rp.EarlierDecisions))
	}
}

func TestAuditSupersedesADecision(t *testing.T) {
	h := directed(t)
	h.accept(h.packet(), "rev-1", review(h.contract, VerdictIndeterminate, map[string]string{"REQ-2": RequirementUnverified}))
	h.must(Resolve(h.state, h.facts(), "E-0002", "confirmed by the product owner", "gov")) // D-0003, by governance
	b := red(h.contract, "again")
	h.accept(h.packet(), "rev-1", b)
	h.audits() // the third failed step calls for a convergence analysis
	h.accept(h.packet(), "impl-1", ImplementerBody{Disputes: []Dispute{{Finding: "F-0002", Rationale: "no"}}})
	ap := h.packet()
	audit := func(sp ...Supersession) AuditorBody {
		return AuditorBody{Resolutions: resolutions(ap, "upheld"), Supersede: sp,
			Adjudications: []Adjudication{{Finding: "F-0002", Decision: DecisionUphold, Rationale: "it stands"}}}
	}
	for _, tc := range []struct {
		name string
		body AuditorBody
		path string
	}{
		{"unknown decision", audit(Supersession{Decision: "D-0042", Rationale: "r"}), "/body/supersede/0/decision"},
		{"a governance decision", audit(Supersession{Decision: "D-0003", Rationale: "r"}), "/body/supersede/0/decision"},
		{"twice", audit(Supersession{Decision: "D-0001", Rationale: "r"}, Supersession{Decision: "D-0001", Rationale: "r"}), "/body/supersede/1/decision"},
		{"no rationale", audit(Supersession{Decision: "D-0001", Rationale: " "}), "/body/supersede/0/rationale"},
	} {
		if ps := h.refused(ap, "aud-1", tc.body, LayerSemantic); !hasProblem(ps, tc.path) {
			t.Fatalf("%s: problems %+v", tc.name, ps)
		}
	}
	d := h.accept(ap, "aud-1", audit(Supersession{Decision: "D-0001", Rationale: "the reproduction shows that empty input is inside the envelope"}))
	eqTypes(t, d, EventReportAccepted, EventDecisionSuperseded, EventEscalationResolved)
	fp := h.packet()
	if slices.Contains(ids(fp.Decisions), "D-0001") || !slices.Contains(ids(fp.EarlierDecisions), "D-0001") {
		t.Fatalf("after the supersession: binding %v, earlier %v", ids(fp.Decisions), ids(fp.EarlierDecisions))
	}
	for _, e := range fp.EarlierDecisions {
		if e.ID == "D-0001" && e.SupersededBy != ap.ID {
			t.Fatalf("superseded decision %+v", e)
		}
	}
	// It cannot be superseded twice.
	h.accept(fp, "impl-1", ImplementerBody{Fixes: []Fix{fix("F-0002", h.commit(true))}})
	h.accept(h.verifying(), "ver-1", verify(map[string]string{"F-0002": VerificationRejected}, "F-0002"))
	h.accept(h.packet(), "impl-1", ImplementerBody{Fixes: []Fix{h.repair("F-0002")}})
	h.accept(h.verifying(), "ver-1", verify(map[string]string{"F-0002": VerificationRejected}, "F-0002"))
	ap = h.packet()
	if ap.Action != StepAudit {
		t.Fatalf("packet %+v", ap.Action)
	}
	again := AuditorBody{Resolutions: resolutions(ap, "carry on"), Supersede: []Supersession{{Decision: "D-0001", Rationale: "r"}}}
	if ps := h.refused(ap, "aud-1", again, LayerTransition); !hasProblem(ps, "/body/supersede/0/decision") {
		t.Fatalf("problems %+v", ps)
	}
}

// A finding records the contract it was established under, and an escalation
// the candidate it was raised on.
func TestFindingsAndEscalationsRecordWhereTheyWereMade(t *testing.T) {
	h := newHarness(t)
	h.checks(true)
	first := contractDigest(t, h.contract)
	x := h.candidate().Digest
	h.accept(h.packet(), "rev-1", red(h.contract, "crash"))
	amended := testContract()
	amended.Requirements[0].Statement = "the binary is smaller than 10 MiB"
	h.contract = amended
	h.must(Pin(h.state, h.facts(), "REQ-1 now means something else", "gov"))
	fp := h.packet()
	if f := fp.Findings[0]; f.Contract != first || f.Contract == fp.ContractDigest {
		t.Fatalf("finding contract %s, packet contract %s, first %s", f.Contract, fp.ContractDigest, first)
	}
	h.accept(fp, "impl-1", ImplementerBody{Disputes: []Dispute{{Finding: "F-0001", Rationale: "REQ-1 no longer says that"}}})
	if e := h.state.Escalations()[0]; e.Candidate != x {
		t.Fatalf("escalation %+v", e)
	}
}
