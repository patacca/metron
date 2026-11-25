package core

import (
	"strings"
	"testing"
)

// Tests of Next, Gate and PacketInvalid on states the scenarios do not reach.

func TestNextRows(t *testing.T) {
	for _, tc := range []struct {
		name  string
		state func(h *harness) State
		facts func(f *Facts)
		want  Step
		owner Role
		about string
	}{
		{name: "ledger unverified", facts: func(f *Facts) { f.LedgerVerified = false }, want: StepRepairLedger, owner: RoleGovernance, about: "failed verification"},
		{name: "ledger empty", state: func(h *harness) State { return State{} }, want: StepRepairLedger, owner: RoleGovernance, about: "not initialized"},
		{name: "nothing pinned", state: func(h *harness) State {
			s, _ := Replay(h.events[:1])
			return s
		}, want: StepPinContract, owner: RoleGovernance, about: "no contract is pinned"},
		{name: "contract unavailable", facts: func(f *Facts) { f.Contract, f.ContractProblem = nil, "anchors are not allowed" }, want: StepPinContract, about: "anchors"},
		{name: "contract invalid", facts: func(f *Facts) {
			c := *f.Contract
			c.Authorizes = ""
			f.Contract = &c
		}, want: StepPinContract, about: "/authorizes"},
		{name: "contract edited", facts: func(f *Facts) { f.ContractDigest = digestOf("edited") }, want: StepPinContract, about: "differs from the pinned"},
		{name: "tree dirty", facts: func(f *Facts) { f.Clean = false }, want: StepCommitChanges, owner: RoleImplementer},
		{name: "checks missing", want: StepRunChecks, owner: RoleImplementer, about: "REQ-1 has no recorded result"},
		{name: "checks failing", state: func(h *harness) State { h.checks(false); return h.state }, want: StepRunChecks, about: "REQ-1 failed"},
		{name: "review", state: func(h *harness) State { h.checks(true); return h.state }, want: StepReview, owner: RoleReviewer},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := newHarness(t)
			s := h.state
			if tc.state != nil {
				s = tc.state(h)
			}
			f := h.facts()
			if tc.facts != nil {
				tc.facts(&f)
			}
			a := Next(s, f)
			if a.Step != tc.want || (tc.owner != "" && a.Owner != tc.owner) || !strings.Contains(a.Reason, tc.about) {
				t.Fatalf("next %+v, want %s by %s about %q", a, tc.want, tc.owner, tc.about)
			}
			if Gate(s, f).Open {
				t.Fatal("the gate is open")
			}
		})
	}
}

func TestNextReasonText(t *testing.T) {
	h := newHarness(t)
	h.checks(true)
	digest := h.candidate().Digest
	if a, want := h.next(), "candidate "+digest+" has no GREEN verdict under the pinned contract (no round of stage prototype has a verdict): review it"; a.Reason != want {
		t.Fatalf("reason\n got %q\nwant %q", a.Reason, want)
	}
	h.accept(h.packet(), "rev-1", red(h.contract, "crash"))
	if a, want := h.next(), "blocking finding F-0001 is open: fix or dispute it"; a.Reason != want {
		t.Fatalf("reason\n got %q\nwant %q", a.Reason, want)
	}

	g := newHarness(t)
	g.checks(true)
	g.accept(g.packet(), "rev-1", review(g.contract, VerdictRed, map[string]string{"REQ-1": RequirementViolated},
		blocker("a", "REQ-1"), blocker("b", "REQ-1")))
	if a, want := g.next(), "blocking findings F-0001, F-0002 are open: fix or dispute each"; a.Reason != want {
		t.Fatalf("reason\n got %q\nwant %q", a.Reason, want)
	}
	// F-0001 is disputed and upheld, then both fixes are rejected.
	g.accept(g.packet(), "impl-1", ImplementerBody{Fixes: []Fix{fix("F-0002", g.commit(true))},
		Disputes: []Dispute{{Finding: "F-0001", Rationale: "not a defect"}}})
	ap := g.packet()
	g.accept(ap, "aud-1", AuditorBody{Resolutions: resolutions(ap, "it stands"),
		Adjudications: []Adjudication{{Finding: "F-0001", Decision: DecisionUphold, Rationale: "reproducible"}}})
	// The upheld finding is fixed before anything is verified: a correction
	// is verified as one batch, on one candidate.
	if a, want := g.next(), "blocking finding F-0001 is open and was already disputed: fix it; the fix of F-0002 is already claimed, on this candidate: if the candidate changes, claim it again in the same report"; a.Step != StepFix || a.Reason != want {
		t.Fatalf("action %+v, want reason %q", a, want)
	}
	gp := g.packet()
	claimed := g.restate("F-0002")
	g.accept(gp, "impl-1", ImplementerBody{Fixes: []Fix{fix("F-0001", g.commit(true)), claimed}})
	g.accept(g.verifying(), "ver-1", verify(map[string]string{"F-0001": VerificationRejected, "F-0002": VerificationRejected}, "F-0001", "F-0002"))
	if a, want := g.next(), "blocking findings F-0001, F-0002 are open: fix F-0001, which was already disputed, and fix or dispute F-0002"; a.Reason != want {
		t.Fatalf("reason\n got %q\nwant %q", a.Reason, want)
	}

	u := newHarness(t)
	u.checks(true)
	u.accept(u.packet(), "rev-1", red(u.contract, "crash"))
	u.accept(u.packet(), "impl-1", ImplementerBody{Disputes: []Dispute{{Finding: "F-0001", Rationale: "not a defect"}}})
	ap = u.packet()
	u.accept(ap, "aud-1", AuditorBody{Resolutions: resolutions(ap, "it stands"),
		Adjudications: []Adjudication{{Finding: "F-0001", Decision: DecisionUphold, Rationale: "reproducible"}}})
	if a, want := u.next(), "blocking finding F-0001 is open and was already disputed: fix it"; a.Reason != want {
		t.Fatalf("reason\n got %q\nwant %q", a.Reason, want)
	}
}

func TestGateNamesAMissingCandidate(t *testing.T) {
	h := newHarness(t)
	f := h.facts()
	f.Candidate = Candidate{}
	g := Gate(h.state, f)
	want := "the current candidate cannot be identified: the commit is unknown and the candidate digest is unknown"
	if g.Open || !strings.Contains(strings.Join(g.Conditions[3].Reasons, "\n"), want) {
		t.Fatalf("integrity reasons %q", g.Conditions[3].Reasons)
	}
	for _, c := range g.Conditions {
		for _, r := range c.Reasons {
			if strings.Contains(r, "{") {
				t.Fatalf("a reason prints a Go value: %q", r)
			}
		}
	}
}

// TestGateIndependentVerdict covers the branches of condition V that
// ingestion never produces: each keeps the gate closed.
func TestGateIndependentVerdict(t *testing.T) {
	reviewable := func(t *testing.T) *harness {
		h := newHarness(t)
		h.checks(true)
		h.accept(h.packet(), "rev-1", red(h.contract, "crash"))
		h.accept(h.packet(), "impl-1", ImplementerBody{Fixes: []Fix{fix("F-0001", h.commit(true))}})
		h.accept(h.verifying(), "ver-1", verify(map[string]string{"F-0001": VerificationConfirmed}, "F-0001"))
		h.checks(true)
		return h
	}
	closed := func(t *testing.T, s State, f Facts, reason string) {
		t.Helper()
		g := Gate(s, f)
		if g.Open || g.Conditions[4].Satisfied || !strings.Contains(strings.Join(g.Conditions[4].Reasons, "\n"), reason) {
			t.Fatalf("condition V %+v", g.Conditions[4])
		}
		if Next(s, f).Step == StepNone {
			t.Fatal("next is none while the gate is closed")
		}
	}

	t.Run("a reviewer who acted as implementer, in a ledger that replays", func(t *testing.T) {
		h := reviewable(t)
		p := h.packet()
		drafts, _, err := h.ingest(h.report(p, "rev-1", green(h.contract)))
		if err != nil {
			t.Fatal(err)
		}
		// Edit the drafts as a hand-edited ledger would: Ingest refuses this
		// author, Apply does not check role separation.
		ra := drafts[0].Data.(ReportAccepted)
		ra.Author.ID = "impl-1"
		drafts[0].Data = ra
		vr := drafts[1].Data.(VerdictRecorded)
		vr.Reviewer = "impl-1"
		drafts[1].Data = vr
		h.seal(drafts)
		if _, err := Replay(h.events); err != nil {
			t.Fatal(err)
		}
		closed(t, h.state, h.facts(), `issued by "impl-1", who acted as implementer`)
	})
	t.Run("a verdict without a reviewer, in a hand-built state", func(t *testing.T) {
		h := reviewable(t)
		h.accept(h.packet(), "rev-1", green(h.contract))
		s := h.state.clone()
		s.rounds[len(s.rounds)-1].Reviewer = ""
		closed(t, s, h.facts(), "names no reviewer")
	})
	t.Run("a verdict with no contract pinned, in a hand-built state", func(t *testing.T) {
		h := reviewable(t)
		h.accept(h.packet(), "rev-1", green(h.contract))
		s := h.state.clone()
		s.pins = nil
		closed(t, s, h.facts(), "no contract is pinned")
	})
}

func TestPacketInvalid(t *testing.T) {
	h := newHarness(t)
	h.checks(true)
	if why := PacketInvalid(h.state, h.facts()); why != "" {
		t.Fatalf("no packet is open, yet %q", why)
	}
	p := h.packet()
	if why := PacketInvalid(h.state, h.facts()); why != "" {
		t.Fatalf("a valid packet reported as %q", why)
	}
	h.clean = false
	if why := PacketInvalid(h.state, h.facts()); why != "the working tree is not clean" {
		t.Fatalf("dirty tree: %q", why)
	}
	h.commit(true)
	why := PacketInvalid(h.state, h.facts())
	want := "the candidate changed from " + p.Candidate.Digest + " to " + h.candidate().Digest
	if why != want {
		t.Fatalf("got %q, want %q", why, want)
	}
	if a := h.next(); !strings.Contains(a.Reason, "packet P-0001 can no longer be answered ("+want+")") {
		t.Fatalf("next does not mention the stale packet: %q", a.Reason)
	}
	f := h.facts()
	f.PacketProblem = "packets/P-0001.json is missing"
	if why := PacketInvalid(h.state, f); why != f.PacketProblem {
		t.Fatalf("packet problem: %q", why)
	}
}
