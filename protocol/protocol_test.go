package protocol

import (
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/patacca/metron/internal/core"
)

func protocolText(t *testing.T) string {
	t.Helper()
	b, err := FS.ReadFile("v1/protocol.md")
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// TestVocabularyMatchesCore checks that protocol.md names every member of
// every fixed vocabulary set, spelled exactly as the constants in
// internal/core, and lists ordered sets in their order.
func TestVocabularyMatchesCore(t *testing.T) {
	text := protocolText(t)
	sets := []struct {
		name    string
		members []string
		ordered bool
	}{
		{"stage", strs(core.Stages()), true},
		{"tier", strs(core.Tiers()), true},
		{"domain", strs(core.Domains()), true},
		{"verdict", []string{string(core.VerdictGreen), string(core.VerdictRed), string(core.VerdictIndeterminate)}, false},
		{"role", []string{string(core.RoleGovernance), string(core.RoleImplementer), string(core.RoleReviewer),
			string(core.RoleVerifier), string(core.RoleAuditor)}, false},
		{"reopen reason", []string{string(core.ReopenNewEvidence), string(core.ReopenRegression),
			string(core.ReopenInvalidVerification), string(core.ReopenRequirementChanged), string(core.ReopenContractConflict)}, true},
		{"escalation kind", strs(escalationKinds), true},
		{"finding status", []string{string(core.StatusOpen), string(core.StatusClaimed), string(core.StatusResolved),
			string(core.StatusWithdrawn)}, true},
		{"requirement status", []string{core.RequirementSatisfied, core.RequirementViolated, core.RequirementUnverified}, true},
		{"coverage status", []string{core.CoverageAssessed, core.CoverageNotApplicable}, true},
		{"verification result", []string{core.VerificationConfirmed, core.VerificationRejected}, true},
		{"adjudication decision", []string{core.DecisionUphold, core.DecisionWithdraw, core.DecisionReclassify}, true},
	}
	for _, set := range sets {
		last := -1
		for _, m := range set.members {
			at := strings.Index(text, "`"+m+"`")
			if at < 0 {
				t.Errorf("protocol.md does not name the %s `%s`", set.name, m)
				continue
			}
			if set.ordered && at < last {
				t.Errorf("protocol.md first names the %s `%s` before the one that precedes it", set.name, m)
			}
			last = at
		}
	}
}

// TestNoUnknownIdentifiers checks the converse: every hyphenated identifier
// protocol.md shows as code is one of the protocol's constants, so a
// misspelling cannot hide next to the correct spelling.
func TestNoUnknownIdentifiers(t *testing.T) {
	known := map[string]bool{}
	for _, set := range [][]string{strs(core.Stages()), strs(core.Domains()), strs(escalationKinds), strs(steps)} {
		for _, m := range set {
			known[m] = true
		}
	}
	for _, m := range []string{string(core.ReopenNewEvidence), string(core.ReopenInvalidVerification),
		string(core.ReopenRequirementChanged), string(core.ReopenContractConflict), core.CoverageNotApplicable} {
		known[m] = true
	}
	token := regexp.MustCompile("`([a-z]+(?:-[a-z]+)+)`")
	for _, m := range token.FindAllStringSubmatch(protocolText(t), -1) {
		if !known[m[1]] {
			t.Errorf("protocol.md shows `%s`, which is not a protocol identifier", m[1])
		}
	}
}

func TestTiersAndOwnersAgreeWithCore(t *testing.T) {
	text := protocolText(t)
	for _, tier := range core.Tiers() {
		blocks := "no"
		if tier.Blocking() {
			blocks = "yes"
		}
		row := regexp.MustCompile("(?m)^\\| `" + string(tier) + "` \\| [^|]+ \\| " + blocks + " \\|")
		if !row.MatchString(text) {
			t.Errorf("protocol.md's tier table does not say that %s blocks the gate: %s", tier, blocks)
		}
	}
	for _, kind := range escalationKinds {
		row := regexp.MustCompile("(?m)^\\| `" + string(kind) + "` \\| [^|]+ \\| `" + string(kind.Owner()) + "` \\|")
		if !row.MatchString(text) {
			t.Errorf("protocol.md's escalation table does not give %s the owner %s", kind, kind.Owner())
		}
	}
	for _, d := range core.Domains() {
		listed := strings.Contains(improvementProne(text), "`"+string(d)+"`")
		if listed != d.NeedsProportionality() {
			t.Errorf("protocol.md lists %s among the improvement-prone domains = %v, want %v", d, listed, d.NeedsProportionality())
		}
	}
	for _, s := range core.Stages() {
		var want []string
		for _, d := range stageAdds(s) {
			want = append(want, "`"+string(d)+"`")
		}
		row := "| `" + string(s) + "` | " + strings.Join(want, ", ") + " |"
		if !strings.Contains(text, row) {
			t.Errorf("protocol.md's stage coverage table lacks the row %s", row)
		}
	}
}

// improvementProne returns the sentence of protocol.md that lists the domains
// whose blocking findings need a proportionality test.
func improvementProne(text string) string {
	_, after, _ := strings.Cut(text, "## 8. Proportionality in improvement-prone domains")
	sentence, _, _ := strings.Cut(after, "`.")
	return sentence + "`"
}

// stageAdds returns the domains stage s adds to the mandatory coverage.
func stageAdds(s core.Stage) []core.Domain {
	all := core.StageCoverage(s)
	if s.Rank() == 0 {
		return all
	}
	before := core.StageCoverage(core.Stages()[s.Rank()-1])
	return slices.Clone(all[len(before):])
}

var escalationKinds = []core.EscalationKind{
	core.EscalationDispute, core.EscalationContradiction, core.EscalationComponentChurn,
	core.EscalationRegressionSpiral, core.EscalationNonConvergence, core.EscalationProcessAudit,
	core.EscalationContractGap, core.EscalationEvidenceGap,
}

var steps = []core.Step{
	core.StepRepairLedger, core.StepPinContract, core.StepSubmitReport, core.StepAdjudicate, core.StepAudit,
	core.StepVerify, core.StepFix, core.StepCommitChanges, core.StepRunChecks, core.StepReview, core.StepNone,
}

func strs[T ~string](items []T) []string {
	out := make([]string, len(items))
	for i, v := range items {
		out[i] = string(v)
	}
	return out
}
