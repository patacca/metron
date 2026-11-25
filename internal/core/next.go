package core

import (
	"fmt"
	"slices"
	"strings"
)

// Action is the single next permitted action.
type Action struct {
	Step Step `json:"step"`
	// Owner is the role that performs the step. It is empty for StepNone.
	Owner  Role   `json:"owner,omitempty"`
	Reason string `json:"reason"`
	// Packet, Findings, Escalations and Requirements identify what the step
	// is about, when it is about something.
	Packet       string   `json:"packet,omitempty"`
	Findings     []string `json:"findings,omitempty"`
	Escalations  []string `json:"escalations,omitempty"`
	Requirements []string `json:"requirements,omitempty"`
}

// Next returns the single next action: the first row of the table in
// docs/architecture.md, section 8, whose condition holds. It returns StepNone
// exactly when Gate reports the gate open. When an open packet can no longer
// be answered, the reason says so, in the words of PacketInvalid.
func Next(s State, f Facts) Action {
	a := next(s, f)
	if a.Step == StepRepairLedger {
		return a
	}
	if why := PacketInvalid(s, f); why != "" {
		op, _ := s.openPacket()
		a.Reason = fmt.Sprintf("packet %s can no longer be answered (%s) and the next packet voids it; %s", op.Packet, why, a.Reason)
	}
	return a
}

func next(s State, f Facts) Action {
	if p := ledgerProblem(s, f); p != "" {
		return Action{Step: StepRepairLedger, Owner: RoleGovernance, Reason: p}
	}
	if p := contractProblem(s, f); p != "" {
		return Action{Step: StepPinContract, Owner: RoleGovernance, Reason: p}
	}
	if op, ok := s.openPacket(); ok && packetInvalid(s, f, op) == "" {
		return Action{Step: StepSubmitReport, Owner: op.Role, Packet: op.Packet,
			Reason: fmt.Sprintf("packet %s (%s) is open and still valid: the %s submits its report", op.Packet, op.Action, op.Role)}
	}
	if ids := s.openEscalations(RoleGovernance); len(ids) > 0 {
		reason := fmt.Sprintf("%s %s a governance decision: amend the contract and pin it, or dismiss with a rationale, then resolve %s",
			strings.Join(ids, ", "), pick(len(ids), "awaits", "await"), pick(len(ids), "it", "each"))
		if e, suspended := s.suspension(); suspended {
			others := slices.DeleteFunc(slices.Clone(ids), func(id string) bool { return id == e.ID })
			reason = fmt.Sprintf("the campaign is suspended by %s (%s) and no role may continue: governance decides whether to resume it, to amend the contract and pin it first, or to leave it suspended; resolving %s resumes it",
				e.ID, e.Reason, e.ID)
			if len(others) > 0 {
				reason += fmt.Sprintf("; %s also %s a governance decision", strings.Join(others, ", "), pick(len(others), "awaits", "await"))
			}
		}
		return Action{Step: StepAdjudicate, Owner: RoleGovernance, Escalations: ids, Reason: reason}
	}
	if !f.Clean {
		return Action{Step: StepCommitChanges, Owner: RoleImplementer,
			Reason: "the working tree has changes outside .metron/: commit or discard them"}
	}
	if ids := s.openEscalations(RoleAuditor); len(ids) > 0 {
		return Action{Step: StepAudit, Owner: RoleAuditor, Escalations: ids,
			Reason: fmt.Sprintf("%s must be discharged by an audit", strings.Join(ids, ", "))}
	}
	if ids := s.findingIDs(func(x Finding) bool { return x.Status == StatusClaimed }); len(ids) > 0 {
		return Action{Step: StepVerify, Owner: RoleVerifier, Findings: ids,
			Reason: fmt.Sprintf("the %s of %s %s claimed and must be verified", pick(len(ids), "fix", "fixes"), strings.Join(ids, ", "), pick(len(ids), "is", "are"))}
	}
	if ids := s.findingIDs(func(x Finding) bool { return x.Status == StatusOpen && x.Tier.Blocking() }); len(ids) > 0 {
		return Action{Step: StepFix, Owner: RoleImplementer, Findings: ids, Reason: s.fixReason(ids)}
	}
	if ids := s.findingIDs(func(x Finding) bool { return x.carriedBy(f.Candidate.Digest) }); len(ids) > 0 {
		return Action{Step: StepCommitChanges, Owner: RoleImplementer, Findings: ids,
			Reason: fmt.Sprintf("blocking %s %s %s recorded on candidate %s, and the %s that resolved %s %s confirmed on another, so this candidate can be neither approved nor reviewed again: move to a candidate that contains the %s and commit it",
				pick(len(ids), "finding", "findings"), strings.Join(ids, ", "), pick(len(ids), "was", "were"), f.Candidate.Digest,
				pick(len(ids), "fix", "fixes"), pick(len(ids), "it", "them"), pick(len(ids), "was", "were"), pick(len(ids), "fix", "fixes"))}
	}
	if ids, reasons := missingChecks(s, *f.Contract, f.Candidate.Digest); len(ids) > 0 {
		return Action{Step: StepRunChecks, Owner: RoleImplementer, Requirements: ids,
			Reason: strings.Join(reasons, "; ") + ": make the checks pass on a committed candidate and record them"}
	}
	g := Gate(s, f)
	if g.Open {
		return Action{Step: StepNone, Reason: "the gate is open"}
	}
	var why []string
	for _, c := range g.Conditions {
		for _, r := range c.Reasons {
			if !slices.Contains(why, r) {
				why = append(why, r)
			}
		}
	}
	return Action{Step: StepReview, Owner: RoleReviewer,
		Reason: fmt.Sprintf("candidate %s has no GREEN verdict under the pinned contract (%s): review it", f.Candidate.Digest, strings.Join(why, "; "))}
}

// openEscalations returns the identifiers of the open escalations owned by
// role, in identifier order.
func (s State) openEscalations(owner Role) []string {
	var ids []string
	for _, e := range s.escalations {
		if e.Open && e.Owner == owner {
			ids = append(ids, e.ID)
		}
	}
	return ids
}

// findingIDs returns the identifiers of the findings that satisfy keep, in
// identifier order.
func (s State) findingIDs(keep func(Finding) bool) []string {
	var ids []string
	for _, x := range s.findings {
		if keep(x) {
			ids = append(ids, x.ID)
		}
	}
	return ids
}

// latestEvidence returns the latest result recorded for a requirement on a
// candidate with the requirement's current command text.
func (s State) latestEvidence(r Requirement, candidate string) (Evidence, bool) {
	for i := len(s.evidence) - 1; i >= 0; i-- {
		e := s.evidence[i]
		if e.Requirement == r.ID && e.Candidate == candidate && e.Command == r.Command {
			return e, true
		}
	}
	return Evidence{}, false
}

// missingChecks returns the applicable requirements with a command whose
// latest result on the candidate is missing or failing, with a reason each.
func missingChecks(s State, c Contract, candidate string) ([]string, []string) {
	var ids, reasons []string
	for _, r := range c.Applicable() {
		if r.Command == "" {
			continue
		}
		e, ok := s.latestEvidence(r, candidate)
		switch {
		case !ok:
			reasons = append(reasons, fmt.Sprintf("%s has no recorded result on this candidate", r.ID))
		case !e.Passed:
			reasons = append(reasons, fmt.Sprintf("%s failed on this candidate (exit %d)", r.ID, e.ExitCode))
		default:
			continue
		}
		ids = append(ids, r.ID)
	}
	return ids, reasons
}

// pick returns the singular or the plural form for n items.
func pick(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}

// fixReason names the open blocking findings and offers a dispute only for
// those that have not been disputed under the pinned contract.
func (s State) fixReason(ids []string) string {
	var disputed, open []string
	for _, id := range ids {
		if s.findings[s.findingIndex(id)].Disputed {
			disputed = append(disputed, id)
		} else {
			open = append(open, id)
		}
	}
	n := len(ids)
	list := strings.Join(ids, ", ")
	switch {
	case len(disputed) == 0:
		return fmt.Sprintf("blocking %s %s %s open: fix or dispute %s", pick(n, "finding", "findings"), list, pick(n, "is", "are"), pick(n, "it", "each"))
	case len(open) == 0:
		return fmt.Sprintf("blocking %s %s %s open and %s already disputed: fix %s", pick(n, "finding", "findings"), list,
			pick(n, "is", "are"), pick(n, "was", "were"), pick(n, "it", "each"))
	}
	return fmt.Sprintf("blocking findings %s are open: fix %s, which %s already disputed, and fix or dispute %s",
		list, strings.Join(disputed, ", "), pick(len(disputed), "was", "were"), strings.Join(open, ", "))
}
