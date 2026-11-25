package core

import (
	"errors"
	"fmt"
	"slices"
)

// The fold of the events that concern the ledger, the contract, evidence,
// rounds, packets, reports, verdicts, escalations and directives.

func (s *State) onInitialized(data []byte) error {
	p, err := payload[LedgerInitialized](data)
	if err != nil {
		return err
	}
	if s.initialized {
		return errors.New("the ledger is already initialized")
	}
	if p.Protocol != ProtocolVersion {
		return fmt.Errorf("the ledger was written under protocol %q; this binary implements %q", p.Protocol, ProtocolVersion)
	}
	s.initialized = true
	s.protocol = p.Protocol
	return nil
}

func (s *State) onPinned(data []byte) error {
	p, err := payload[ContractPinned](data)
	if err != nil {
		return err
	}
	switch {
	case !ValidDigest(p.Digest):
		return fmt.Errorf("contract digest %q is malformed", p.Digest)
	case !ValidDigest(p.RawDigest):
		return fmt.Errorf("raw contract digest %q is malformed", p.RawDigest)
	case !p.Stage.Valid():
		return fmt.Errorf("%q is not a lifecycle stage", p.Stage)
	case blank(p.Reason):
		return errors.New("the pin has no reason")
	case blank(p.By):
		return errors.New("the pin names no identity")
	}
	if op, open := s.openPacket(); open {
		return fmt.Errorf("packet %s is still open; a pin must void it first", op.Packet)
	}
	// A finding is judged against a new contract now, so it may be disputed
	// again.
	for i := range s.findings {
		s.findings[i].Disputed = false
	}
	// A new stage is a new question for the gate, which then looks only at
	// the rounds held since. It is not a new campaign: what the loop has
	// accumulated towards an escalation stays.
	if cur, ok := s.pin(); !ok || cur.Stage != p.Stage {
		s.epochStart = len(s.rounds)
	}
	s.pins = append(s.pins, p)
	return nil
}

func (s *State) onEvidence(data []byte) error {
	p, err := payload[Evidence](data)
	if err != nil {
		return err
	}
	if _, ok := s.pin(); !ok {
		return errors.New("evidence was recorded before any contract was pinned")
	}
	switch {
	case blank(p.Requirement):
		return errors.New("the evidence names no requirement")
	case blank(p.Command):
		return errors.New("the evidence names no command")
	case !ValidDigest(p.Candidate):
		return fmt.Errorf("candidate digest %q is malformed", p.Candidate)
	case !ValidDigest(p.OutputDigest):
		return fmt.Errorf("output digest %q is malformed", p.OutputDigest)
	case p.Passed != (p.ExitCode == 0):
		return fmt.Errorf("passed is %t but the exit code is %d", p.Passed, p.ExitCode)
	}
	s.evidence = append(s.evidence, p)
	if !p.Passed {
		s.fail()
	}
	return nil
}

func (s *State) onRoundOpened(data []byte) error {
	p, err := payload[RoundOpened](data)
	if err != nil {
		return err
	}
	pin, ok := s.pin()
	if !ok {
		return errors.New("a round opened before any contract was pinned")
	}
	if op, open := s.openPacket(); open {
		return fmt.Errorf("packet %s is open; a round opens only with a new review packet", op.Packet)
	}
	if err := s.danglingRound(); err != nil {
		return err
	}
	switch {
	case p.Protocol != ProtocolVersion:
		return fmt.Errorf("the round is under protocol %q, not %q", p.Protocol, ProtocolVersion)
	case p.Stage != pin.Stage:
		return fmt.Errorf("the round is for stage %q but stage %q is pinned", p.Stage, pin.Stage)
	case p.Contract != pin.Digest:
		return fmt.Errorf("the round is under contract %s but %s is pinned", p.Contract, pin.Digest)
	case p.Round != len(s.rounds)+1:
		return fmt.Errorf("round %d opened; the next round is %d", p.Round, len(s.rounds)+1)
	case !validCandidate(p.Candidate):
		return fmt.Errorf("the candidate cannot be identified: %s", candidateProblem(p.Candidate))
	}
	s.rounds = append(s.rounds, Round{Number: p.Round, Stage: p.Stage, Candidate: p.Candidate, Contract: p.Contract})
	return nil
}

func (s *State) onPacketIssued(data []byte) error {
	p, err := payload[PacketIssued](data)
	if err != nil {
		return err
	}
	pin, ok := s.pin()
	if !ok {
		return errors.New("a packet was issued before any contract was pinned")
	}
	if op, open := s.openPacket(); open {
		return fmt.Errorf("packet %s is still open; at most one packet is open at a time", op.Packet)
	}
	if want := seqID("P", len(s.packets)+1); p.Packet != want {
		return fmt.Errorf("packet %q issued; the next packet identifier is %s", p.Packet, want)
	}
	if role, ok := p.Action.PacketRole(); !ok || role != p.Role {
		return fmt.Errorf("action %q cannot be issued to role %q", p.Action, p.Role)
	}
	switch {
	case p.Protocol != ProtocolVersion:
		return fmt.Errorf("the packet is under protocol %q, not %q", p.Protocol, ProtocolVersion)
	case p.Contract != pin.Digest:
		return fmt.Errorf("the packet is under contract %s but %s is pinned", p.Contract, pin.Digest)
	case !validCandidate(p.Candidate):
		return fmt.Errorf("the candidate cannot be identified: %s", candidateProblem(p.Candidate))
	case !ValidDigest(p.Digest):
		return fmt.Errorf("packet digest %q is malformed", p.Digest)
	case p.Round < 0:
		return fmt.Errorf("round %d is negative", p.Round)
	}
	for _, list := range [][]string{p.Subjects, p.RootCauseRequired, p.Adjudicable} {
		for _, id := range list {
			if s.findingIndex(id) < 0 {
				return fmt.Errorf("the packet cites finding %q, which does not exist", id)
			}
		}
	}
	for _, id := range p.Escalations {
		if s.escalationIndex(id) < 0 {
			return fmt.Errorf("the packet cites escalation %q, which does not exist", id)
		}
	}
	if (p.Base != nil) != (p.Action == StepFix || p.Action == StepVerify) {
		return fmt.Errorf("a %s packet has a base exactly when it is a fix or verify packet", p.Action)
	}
	if n := len(s.rounds); p.Action == StepReview {
		if n == 0 || s.rounds[n-1].Number != p.Round || s.rounds[n-1].Packet != "" || s.rounds[n-1].Candidate != p.Candidate {
			return fmt.Errorf("review packet %s does not follow the opening of round %d on its candidate", p.Packet, p.Round)
		}
		s.rounds[n-1].Packet = p.Packet
	} else {
		if err := s.danglingRound(); err != nil {
			return err
		}
		// An audit can be due before any round has a verdict, when the
		// checks keep failing; it then names round 0.
		_, reviewed := s.base()
		if before := p.Action == StepAudit && p.Round == 0 && !reviewed; !before && (p.Round < 1 || p.Round > n || s.rounds[p.Round-1].Verdict == "") {
			return fmt.Errorf("%s packet %s names round %d, which has no verdict", p.Action, p.Packet, p.Round)
		}
		if p.Base != nil && *p.Base != s.rounds[p.Round-1].Candidate {
			return fmt.Errorf("the base of packet %s is not the candidate of round %d", p.Packet, p.Round)
		}
	}
	s.packets = append(s.packets, PacketRecord{PacketIssued: p, Open: true})
	s.steps++
	return nil
}

func (s *State) onPacketVoided(data []byte) error {
	p, err := payload[PacketVoided](data)
	if err != nil {
		return err
	}
	i := s.packetIndex(p.Packet)
	if i < 0 {
		return fmt.Errorf("packet %q does not exist", p.Packet)
	}
	if !s.packets[i].Open {
		return fmt.Errorf("packet %s is not open", p.Packet)
	}
	if blank(p.Reason) {
		return errors.New("the void has no reason")
	}
	s.packets[i].Open = false
	s.packets[i].VoidReason = p.Reason
	if s.packets[i].Action == StepReview {
		for j := range s.rounds {
			if s.rounds[j].Packet == p.Packet {
				s.rounds[j].Voided = true
			}
		}
	}
	return nil
}

func (s *State) onReportAccepted(data []byte) error {
	p, err := payload[ReportAccepted](data)
	if err != nil {
		return err
	}
	i := s.packetIndex(p.Packet)
	if i < 0 {
		return fmt.Errorf("packet %q does not exist", p.Packet)
	}
	if !s.packets[i].Open {
		return fmt.Errorf("packet %s is not open", p.Packet)
	}
	switch {
	case p.Role != s.packets[i].Role:
		return fmt.Errorf("a %s report answers packet %s, which was issued to the %s", p.Role, p.Packet, s.packets[i].Role)
	case blank(p.Author.ID):
		return errors.New("the report names no author")
	case !ValidDigest(p.Digest):
		return fmt.Errorf("report digest %q is malformed", p.Digest)
	case !ValidDigest(p.RawDigest):
		return fmt.Errorf("raw report digest %q is malformed", p.RawDigest)
	case p.Provenance.Mode != ProvenanceDeclared:
		return fmt.Errorf("provenance mode %q is not supported", p.Provenance.Mode)
	}
	s.packets[i].Open = false
	s.packets[i].Report = &p
	if a := (actor{id: p.Author.ID, role: p.Role}); !slices.Contains(s.actors, a) {
		s.actors = append(s.actors, a)
	}
	return nil
}

func (s *State) onVerdict(data []byte) error {
	p, err := payload[VerdictRecorded](data)
	if err != nil {
		return err
	}
	if !p.Verdict.Valid() {
		return fmt.Errorf("%q is not a verdict", p.Verdict)
	}
	if p.Round <= s.epochStart || p.Round > len(s.rounds) {
		return fmt.Errorf("round %d does not exist in the current stage", p.Round)
	}
	r := &s.rounds[p.Round-1]
	if r.Verdict != "" || r.Voided || r.Packet == "" {
		return fmt.Errorf("round %d cannot receive a verdict", p.Round)
	}
	pi := s.packetIndex(r.Packet)
	if pi < 0 {
		return fmt.Errorf("round %d names packet %s, which does not exist", p.Round, r.Packet)
	}
	pk := s.packets[pi]
	switch {
	case pk.Report == nil:
		return fmt.Errorf("round %d has no accepted report", p.Round)
	case p.Reviewer != pk.Report.Author.ID:
		return fmt.Errorf("the verdict names reviewer %q but the report was written by %q", p.Reviewer, pk.Report.Author.ID)
	case p.Candidate != r.Candidate.Digest:
		return fmt.Errorf("the verdict is for candidate %s but round %d reviewed %s", p.Candidate, p.Round, r.Candidate.Digest)
	case p.Contract != r.Contract:
		return fmt.Errorf("the verdict is under contract %s but round %d is under %s", p.Contract, p.Round, r.Contract)
	}
	for _, rr := range p.Requirements {
		switch rr.Status {
		case RequirementSatisfied, RequirementViolated, RequirementUnverified:
		default:
			return fmt.Errorf("requirement status %q is not satisfied, violated or unverified", rr.Status)
		}
	}
	r.Verdict = p.Verdict
	r.Reviewer = p.Reviewer
	r.Requirements = p.Requirements
	s.verified = nil
	if p.Verdict == VerdictGreen {
		s.streak = 0
		s.convergenceRaised = false
	} else {
		s.fail()
	}
	return nil
}

// fail counts the batch of events being folded as one failed step.
func (s *State) fail() {
	if !s.counted {
		s.counted = true
		s.streak++
	}
}

// approved reports whether the campaign has ended in approval, as far as the
// ledger alone can tell: the latest round is a GREEN round of the current
// stage under the pinned contract, no escalation is open, and no blocking
// finding is unresolved or carried by that round's candidate. These are the
// gate conditions that do not depend on the working tree.
func (s State) approved() bool {
	n := len(s.rounds)
	pin, pinned := s.pin()
	if n == 0 || n <= s.epochStart || !pinned || s.rounds[n-1].Verdict != VerdictGreen || s.rounds[n-1].Contract != pin.Digest {
		return false
	}
	for _, e := range s.escalations {
		if e.Open {
			return false
		}
	}
	for _, x := range s.findings {
		if x.Tier.Blocking() && (x.Status.Unresolved() || x.carriedBy(s.rounds[n-1].Candidate.Digest)) {
			return false
		}
	}
	return true
}

// suspension returns the open suspension, if the campaign is suspended.
func (s State) suspension() (Escalation, bool) {
	for _, e := range s.escalations {
		if e.Open && e.Kind == EscalationSuspension {
			return e, true
		}
	}
	return Escalation{}, false
}

func (s *State) onEscalationRaised(data []byte) error {
	p, err := payload[EscalationRaised](data)
	if err != nil {
		return err
	}
	if want := seqID("E", len(s.escalations)+1); p.ID != want {
		return fmt.Errorf("escalation %q raised; the next escalation identifier is %s", p.ID, want)
	}
	owner := p.Kind.Owner()
	switch {
	case owner == "":
		return fmt.Errorf("%q is not an escalation kind", p.Kind)
	case p.Owner != owner:
		return fmt.Errorf("a %s escalation is owned by %s, not %q", p.Kind, owner, p.Owner)
	case blank(p.Reason):
		return errors.New("the escalation has no reason")
	case p.Round < 0:
		return fmt.Errorf("round %d is negative", p.Round)
	}
	if err := s.raiser(p); err != nil {
		return err
	}
	for _, id := range p.Findings {
		if s.findingIndex(id) < 0 {
			return fmt.Errorf("the escalation cites finding %q, which does not exist", id)
		}
	}
	switch p.Kind {
	case EscalationDispute:
		if len(p.Findings) != 1 {
			return errors.New("a dispute names exactly one finding")
		}
		f := &s.findings[s.findingIndex(p.Findings[0])]
		if f.Disputed {
			return fmt.Errorf("finding %s was already disputed", f.ID)
		}
		f.Disputed = true
	case EscalationNonConvergence:
		s.convergenceRaised = true
	case EscalationComponentChurn:
		if len(p.Components) == 0 {
			return errors.New("a component-churn escalation names no path")
		}
		for _, c := range p.Components {
			s.churnEntry(c).raised = true
		}
	}
	if p.Candidate != "" && !ValidDigest(p.Candidate) {
		return fmt.Errorf("candidate digest %q is malformed", p.Candidate)
	}
	if adjudicable(p.Kind) {
		for _, id := range p.Findings {
			s.record(&s.findings[s.findingIndex(id)], EventEscalationRaised, p.Packet, s.examined(p.Packet), p.Reason)
		}
	}
	s.escalations = append(s.escalations, Escalation{
		ID: p.ID, Kind: p.Kind, Owner: p.Owner, Round: p.Round, Reason: p.Reason, Candidate: p.Candidate,
		Findings: p.Findings, Components: p.Components, Requirements: p.Requirements, Open: true,
	})
	return nil
}

// raisers lists the actions whose report raises an escalation of kind k.
func raisers(k EscalationKind) []Step {
	switch k {
	case EscalationDispute, EscalationComponentChurn:
		return []Step{StepFix}
	case EscalationContradiction, EscalationNonConvergence, EscalationProcessAudit:
		return []Step{StepReview, StepVerify}
	case EscalationRegressionSpiral:
		return []Step{StepVerify}
	case EscalationSuspension:
		return []Step{StepAudit}
	}
	return []Step{StepReview}
}

// raiser checks what raised an escalation. A report raises it, with two
// exceptions: a failed check recording, which names no packet, raises the
// escalations of the failure streak, and the packet that uses up the campaign
// budget raises a suspension when it is issued.
func (s State) raiser(p EscalationRaised) error {
	if p.Packet == "" {
		if p.Kind == EscalationNonConvergence || p.Kind == EscalationProcessAudit {
			return nil
		}
		return fmt.Errorf("a %s escalation names the packet whose report raised it", p.Kind)
	}
	i := s.packetIndex(p.Packet)
	if i < 0 {
		return fmt.Errorf("a %s escalation names packet %q, which does not exist", p.Kind, p.Packet)
	}
	pk := s.packets[i]
	if p.Kind == EscalationSuspension && pk.Open && i == len(s.packets)-1 {
		return nil
	}
	if pk.Report == nil || !slices.Contains(raisers(p.Kind), pk.Action) {
		return fmt.Errorf("a %s escalation is not raised by the report for packet %q", p.Kind, p.Packet)
	}
	return nil
}

// adjudicable reports whether the findings of an escalation of kind k are
// decided on by the audit that discharges it.
func adjudicable(k EscalationKind) bool {
	return k == EscalationDispute || k == EscalationContradiction
}

func (s *State) onEscalationResolved(data []byte) error {
	p, err := payload[EscalationResolved](data)
	if err != nil {
		return err
	}
	i := s.escalationIndex(p.ID)
	if i < 0 {
		return fmt.Errorf("escalation %q does not exist", p.ID)
	}
	e := &s.escalations[i]
	switch {
	case !e.Open:
		return fmt.Errorf("escalation %s is not open", p.ID)
	case p.By != e.Owner:
		return fmt.Errorf("escalation %s is owned by %s, not %q", p.ID, e.Owner, p.By)
	case blank(p.Actor):
		return errors.New("the resolution names no actor")
	case blank(p.Resolution):
		return errors.New("the resolution is blank")
	}
	if p.By == RoleAuditor {
		if err := s.answered(p.Packet, StepAudit); err != nil {
			return err
		}
		if !slices.Contains(s.packets[s.packetIndex(p.Packet)].Escalations, p.ID) {
			return fmt.Errorf("packet %s does not cover escalation %s", p.Packet, p.ID)
		}
	} else if p.Packet != "" {
		return errors.New("a governance resolution names no packet")
	}
	e.Open = false
	e.Resolution = p.Resolution
	// Resolving a suspension is governance's decision to resume: the
	// campaign starts over with nothing counted against it.
	if e.Kind == EscalationSuspension {
		s.streak, s.convergenceRaised, s.steps, s.churn = 0, false, 0, nil
	}
	if adjudicable(e.Kind) {
		for _, id := range e.Findings {
			s.record(&s.findings[s.findingIndex(id)], EventEscalationResolved, p.Packet, s.examined(p.Packet), p.Resolution)
		}
	}
	// An auditor decides on the candidate it examined; governance decides
	// about the candidate the escalation was raised on.
	candidate := e.Candidate
	if p.By == RoleAuditor {
		candidate = s.examined(p.Packet)
	}
	s.decide(Decision{Source: DecisionResolution, Escalation: p.ID, Kind: e.Kind, Packet: p.Packet, By: p.By, Text: p.Resolution,
		Candidate: candidate, Requirements: e.Requirements})
	return nil
}

func (s *State) onDirective(data []byte) error {
	p, err := payload[DirectiveIssued](data)
	if err != nil {
		return err
	}
	if err := s.answered(p.Packet, StepAudit); err != nil {
		return err
	}
	if blank(p.Text) {
		return errors.New("the directive is blank")
	}
	s.decide(Decision{Source: DecisionDirective, Packet: p.Packet, By: RoleAuditor, Text: p.Text, Candidate: s.examined(p.Packet)})
	return nil
}

// decide records a decision with its identifier and the stage and contract
// pinned now, under which it binds.
func (s *State) decide(d Decision) {
	pin, _ := s.pin()
	d.ID, d.Stage, d.Contract = seqID("D", len(s.decisions)+1), pin.Stage, pin.Digest
	s.decisions = append(s.decisions, d)
}

func (s *State) onSuperseded(data []byte) error {
	p, err := payload[DecisionSuperseded](data)
	if err != nil {
		return err
	}
	if err := s.answered(p.Packet, StepAudit); err != nil {
		return err
	}
	i := slices.IndexFunc(s.decisions, func(d Decision) bool { return d.ID == p.Decision })
	switch {
	case i < 0:
		return fmt.Errorf("decision %q does not exist", p.Decision)
	case s.decisions[i].By != RoleAuditor:
		return fmt.Errorf("decision %s was made by %s: an audit supersedes only audit decisions", p.Decision, s.decisions[i].By)
	case s.decisions[i].SupersededBy != "":
		return fmt.Errorf("decision %s is already superseded", p.Decision)
	case blank(p.Rationale):
		return errors.New("the supersession has no rationale")
	}
	s.decisions[i].SupersededBy = p.Packet
	return nil
}
