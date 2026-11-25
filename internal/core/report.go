package core

import "encoding/json"

// Report is the versioned envelope in which every role submits its work.
// Body holds the role-specific content: a ReviewerBody, ImplementerBody,
// VerifierBody or AuditorBody, according to Role.
type Report struct {
	Schema string `json:"schema"`
	Packet string `json:"packet"`
	// PacketDigest is the digest of the packet this report answers, as
	// recorded when the packet was issued.
	PacketDigest string          `json:"packet_digest"`
	Role         Role            `json:"role"`
	Author       Author          `json:"author"`
	Body         json.RawMessage `json:"body"`
}

// Author is the identity a report's submitter declares. In local trust mode
// it is recorded, not verified.
type Author struct {
	// ID is a stable name for whoever acted in the role. Role separation is
	// decided on it.
	ID      string `json:"id"`
	Kind    string `json:"kind"`
	Model   string `json:"model,omitempty"`
	Harness string `json:"harness,omitempty"`
	Session string `json:"session,omitempty"`
}

// Author kinds.
const (
	AuthorAgent = "agent"
	AuthorHuman = "human"
)

// Provenance records how a report's origin was established.
type Provenance struct {
	Mode string `json:"mode"`
}

// ProvenanceDeclared is the only provenance mode of local trust mode: the
// author is whoever the submitter says it is.
const ProvenanceDeclared = "declared"

// ReviewerBody is a reviewer's report on one candidate.
type ReviewerBody struct {
	// Coverage has exactly one entry per review domain.
	Coverage []DomainAssessment `json:"coverage"`
	// Requirements has exactly one entry per applicable requirement.
	Requirements []RequirementAssessment `json:"requirements"`
	// Findings are the new findings of this review. The list may be empty.
	Findings []FindingInput `json:"findings"`
	// Reopen lists resolved findings the reviewer reopens.
	Reopen []Reopen `json:"reopen,omitempty"`
	// ContractGaps are risks the contract does not cover. They go to
	// governance and cannot block by themselves.
	ContractGaps  []ContractGap `json:"contract_gaps,omitempty"`
	Verdict       Verdict       `json:"verdict"`
	Justification string        `json:"justification"`
	ResidualRisks []string      `json:"residual_risks,omitempty"`
	// Limitations are gaps in inspection coverage and environment.
	Limitations []string `json:"limitations,omitempty"`
}

// DomainAssessment is the reviewer's conclusion for one review domain.
type DomainAssessment struct {
	Domain Domain `json:"domain"`
	Status string `json:"status"`
	// Summary is the assessment, or the rationale when not applicable.
	Summary string `json:"summary"`
}

// Coverage statuses.
const (
	CoverageAssessed      = "assessed"
	CoverageNotApplicable = "not-applicable"
)

// RequirementAssessment is the reviewer's conclusion for one requirement.
type RequirementAssessment struct {
	Requirement string `json:"requirement"`
	Status      string `json:"status"`
	Evidence    string `json:"evidence"`
}

// Requirement statuses.
const (
	RequirementSatisfied  = "satisfied"
	RequirementViolated   = "violated"
	RequirementUnverified = "unverified"
)

// Reopen asks to reopen a resolved finding.
type Reopen struct {
	Finding       string       `json:"finding"`
	Reason        ReopenReason `json:"reason"`
	Justification string       `json:"justification"`
}

// ContractGap is a risk the reviewer believes the contract should cover.
type ContractGap struct {
	Title       string `json:"title"`
	Description string `json:"description"`
	// SuggestedRequirement is wording governance could adopt.
	SuggestedRequirement string `json:"suggested_requirement,omitempty"`
}

// ImplementerBody is an implementer's report on a batch of findings.
type ImplementerBody struct {
	Fixes    []Fix     `json:"fixes"`
	Disputes []Dispute `json:"disputes,omitempty"`
	Notes    string    `json:"notes,omitempty"`
}

// Dispute contests a finding instead of fixing it.
type Dispute struct {
	Finding   string `json:"finding"`
	Rationale string `json:"rationale"`
}

// VerifierBody is a verifier's report on a batch of claimed fixes.
type VerifierBody struct {
	// Verifications has exactly one entry per claimed fix in the packet.
	Verifications []Verification `json:"verifications"`
	// Regressions are new findings introduced by the fixes.
	Regressions []FindingInput `json:"regressions,omitempty"`
	Notes       string         `json:"notes,omitempty"`
}

// Verification is the verifier's result for one claimed fix.
type Verification struct {
	Finding  string `json:"finding"`
	Result   string `json:"result"`
	Evidence string `json:"evidence"`
}

// Verification results.
const (
	VerificationConfirmed = "confirmed"
	VerificationRejected  = "rejected"
)

// AuditorBody is an auditor's report discharging a packet's escalations.
type AuditorBody struct {
	// Resolutions has exactly one entry per escalation in the packet.
	Resolutions []EscalationResolution `json:"resolutions"`
	// Adjudications has exactly one entry per adjudicable finding.
	Adjudications []Adjudication `json:"adjudications,omitempty"`
	// Directives are binding guidance carried into every later packet.
	Directives []string `json:"directives,omitempty"`
	// ContractRecommendations are suggestions for governance. They change
	// nothing by themselves.
	ContractRecommendations []string `json:"contract_recommendations,omitempty"`
	// Suspend asks for the campaign to be suspended until governance decides,
	// and says why. It keeps the gate closed and stops every role.
	Suspend string `json:"suspend,omitempty"`
}

// EscalationResolution is an auditor's analysis and conclusion for one
// escalation of the packet. The conclusion discharges it.
type EscalationResolution struct {
	Escalation string `json:"escalation"`
	Analysis   string `json:"analysis"`
	Conclusion string `json:"conclusion"`
}

// Adjudication is an auditor's decision on one finding.
type Adjudication struct {
	Finding  string `json:"finding"`
	Decision string `json:"decision"`
	// Tier is the new tier. It is required for, and only for, a reclassify
	// decision.
	Tier      Tier   `json:"tier,omitempty"`
	Rationale string `json:"rationale"`
}

// Adjudication decisions.
const (
	DecisionUphold     = "uphold"
	DecisionWithdraw   = "withdraw"
	DecisionReclassify = "reclassify"
)
