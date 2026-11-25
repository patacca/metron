package core

import (
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"slices"
	"testing"
)

// The command fuzzer reads its input as pairs of bytes: an operation and a
// parameter whose bits select the variant of that operation. A zero parameter
// is the cooperative path; other values produce invalid commands, refusals
// and every safeguard.
const (
	opChecks = iota
	opPacket
	opAct
	opCommit
	opTree
	opResolve
	opContract
	opPin
	opReingest
	opImpostor
	opLose
	opCount
)

type sim struct {
	*harness
	t    *testing.T
	last *Report
}

func bit(param byte, n uint) bool { return param&(1<<n) != 0 }

func contractVariant(n int) Contract {
	c := testContract()
	switch n % 6 {
	case 1:
		c.Authorizes = "a pilot with external users"
	case 2:
		c = stageTwoContract()
	case 3:
		c.Convergence = &Convergence{RootCauseAfter: 1, ConvergenceAuditAfter: 2, ProcessAuditAfter: 3}
	case 4:
		c.Requirements = slices.Clone(c.Requirements)
		c.Requirements[0].Command = "go test -race ./..."
	case 5:
		c.Stage = "beta"
	}
	return c
}

func errText(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

// run executes a command twice, requires the same outcome both times and no
// drafts from a refusal, and seals the drafts of an accepted command.
func (s *sim) run(name string, cmd func() ([]Draft, error)) bool {
	s.t.Helper()
	d1, e1 := cmd()
	d2, e2 := cmd()
	if !reflect.DeepEqual(d1, d2) || errText(e1) != errText(e2) {
		s.t.Fatalf("%s is not deterministic: %v / %v", name, e1, e2)
	}
	if e1 != nil {
		if len(d1) != 0 {
			s.t.Fatalf("%s refused (%v) but produced %d drafts", name, e1, len(d1))
		}
		return false
	}
	s.seal(d1)
	return true
}

func (s *sim) ingestRun(r Report) bool {
	s.t.Helper()
	var dup bool
	accepted := s.run("ingest", func() ([]Draft, error) {
		d, isDup, err := s.ingest(r)
		dup = isDup
		if isDup && (len(d) != 0 || err != nil) {
			s.t.Fatalf("a duplicate yielded %d drafts and %v", len(d), err)
		}
		return d, err
	})
	if accepted && !dup {
		s.last = &r
	}
	return accepted
}

func (s *sim) step(op, param byte) {
	f := s.facts()
	switch int(op) % opCount {
	case opChecks:
		var ev []Evidence
		for _, r := range f.Contract.Applicable() {
			if r.Command == "" {
				continue
			}
			e := Evidence{Requirement: r.ID, Command: r.Command, Candidate: f.Candidate.Digest, Passed: true, OutputDigest: digestOf("out")}
			if bit(param, 0) {
				e.ExitCode, e.Passed = 1, false
			}
			if bit(param, 1) {
				e.Candidate = digestOf("elsewhere")
			}
			ev = append(ev, e)
		}
		s.run("record evidence", func() ([]Draft, error) { return RecordEvidence(s.state, f, ev) })
	case opPacket:
		plan, err := PlanPacket(s.state, f)
		if err != nil || plan.Open != "" {
			return
		}
		p := plan.Packet
		noNull(s.t, "packet "+p.ID, mustJSON(s.t, p))
		p.Instructions = "brief"
		if bit(param, 1) {
			p.Instructions = ""
		}
		if bit(param, 0) {
			p.Round++
		}
		if s.run("issue packet", func() ([]Draft, error) { return IssuePacket(s.state, f, p, digestOf(string(mustJSON(s.t, p)))) }) {
			s.packets[p.ID] = p
		}
	case opAct, opImpostor:
		open, ok := s.state.OpenPacket()
		if !ok {
			return
		}
		p := s.packets[open.Packet]
		author := map[Role]string{RoleReviewer: "rev-1", RoleImplementer: "impl-1", RoleVerifier: "ver-1", RoleAuditor: "aud-1"}[p.Role]
		if p.Role == RoleReviewer && p.Round%2 == 0 {
			author = "rev-2"
		}
		var body any
		switch p.Role {
		case RoleReviewer:
			body = s.reviewer(p, param)
		case RoleImplementer:
			body = s.implementer(p, param)
		case RoleVerifier:
			body = s.verifier(p, param)
		case RoleAuditor:
			body = s.auditor(p, param)
		}
		r := s.report(p, author, body)
		unusable := s.facts().PacketProblem != ""
		if int(op)%opCount == opImpostor {
			r.Author.ID = []string{"rev-1", "impl-1", "ver-1", "aud-1", "rev-2"}[int(param>>4)%5]
			if bit(param, 7) {
				r.Role = RoleAuditor
				if p.Role == RoleAuditor {
					r.Role = RoleReviewer
				}
			}
		}
		if s.ingestRun(r) && unusable {
			s.t.Fatalf("a report for packet %s was accepted although its file is unusable", p.ID)
		}
	case opLose:
		if open, ok := s.state.OpenPacket(); ok {
			s.lost = open.Packet
		}
	case opCommit:
		s.commit(!bit(param, 0))
	case opTree:
		switch {
		case s.clean:
			s.clean = false
		case bit(param, 0):
			s.commit(true)
		default:
			s.clean = true
		}
	case opResolve:
		es := s.state.Escalations()
		if len(es) == 0 {
			return
		}
		e := es[int(param&0x7f)%len(es)]
		decision := "decided after review"
		if bit(param, 7) {
			decision = ""
		}
		s.run("resolve", func() ([]Draft, error) { return Resolve(s.state, f, e.ID, decision, "gov") })
	case opContract:
		s.contract = contractVariant(int(param))
	case opPin:
		reason := "authorized"
		if bit(param, 0) {
			reason = " "
		}
		s.run("pin", func() ([]Draft, error) { return Pin(s.state, f, reason, "gov") })
	case opReingest:
		if s.last == nil {
			return
		}
		r := *s.last
		if bit(param, 0) {
			r.Author.Session = "another session"
		}
		_, dup, err := s.ingest(r)
		if !bit(param, 0) && (!dup || err != nil) {
			s.t.Fatalf("re-ingesting an accepted report: duplicate %t, %v", dup, err)
		}
		if bit(param, 0) && (dup || err == nil) {
			s.t.Fatalf("a different report for a consumed packet: duplicate %t, %v", dup, err)
		}
	}
}

func findIn(fs []Finding, id string) Finding {
	if i := slices.IndexFunc(fs, func(x Finding) bool { return x.ID == id }); i >= 0 {
		return fs[i]
	}
	return Finding{}
}

func (s *sim) reviewer(p Packet, param byte) ReviewerBody {
	statuses := map[string]string{}
	var findings []FindingInput
	req := p.ApplicableRequirements
	if bit(param, 0) {
		statuses[req[0]] = RequirementViolated
		findings = append(findings, blocker("violation", req[0]))
	}
	if bit(param, 1) {
		statuses[req[len(req)-1]] = RequirementUnverified
	}
	if bit(param, 2) {
		findings = append(findings, improvement("improve"))
	}
	if bit(param, 6) {
		x := blocker("slow", "")
		x.Domain, x.Risk = DomainPerformance, "RISK-1"
		x.Proportionality = &Proportionality{ProposedChange: "cache", RegressionRisk: "staleness", DeferralCost: "rewrite"}
		findings = append(findings, x)
	}
	var resolved []Finding
	for _, x := range p.Findings {
		if x.Status == StatusResolved {
			resolved = append(resolved, x)
		}
	}
	if bit(param, 7) && len(resolved) > 0 {
		x := improvement("reversal")
		x.Tier, x.EvidenceNeeded, x.ReassessWhen = TierCalibration, "load data", "after the pilot"
		x.Contradicts = resolved[0].ID
		findings = append(findings, x)
	}
	reopen := bit(param, 4) && len(resolved) > 0
	if reopen && resolved[0].Tier.Blocking() && slices.Contains(req, resolved[0].Input.Requirement) {
		statuses[resolved[0].Input.Requirement] = RequirementViolated
	}
	b := review(p.Contract, "", statuses, findings...)
	blocking := slices.ContainsFunc(findings, func(x FindingInput) bool { return x.Tier.Blocking() })
	if reopen {
		b.Reopen = []Reopen{{Finding: resolved[0].ID, Reason: ReopenRegression, Justification: "it is back"}}
		blocking = blocking || resolved[0].Tier.Blocking()
	}
	if bit(param, 3) {
		b.ContractGaps = []ContractGap{{Title: "uncovered", Description: "a risk the contract misses"}}
	}
	switch {
	case blocking:
		b.Verdict = VerdictRed
	case bit(param, 1):
		b.Verdict = VerdictIndeterminate
	default:
		b.Verdict = VerdictGreen
	}
	if bit(param, 5) {
		b.Verdict = VerdictGreen
		if !blocking && !bit(param, 1) {
			b.Verdict = VerdictRed
		}
	}
	return b
}

func (s *sim) implementer(p Packet, param byte) ImplementerBody {
	var b ImplementerBody
	commit := s.history[len(s.history)-1].Hash
	if !bit(param, 3) {
		commit = s.commit(true)
	}
	components := []string{[]string{"parser", "lexer", "cli", "store"}[int(param>>4)%4]}
	if bit(param, 7) {
		components = append(components, "parser")
	}
	disputed := false
	for _, id := range p.Subjects {
		if bit(param, 0) && !disputed && !findIn(p.Findings, id).Disputed {
			disputed = true
			b.Disputes = append(b.Disputes, Dispute{Finding: id, Rationale: "not a defect"})
			continue
		}
		x := fix(id, commit, components...)
		if slices.Contains(p.RootCauseRequired, id) && !bit(param, 2) {
			x.RootCause = "shared assumption about empty input"
		}
		b.Fixes = append(b.Fixes, x)
	}
	if bit(param, 1) {
		for _, id := range p.Backlog {
			x := fix(id, commit, components...)
			if slices.Contains(p.RootCauseRequired, id) {
				x.RootCause = "r"
			}
			b.Fixes = append(b.Fixes, x)
		}
	}
	if bit(param, 6) {
		s.clean = false
	}
	return b
}

func (s *sim) verifier(p Packet, param byte) VerifierBody {
	results := map[string]string{}
	for i, id := range p.Subjects {
		results[id] = VerificationConfirmed
		if bit(param, 1) || (bit(param, 0) && i == 0) {
			results[id] = VerificationRejected
		}
	}
	subjects := p.Subjects
	if bit(param, 4) && len(subjects) > 0 {
		subjects = subjects[1:]
	}
	b := verify(results, subjects...)
	if len(p.Subjects) > 0 {
		if bit(param, 2) {
			x := blocker("regression", p.ApplicableRequirements[0])
			x.IntroducedBy = p.Subjects[0]
			b.Regressions = append(b.Regressions, x)
		}
		if bit(param, 3) {
			x := improvement("side effect")
			x.IntroducedBy = p.Subjects[len(p.Subjects)-1]
			b.Regressions = append(b.Regressions, x)
		}
	}
	return b
}

func (s *sim) auditor(p Packet, param byte) AuditorBody {
	b := AuditorBody{Resolutions: resolutions(p, "discharged")}
	for i, id := range p.Adjudicable {
		a := Adjudication{Finding: id, Decision: DecisionUphold, Rationale: "r"}
		switch (int(param) + i) % 4 {
		case 1:
			a.Decision = DecisionWithdraw
		case 2:
			a.Decision, a.Tier = DecisionReclassify, TierImprovement
			if !findIn(p.Findings, id).Tier.Blocking() {
				a.Tier = TierMaterial
			}
		}
		b.Adjudications = append(b.Adjudications, a)
	}
	if bit(param, 2) {
		b.Directives = []string{"investigate shared causes first"}
	}
	if bit(param, 3) && len(b.Resolutions) > 0 {
		b.Resolutions[0].Conclusion = ""
	}
	if bit(param, 4) && len(b.Resolutions) > 0 {
		b.Resolutions = b.Resolutions[1:]
	}
	return b
}

// check asserts the invariants that hold after every command.
func (s *sim) check(prev State) {
	s.t.Helper()
	f := s.facts()
	replayed, err := Replay(s.events)
	if err != nil {
		s.t.Fatalf("replay: %v", err)
	}
	if !reflect.DeepEqual(replayed, s.state) {
		s.t.Fatal("replay differs from the folded state")
	}
	open := 0
	for i, p := range s.state.Packets() {
		if p.Packet != seqID("P", i+1) {
			s.t.Fatalf("packet %d is %s", i+1, p.Packet)
		}
		if p.Open {
			open++
		}
	}
	if open > 1 {
		s.t.Fatalf("%d packets are open", open)
	}
	for i, x := range s.state.Findings() {
		if x.ID != seqID("F", i+1) {
			s.t.Fatalf("finding %d is %s", i+1, x.ID)
		}
	}
	for i, x := range s.state.Escalations() {
		if x.ID != seqID("E", i+1) {
			s.t.Fatalf("escalation %d is %s", i+1, x.ID)
		}
	}
	for i, r := range s.state.Rounds() {
		if r.Number != i+1 {
			s.t.Fatalf("round %d is numbered %d", i+1, r.Number)
		}
	}
	for _, x := range prev.Findings() {
		y, ok := s.state.Finding(x.ID)
		if !ok || y.Recurrences < x.Recurrences || (x.RootCauseRequired && !y.RootCauseRequired) ||
			!reflect.DeepEqual(x.History, y.History[:min(len(x.History), len(y.History))]) || len(y.History) < len(x.History) {
			s.t.Fatalf("finding %s went from %+v to %+v", x.ID, x, y)
		}
	}
	g := Gate(s.state, f)
	a := Next(s.state, f)
	if !reflect.DeepEqual(g, Gate(s.state, f)) || !reflect.DeepEqual(a, Next(s.state, f)) {
		s.t.Fatal("Gate or Next is not deterministic")
	}
	if f.PacketProblem != "" && a.Step == StepSubmitReport {
		s.t.Fatalf("next is submit-report for packet %s, whose file is unusable", a.Packet)
	}
	if (a.Step == StepNone) != g.Open {
		s.t.Fatalf("next is %s (%s) while the gate open is %t: %+v", a.Step, a.Reason, g.Open, g.Conditions)
	}
	if g.Open {
		for _, x := range s.state.Findings() {
			if x.Status.Unresolved() && x.Tier.Blocking() {
				s.t.Fatalf("the gate is open with %s %s %s", x.ID, x.Tier, x.Status)
			}
		}
		for _, e := range s.state.Escalations() {
			if e.Open {
				s.t.Fatalf("the gate is open with escalation %s open", e.ID)
			}
		}
		r, ok := s.state.LatestRound()
		pin, _ := s.state.Pin()
		if !ok || r.Verdict != VerdictGreen || r.Candidate.Digest != f.Candidate.Digest ||
			r.Contract != pin.Digest || pin.Digest != f.ContractDigest || !f.Clean {
			s.t.Fatalf("the gate is open without a GREEN verdict bound to the candidate and contract: %+v", r)
		}
	}
	if _, ok := a.Step.PacketRole(); ok || a.Step == StepSubmitReport {
		if _, err := PlanPacket(s.state, f); err != nil {
			s.t.Fatalf("next is %s but no packet can be planned: %v", a.Step, err)
		}
	}
}

func commandSeeds() [][]byte {
	happy := []byte{opChecks, 0, opPacket, 0, opAct, 0}
	red := []byte{opChecks, 0, opPacket, 0, opAct, 1, opPacket, 0, opAct, 0, opPacket, 0, opAct, 0, opChecks, 0, opPacket, 0, opAct, 0}
	var churn []byte
	for range 4 {
		churn = append(churn, opPacket, 0, opAct, 1, opPacket, 0, opAct, 0, opPacket, 0, opAct, 0, opPacket, 0, opAct, 0, opChecks, 0)
	}
	dispute := []byte{opChecks, 0, opPacket, 0, opAct, 1, opPacket, 0, opAct, 1, opPacket, 0, opAct, 0, opPacket, 0, opAct, 0, opPacket, 0, opAct, 0}
	withdraw := []byte{opChecks, 0, opPacket, 0, opAct, 1, opPacket, 0, opAct, 1, opPacket, 0, opAct, 1, opPacket, 0, opAct, 0}
	indeterminate := []byte{opChecks, 0, opPacket, 0, opAct, 2, opResolve, 0, opPacket, 0, opAct, 0}
	repin := []byte{opChecks, 0, opPacket, 0, opAct, 0, opContract, 1, opPin, 0, opPacket, 0, opContract, 3, opPin, 0, opPacket, 0, opAct, 0}
	stage := []byte{opChecks, 0, opPacket, 0, opAct, 1, opContract, 2, opPin, 0, opPacket, 0, opAct, 0, opPacket, 0, opAct, 0, opChecks, 0, opPacket, 0, opAct, 0}
	spiral := []byte{opChecks, 0, opPacket, 0, opAct, 1, opPacket, 0, opAct, 0, opPacket, 0, opAct, 4, opPacket, 0, opAct, 0}
	lost := []byte{opChecks, 0, opPacket, 0, opLose, 0, opAct, 0, opPacket, 0, opAct, 0}
	misc := []byte{opTree, 0, opChecks, 0, opTree, 1, opChecks, 2, opPacket, 1, opPacket, 2, opPacket, 0, opImpostor, 0x19, opReingest, 1, opAct, 0, opReingest, 0, opReingest, 1, opCommit, 1, opPin, 1}
	return [][]byte{happy, red, churn, dispute, withdraw, indeterminate, repin, stage, spiral, misc, lost}
}

func FuzzCommands(f *testing.F) {
	for _, seed := range commandSeeds() {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		s := &sim{harness: newHarness(t), t: t}
		s.check(State{})
		for i := 0; i+1 < len(data) && i < 240; i += 2 {
			prev := s.state
			s.step(data[i], data[i+1])
			s.check(prev)
		}
	})
}

func TestCommandSeeds(t *testing.T) {
	for i, seed := range commandSeeds() {
		t.Run(fmt.Sprint(i), func(t *testing.T) {
			s := &sim{harness: newHarness(t), t: t}
			for j := 0; j+1 < len(seed); j += 2 {
				prev := s.state
				s.step(seed[j], seed[j+1])
				s.check(prev)
			}
		})
	}
	// The cooperative seed reaches an open gate, and so does the seed that
	// loses the first packet's file, through a reissued packet.
	seeds := commandSeeds()
	for _, seed := range [][]byte{seeds[0], seeds[len(seeds)-1]} {
		s := &sim{harness: newHarness(t), t: t}
		for j := 0; j+1 < len(seed); j += 2 {
			s.step(seed[j], seed[j+1])
		}
		if !s.gate().Open {
			t.Fatalf("seed %v does not open the gate", seed)
		}
	}
	s := &sim{harness: newHarness(t), t: t}
	for j, seed := 0, seeds[len(seeds)-1]; j+1 < len(seed); j += 2 {
		s.step(seed[j], seed[j+1])
	}
	if p, _ := s.state.Packet("P-0001"); p.Open || p.Report != nil || p.VoidReason == "" {
		t.Fatalf("the packet with the lost file was not voided: %+v", p)
	}
}

// applyBases returns ledgers at several points of a flow that uses most event
// types.
func applyBases(tb testing.TB) [][]Event {
	h := newHarness(tb)
	h.checks(true)
	h.accept(h.packet(), "rev-1", review(h.contract, VerdictRed, map[string]string{"REQ-1": RequirementViolated},
		blocker("crash", "REQ-1"), improvement("long")))
	mid := len(h.events)
	h.accept(h.packet(), "impl-1", ImplementerBody{Disputes: []Dispute{{Finding: "F-0001", Rationale: "r"}}})
	ap := h.packet()
	h.accept(ap, "aud-1", AuditorBody{Resolutions: resolutions(ap, "c"), Directives: []string{"d"},
		Adjudications: []Adjudication{{Finding: "F-0001", Decision: DecisionReclassify, Tier: TierCritical, Rationale: "r"}}})
	h.accept(h.packet(), "impl-1", ImplementerBody{Fixes: []Fix{fix("F-0001", h.commit(true))}})
	return [][]Event{nil, h.events[:1], h.events[:2], h.events[:mid], h.events}
}

func FuzzApply(f *testing.F) {
	bases := applyBases(f)
	all := bases[len(bases)-1]
	for i, e := range all {
		f.Add(uint8(i), string(e.Type), []byte(e.Data))
	}
	f.Fuzz(func(t *testing.T, which uint8, typ string, data []byte) {
		events := bases[int(which)%len(bases)]
		s, err := Replay(events)
		if err != nil {
			t.Fatal(err)
		}
		e := Event{Seq: len(events) + 1, Type: EventType(typ), Data: json.RawMessage(data)}
		n, err := Apply(s, e)
		if again, _ := Replay(events); !reflect.DeepEqual(s, again) {
			t.Fatal("Apply modified its argument")
		}
		if err != nil {
			if !reflect.DeepEqual(n, State{}) {
				t.Fatal("a failed Apply returned a non-zero state")
			}
			return
		}
		replayed, err := Replay(append(slices.Clone(events), e))
		if err != nil || !reflect.DeepEqual(replayed, n) {
			t.Fatalf("Replay disagrees with Apply: %v", err)
		}
		// Whatever the ledger holds, the derived views never panic and the
		// gate is never open without a verdict.
		g := Gate(n, Facts{LedgerVerified: true})
		if g.Open {
			t.Fatal("the gate opened on facts without a contract")
		}
		_ = Next(n, Facts{LedgerVerified: true})
		var r *Rejection
		if _, _, err := Ingest(n, Facts{LedgerVerified: true}, Report{Packet: "P-0001"}, digestOf("a"), digestOf("b")); err != nil && !errors.As(err, &r) {
			t.Fatalf("ingest against a fuzzed ledger: %v", err)
		}
	})
}
