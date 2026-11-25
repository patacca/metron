package core

import "fmt"

// Gate condition names, as in docs/architecture.md, section 11.
const (
	ConditionCriteria    = "C"
	ConditionBlockers    = "B"
	ConditionEvidence    = "E"
	ConditionIntegrity   = "I"
	ConditionIndependent = "V"
)

// Condition is one conjunct of the gate.
type Condition struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Satisfied   bool   `json:"satisfied"`
	// Reasons says why the condition does not hold. It is empty when it does.
	Reasons []string `json:"reasons,omitempty"`
}

// GateResult is the outcome of the gate G = C ∧ B ∧ E ∧ I ∧ V.
type GateResult struct {
	Open bool `json:"open"`
	// Stage is the pinned stage and Authorizes the activity a GREEN gate
	// permits. They are empty when unknown.
	Stage      Stage     `json:"stage,omitempty"`
	Authorizes string    `json:"authorizes,omitempty"`
	Candidate  Candidate `json:"candidate"`
	// Conditions lists C, B, E, I and V in that order.
	Conditions []Condition `json:"conditions"`
}

// Gate evaluates the five gate conditions. "The latest round" is the most
// recent round of the current stage that has a verdict. Anything it cannot
// establish keeps the gate closed.
func Gate(s State, f Facts) GateResult {
	ledger := ledgerProblem(s, f)
	contract := contractProblem(s, f)
	pin, pinned := s.pin()
	latest, reviewed := s.latestRound()
	noRound := "no round of the current stage has a verdict"
	if pinned {
		noRound = fmt.Sprintf("no round of stage %s has a verdict", pin.Stage)
	}

	var c, b, e, i, v []string

	switch {
	case contract != "":
		c = append(c, "the applicable requirements are unknown: "+contract)
	case !reviewed:
		c = append(c, noRound)
	default:
		for _, r := range f.Contract.Applicable() {
			switch st := statusOf(latest.Requirements, r.ID); st {
			case RequirementSatisfied:
			case "":
				c = append(c, fmt.Sprintf("%s was not assessed in round %d", r.ID, latest.Number))
			default:
				c = append(c, fmt.Sprintf("%s is %s in round %d", r.ID, st, latest.Number))
			}
		}
	}

	for _, x := range s.findings {
		switch {
		case x.Status.Unresolved() && x.Tier.Blocking():
			b = append(b, fmt.Sprintf("%s (%s) is %s", x.ID, x.Tier, x.Status))
		case x.carriedBy(f.Candidate.Digest):
			b = append(b, fmt.Sprintf("%s (%s) was recorded on this candidate, and the fix that resolved it was confirmed on another", x.ID, x.Tier))
		}
	}
	for _, x := range s.escalations {
		if x.Open {
			b = append(b, fmt.Sprintf("escalation %s (%s, owned by %s) is open", x.ID, x.Kind, x.Owner))
		}
	}

	if contract != "" {
		e = append(e, "the required checks are unknown: "+contract)
	} else {
		_, e = missingChecks(s, *f.Contract, f.Candidate.Digest)
	}

	if ledger != "" {
		i = append(i, ledger)
	}
	if contract != "" {
		i = append(i, contract)
	}
	switch {
	case !validCandidate(f.Candidate):
		i = append(i, "the current candidate cannot be identified: "+candidateProblem(f.Candidate))
	case !reviewed:
		i = append(i, noRound)
	case latest.Candidate.Digest != f.Candidate.Digest:
		i = append(i, fmt.Sprintf("the candidate %s differs from %s, reviewed in round %d", f.Candidate.Digest, latest.Candidate.Digest, latest.Number))
	}
	if !f.Clean {
		i = append(i, "the working tree has changes outside .metron/")
	}

	if !reviewed {
		v = append(v, noRound)
	} else {
		if latest.Verdict != VerdictGreen {
			v = append(v, fmt.Sprintf("round %d ended %s", latest.Number, latest.Verdict))
		}
		if latest.Candidate.Digest != f.Candidate.Digest {
			v = append(v, fmt.Sprintf("the verdict of round %d is for candidate %s, not %s", latest.Number, latest.Candidate.Digest, f.Candidate.Digest))
		}
		switch {
		case !pinned:
			// Cannot occur in a replayable ledger: a round opens only under
			// a pin and pins are never removed. Kept as the fail-closed default.
			v = append(v, fmt.Sprintf("the verdict of round %d is under contract %s, but no contract is pinned", latest.Number, latest.Contract))
		case latest.Contract != pin.Digest:
			v = append(v, fmt.Sprintf("the verdict of round %d was issued under contract %s, not the pinned one", latest.Number, latest.Contract))
		}
		switch {
		case blank(latest.Reviewer):
			// Cannot occur in a replayable ledger: Apply records a verdict
			// only for the non-blank author of the round's accepted report.
			// Kept as the fail-closed default.
			v = append(v, fmt.Sprintf("the verdict of round %d names no reviewer", latest.Number))
		case s.actedAs(latest.Reviewer, RoleImplementer):
			// Ingest refuses such a report, but Apply does not enforce role
			// separation, so a hand-edited ledger that replays can hold one.
			v = append(v, fmt.Sprintf("the verdict of round %d was issued by %q, who acted as implementer", latest.Number, latest.Reviewer))
		}
	}

	g := GateResult{Candidate: f.Candidate, Conditions: []Condition{
		condition(ConditionCriteria, "Every applicable requirement is satisfied in the reviewer report of the latest round", c),
		condition(ConditionBlockers, "No unresolved T0 or T1 finding, none that was recorded on this candidate and resolved on another, and no open escalation", b),
		condition(ConditionEvidence, "Every applicable requirement with a command has a passing result for this candidate and this command text", e),
		condition(ConditionIntegrity, "The candidate is the reviewed one, the tree is clean, the contract is the pinned one, and the ledger verifies under this protocol version", i),
		condition(ConditionIndependent, "The latest round's verdict is GREEN, for this candidate, under this contract, by an author who never acted as implementer", v),
	}}
	if pinned {
		g.Stage = pin.Stage
	}
	if contract == "" {
		g.Authorizes = f.Contract.Authorizes
	}
	g.Open = true
	for _, x := range g.Conditions {
		g.Open = g.Open && x.Satisfied
	}
	return g
}

func condition(name, description string, reasons []string) Condition {
	return Condition{Name: name, Description: description, Satisfied: len(reasons) == 0, Reasons: reasons}
}

// statusOf returns a requirement's status in a verdict, or "".
func statusOf(results []RequirementResult, id string) string {
	for _, r := range results {
		if r.Requirement == id {
			return r.Status
		}
	}
	return ""
}
