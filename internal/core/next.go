package core

import (
	"fmt"
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
// exactly when Gate reports the gate open.
func Next(s State, f Facts) Action {
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
		return Action{Step: StepAdjudicate, Owner: RoleGovernance, Escalations: ids,
			Reason: fmt.Sprintf("%s await a governance decision: amend the contract and pin it, or dismiss with a rationale, then resolve each", strings.Join(ids, ", "))}
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
			Reason: fmt.Sprintf("the fixes of %s are claimed and must be verified", strings.Join(ids, ", "))}
	}
	if ids := s.findingIDs(func(x Finding) bool { return x.Status == StatusOpen && x.Tier.Blocking() }); len(ids) > 0 {
		return Action{Step: StepFix, Owner: RoleImplementer, Findings: ids,
			Reason: fmt.Sprintf("blocking findings %s are open: fix or dispute each", strings.Join(ids, ", "))}
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
		why = append(why, c.Reasons...)
	}
	if r := s.resubmission(f); r != "" {
		return Action{Step: StepCommitChanges, Owner: RoleImplementer, Reason: r}
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

// resubmission explains why the current candidate may not be reviewed again,
// or returns "". A candidate whose latest verdict under the pinned contract is
// not GREEN can be reviewed again only after an escalation has been resolved;
// re-running checks does not count as a change.
func (s State) resubmission(f Facts) string {
	pin, ok := s.pin()
	if !ok {
		return ""
	}
	for i := len(s.rounds) - 1; i >= 0; i-- {
		r := s.rounds[i]
		if r.Verdict == "" || r.Candidate.Digest != f.Candidate.Digest || r.Contract != pin.Digest {
			continue
		}
		if r.Verdict == VerdictGreen || r.verdictAt < s.resolvedAt {
			return ""
		}
		return fmt.Sprintf("candidate %s already received %s in round %d of stage %s under this contract, and no escalation has been resolved since: change the candidate and commit it",
			f.Candidate.Digest, r.Verdict, r.Number, r.Stage)
	}
	return ""
}
