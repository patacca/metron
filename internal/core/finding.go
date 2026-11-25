package core

import "slices"

// FindingInput is a finding as written by a reviewer or a verifier.
//
// Which members are required depends on the tier; see docs/architecture.md,
// section 9.
type FindingInput struct {
	// Key identifies the finding within the report that introduces it.
	Key    string `json:"key"`
	Tier   Tier   `json:"tier"`
	Domain Domain `json:"domain"`
	Title  string `json:"title"`
	// Components are the affected files, modules or subsystems.
	Components []string `json:"components"`
	// Description states the concrete failure mechanism.
	Description string `json:"description"`
	// Evidence is a reproduction, an analysis or a measurement.
	Evidence string `json:"evidence"`
	// Impact is the expected effect inside the operating envelope.
	Impact string `json:"impact"`

	// Requirement and Risk tie a blocking finding to the contract. A blocking
	// finding sets at least one.
	Requirement string `json:"requirement,omitempty"`
	Risk        string `json:"risk,omitempty"`
	// BlockingRationale says why resolution is necessary before this stage.
	BlockingRationale string `json:"blocking_rationale,omitempty"`
	// Verification says how a fix can be verified.
	Verification string `json:"verification,omitempty"`
	// Proportionality is required for blocking findings in the domains for
	// which Domain.NeedsProportionality is true.
	Proportionality *Proportionality `json:"proportionality,omitempty"`

	// ExpectedBenefit and DeferralRationale are required for T2.
	ExpectedBenefit   string `json:"expected_benefit,omitempty"`
	DeferralRationale string `json:"deferral_rationale,omitempty"`
	// EvidenceNeeded and ReassessWhen are required for T3.
	EvidenceNeeded string `json:"evidence_needed,omitempty"`
	ReassessWhen   string `json:"reassess_when,omitempty"`

	// IntroducedBy names the finding whose fix introduced this one. It is
	// required for regressions reported by a verifier.
	IntroducedBy string `json:"introduced_by,omitempty"`
	// Contradicts names a finding whose resolution this finding would reverse.
	Contradicts string `json:"contradicts,omitempty"`
	// Related names other findings that share a cause or a component.
	Related []string `json:"related,omitempty"`
}

// Proportionality justifies blocking on an improvement-prone domain.
type Proportionality struct {
	// ProposedChange is the smallest change that would resolve the finding.
	ProposedChange string `json:"proposed_change"`
	// RegressionRisk is what the change could break.
	RegressionRisk string `json:"regression_risk"`
	// DeferralCost is why resolving it later would cost substantially more.
	DeferralCost string `json:"deferral_cost"`
}

// Finding is a finding as tracked by the ledger.
type Finding struct {
	// ID is "F-" followed by a zero-padded sequence number.
	ID     string        `json:"id"`
	Status FindingStatus `json:"status"`
	// Tier is the current tier. It differs from Input.Tier after an auditor
	// reclassifies the finding.
	Tier Tier `json:"tier"`
	// Round is the round in which the finding was opened. Round numbers run
	// from 1 across the whole ledger. A review packet leaves it out, here
	// and in the history: a reviewer is not told how many reviews there were.
	Round int `json:"round,omitempty"`
	// Contract is the digest of the contract the finding was established
	// under. A requirement or a risk it cites is one of that contract.
	Contract string `json:"contract"`
	// Source is the role that reported it.
	Source Role `json:"source"`
	// Recurrences counts rejected fixes and reopenings.
	Recurrences int `json:"recurrences"`
	// RootCauseRequired is set once Recurrences reaches the contract's
	// threshold. The next fix must then include a root-cause analysis.
	RootCauseRequired bool `json:"root_cause_required"`
	// Disputed is set once the finding has been disputed under the pinned
	// contract. It cannot be disputed again until a contract is pinned.
	Disputed bool `json:"disputed"`
	// Fix is the most recent claimed fix, if any.
	Fix *Fix `json:"fix,omitempty"`
	// Input is the finding as reported.
	Input FindingInput `json:"finding"`
	// History lists the steps the finding went through, in ledger order.
	History []FindingEvent `json:"history,omitempty"`
}

// FindingEvent is one step in a finding's history.
type FindingEvent struct {
	// Event is the ledger event type that recorded the step.
	Event EventType `json:"event"`
	// Round is the latest round at that moment; 0 before the first round.
	Round int `json:"round,omitempty"`
	// Packet is the packet whose report caused the step.
	Packet string `json:"packet,omitempty"`
	// Candidate is the digest of the candidate the step was made on: the one
	// a reviewer, verifier or auditor examined, or the one a fix was claimed
	// on. It is empty for a step that is bound to no candidate.
	Candidate string `json:"candidate,omitempty"`
	// Note is the evidence, justification or rationale recorded with it.
	Note string `json:"note,omitempty"`
}

// RecordedOn returns the digests of the candidates on which the finding was
// recorded as present, oldest first and each once: those it was opened or
// reopened on and those on which a fix for it was rejected. A fix confirmed on
// another candidate says nothing about these.
func (f Finding) RecordedOn() []string {
	var out []string
	for _, h := range f.History {
		switch h.Event {
		case EventFindingOpened, EventFindingReopened, EventFindingFixRejected:
			if h.Candidate != "" && !slices.Contains(out, h.Candidate) {
				out = append(out, h.Candidate)
			}
		}
	}
	return out
}

// recordedBy returns the latest step that recorded the finding as present on
// candidate.
func (f Finding) recordedBy(candidate string) (FindingEvent, bool) {
	for i := len(f.History) - 1; i >= 0; i-- {
		switch h := f.History[i]; h.Event {
		case EventFindingOpened, EventFindingReopened, EventFindingFixRejected:
			if h.Candidate == candidate {
				return h, true
			}
		}
	}
	return FindingEvent{}, false
}

// carriedBy reports whether a candidate carries the finding although it is
// resolved: the finding blocks, and the candidate is one it was recorded on,
// so the fix that resolved it was confirmed on another candidate.
func (f Finding) carriedBy(candidate string) bool {
	return f.Status == StatusResolved && f.Tier.Blocking() && slices.Contains(f.RecordedOn(), candidate)
}

// Fix is an implementer's claim that a finding is resolved.
type Fix struct {
	Finding string `json:"finding"`
	// Commits are full commit hashes containing the change.
	Commits []string `json:"commits"`
	Summary string   `json:"summary"`
	// Components are the files, modules or subsystems the fix touched.
	Components []string `json:"components"`
	// RootCause is required when the finding is flagged root-cause-required.
	RootCause string `json:"root_cause,omitempty"`
	// Tests describes the regression tests added or the checks performed.
	Tests string `json:"tests,omitempty"`
}

// Escalation is a convergence safeguard that has fired.
type Escalation struct {
	// ID is "E-" followed by a zero-padded sequence number.
	ID    string         `json:"id"`
	Kind  EscalationKind `json:"kind"`
	Owner Role           `json:"owner"`
	// Round is the round during which it was raised, numbered across the
	// whole ledger.
	Round  int    `json:"round"`
	Reason string `json:"reason"`
	// Candidate is the digest of the candidate the escalation was raised on,
	// when it was raised on one.
	Candidate string `json:"candidate,omitempty"`
	// Findings, Components and Requirements are the subjects, when the kind
	// has any.
	Findings     []string `json:"findings,omitempty"`
	Components   []string `json:"components,omitempty"`
	Requirements []string `json:"requirements,omitempty"`
	Open         bool     `json:"open"`
	// Resolution is the owner's conclusion once discharged.
	Resolution string `json:"resolution,omitempty"`
}

// Candidate identifies the commit under review.
type Candidate struct {
	// Commit is the full commit hash.
	Commit string `json:"commit"`
	// Digest is the candidate digest, "sha256:" followed by lowercase hex.
	Digest string `json:"digest"`
}

// Evidence is the recorded result of running a requirement's check command.
type Evidence struct {
	Requirement string `json:"requirement"`
	Command     string `json:"command"`
	// Candidate is the digest of the candidate the command ran against.
	Candidate    string `json:"candidate"`
	ExitCode     int    `json:"exit_code"`
	Passed       bool   `json:"passed"`
	OutputDigest string `json:"output_digest"`
}
