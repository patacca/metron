package core

import (
	"errors"
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
	// The packet exists: answered checked it. A finding is established
	// under the contract its packet was issued under.
	s.findings = append(s.findings, Finding{
		ID: p.ID, Status: StatusOpen, Tier: p.Input.Tier, Round: p.Round, Source: p.Source, Input: p.Input,
		Contract: s.packets[s.packetIndex(p.Packet)].Contract,
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
	// A fix is claimed for an open finding, or claimed again for one whose
	// claim was made on another candidate.
	i := s.findingIndex(p.ID)
	if i < 0 {
		return fmt.Errorf("finding %q does not exist", p.ID)
	}
	f := &s.findings[i]
	if f.Status != StatusOpen && f.Status != StatusClaimed {
		return fmt.Errorf("finding %s is %s, not %s or %s", p.ID, f.Status, StatusOpen, StatusClaimed)
	}
	if p.Fix.Finding != p.ID {
		return fmt.Errorf("the fix names finding %q, not %s", p.Fix.Finding, p.ID)
	}
	if !ValidDigest(p.Candidate) {
		return fmt.Errorf("candidate digest %q is malformed", p.Candidate)
	}
	if len(p.Paths) == 0 {
		return fmt.Errorf("the fix of %s lists no changed path: a fix changes the candidate", p.ID)
	}
	fix := p.Fix
	f.Status = StatusClaimed
	f.Fix = &fix
	s.record(f, EventFindingFixClaimed, p.Packet, p.Candidate, fix.Summary)
	s.claimedOn = p.Candidate
	for _, c := range fix.Commits {
		if !slices.Contains(s.accounted, c) {
			s.accounted = append(s.accounted, c)
		}
	}
	for _, path := range p.Paths {
		s.noteChurn(path, p.Round)
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
	if err := checkScope(p.Scope, p.ScopeEvidence); err != nil {
		return err
	}
	if p.Scope != ScopeProportionate {
		return fmt.Errorf("the confirmed fix of %s has scope %q: only a proportionate fix is confirmed", p.ID, p.Scope)
	}
	f.Status = StatusResolved
	s.record(f, EventFindingFixConfirmed, p.Packet, s.examined(p.Packet), p.Evidence)
	s.verify(p.Packet)
	return nil
}

func checkScope(scope, evidence string) error {
	if scope != ScopeProportionate && scope != ScopeExcessive {
		return fmt.Errorf("%q is not a scope: use %s or %s", scope, ScopeProportionate, ScopeExcessive)
	}
	if blank(evidence) {
		return errors.New("the scope has no evidence")
	}
	return nil
}

// verify records the candidate a verification examined.
func (s *State) verify(packet string) {
	if i := s.packetIndex(packet); i >= 0 {
		c := s.packets[i].Candidate
		s.verified = &c
	}
}

func (s *State) onAddressed(data []byte) error {
	p, err := payload[FindingAddressed](data)
	if err != nil {
		return err
	}
	if err := s.answered(p.Packet, StepReview); err != nil {
		return err
	}
	f, err := s.findingIn(p.ID, StatusOpen)
	if err != nil {
		return err
	}
	switch {
	case f.Tier.Blocking():
		return fmt.Errorf("finding %s is %s: a blocking finding is resolved by a verified fix", p.ID, f.Tier)
	case blank(p.Evidence):
		return errors.New("the evidence is blank")
	}
	f.Status = StatusResolved
	s.record(f, EventFindingAddressed, p.Packet, s.examined(p.Packet), p.Evidence)
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
	if err := checkScope(p.Scope, p.ScopeEvidence); err != nil {
		return err
	}
	f.Status = StatusOpen
	f.Recurrences++
	f.RootCauseRequired = f.RootCauseRequired || p.RootCauseRequired
	note := p.Evidence
	if p.Scope == ScopeExcessive {
		note = "the fix changes more than the finding needs: " + p.ScopeEvidence + " | " + p.Evidence
	}
	s.record(f, EventFindingFixRejected, p.Packet, s.examined(p.Packet), note)
	s.verify(p.Packet)
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

func (s *State) churnEntry(path string) *churn {
	for i := range s.churn {
		if s.churn[i].path == path {
			return &s.churn[i]
		}
	}
	s.churn = append(s.churn, churn{path: path})
	return &s.churn[len(s.churn)-1]
}

func (s *State) noteChurn(path string, round int) {
	c := s.churnEntry(path)
	if !slices.Contains(c.rounds, round) {
		// Clip so that a state sharing this backing array is never touched.
		c.rounds = append(slices.Clip(c.rounds), round)
	}
}

// churnRounds returns the distinct rounds of the current campaign in which
// a fix changed a path, and whether churn was raised for it.
func (s State) churnRounds(path string) ([]int, bool) {
	for _, c := range s.churn {
		if c.path == path {
			return c.rounds, c.raised
		}
	}
	return nil, false
}
