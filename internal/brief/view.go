package brief

import (
	"fmt"
	"slices"

	"github.com/patacca/metron/internal/core"
)

// view is the data the templates are executed with: the packet itself, plus
// what the templates need resolved or grouped. Every list keeps the packet's
// order.
type view struct {
	core.Packet

	// Exactly one is set, according to the packet's action.
	IsReview, IsFix, IsVerify, IsAudit bool

	// Applicable are the applicable requirements in contract order, each with
	// its latest check result. Later are the requirements that apply only from
	// a later stage.
	Applicable []requirement
	Later      []core.Requirement

	// The packet's findings, resolved from its identifier lists. Others are
	// the findings in none of the three lists.
	SubjectFindings     []core.Finding
	BacklogFindings     []core.Finding
	AdjudicableFindings []core.Finding
	OtherFindings       []core.Finding

	// RootCause lists the subjects and backlog findings whose fix must include
	// a root-cause analysis, whether the packet or the finding itself says so.
	// Disputed lists the subjects that have already been disputed.
	RootCause []string
	Disputed  []string

	// Kinds groups an audit packet's escalations by kind.
	Kinds escalationKinds

	Thresholds  core.Convergence
	ChurnRounds int

	// Brief and Skeleton are set only when rendering a prompt.
	Brief    string
	Skeleton string
}

type requirement struct {
	core.Requirement
	// Evidence is the latest check result on the packet's candidate, or nil.
	Evidence *core.Evidence
}

// escalationKinds holds the escalations an audit can discharge, by kind.
type escalationKinds struct {
	Dispute          []core.Escalation
	Contradiction    []core.Escalation
	ComponentChurn   []core.Escalation
	RegressionSpiral []core.Escalation
	NonConvergence   []core.Escalation
	ProcessAudit     []core.Escalation
}

// newView checks that p is a packet this protocol version can brief and
// prepares the data its templates need.
func newView(p core.Packet) (*view, error) {
	if p.Protocol != core.ProtocolVersion {
		return nil, fmt.Errorf("packet %s was issued under protocol %q, but this metron renders %s; use a metron release that implements %q",
			p.ID, p.Protocol, core.ProtocolVersion, p.Protocol)
	}
	if p.ReportSchema != core.SchemaReport {
		return nil, fmt.Errorf("packet %s asks for report schema %q, but this metron produces %s; use a metron release that implements %q",
			p.ID, p.ReportSchema, core.SchemaReport, p.ReportSchema)
	}
	role, ok := p.Action.PacketRole()
	if !ok {
		return nil, fmt.Errorf("packet %s has action %q, which no role performs through a packet", p.ID, p.Action)
	}
	if p.Role != role {
		return nil, fmt.Errorf("packet %s has action %q but role %q; that action belongs to the %s", p.ID, p.Action, p.Role, role)
	}

	v := &view{
		Packet:      p,
		IsReview:    p.Action == core.StepReview,
		IsFix:       p.Action == core.StepFix,
		IsVerify:    p.Action == core.StepVerify,
		IsAudit:     p.Action == core.StepAudit,
		Thresholds:  p.Contract.Thresholds(),
		ChurnRounds: core.ComponentChurnRounds,
	}
	if err := v.requirements(); err != nil {
		return nil, err
	}
	if err := v.findings(); err != nil {
		return nil, err
	}
	if err := v.action(); err != nil {
		return nil, err
	}
	return v, nil
}

// requirements splits the contract's requirements into applicable and later
// ones and attaches the evidence to the applicable ones.
func (v *view) requirements() error {
	var ids []string
	for _, r := range v.Contract.Requirements {
		if !r.AppliesAt(v.Contract.Stage) {
			v.Later = append(v.Later, r)
			continue
		}
		ids = append(ids, r.ID)
		v.Applicable = append(v.Applicable, requirement{Requirement: r})
	}
	if !slices.Equal(ids, v.ApplicableRequirements) {
		return fmt.Errorf("packet %s lists applicable requirements %v, but its contract makes %v applicable at stage %q",
			v.ID, v.ApplicableRequirements, ids, v.Contract.Stage)
	}
	for i := range v.Evidence {
		e := &v.Evidence[i]
		at := slices.Index(ids, e.Requirement)
		if at < 0 {
			return fmt.Errorf("packet %s carries evidence for %s, which is not an applicable requirement", v.ID, e.Requirement)
		}
		if v.Applicable[at].Evidence != nil {
			return fmt.Errorf("packet %s carries more than one check result for %s; it must carry only the latest", v.ID, e.Requirement)
		}
		if e.Candidate != v.Candidate.Digest {
			return fmt.Errorf("packet %s carries evidence for %s recorded on candidate %s, not on its candidate %s",
				v.ID, e.Requirement, e.Candidate, v.Candidate.Digest)
		}
		v.Applicable[at].Evidence = e
	}
	return nil
}

// findings resolves the packet's identifier lists against its findings.
func (v *view) findings() error {
	byID := make(map[string]core.Finding, len(v.Findings))
	for _, f := range v.Findings {
		if _, dup := byID[f.ID]; dup {
			return fmt.Errorf("packet %s lists finding %s more than once", v.ID, f.ID)
		}
		byID[f.ID] = f
	}
	resolve := func(list string, ids []string) ([]core.Finding, error) {
		var out []core.Finding
		for _, id := range ids {
			f, ok := byID[id]
			if !ok {
				return nil, fmt.Errorf("packet %s names %s among its %s, but does not include that finding", v.ID, id, list)
			}
			out = append(out, f)
		}
		return out, nil
	}
	var err error
	if v.SubjectFindings, err = resolve("subjects", v.Subjects); err != nil {
		return err
	}
	if v.BacklogFindings, err = resolve("backlog", v.Backlog); err != nil {
		return err
	}
	if v.AdjudicableFindings, err = resolve("adjudicable findings", v.Adjudicable); err != nil {
		return err
	}
	for _, id := range v.RootCauseRequired {
		if !slices.Contains(v.Subjects, id) && !slices.Contains(v.Backlog, id) {
			return fmt.Errorf("packet %s requires a root-cause analysis for %s, which is neither a subject nor in the backlog", v.ID, id)
		}
	}
	for _, f := range slices.Concat(v.SubjectFindings, v.BacklogFindings) {
		if f.RootCauseRequired || slices.Contains(v.RootCauseRequired, f.ID) {
			v.RootCause = append(v.RootCause, f.ID)
		}
	}
	for _, f := range v.SubjectFindings {
		if f.Disputed {
			v.Disputed = append(v.Disputed, f.ID)
		}
	}
	for _, f := range v.Findings {
		if !slices.Contains(v.Subjects, f.ID) && !slices.Contains(v.Backlog, f.ID) && !slices.Contains(v.Adjudicable, f.ID) {
			v.OtherFindings = append(v.OtherFindings, f)
		}
	}
	return nil
}

// action checks what the packet's action requires, as listed in section 9 of
// docs/architecture.md, and groups an audit's escalations.
func (v *view) action() error {
	refuse := func(what string) error {
		return fmt.Errorf("packet %s is a %s packet but %s", v.ID, v.Action, what)
	}
	if !v.IsFix && !v.IsVerify && len(v.Subjects) > 0 {
		return refuse("lists subjects")
	}
	if !v.IsFix && (len(v.Backlog) > 0 || len(v.RootCauseRequired) > 0) {
		return refuse("lists a backlog or root-cause requirements")
	}
	if !v.IsAudit && (len(v.Escalations) > 0 || len(v.Adjudicable) > 0) {
		return refuse("lists escalations or adjudicable findings")
	}
	if (v.IsFix || v.IsVerify) && v.Base == nil {
		return refuse("has no base candidate")
	}
	if (v.IsFix || v.IsVerify) && len(v.Subjects) == 0 {
		return refuse("has no subjects")
	}
	if v.IsVerify {
		for _, f := range v.SubjectFindings {
			if f.Fix == nil {
				return refuse(fmt.Sprintf("its subject %s has no claimed fix", f.ID))
			}
		}
	}
	if !v.IsAudit {
		return nil
	}
	if len(v.Escalations) == 0 {
		return refuse("has no escalations")
	}
	k := &v.Kinds
	for _, e := range v.Escalations {
		if !e.Open {
			return refuse(fmt.Sprintf("its escalation %s is already discharged", e.ID))
		}
		switch e.Kind {
		case core.EscalationDispute:
			k.Dispute = append(k.Dispute, e)
		case core.EscalationContradiction:
			k.Contradiction = append(k.Contradiction, e)
		case core.EscalationComponentChurn:
			k.ComponentChurn = append(k.ComponentChurn, e)
		case core.EscalationRegressionSpiral:
			k.RegressionSpiral = append(k.RegressionSpiral, e)
		case core.EscalationNonConvergence:
			k.NonConvergence = append(k.NonConvergence, e)
		case core.EscalationProcessAudit:
			k.ProcessAudit = append(k.ProcessAudit, e)
		default:
			return refuse(fmt.Sprintf("its escalation %s is of kind %q, which an auditor does not discharge", e.ID, e.Kind))
		}
	}
	return nil
}
