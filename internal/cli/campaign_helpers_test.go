package cli

import (
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/patacca/metron/internal/core"
	"github.com/patacca/metron/internal/store"
)

// Builders shared by the end-to-end campaigns: contracts with small
// thresholds, reports for every role, and a transcript that records what an
// orchestrator sees after each command.

// campaignContract is testContract with the given convergence thresholds
// and a check of REQ-1 that also fails while a file named broken is
// committed, so that a campaign can fail a check at will.
func campaignContract(rootCause, convergence, process, suspend int) string {
	c := strings.Replace(testContract, `"git ls-files --error-unmatch greeting.txt"`,
		`"git ls-files --error-unmatch greeting.txt && test ! -e broken"`, 1)
	return c + fmt.Sprintf("convergence:\n  root_cause_after: %d\n  convergence_audit_after: %d\n  process_audit_after: %d\n  suspend_after: %d\n",
		rootCause, convergence, process, suspend)
}

// stageTwo is a contract for the controlled-validation stage that waives
// every domain the stage adds, so that it needs no further requirement.
func stageTwo(contract string) string {
	c := strings.Replace(contract, "stage: prototype", "stage: controlled-validation", 1)
	c += "not_applicable:\n"
	for _, d := range []core.Domain{core.DomainDataIntegrity, core.DomainTestability, core.DomainObservability, core.DomainStability} {
		c += fmt.Sprintf("  - domain: %s\n    rationale: \"One static greeting\"\n", d)
	}
	return c
}

// campaign is newRepo with metron initialized and the given contract pinned
// and committed.
func campaign(t *testing.T, contract string) *repo {
	t.Helper()
	r := newRepo(t)
	r.must("init")
	r.write(".metron/contract.yaml", contract)
	r.must("contract", "pin", "-m", "first contract")
	r.commit("pin the contract")
	return r
}

// change writes content to name and commits that file alone: a change to the
// candidate that leaves the state directory out of the commit. It returns
// the commit's hash.
func (r *repo) change(msg, name, content string) string {
	r.t.Helper()
	r.write(name, content)
	return r.commitOnly(msg, name)
}

// remove deletes a file from the working tree.
func (r *repo) remove(name string) {
	r.t.Helper()
	if err := os.Remove(r.path(name)); err != nil {
		r.t.Fatal(err)
	}
}

// revert commits the inverse of HEAD and returns the new commit.
func (r *repo) revert() string {
	r.t.Helper()
	r.git("revert", "--no-edit", "HEAD")
	return r.git("rev-parse", "HEAD")
}

// status returns metron status --json.
func (r *repo) status() statusDoc {
	r.t.Helper()
	var st statusDoc
	unmarshal(r.t, r.must("status", "--json").out, &st)
	return st
}

// gateDoc returns metron gate --json, which must exit with code.
func (r *repo) gateDoc(code int) gateDoc {
	r.t.Helper()
	var g gateDoc
	unmarshal(r.t, r.expect(code, "gate", "--json").out, &g)
	return g
}

// failing returns the names of the gate conditions that do not hold.
func failing(g gateDoc) []string {
	var out []string
	for _, c := range g.Conditions {
		if !c.Satisfied {
			out = append(out, c.Name)
		}
	}
	return out
}

// reasons returns the reasons the named gate condition gives.
func reasons(g gateDoc, name string) string {
	for _, c := range g.Conditions {
		if c.Name == name {
			return strings.Join(c.Reasons, "\n")
		}
	}
	return ""
}

// findings returns metron findings --json with the given filter flag.
func (r *repo) findings(filter string) []core.Finding {
	r.t.Helper()
	var d findingsDoc
	unmarshal(r.t, r.must("findings", filter, "--json").out, &d)
	return d.Findings
}

// ledgerEvents returns the number of events in the ledger.
func (r *repo) ledgerEvents() int {
	r.t.Helper()
	events, err := store.New(r.path(".metron/state")).Load()
	if err != nil {
		r.t.Fatal(err)
	}
	return len(events)
}

// expectAction checks the step and the owner that metron next --json names,
// and that next and gate agree.
func (r *repo) expectAction(step core.Step, owner core.Role) actionDoc {
	r.t.Helper()
	a := r.expectNext(step)
	if a.Owner != owner {
		r.t.Fatalf("next is %s owned by %q, want %q: %s", a.Step, a.Owner, owner, a.Reason)
	}
	return a
}

// ingestDoc submits a report with --json and returns the document; the
// command must exit with code.
func (r *repo) ingestDoc(code int, report []byte) ingestDoc {
	r.t.Helper()
	var d ingestDoc
	unmarshal(r.t, r.ingest(code, report, "--json").out, &d)
	return d
}

// rejected checks that a report is refused by the given layer with exactly
// one problem, at path, whose message contains want, and that the ledger did
// not change. It returns the message.
func (r *repo) rejected(report []byte, layer core.Layer, path, want string) string {
	r.t.Helper()
	before := r.read(".metron/state/ledger.jsonl")
	d := r.ingestDoc(exitNegative, report)
	if d.Accepted || d.Rejection == nil || d.Rejection.Layer != layer {
		r.t.Fatalf("ingest document %+v, want a rejection by the %s layer", d, layer)
	}
	if ps := d.Rejection.Problems; len(ps) != 1 || ps[0].Path != path || !strings.Contains(ps[0].Message, want) {
		r.t.Fatalf("the %s layer found %+v, want one problem at %s saying %q", layer, ps, path, want)
	}
	if string(before) != string(r.read(".metron/state/ledger.jsonl")) {
		r.t.Fatal("a rejected report changed the ledger")
	}
	return d.Rejection.Problems[0].Message
}

// red completes a reviewer skeleton with REQ-2 violated by the given
// blocking findings.
func red(t *testing.T, sk core.Report, author string, findings ...core.FindingInput) []byte {
	t.Helper()
	return review(t, sk, author, map[string]string{"REQ-2": core.RequirementViolated}, findings...)
}

// improvement is a T2 finding that cites nothing.
func improvement(key string) core.FindingInput {
	return core.FindingInput{Key: key, Tier: core.TierImprovement, Domain: core.DomainMaintainability,
		Title: "The greeting has no punctuation", Components: []string{"greeting.txt"},
		Description: "greeting.txt ends without a full stop", Evidence: "cat greeting.txt", Impact: "The greeting reads curtly",
		ExpectedBenefit: "A warmer greeting", DeferralRationale: "No reader has complained"}
}

// eligible is a T2 finding that already meets every rule of a blocking one,
// so that an audit may reclassify it to T1, and that declares it reverses the
// resolution of contradicts.
func eligible(key, contradicts string) core.FindingInput {
	f := improvement(key)
	f.Domain, f.Requirement, f.Contradicts = core.DomainObjectiveAlignment, "REQ-2", contradicts
	f.BlockingRationale, f.Verification = "The demonstration shows the greeting", "cat greeting.txt prints the agreed greeting"
	return f
}

// claim completes an implementer skeleton: every subject fixed by the given
// commits, with rootCause as its root-cause analysis when it is not empty.
func claim(t *testing.T, sk core.Report, author, rootCause string, commits ...string) []byte {
	t.Helper()
	var b core.ImplementerBody
	unmarshal(t, string(sk.Body), &b)
	for i := range b.Fixes {
		b.Fixes[i].Commits = commits
		b.Fixes[i].Summary = "change the greeting"
		b.Fixes[i].Components = []string{"greeting.txt"}
		b.Fixes[i].RootCause = rootCause
	}
	return complete(t, sk, author, b)
}

// dispute completes an implementer skeleton that disputes every subject.
func dispute(t *testing.T, sk core.Report, author string) []byte {
	t.Helper()
	var b core.ImplementerBody
	unmarshal(t, string(sk.Body), &b)
	d := core.ImplementerBody{Fixes: []core.Fix{}}
	for _, fx := range b.Fixes {
		d.Disputes = append(d.Disputes, core.Dispute{Finding: fx.Finding, Rationale: "helo is the house style"})
	}
	return complete(t, sk, author, d)
}

// auditBody fills an auditor skeleton: every escalation analysed and
// discharged, and every adjudicable finding upheld.
func auditBody(t *testing.T, sk core.Report) core.AuditorBody {
	t.Helper()
	var b core.AuditorBody
	unmarshal(t, string(sk.Body), &b)
	for i := range b.Resolutions {
		b.Resolutions[i].Analysis, b.Resolutions[i].Conclusion = "read the ledger and the candidate", "carry on"
	}
	for i := range b.Adjudications {
		b.Adjudications[i].Decision, b.Adjudications[i].Rationale = core.DecisionUphold, "it stands"
	}
	return b
}

// upheld completes an auditor skeleton with auditBody.
func upheld(t *testing.T, sk core.Report, author string) []byte {
	t.Helper()
	return complete(t, sk, author, auditBody(t, sk))
}

// decisionIDs returns the identifiers of decisions.
func decisionIDs(ds []core.Decision) string {
	ids := []string{}
	for _, d := range ds {
		ids = append(ids, d.ID)
	}
	return strings.Join(ids, " ")
}

// transcript records a campaign as an orchestrator sees it: each command,
// its exit status, the findings and escalations it created, and the action
// metron next names after it.
type transcript struct {
	r     *repo
	lines []string
}

// created returns the number of findings and escalations in the ledger.
func (tr *transcript) created() (int, int) {
	st := tr.r.state(tr.r.t)
	return len(st.Findings()), len(st.Escalations())
}

// record notes what was done, with its exit status and what it created
// since the counts findings and escalations, and checks that next then
// names step, owned by owner.
func (tr *transcript) record(what string, code, findings, escalations int, step core.Step, owner core.Role) actionDoc {
	tr.r.t.Helper()
	a := tr.r.expectAction(step, owner)
	line := fmt.Sprintf("%-44s exit %d", what, code)
	st := tr.r.state(tr.r.t)
	for _, f := range st.Findings()[findings:] {
		line += fmt.Sprintf("  opened %s (%s)", f.ID, f.Tier)
	}
	for _, e := range st.Escalations()[escalations:] {
		line += fmt.Sprintf("  raised %s (%s)", e.ID, e.Kind)
	}
	line += "\n    next: " + string(a.Step)
	if a.Owner != "" {
		line += " (" + string(a.Owner) + ")"
	}
	for _, l := range []struct {
		label string
		ids   []string
	}{{"packet", []string{a.Packet}}, {"findings", a.Findings}, {"escalations", a.Escalations}, {"requirements", a.Requirements}} {
		if len(l.ids) > 0 && l.ids[0] != "" {
			line += " " + l.label + " " + strings.Join(l.ids, ",")
		}
	}
	tr.lines = append(tr.lines, line)
	return a
}

// metron runs a command that must exit with code and records it.
func (tr *transcript) metron(code int, step core.Step, owner core.Role, args ...string) (result, actionDoc) {
	tr.r.t.Helper()
	f, e := tr.created()
	res := tr.r.expect(code, args...)
	return res, tr.record("metron "+strings.Join(args, " "), code, f, e, step, owner)
}

// packet issues the packet for the next action, which must be for role, and
// records it.
func (tr *transcript) packet(role core.Role) (core.Packet, core.Report) {
	tr.r.t.Helper()
	f, e := tr.created()
	p, sk := tr.r.packet()
	if p.Role != role {
		tr.r.t.Fatalf("packet %s is for the %s, want the %s", p.ID, p.Role, role)
	}
	tr.record(fmt.Sprintf("metron packet: %s %s", p.ID, p.Action), exitOK, f, e, core.StepSubmitReport, role)
	return p, sk
}

// ingest submits a report that must exit with code, records it under what,
// and returns the ingest document.
func (tr *transcript) ingest(what string, code int, report []byte, step core.Step, owner core.Role) ingestDoc {
	tr.r.t.Helper()
	f, e := tr.created()
	d := tr.r.ingestDoc(code, report)
	tr.record("ingest "+what, code, f, e, step, owner)
	return d
}

// git records a change made with git.
func (tr *transcript) git(what string, step core.Step, owner core.Role) actionDoc {
	tr.r.t.Helper()
	f, e := tr.created()
	return tr.record("git: "+what, exitOK, f, e, step, owner)
}

// String returns the transcript, one entry per command.
func (tr *transcript) String() string { return strings.Join(tr.lines, "\n") + "\n" }
