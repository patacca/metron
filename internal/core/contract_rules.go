package core

import (
	"fmt"
	"strings"
)

// ValidateContract applies the semantic rules of docs/architecture.md,
// section 6, to a contract that has passed structural validation. It returns
// nil or a *Rejection of layer LayerSemantic listing every problem. It never
// panics, whatever the value.
func ValidateContract(c Contract) error {
	var ps problems
	if c.Schema != SchemaContract {
		ps.add("/schema", "schema is %q: it must be %q", c.Schema, SchemaContract)
	}
	if blank(c.Project.Name) {
		ps.add("/project/name", "the project name is blank: name the project")
	}
	if len(c.Project.Objectives) == 0 {
		ps.add("/project/objectives", "no objective is listed: state at least one thing the project is for")
	}
	statements(&ps, "/project/objectives", c.Project.Objectives, "objective")
	statements(&ps, "/project/non_objectives", c.Project.NonObjectives, "non-objective")
	if !c.Stage.Valid() {
		ps.add("/stage", "%q is not a lifecycle stage: use one of %s", c.Stage, stageList())
	}
	if blank(c.Authorizes) {
		ps.add("/authorizes", "authorizes is blank: name the activity a GREEN gate permits")
	}
	if len(c.Scope) == 0 {
		ps.add("/scope", "the scope is empty: describe what is under review")
	}
	statements(&ps, "/scope", c.Scope, "scope statement")
	if len(c.Envelope) == 0 {
		ps.add("/operating_envelope", "the operating envelope is empty: state the expected conditions and load")
	}
	statements(&ps, "/operating_envelope", c.Envelope, "envelope statement")

	if len(c.Requirements) == 0 {
		ps.add("/requirements", "no requirement is listed: add at least one")
	}
	reqIDs := make(map[string]int)
	for i, r := range c.Requirements {
		p := ptr("requirements", i)
		uniqueID(&ps, p+"/id", r.ID, "REQ-", "/requirements", i, reqIDs)
		if !r.Domain.Valid() {
			ps.add(p+"/domain", "%q is not a review domain: use one of the 18 protocol domains", r.Domain)
		}
		if blank(r.Statement) {
			ps.add(p+"/statement", "the statement is blank: say what must hold")
		}
		if blank(r.Criterion) {
			ps.add(p+"/criterion", "the criterion is blank: say how anyone decides that the statement holds")
		}
		if r.Command != "" && blank(r.Command) {
			ps.add(p+"/command", "the command is blank: give the check to run or remove the member")
		}
		if r.From != "" && !r.From.Valid() {
			ps.add(p+"/from", "%q is not a lifecycle stage: use one of %s", r.From, stageList())
		}
	}

	if len(c.Risks.Unacceptable) == 0 {
		ps.add("/risks/unacceptable", "no unacceptable risk is listed: list at least one RISK-n statement")
	}
	risks(&ps, "/risks/unacceptable", c.Risks.Unacceptable, "RISK-")
	risks(&ps, "/risks/accepted", c.Risks.Accepted, "ACC-")

	waivers := make(map[Domain]int)
	for i, w := range c.NotApplicable {
		p := ptr("not_applicable", i)
		switch j, dup := waivers[w.Domain]; {
		case !w.Domain.Valid():
			ps.add(p+"/domain", "%q is not a review domain: use one of the 18 protocol domains", w.Domain)
		case dup:
			ps.add(p+"/domain", "domain %q is already waived at %s: waive each domain once", w.Domain, ptr("not_applicable", j))
		default:
			waivers[w.Domain] = i
		}
		if blank(w.Rationale) {
			ps.add(p+"/rationale", "the rationale is blank: say why the domain does not apply at this stage")
		}
	}

	calIDs := make(map[string]int)
	for i, cal := range c.Calibrations {
		p := ptr("calibrations", i)
		uniqueID(&ps, p+"/id", cal.ID, "CAL-", "/calibrations", i, calIDs)
		if blank(cal.Parameter) {
			ps.add(p+"/parameter", "the parameter is blank: name the provisional parameter")
		}
		if blank(cal.EvidenceNeeded) {
			ps.add(p+"/evidence_needed", "evidence_needed is blank: say what evidence would settle it")
		}
		if blank(cal.ReassessWhen) {
			ps.add(p+"/reassess_when", "reassess_when is blank: say when to reassess it")
		}
	}

	if cv := c.Convergence; cv != nil {
		if cv.RootCauseAfter < 1 {
			ps.add("/convergence/root_cause_after", "root_cause_after is %d: it must be positive", cv.RootCauseAfter)
		}
		if cv.ConvergenceAuditAfter < 1 {
			ps.add("/convergence/convergence_audit_after", "convergence_audit_after is %d: it must be positive", cv.ConvergenceAuditAfter)
		}
		if cv.ProcessAuditAfter < 1 {
			ps.add("/convergence/process_audit_after", "process_audit_after is %d: it must be positive", cv.ProcessAuditAfter)
		}
		if cv.ConvergenceAuditAfter >= cv.ProcessAuditAfter {
			ps.add("/convergence", "convergence_audit_after (%d) must be below process_audit_after (%d)",
				cv.ConvergenceAuditAfter, cv.ProcessAuditAfter)
		}
	}

	if c.Stage.Valid() {
		applicable := c.Applicable()
		if len(applicable) == 0 && len(c.Requirements) > 0 {
			ps.add("/requirements", "no requirement applies at stage %q: make at least one requirement applicable", c.Stage)
		}
		for _, d := range StageCoverage(c.Stage) {
			if _, waived := waivers[d]; !waived && firstIn(applicable, d) == "" {
				ps.add("/requirements", "domain %q must be covered at stage %q: add an applicable requirement in it or a not_applicable waiver", d, c.Stage)
			}
		}
		for i, w := range c.NotApplicable {
			if id := firstIn(applicable, w.Domain); id != "" && w.Domain.Valid() {
				ps.add(ptr("not_applicable", i, "domain"), "domain %q is waived but %s applies in it at stage %q: remove the waiver or the requirement", w.Domain, id, c.Stage)
			}
		}
	}
	return ps.err(LayerSemantic)
}

// firstIn returns the identifier of the first requirement in domain d, or "".
func firstIn(reqs []Requirement, d Domain) string {
	for _, r := range reqs {
		if r.Domain == d {
			return r.ID
		}
	}
	return ""
}

func statements(ps *problems, path string, list []string, what string) {
	for i, s := range list {
		if blank(s) {
			ps.add(fmt.Sprintf("%s/%d", path, i), "the %s is blank: write it or remove the entry", what)
		}
	}
}

func risks(ps *problems, path string, list []Risk, prefix string) {
	seen := make(map[string]int)
	for i, r := range list {
		p := fmt.Sprintf("%s/%d", path, i)
		uniqueID(ps, p+"/id", r.ID, prefix, path, i, seen)
		if blank(r.Statement) {
			ps.add(p+"/statement", "the statement is blank: describe the risk")
		}
	}
}

// uniqueID checks that id is well formed and not already in seen, and records
// it at index i of the list at listPath.
func uniqueID(ps *problems, path, id, prefix, listPath string, i int, seen map[string]int) {
	if !wellFormedID(id, prefix) {
		ps.add(path, "identifier %q is not %s followed by digits", id, prefix)
		return
	}
	if j, dup := seen[id]; dup {
		ps.add(path, "identifier %q is already used at %s/%d: give each entry its own identifier", id, listPath, j)
		return
	}
	seen[id] = i
}

func stageList() string {
	names := make([]string, len(stages))
	for i, s := range stages {
		names[i] = string(s)
	}
	return strings.Join(names, ", ")
}
