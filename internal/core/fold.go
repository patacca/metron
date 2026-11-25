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
	if cur, ok := s.pin(); !ok || cur.Stage != p.Stage {
		s.epochStart = len(s.rounds)
		s.streak = 0
		s.convergenceRaised = false
		s.churn = nil
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
	for _, list := range [][]string{p.Subjects, p.Backlog, p.RootCauseRequired, p.Adjudicable} {
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
		if p.Round < 1 || p.Round > n || s.rounds[p.Round-1].Verdict == "" {
			return fmt.Errorf("%s packet %s names round %d, which has no verdict", p.Action, p.Packet, p.Round)
		}
		if p.Base != nil && *p.Base != s.rounds[p.Round-1].Candidate {
			return fmt.Errorf("the base of packet %s is not the candidate of round %d", p.Packet, p.Round)
		}
	}
	s.packets = append(s.packets, PacketRecord{PacketIssued: p, Open: true})
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
	r.verdictAt = s.applied
	if p.Verdict == VerdictGreen {
		s.streak = 0
		s.convergenceRaised = false
	} else {
		s.streak++
	}
	return nil
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
	if i := s.packetIndex(p.Packet); i < 0 || s.packets[i].Report == nil || !slices.Contains(raisers(p.Kind), s.packets[i].Action) {
		return fmt.Errorf("a %s escalation is not raised by the report for packet %q", p.Kind, p.Packet)
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
			return errors.New("a component-churn escalation names no component")
		}
		for _, c := range p.Components {
			s.churnEntry(c).raised = true
		}
	}
	if adjudicable(p.Kind) {
		for _, id := range p.Findings {
			s.record(&s.findings[s.findingIndex(id)], EventEscalationRaised, p.Packet, p.Reason)
		}
	}
	s.escalations = append(s.escalations, Escalation{
		ID: p.ID, Kind: p.Kind, Owner: p.Owner, Round: p.Round, Reason: p.Reason,
		Findings: p.Findings, Components: p.Components, Requirements: p.Requirements, Open: true,
	})
	return nil
}

// raisers lists the actions whose report raises an escalation of kind k.
func raisers(k EscalationKind) []Step {
	switch k {
	case EscalationDispute, EscalationComponentChurn:
		return []Step{StepFix}
	case EscalationContradiction:
		return []Step{StepReview, StepVerify}
	case EscalationRegressionSpiral:
		return []Step{StepVerify}
	}
	return []Step{StepReview}
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
	if adjudicable(e.Kind) {
		for _, id := range e.Findings {
			s.record(&s.findings[s.findingIndex(id)], EventEscalationResolved, p.Packet, p.Resolution)
		}
	}
	s.decisions = append(s.decisions, Decision{Source: DecisionResolution, Escalation: p.ID, Packet: p.Packet, By: p.By, Text: p.Resolution})
	s.resolvedAt = s.applied
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
	s.decisions = append(s.decisions, Decision{Source: DecisionDirective, Packet: p.Packet, By: RoleAuditor, Text: p.Text})
	return nil
}
