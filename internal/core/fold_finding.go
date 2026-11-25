package core

import (
	"fmt"
	"slices"
)

// The fold of the finding events, and the per-finding records it keeps:
// history and component churn.

func (s *State) onFindingOpened(data []byte) error {
	p, err := payload[FindingOpened](data)
	if err != nil {
		return err
	}
	if want := seqID("F", len(s.findings)+1); p.ID != want {
		return fmt.Errorf("finding %q opened; the next finding identifier is %s", p.ID, want)
	}
	var action Step
	switch p.Source {
	case RoleReviewer:
		action = StepReview
	case RoleVerifier:
		action = StepVerify
	default:
		return fmt.Errorf("a %q cannot open a finding", p.Source)
	}
	if err := s.answered(p.Packet, action); err != nil {
		return err
	}
	switch {
	case !p.Input.Tier.Valid():
		return fmt.Errorf("%q is not a tier", p.Input.Tier)
	case !p.Input.Domain.Valid():
		return fmt.Errorf("%q is not a review domain", p.Input.Domain)
	case p.Round < 0:
		return fmt.Errorf("round %d is negative", p.Round)
	}
	s.findings = append(s.findings, Finding{
		ID: p.ID, Status: StatusOpen, Tier: p.Input.Tier, Round: p.Round, Source: p.Source, Input: p.Input,
	})
	s.record(&s.findings[len(s.findings)-1], EventFindingOpened, p.Packet, s.examined(p.Packet), "")
	// A blocking regression makes its verification a failed step.
	if p.Source == RoleVerifier && p.Input.Tier.Blocking() {
		s.fail()
	}
	return nil
}

func (s *State) onFixClaimed(data []byte) error {
	p, err := payload[FindingFixClaimed](data)
	if err != nil {
		return err
	}
	if err := s.answered(p.Packet, StepFix); err != nil {
		return err
	}
	f, err := s.findingIn(p.ID, StatusOpen)
	if err != nil {
		return err
	}
	if p.Fix.Finding != p.ID {
		return fmt.Errorf("the fix names finding %q, not %s", p.Fix.Finding, p.ID)
	}
	if !ValidDigest(p.Candidate) {
		return fmt.Errorf("candidate digest %q is malformed", p.Candidate)
	}
	fix := p.Fix
	f.Status = StatusClaimed
	f.Fix = &fix
	s.record(f, EventFindingFixClaimed, p.Packet, p.Candidate, fix.Summary)
	for _, c := range fix.Components {
		s.noteChurn(c, p.Round)
	}
	return nil
}

func (s *State) onFixConfirmed(data []byte) error {
	p, err := payload[FindingFixConfirmed](data)
	if err != nil {
		return err
	}
	if err := s.answered(p.Packet, StepVerify); err != nil {
		return err
	}
	f, err := s.findingIn(p.ID, StatusClaimed)
	if err != nil {
		return err
	}
	f.Status = StatusResolved
	s.record(f, EventFindingFixConfirmed, p.Packet, s.examined(p.Packet), p.Evidence)
	return nil
}

func (s *State) onFixRejected(data []byte) error {
	p, err := payload[FindingFixRejected](data)
	if err != nil {
		return err
	}
	if err := s.answered(p.Packet, StepVerify); err != nil {
		return err
	}
	f, err := s.findingIn(p.ID, StatusClaimed)
	if err != nil {
		return err
	}
	f.Status = StatusOpen
	f.Recurrences++
	f.RootCauseRequired = f.RootCauseRequired || p.RootCauseRequired
	s.record(f, EventFindingFixRejected, p.Packet, s.examined(p.Packet), p.Evidence)
	s.fail()
	return nil
}

func (s *State) onReopened(data []byte) error {
	p, err := payload[FindingReopened](data)
	if err != nil {
		return err
	}
	if err := s.answered(p.Packet, StepReview); err != nil {
		return err
	}
	if !p.Reason.Valid() {
		return fmt.Errorf("%q is not a reopen reason", p.Reason)
	}
	f, err := s.findingIn(p.ID, StatusResolved)
	if err != nil {
		return err
	}
	f.Status = StatusOpen
	f.Recurrences++
	f.RootCauseRequired = f.RootCauseRequired || p.RootCauseRequired
	s.record(f, EventFindingReopened, p.Packet, s.examined(p.Packet), fmt.Sprintf("%s: %s", p.Reason, p.Justification))
	return nil
}

func (s *State) onWithdrawn(data []byte) error {
	p, err := payload[FindingWithdrawn](data)
	if err != nil {
		return err
	}
	if err := s.answered(p.Packet, StepAudit); err != nil {
		return err
	}
	i := s.findingIndex(p.ID)
	if i < 0 {
		return fmt.Errorf("finding %q does not exist", p.ID)
	}
	if !s.findings[i].Status.Unresolved() {
		return fmt.Errorf("finding %s is %s; only an unresolved finding can be withdrawn", p.ID, s.findings[i].Status)
	}
	s.findings[i].Status = StatusWithdrawn
	s.record(&s.findings[i], EventFindingWithdrawn, p.Packet, s.examined(p.Packet), p.Rationale)
	return nil
}

func (s *State) onReclassified(data []byte) error {
	p, err := payload[FindingReclassified](data)
	if err != nil {
		return err
	}
	if err := s.answered(p.Packet, StepAudit); err != nil {
		return err
	}
	i := s.findingIndex(p.ID)
	if i < 0 {
		return fmt.Errorf("finding %q does not exist", p.ID)
	}
	f := &s.findings[i]
	switch {
	case f.Status == StatusWithdrawn:
		return fmt.Errorf("finding %s is withdrawn", p.ID)
	case p.From != f.Tier:
		return fmt.Errorf("finding %s is %s, not %s", p.ID, f.Tier, p.From)
	case !p.To.Valid() || p.To == p.From:
		return fmt.Errorf("finding %s cannot be reclassified from %s to %q", p.ID, p.From, p.To)
	}
	f.Tier = p.To
	s.record(f, EventFindingReclassified, p.Packet, s.examined(p.Packet), fmt.Sprintf("%s to %s: %s", p.From, p.To, p.Rationale))
	return nil
}

// record appends a step to a finding's history, at the number of the latest
// round in the ledger (rounds are numbered 1..n, so that is n, or 0 before
// any round) and with the candidate the step was made on. It clips the
// history first so that a state sharing its backing array is never touched.
func (s *State) record(f *Finding, event EventType, packet, candidate, note string) {
	f.History = append(slices.Clip(f.History), FindingEvent{
		Event: event, Round: len(s.rounds), Packet: packet, Candidate: candidate, Note: note,
	})
}

// examined returns the digest of the candidate a read-only role examined
// under a packet: the candidate the packet is bound to, which its report
// could not be accepted on any other. It returns "" for a packet that does
// not bind its report to a candidate.
func (s State) examined(packet string) string {
	if i := s.packetIndex(packet); i >= 0 && s.packets[i].Role.ReadOnly() {
		return s.packets[i].Candidate.Digest
	}
	return ""
}

// findingIn returns the finding id, which must be in status want.
func (s *State) findingIn(id string, want FindingStatus) (*Finding, error) {
	i := s.findingIndex(id)
	if i < 0 {
		return nil, fmt.Errorf("finding %q does not exist", id)
	}
	if got := s.findings[i].Status; got != want {
		return nil, fmt.Errorf("finding %s is %s, not %s", id, got, want)
	}
	return &s.findings[i], nil
}

func (s *State) churnEntry(component string) *churn {
	for i := range s.churn {
		if s.churn[i].component == component {
			return &s.churn[i]
		}
	}
	s.churn = append(s.churn, churn{component: component})
	return &s.churn[len(s.churn)-1]
}

func (s *State) noteChurn(component string, round int) {
	c := s.churnEntry(component)
	if !slices.Contains(c.rounds, round) {
		// Clip so that a state sharing this backing array is never touched.
		c.rounds = append(slices.Clip(c.rounds), round)
	}
}

// churnRounds returns the distinct rounds of the current stage in which a
// component was named in a fix, and whether churn was raised for it.
func (s State) churnRounds(component string) ([]int, bool) {
	for _, c := range s.churn {
		if c.component == component {
			return c.rounds, c.raised
		}
	}
	return nil, false
}
