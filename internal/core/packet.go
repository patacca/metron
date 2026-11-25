package core

// Packet is everything a role needs to perform one action. Metron issues it;
// the role answers with a Report that names it.
type Packet struct {
	Schema string `json:"schema"`
	// ID is "P-" followed by a zero-padded sequence number. It is never
	// reused.
	ID       string `json:"id"`
	Protocol string `json:"protocol"`
	Role     Role   `json:"role"`
	// Action is one of StepReview, StepFix, StepVerify or StepAudit.
	Action Step `json:"action"`
	Round  int  `json:"round"`
	// IssuedAt is RFC 3339 UTC with second precision.
	IssuedAt  string    `json:"issued_at"`
	Candidate Candidate `json:"candidate"`
	// Base is the candidate the round reviewed. It is set for fix and verify
	// packets, where the role works on the changes since then.
	Base *Candidate `json:"base,omitempty"`
	// Verified is the candidate the latest verification examined. It is set
	// for a review packet when fixes were verified since the previous
	// verdict: a reviewed candidate that differs from it holds changes no
	// verifier saw.
	Verified *Candidate `json:"verified,omitempty"`
	// ReadOnly tells the role it must not change the candidate. Metron
	// rejects the report if it did.
	ReadOnly bool `json:"read_only"`

	ContractDigest string   `json:"contract_digest"`
	Contract       Contract `json:"contract"`
	// ApplicableRequirements are the identifiers of the requirements that are
	// mandatory at the contract's stage.
	ApplicableRequirements []string `json:"applicable_requirements"`
	// Evidence holds the latest check result per applicable requirement for
	// this candidate.
	Evidence []Evidence `json:"evidence,omitempty"`

	// Subjects are the findings the report must address: the blocking
	// findings to fix, or the claimed fixes to verify.
	Subjects []string `json:"subjects,omitempty"`
	// RootCauseRequired lists the subjects whose fix must include a
	// root-cause analysis.
	RootCauseRequired []string `json:"root_cause_required,omitempty"`
	// Escalations are the escalations an audit must discharge.
	Escalations []Escalation `json:"escalations,omitempty"`
	// Adjudicable lists the findings an audit must decide on.
	Adjudicable []string `json:"adjudicable,omitempty"`

	// Findings are the ledger's records relevant to the action. A reviewer
	// and an auditor receive every finding; an implementer and a verifier
	// receive the ones they work on.
	Findings []Finding `json:"findings,omitempty"`
	// History summarizes the earlier rounds of the current stage.
	History []RoundSummary `json:"history,omitempty"`
	// Decisions are audit directives and escalation resolutions, by the
	// auditor or by governance. They bind every role.
	Decisions []Decision `json:"decisions,omitempty"`

	ReportSchema string `json:"report_schema"`
	// Instructions is the role's brief, rendered from the protocol.
	Instructions string `json:"instructions"`
}

// RoundSummary is one earlier round as shown to a role.
type RoundSummary struct {
	Round     int       `json:"round"`
	Candidate Candidate `json:"candidate"`
	// Verdict is empty while the round has none.
	Verdict  Verdict `json:"verdict,omitempty"`
	Reviewer string  `json:"reviewer,omitempty"`
}

// Decision is binding guidance recorded in the ledger.
type Decision struct {
	// Source is DecisionDirective or DecisionResolution.
	Source string `json:"source"`
	// Escalation is the escalation the decision came out of, if any.
	Escalation string `json:"escalation,omitempty"`
	// Packet is the audit packet whose report recorded the decision. It is
	// empty for a governance resolution.
	Packet string `json:"packet,omitempty"`
	By     Role   `json:"by"`
	Text   string `json:"text"`
}

// Decision sources.
const (
	DecisionDirective  = "directive"
	DecisionResolution = "resolution"
)
