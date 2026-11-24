// Package core is Metron's deterministic domain: the fixed vocabulary of the
// review protocol, the documents exchanged with roles, the ledger events, and
// the rules that turn events into state, state into the next permitted action,
// and state into a gate outcome.
//
// The package performs no I/O. It does not read files, the clock, the
// environment or git. Everything observed from outside is passed in.
// docs/architecture.md is the normative description of its behaviour.
package core

// Identifiers of the public protocol. They are compared exactly.
const (
	ProtocolVersion = "metron.protocol/v1"
	SchemaContract  = "metron.contract/v1"
	SchemaPacket    = "metron.packet/v1"
	SchemaReport    = "metron.report/v1"
)

// Stage is a lifecycle stage. Stages are totally ordered.
type Stage string

const (
	StagePrototype             Stage = "prototype"
	StageControlledValidation  Stage = "controlled-validation"
	StagePilot                 Stage = "pilot"
	StageProduction            Stage = "production"
	StageContinuousImprovement Stage = "continuous-improvement"
)

var stages = [...]Stage{
	StagePrototype,
	StageControlledValidation,
	StagePilot,
	StageProduction,
	StageContinuousImprovement,
}

// Stages returns every stage in lifecycle order.
func Stages() []Stage { return append([]Stage(nil), stages[:]...) }

// Rank returns the stage's position in lifecycle order, or -1 if s is not a
// stage.
func (s Stage) Rank() int {
	for i, v := range stages {
		if v == s {
			return i
		}
	}
	return -1
}

// Valid reports whether s is a stage.
func (s Stage) Valid() bool { return s.Rank() >= 0 }

// Tier classifies a finding by evidence and impact.
type Tier string

const (
	TierCritical    Tier = "T0"
	TierMaterial    Tier = "T1"
	TierImprovement Tier = "T2"
	TierCalibration Tier = "T3"
	TierPreference  Tier = "T4"
)

var tiers = [...]Tier{TierCritical, TierMaterial, TierImprovement, TierCalibration, TierPreference}

// Tiers returns every tier from most to least severe.
func Tiers() []Tier { return append([]Tier(nil), tiers[:]...) }

// Valid reports whether t is a tier.
func (t Tier) Valid() bool {
	for _, v := range tiers {
		if v == t {
			return true
		}
	}
	return false
}

// Blocking reports whether an unresolved finding of this tier closes the gate.
func (t Tier) Blocking() bool { return t == TierCritical || t == TierMaterial }

// Domain is a review domain.
type Domain string

const (
	DomainFunctionalCorrectness Domain = "functional-correctness"
	DomainObjectiveAlignment    Domain = "objective-alignment"
	DomainSecurityPrivacy       Domain = "security-privacy"
	DomainDataIntegrity         Domain = "data-integrity"
	DomainStability             Domain = "stability"
	DomainReliability           Domain = "reliability"
	DomainPerformance           Domain = "performance"
	DomainOptimization          Domain = "optimization"
	DomainScalability           Domain = "scalability"
	DomainArchitecture          Domain = "architecture"
	DomainMaintainability       Domain = "maintainability"
	DomainTechnicalDebt         Domain = "technical-debt"
	DomainSimplicity            Domain = "simplicity"
	DomainTestability           Domain = "testability"
	DomainObservability         Domain = "observability"
	DomainCompatibility         Domain = "compatibility"
	DomainOperationalReadiness  Domain = "operational-readiness"
	DomainCostEfficiency        Domain = "cost-efficiency"
)

var domains = [...]Domain{
	DomainFunctionalCorrectness,
	DomainObjectiveAlignment,
	DomainSecurityPrivacy,
	DomainDataIntegrity,
	DomainStability,
	DomainReliability,
	DomainPerformance,
	DomainOptimization,
	DomainScalability,
	DomainArchitecture,
	DomainMaintainability,
	DomainTechnicalDebt,
	DomainSimplicity,
	DomainTestability,
	DomainObservability,
	DomainCompatibility,
	DomainOperationalReadiness,
	DomainCostEfficiency,
}

// Domains returns every review domain in protocol order.
func Domains() []Domain { return append([]Domain(nil), domains[:]...) }

// Valid reports whether d is a review domain.
func (d Domain) Valid() bool {
	for _, v := range domains {
		if v == d {
			return true
		}
	}
	return false
}

// NeedsProportionality reports whether a blocking finding in this domain must
// justify the proportionality of its proposed change, its regression risk and
// the cost of deferring it. These are the domains in which "it could be
// better" is most easily mistaken for "it must be fixed".
func (d Domain) NeedsProportionality() bool {
	switch d {
	case DomainOptimization, DomainPerformance, DomainScalability, DomainArchitecture,
		DomainMaintainability, DomainTechnicalDebt, DomainSimplicity, DomainCostEfficiency:
		return true
	}
	return false
}

// stageDomains lists the domains each stage adds to mandatory contract
// coverage, in lifecycle order.
var stageDomains = [...][]Domain{
	{DomainFunctionalCorrectness, DomainObjectiveAlignment, DomainArchitecture},
	{DomainDataIntegrity, DomainTestability, DomainObservability, DomainStability},
	{DomainReliability, DomainPerformance, DomainCompatibility, DomainSecurityPrivacy},
	{DomainScalability, DomainOperationalReadiness, DomainMaintainability},
	{DomainOptimization, DomainTechnicalDebt, DomainSimplicity, DomainCostEfficiency},
}

// StageCoverage returns the domains a contract targeting stage s must cover,
// each with a requirement or a waiver. Coverage is cumulative. It returns nil
// if s is not a stage.
func StageCoverage(s Stage) []Domain {
	rank := s.Rank()
	if rank < 0 {
		return nil
	}
	var out []Domain
	for i := 0; i <= rank; i++ {
		out = append(out, stageDomains[i]...)
	}
	return out
}

// Verdict is a reviewer's conclusion for a round.
type Verdict string

const (
	VerdictGreen         Verdict = "GREEN"
	VerdictRed           Verdict = "RED"
	VerdictIndeterminate Verdict = "INDETERMINATE"
)

// Valid reports whether v is a verdict.
func (v Verdict) Valid() bool {
	return v == VerdictGreen || v == VerdictRed || v == VerdictIndeterminate
}

// Role is a party to the review process.
type Role string

const (
	RoleGovernance  Role = "governance"
	RoleImplementer Role = "implementer"
	RoleReviewer    Role = "reviewer"
	RoleVerifier    Role = "verifier"
	RoleAuditor     Role = "auditor"
)

// Valid reports whether r is a role.
func (r Role) Valid() bool {
	switch r {
	case RoleGovernance, RoleImplementer, RoleReviewer, RoleVerifier, RoleAuditor:
		return true
	}
	return false
}

// ReadOnly reports whether the role must leave the candidate untouched.
func (r Role) ReadOnly() bool {
	return r == RoleReviewer || r == RoleVerifier || r == RoleAuditor
}

// Step is an action in the review loop. `metron next` names exactly one.
type Step string

const (
	StepRepairLedger  Step = "repair-ledger"
	StepPinContract   Step = "pin-contract"
	StepSubmitReport  Step = "submit-report"
	StepAdjudicate    Step = "adjudicate"
	StepAudit         Step = "audit"
	StepVerify        Step = "verify"
	StepFix           Step = "fix"
	StepCommitChanges Step = "commit-changes"
	StepRunChecks     Step = "run-checks"
	StepReview        Step = "review"
	StepNone          Step = "none"
)

// PacketRole returns the role that receives a packet for this step, and false
// if the step is not performed through a packet.
func (s Step) PacketRole() (Role, bool) {
	switch s {
	case StepReview:
		return RoleReviewer, true
	case StepFix:
		return RoleImplementer, true
	case StepVerify:
		return RoleVerifier, true
	case StepAudit:
		return RoleAuditor, true
	}
	return "", false
}

// FindingStatus is a state of the finding state machine.
type FindingStatus string

const (
	StatusOpen      FindingStatus = "open"
	StatusClaimed   FindingStatus = "claimed"
	StatusResolved  FindingStatus = "resolved"
	StatusWithdrawn FindingStatus = "withdrawn"
)

// Unresolved reports whether a finding in this status still needs work.
func (s FindingStatus) Unresolved() bool { return s == StatusOpen || s == StatusClaimed }

// ReopenReason is why a resolved finding is reopened.
type ReopenReason string

const (
	ReopenNewEvidence         ReopenReason = "new-evidence"
	ReopenRegression          ReopenReason = "regression"
	ReopenInvalidVerification ReopenReason = "invalid-verification"
	ReopenRequirementChanged  ReopenReason = "requirement-changed"
	ReopenContractConflict    ReopenReason = "contract-conflict"
)

// Valid reports whether r is a reopen reason.
func (r ReopenReason) Valid() bool {
	switch r {
	case ReopenNewEvidence, ReopenRegression, ReopenInvalidVerification,
		ReopenRequirementChanged, ReopenContractConflict:
		return true
	}
	return false
}

// EscalationKind is a convergence safeguard that requires someone to act
// before the loop may continue.
type EscalationKind string

const (
	EscalationDispute          EscalationKind = "dispute"
	EscalationContradiction    EscalationKind = "contradiction"
	EscalationComponentChurn   EscalationKind = "component-churn"
	EscalationRegressionSpiral EscalationKind = "regression-spiral"
	EscalationNonConvergence   EscalationKind = "non-convergence"
	EscalationProcessAudit     EscalationKind = "process-audit"
	EscalationContractGap      EscalationKind = "contract-gap"
	EscalationEvidenceGap      EscalationKind = "evidence-gap"
)

// Owner returns the role that must discharge an escalation of this kind, or
// the empty role if k is not an escalation kind.
func (k EscalationKind) Owner() Role {
	switch k {
	case EscalationContractGap, EscalationEvidenceGap:
		return RoleGovernance
	case EscalationDispute, EscalationContradiction, EscalationComponentChurn,
		EscalationRegressionSpiral, EscalationNonConvergence, EscalationProcessAudit:
		return RoleAuditor
	}
	return ""
}

// ComponentChurnRounds is the number of distinct rounds in which the same
// component is fixed before a component-level audit is required.
const ComponentChurnRounds = 3
