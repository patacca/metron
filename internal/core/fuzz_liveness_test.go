package core

import (
	"slices"
	"testing"
)

// The properties below start from whatever state a fuzzed command sequence
// reaches and ask what follows from it, with two kinds of roles: ones that
// cooperate, and ones that never converge.

// cooperate performs the next action the way roles and a governance that
// want the campaign to succeed would: governance pins a usable contract and
// resolves what awaits it, fixes are made and confirmed, audits uphold, and
// reviews find nothing. It reports whether there was anything to do. Every
// command must be accepted: the action Next names is one that can be
// performed.
func (s *sim) cooperate() bool {
	s.t.Helper()
	a := s.next()
	switch a.Step {
	case StepNone:
		return false
	case StepPinContract:
		if ValidateContract(s.contract) != nil {
			s.contract = testContract()
		}
		if pin, ok := s.state.Pin(); !ok || pin.Digest != contractDigest(s.t, s.contract) {
			s.must(Pin(s.state, s.facts(), "authorized", "gov"))
		}
	case StepAdjudicate:
		s.must(Resolve(s.state, s.facts(), a.Escalations[0], "decided", "gov"))
	case StepCommitChanges:
		s.commit(true)
	case StepRunChecks:
		s.checks(true)
	case StepSubmitReport, StepReview, StepFix, StepVerify, StepAudit:
		s.lost = ""
		p := s.packet()
		s.packets[p.ID] = p
		switch p.Role {
		case RoleReviewer:
			s.accept(p, "co-rev", green(p.Contract))
		case RoleImplementer:
			// The report accounts for every commit since the reviewed one,
			// whoever made it.
			s.commit(true)
			var b ImplementerBody
			for _, id := range p.Subjects {
				fx := s.restate(id)
				fx.RootCause = "analysed"
				b.Fixes = append(b.Fixes, fx)
			}
			s.accept(p, "co-impl", b)
		case RoleVerifier:
			results := map[string]string{}
			for _, id := range p.Subjects {
				results[id] = VerificationConfirmed
			}
			s.accept(p, "co-ver", verify(results, p.Subjects...))
		case RoleAuditor:
			var adj []Adjudication
			for _, id := range p.Adjudicable {
				adj = append(adj, Adjudication{Finding: id, Decision: DecisionUphold, Rationale: "it stands"})
			}
			s.accept(p, "co-aud", AuditorBody{Resolutions: resolutions(p, "carry on"), Adjudications: adj})
		}
	default:
		s.t.Fatalf("no cooperative way to perform %+v", a)
	}
	return true
}

// obstruct performs the next action the way roles that never converge do:
// every review is RED, every fix is rejected, every audit carries on. It
// must not be called when the next action belongs to governance.
func (s *sim) obstruct() {
	s.t.Helper()
	a := s.next()
	switch a.Step {
	case StepNone:
		// An approved candidate: the roles move on to the next one.
		s.commit(true)
	case StepCommitChanges:
		s.commit(true)
	case StepRunChecks:
		s.checks(true)
	case StepSubmitReport, StepReview, StepFix, StepVerify, StepAudit:
		s.lost = ""
		p := s.packet()
		s.packets[p.ID] = p
		switch p.Role {
		case RoleReviewer:
			s.accept(p, "co-rev", red(p.Contract, "never good enough"))
		case RoleImplementer:
			// The report accounts for every commit since the reviewed one,
			// whoever made it.
			s.commit(true)
			var b ImplementerBody
			for _, id := range p.Subjects {
				fx := s.restate(id)
				fx.RootCause = "analysed"
				b.Fixes = append(b.Fixes, fx)
			}
			s.accept(p, "co-impl", b)
		case RoleVerifier:
			results := map[string]string{}
			for _, id := range p.Subjects {
				results[id] = VerificationRejected
			}
			s.accept(p, "co-ver", verify(results, p.Subjects...))
		case RoleAuditor:
			var adj []Adjudication
			for _, id := range p.Adjudicable {
				adj = append(adj, Adjudication{Finding: id, Decision: DecisionUphold, Rationale: "it stands"})
			}
			s.accept(p, "co-aud", AuditorBody{Resolutions: resolutions(p, "carry on"), Adjudications: adj})
		}
	default:
		s.t.Fatalf("no way for a role to perform %+v", a)
	}
}

// converges checks cooperative liveness: with cooperating roles the gate
// opens within a bound that depends only on what is open in the state.
func (s *sim) converges() {
	s.t.Helper()
	open := 0
	for _, e := range s.state.Escalations() {
		if e.Open {
			open++
		}
	}
	// Each open escalation costs at most an audit and a resolution; the rest
	// is one correction cycle, a review, and at most two suspensions of a
	// small budget on the way.
	bound := 40 + 4*open
	for steps, prev := 0, s.state; s.cooperate(); steps, prev = steps+1, s.state {
		if steps > bound {
			s.t.Fatalf("cooperating roles did not open the gate in %d steps; next is %+v", bound, s.next())
		}
		s.check(prev)
	}
	if !s.gate().Open {
		s.t.Fatalf("next is none but the gate is closed: %+v", s.gate())
	}
}

// stalls checks that roles which never converge cannot keep a campaign going
// and cannot get it approved: after each decision by governance, a
// governance-owned action arrives within the bound the contract's thresholds
// set, and the gate does not open on the way.
func (s *sim) stalls() {
	s.t.Helper()
	for campaign := 0; campaign < 3; campaign++ {
		for s.next().Owner == RoleGovernance {
			s.cooperate()
		}
		// From now on no verdict is GREEN, so the gate can be open only on
		// a round that was GREEN before: a candidate approved earlier whose
		// checks pass again. Nothing these roles do approves anything.
		before := len(s.state.rounds)
		bound := 4*s.contract.Thresholds().SuspendAfter + 40
		for steps := 0; s.next().Owner != RoleGovernance; steps++ {
			if steps > bound {
				s.t.Fatalf("%d steps of roles that never converge and no action owned by governance; next is %+v", bound, s.next())
			}
			prev := s.state
			s.obstruct()
			s.check(prev)
			if r, _ := s.state.latestRound(); s.gate().Open && (r.Number > before || r.Verdict != VerdictGreen) {
				s.t.Fatalf("the gate opened on round %d (%s) although no verdict was GREEN since round %d", r.Number, r.Verdict, before)
			}
		}
	}
}

// blockingInvariant checks that every finding that blocks has what a blocking
// finding needs: nothing blocks on a reviewer's or an auditor's word alone.
func (s *sim) blockingInvariant() {
	s.t.Helper()
	for _, x := range s.state.Findings() {
		if !x.Tier.Blocking() || x.Status == StatusWithdrawn {
			continue
		}
		if x.Input.Requirement == "" && x.Input.Risk == "" || len(blockingGaps(x.Input)) > 0 {
			s.t.Fatalf("%s is %s but does not meet the rules of a blocking finding: %+v", x.ID, x.Tier, x.Input)
		}
	}
}

func prefix(t *testing.T, data []byte) *sim {
	s := &sim{harness: newHarness(t), t: t}
	for i := 0; i+1 < len(data) && i < 240; i += 2 {
		s.step(data[i], data[i+1])
	}
	return s
}

func FuzzLiveness(f *testing.F) {
	for _, seed := range commandSeeds() {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		prefix(t, data).converges()
		prefix(t, data).stalls()
	})
}

func TestLivenessSeeds(t *testing.T) {
	for i, seed := range commandSeeds() {
		prefix(t, seed).converges()
		s := prefix(t, seed)
		s.stalls()
		// Roles that never converge leave the campaign waiting for
		// governance, not approved.
		if a := s.next(); a.Owner != RoleGovernance || s.gate().Open {
			t.Fatalf("seed %d ends with %+v and the gate open %t", i, a, s.gate().Open)
		}
		if _, suspended := s.state.Suspension(); !suspended && !slices.Contains([]Step{StepAdjudicate, StepPinContract}, s.next().Step) {
			t.Fatalf("seed %d ends with %+v", i, s.next())
		}
	}
}
