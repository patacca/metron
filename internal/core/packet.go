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
	// Round is the round a fix, a verification or an audit follows. A review
	// packet does not carry it, nor the earlier rounds: how many reviews came
	// before, how they ended and who held them has no bearing on what a
	// reviewer must decide, and the ledger knows which round a report is for.
	Round int `json:"round,omitempty"`
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
	// History summarizes the earlier rounds of the current stage. A review
	// packet does not carry it.
	History []RoundSummary `json:"history,omitempty"`
	// Decisions are the audit directives and escalation resolutions, by the
	// auditor or by governance, that apply to this packet's contract and
	// candidate. They bind every role.
	Decisions []Decision `json:"decisions,omitempty"`
	// EarlierDecisions are the decisions that no longer bind: superseded,
	// made under another contract, or establishing something about another
	// candidate. They are context for what was decided and why.
	EarlierDecisions []Decision `json:"earlier_decisions,omitempty"`

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

// Decision is guidance recorded in the ledger: an audit directive, or the
// resolution of an escalation by an auditor or by governance. It records
// where it was made, which decides where it applies.
type Decision struct {
	// ID is "D-" followed by a zero-padded sequence number.
	ID string `json:"id"`
	// Source is DecisionDirective or DecisionResolution.
	Source string `json:"source"`
	// Escalation is the escalation a resolution discharged, and Kind its
	// kind. Both are empty for a directive.
	Escalation string         `json:"escalation,omitempty"`
	Kind       EscalationKind `json:"kind,omitempty"`
	// Packet is the audit packet whose report recorded the decision. It is
	// empty for a governance resolution.
	Packet string `json:"packet,omitempty"`
	By     Role   `json:"by"`
	Text   string `json:"text"`
	// Stage and Contract are the stage and the contract digest pinned when
	// the decision was made. It binds only under that contract.
	Stage    Stage  `json:"stage"`
	Contract string `json:"contract"`
	// Candidate is the digest of the candidate the decision was made on: the
	// one the auditor examined or, for a governance resolution, the one the
	// escalation was raised on. It is empty when that is unknown.
	Candidate string `json:"candidate,omitempty"`
	// Requirements are the requirements the decision is about, when the
	// escalation named any.
	Requirements []string `json:"requirements,omitempty"`
	// SupersededBy is the audit packet whose report superseded the decision.
	SupersededBy string `json:"superseded_by,omitempty"`
}

// Decision sources.
const (
	DecisionDirective  = "directive"
	DecisionResolution = "resolution"
)

// Binds reports whether the decision binds a role working under the given
// contract digest on the given candidate. A decision binds until it is
// superseded and while the contract it was made under is the pinned one: a
// change of contract is a change of the question it answered, whether or not
// the stage changed. Evidence that governance supplied for an evidence gap
// establishes something about one candidate, so it binds on that candidate
// only.
func (d Decision) Binds(contract, candidate string) bool {
	switch {
	case d.SupersededBy != "", d.Contract != contract:
		return false
	case d.Kind == EscalationEvidenceGap:
		return d.Candidate != "" && d.Candidate == candidate
	}
	return true
}
