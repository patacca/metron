package core

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
	// Round is the round in which the finding was opened.
	Round int `json:"round"`
	// Source is the role that reported it.
	Source Role `json:"source"`
	// Recurrences counts rejected fixes and reopenings.
	Recurrences int `json:"recurrences"`
	// RootCauseRequired is set once Recurrences reaches the contract's
	// threshold. The next fix must then include a root-cause analysis.
	RootCauseRequired bool `json:"root_cause_required"`
	// Disputed is set once the finding has been disputed. It cannot be
	// disputed again.
	Disputed bool `json:"disputed"`
	// Fix is the most recent claimed fix, if any.
	Fix *Fix `json:"fix,omitempty"`
	// Input is the finding as reported.
	Input FindingInput `json:"finding"`
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
	// Round is the round during which it was raised.
	Round  int    `json:"round"`
	Reason string `json:"reason"`
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
