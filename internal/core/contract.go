package core

// Contract is a repository's acceptance contract: the only project-specific
// input to a review. It is authored by governance in .metron/contract.yaml.
type Contract struct {
	Schema        string        `json:"schema"`
	Project       Project       `json:"project"`
	Stage         Stage         `json:"stage"`
	Authorizes    string        `json:"authorizes"`
	Scope         []string      `json:"scope"`
	Envelope      []string      `json:"operating_envelope"`
	Requirements  []Requirement `json:"requirements"`
	Risks         Risks         `json:"risks"`
	NotApplicable []Waiver      `json:"not_applicable,omitempty"`
	Calibrations  []Calibration `json:"calibrations,omitempty"`
	Convergence   *Convergence  `json:"convergence,omitempty"`
}

// Project says what the project is for.
type Project struct {
	Name          string   `json:"name"`
	Objectives    []string `json:"objectives"`
	NonObjectives []string `json:"non_objectives,omitempty"`
}

// Requirement is one mandatory acceptance criterion.
type Requirement struct {
	// ID is "REQ-" followed by digits, unique within the contract.
	ID     string `json:"id"`
	Domain Domain `json:"domain"`
	// Statement is what must hold.
	Statement string `json:"statement"`
	// Criterion is the acceptance boundary: how anyone decides that the
	// statement holds.
	Criterion string `json:"criterion"`
	// Command, when set, is a check Metron runs itself. It passes when it
	// exits with status 0.
	Command string `json:"command,omitempty"`
	// From is the first stage at which the requirement is mandatory. Empty
	// means the first stage.
	From Stage `json:"from,omitempty"`
}

// AppliesAt reports whether the requirement is mandatory for a gate into the
// given stage.
func (r Requirement) AppliesAt(stage Stage) bool {
	from := r.From
	if from == "" {
		from = StagePrototype
	}
	return from.Valid() && stage.Valid() && from.Rank() <= stage.Rank()
}

// Risks separates what must never be accepted from what already has been.
type Risks struct {
	// Unacceptable risks may be cited by a blocking finding.
	Unacceptable []Risk `json:"unacceptable"`
	// Accepted risks are residual risks governance has accepted. A finding
	// about one of them cannot block.
	Accepted []Risk `json:"accepted,omitempty"`
}

// Risk is a statement of risk. Unacceptable risks are identified "RISK-n",
// accepted ones "ACC-n".
type Risk struct {
	ID        string `json:"id"`
	Statement string `json:"statement"`
}

// Waiver declares a domain not applicable at the contract's stage.
type Waiver struct {
	Domain    Domain `json:"domain"`
	Rationale string `json:"rationale"`
}

// Calibration is a provisional parameter that cannot be settled without
// evidence the project does not have yet. It is identified "CAL-n".
type Calibration struct {
	ID             string `json:"id"`
	Parameter      string `json:"parameter"`
	EvidenceNeeded string `json:"evidence_needed"`
	ReassessWhen   string `json:"reassess_when"`
}

// Convergence holds the only tunable thresholds of the protocol.
type Convergence struct {
	// RootCauseAfter is the recurrence count at which a finding's next fix
	// must include a root-cause analysis.
	RootCauseAfter int `json:"root_cause_after"`
	// ConvergenceAuditAfter is the number of failed steps without a GREEN
	// verdict that triggers a convergence analysis.
	ConvergenceAuditAfter int `json:"convergence_audit_after"`
	// ProcessAuditAfter is the number of failed steps without a GREEN verdict
	// that triggers an independent process audit, and again at each
	// multiple. Discharging that audit suspends the campaign.
	ProcessAuditAfter int `json:"process_audit_after"`
	// SuspendAfter is the campaign budget: the number of packets a campaign
	// may issue, whatever their outcome, before it is suspended.
	SuspendAfter int `json:"suspend_after"`
}

// DefaultConvergence returns the protocol's default thresholds.
func DefaultConvergence() Convergence {
	return Convergence{RootCauseAfter: 2, ConvergenceAuditAfter: 3, ProcessAuditAfter: 5, SuspendAfter: 20}
}

// Thresholds returns the contract's convergence thresholds, or the defaults if
// it sets none.
func (c Contract) Thresholds() Convergence {
	if c.Convergence == nil {
		return DefaultConvergence()
	}
	return *c.Convergence
}

// Applicable returns the requirements that are mandatory at the contract's
// stage, in contract order.
func (c Contract) Applicable() []Requirement {
	var out []Requirement
	for _, r := range c.Requirements {
		if r.AppliesAt(c.Stage) {
			out = append(out, r)
		}
	}
	return out
}
