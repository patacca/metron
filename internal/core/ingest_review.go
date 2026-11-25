package core

import (
	"fmt"
	"slices"
	"strings"
)

// Ingestion of reviewer reports, and the tier rules every reported finding
// follows.

func (in *ingestion) checkReviewer(b ReviewerBody) {
	seen := make(map[Domain]int)
	for i, a := range b.Coverage {
		p := ptr("body", "coverage", i)
		if !a.Domain.Valid() {
			in.sem.add(p+"/domain", "%q is not a review domain", a.Domain)
		} else if j, dup := seen[a.Domain]; dup {
			in.sem.add(p+"/domain", "domain %q is already covered at %s: give exactly one entry per domain", a.Domain, ptr("body", "coverage", j))
		} else {
			seen[a.Domain] = i
		}
		switch a.Status {
		case CoverageAssessed:
		case CoverageNotApplicable:
			if id := firstIn(in.c.Applicable(), a.Domain); id != "" {
				in.sem.add(p+"/status", "domain %q is marked not-applicable but %s applies in it at stage %s: assess it", a.Domain, id, in.c.Stage)
			}
		default:
			in.sem.add(p+"/status", "%q is not %q or %q", a.Status, CoverageAssessed, CoverageNotApplicable)
		}
		if blank(a.Summary) {
			in.sem.add(p+"/summary", "the summary is blank: give the assessment or the rationale")
		}
	}
	for _, d := range Domains() {
		if _, ok := seen[d]; !ok {
			in.sem.add("/body/coverage", "domain %q has no entry: cover every one of the 18 domains", d)
		}
	}

	assessed := make(map[string]int)
	for i, ra := range b.Requirements {
		p := ptr("body", "requirements", i)
		if _, ok := in.applicable(ra.Requirement); !ok {
			in.sem.add(p+"/requirement", "%q is not an applicable requirement at stage %s: assess only applicable requirements", ra.Requirement, in.c.Stage)
		} else if j, dup := assessed[ra.Requirement]; dup {
			in.sem.add(p+"/requirement", "%s is already assessed at %s: give exactly one entry per applicable requirement", ra.Requirement, ptr("body", "requirements", j))
		} else {
			assessed[ra.Requirement] = i
		}
		switch ra.Status {
		case RequirementSatisfied, RequirementViolated, RequirementUnverified:
		default:
			in.sem.add(p+"/status", "%q is not %q, %q or %q", ra.Status, RequirementSatisfied, RequirementViolated, RequirementUnverified)
		}
		if blank(ra.Evidence) {
			in.sem.add(p+"/evidence", "the evidence is blank: say what the status rests on")
		}
	}
	for _, r := range in.c.Applicable() {
		if _, ok := assessed[r.ID]; !ok {
			in.sem.add("/body/requirements", "%s has no entry: assess every applicable requirement", r.ID)
		}
	}

	keys := make(map[string]string)
	for i, fi := range b.Findings {
		in.checkFinding(ptr("body", "findings", i), fi, keys, false)
	}

	reopened := make(map[string]string)
	for i, ro := range b.Reopen {
		p := ptr("body", "reopen", i)
		if !in.exists(ro.Finding) {
			in.sem.add(p+"/finding", "finding %q does not exist: reopen a finding of this ledger", ro.Finding)
		} else if prev, dup := reopened[ro.Finding]; dup {
			in.sem.add(p+"/finding", "finding %s is already reopened at %s: reopen it once", ro.Finding, prev)
		} else {
			reopened[ro.Finding] = p
			if x := in.find(ro.Finding); x.Tier.Blocking() && !in.citesContract(x.Input) {
				in.sem.add(p+"/finding", "blocking finding %s no longer cites an applicable requirement or an unacceptable risk of the pinned contract: open a new finding against the current contract instead", x.ID)
			}
		}
		if !ro.Reason.Valid() {
			in.sem.add(p+"/reason", "%q is not a reopen reason: use new-evidence, regression, invalid-verification, requirement-changed or contract-conflict", ro.Reason)
		}
		if blank(ro.Justification) {
			in.sem.add(p+"/justification", "the justification is blank: say what makes the resolution wrong")
		}
	}

	for i, g := range b.ContractGaps {
		p := ptr("body", "contract_gaps", i)
		if blank(g.Title) {
			in.sem.add(p+"/title", "the title is blank: name the uncovered risk")
		}
		if blank(g.Description) {
			in.sem.add(p+"/description", "the description is blank: describe the uncovered risk")
		}
		if g.SuggestedRequirement != "" && blank(g.SuggestedRequirement) {
			in.sem.add(p+"/suggested_requirement", "the suggested requirement is blank: write it or remove the member")
		}
	}

	// Requirements cited by the blocking findings this report opens or
	// reopens, each with the first finding that cites it, and whether the
	// report opens or reopens any blocking finding.
	cited := make(map[string]string)
	cite := func(req, by string) {
		if _, dup := cited[req]; req != "" && !dup {
			cited[req] = by
		}
	}
	blocking := false
	for i, fi := range b.Findings {
		if fi.Tier.Blocking() {
			blocking = true
			cite(fi.Requirement, ptr("body", "findings", i))
		}
	}
	for i, ro := range b.Reopen {
		if j := in.s.findingIndex(ro.Finding); j >= 0 && in.s.findings[j].Tier.Blocking() {
			blocking = true
			cite(in.s.findings[j].Input.Requirement, fmt.Sprintf("%s (%s)", ptr("body", "reopen", i), ro.Finding))
		}
	}
	unverified := false
	for i, ra := range b.Requirements {
		by, isCited := cited[ra.Requirement]
		switch {
		case ra.Status == RequirementViolated && !isCited:
			in.sem.add(ptr("body", "requirements", i, "status"), "%s is violated but no blocking finding this report opens or reopens cites it: open one, or correct the status", ra.Requirement)
		case ra.Status != RequirementViolated && isCited:
			in.sem.add(ptr("body", "requirements", i, "status"), "%s is cited by the blocking finding at %s but marked %q: mark it violated, or correct the finding", ra.Requirement, by, ra.Status)
		}
		if ra.Status == RequirementUnverified {
			unverified = true
		}
	}
	implied := VerdictGreen
	switch {
	case blocking:
		implied = VerdictRed
	case unverified:
		implied = VerdictIndeterminate
	}
	if !b.Verdict.Valid() {
		in.sem.add("/body/verdict", "%q is not GREEN, RED or INDETERMINATE", b.Verdict)
	} else if b.Verdict != implied {
		in.sem.add("/body/verdict", "the verdict is %s but the content implies %s (RED if a blocking finding is opened or reopened, else INDETERMINATE if a requirement is unverified, else GREEN): correct the verdict or the content", b.Verdict, implied)
	}
	if blank(b.Justification) {
		in.sem.add("/body/justification", "the justification is blank: justify the verdict")
	}
	statements(&in.sem, "/body/residual_risks", b.ResidualRisks, "residual risk")
	statements(&in.sem, "/body/limitations", b.Limitations, "limitation")
}

// checkFinding applies the tier rules to a finding a reviewer opens or a
// verifier reports as a regression.
func (in *ingestion) checkFinding(p string, fi FindingInput, keys map[string]string, regression bool) {
	if blank(fi.Key) {
		in.sem.add(p+"/key", "the key is blank: give each finding a key unique in the report")
	} else if prev, dup := keys[fi.Key]; dup {
		in.sem.add(p+"/key", "key %q is already used at %s: give each finding its own key", fi.Key, prev)
	} else {
		keys[fi.Key] = p
	}
	if !fi.Tier.Valid() {
		in.sem.add(p+"/tier", "%q is not a tier: use T0 to T4", fi.Tier)
	}
	if !fi.Domain.Valid() {
		in.sem.add(p+"/domain", "%q is not a review domain", fi.Domain)
	}
	for _, m := range []struct{ name, value, want string }{
		{"title", fi.Title, "name the finding"},
		{"description", fi.Description, "state the concrete failure mechanism"},
		{"evidence", fi.Evidence, "give a reproduction, an analysis or a measurement"},
		{"impact", fi.Impact, "state the expected effect inside the operating envelope"},
	} {
		if blank(m.value) {
			in.sem.add(p+"/"+m.name, "the %s is blank: %s", m.name, m.want)
		}
	}
	statements(&in.sem, p+"/components", fi.Components, "component")

	if fi.Requirement != "" {
		if _, ok := in.applicable(fi.Requirement); !ok {
			in.sem.add(p+"/requirement", "%q is not an applicable requirement at stage %s: cite an applicable requirement", fi.Requirement, in.c.Stage)
		}
	}
	if fi.Risk != "" && !slices.ContainsFunc(in.c.Risks.Unacceptable, func(r Risk) bool { return r.ID == fi.Risk }) {
		in.sem.add(p+"/risk", "%q is not one of the contract's unacceptable risks: cite an unacceptable risk", fi.Risk)
	}
	if fi.Tier.Blocking() {
		if fi.Requirement == "" && fi.Risk == "" {
			in.sem.add(p, "a blocking finding cites neither a requirement nor a risk: cite an applicable requirement or an unacceptable risk, or report the concern in contract_gaps")
		}
		for _, g := range blockingGaps(fi) {
			in.sem.add(p+"/"+g.member, "%s", g.message)
		}
	}
	switch fi.Tier {
	case TierImprovement:
		if blank(fi.ExpectedBenefit) {
			in.sem.add(p+"/expected_benefit", "a T2 finding states its expected benefit")
		}
		if blank(fi.DeferralRationale) {
			in.sem.add(p+"/deferral_rationale", "a T2 finding states why deferring it is acceptable")
		}
	case TierCalibration:
		if blank(fi.EvidenceNeeded) {
			in.sem.add(p+"/evidence_needed", "a T3 finding states the evidence needed")
		}
		if blank(fi.ReassessWhen) {
			in.sem.add(p+"/reassess_when", "a T3 finding states when to reassess it")
		}
	}
	for j, id := range fi.Related {
		if !in.exists(id) {
			in.sem.add(fmt.Sprintf("%s/related/%d", p, j), "finding %q does not exist: relate only findings of this ledger", id)
		}
	}
	if regression {
		if fi.IntroducedBy == "" {
			in.sem.add(p+"/introduced_by", "a regression names the finding whose fix introduced it")
		} else if !slices.Contains(in.pk.Subjects, fi.IntroducedBy) {
			in.sem.add(p+"/introduced_by", "%q is not a claimed fix of packet %s: name the fix that introduced the regression", fi.IntroducedBy, in.pk.Packet)
		}
	} else if fi.IntroducedBy != "" && !in.exists(fi.IntroducedBy) {
		in.sem.add(p+"/introduced_by", "finding %q does not exist: name a finding of this ledger", fi.IntroducedBy)
	}
	if fi.Contradicts != "" && !in.exists(fi.Contradicts) {
		in.sem.add(p+"/contradicts", "finding %q does not exist: name a finding of this ledger", fi.Contradicts)
	}
}

// gap is one thing a finding lacks to be a blocking one: the member that
// would hold it and what to write there.
type gap struct{ member, message string }

// blockingGaps lists what a finding lacks, beyond a citation of the
// contract, to meet the rules of a blocking finding: why it must be resolved
// before this stage, how a fix can be verified and, in the improvement-prone
// domains, the proportionality of the change.
func blockingGaps(fi FindingInput) []gap {
	var gaps []gap
	if blank(fi.BlockingRationale) {
		gaps = append(gaps, gap{"blocking_rationale", "the blocking rationale is blank: say why it must be resolved before this stage"})
	}
	if blank(fi.Verification) {
		gaps = append(gaps, gap{"verification", "the verification is blank: say how a fix can be verified"})
	}
	if !fi.Domain.NeedsProportionality() {
		return gaps
	}
	pr := fi.Proportionality
	if pr == nil {
		return append(gaps, gap{"proportionality", fmt.Sprintf("a blocking %s finding needs a proportionality statement: give the proposed change, its regression risk and the cost of deferring it", fi.Domain)})
	}
	for _, m := range []struct{ name, value string }{
		{"proposed_change", pr.ProposedChange}, {"regression_risk", pr.RegressionRisk}, {"deferral_cost", pr.DeferralCost},
	} {
		if blank(m.value) {
			gaps = append(gaps, gap{"proportionality/" + m.name, m.name + " is blank: state it"})
		}
	}
	return gaps
}

func (in *ingestion) reviewerTransition(b ReviewerBody) {
	for i, ro := range b.Reopen {
		if st := in.find(ro.Finding).Status; st != StatusResolved {
			in.tr.add(ptr("body", "reopen", i, "finding"), "finding %s is %s: only a resolved finding can be reopened", ro.Finding, st)
		}
	}
}

func (in *ingestion) reviewerEvents(b ReviewerBody) {
	pk := in.pk
	for _, ro := range b.Reopen {
		x := in.find(ro.Finding)
		in.add(EventFindingReopened, FindingReopened{ID: x.ID, Packet: pk.Packet, Reason: ro.Reason, Justification: ro.Justification,
			RootCauseRequired: x.Recurrences+1 >= in.th.RootCauseAfter})
	}
	for _, fi := range b.Findings {
		id := in.open(RoleReviewer, fi)
		if fi.Contradicts != "" {
			in.raise(EscalationContradiction, fmt.Sprintf("%s declares that it reverses the resolution of %s", id, fi.Contradicts), []string{id}, nil, nil)
		}
	}
	applicable := in.c.Applicable()
	results := make([]RequirementResult, 0, len(applicable))
	var unverified []string
	for _, r := range applicable {
		st := ""
		for _, ra := range b.Requirements {
			if ra.Requirement == r.ID {
				st = ra.Status
			}
		}
		results = append(results, RequirementResult{Requirement: r.ID, Status: st})
		if st == RequirementUnverified {
			unverified = append(unverified, r.ID)
		}
	}
	in.add(EventVerdictRecorded, VerdictRecorded{Round: pk.Round, Verdict: b.Verdict, Candidate: pk.Candidate.Digest,
		Contract: pk.Contract, Reviewer: in.r.Author.ID, Requirements: results})
	if b.Verdict != VerdictGreen {
		in.stall(fmt.Sprintf("the review of round %d, which ended %s", pk.Round, b.Verdict))
	}
	for _, g := range b.ContractGaps {
		in.raise(EscalationContractGap, g.Title+": "+g.Description, nil, nil, nil)
	}
	if b.Verdict == VerdictIndeterminate {
		in.raise(EscalationEvidenceGap, fmt.Sprintf("round %d is INDETERMINATE: %s unverified", pk.Round, strings.Join(unverified, ", ")), nil, nil, unverified)
	}
}
