package main

import (
	"bytes"
	jsonv2 "encoding/json/v2"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/patacca/metron/internal/core"
	"github.com/patacca/metron/internal/store"
)

// The values eval/run records and truth.json uses.
const (
	armMetron       = "metron"
	armUnrestricted = "unrestricted"
	armBounded      = "bounded"

	endApproved      = "approved"
	endNotReady      = "not-ready"
	endCap           = "cap"
	endRoleFailure   = "role-failure"
	endDriverCap     = "driver-cap"
	governancePrefix = "governance:"

	expectApproved   = "approved"
	expectGovernance = "governance"

	kindBlocker = "blocker"
	kindDecoy   = "decoy"

	severityBlocker    = "blocker"
	severityNonBlocker = "non-blocker"
	verdictApprove     = "approve"
	verdictChanges     = "request_changes"

	outcomePass  = "pass"
	outcomeFail  = "fail"
	outcomeError = "error"

	stepCheck = "check"
)

// arms lists the arms in the order the report shows them.
var arms = []string{armMetron, armUnrestricted, armBounded}

// truth is a case's truth.json. A variant names its base and inherits the
// base's plants and decoys unless it lists its own.
type truth struct {
	Case          string  `json:"case"`
	Base          string  `json:"base,omitempty"`
	Synthetic     bool    `json:"synthetic,omitempty"`
	Repo          string  `json:"repo,omitempty"`
	Contract      string  `json:"contract,omitempty"`
	TestCommand   string  `json:"test_command,omitempty"`
	OracleCommand string  `json:"oracle_command,omitempty"`
	ExpectedEnd   string  `json:"expected_end,omitempty"`
	Plants        []plant `json:"plants,omitempty"`
	Decoys        []plant `json:"decoys,omitempty"`
}

// plant is a planted defect or a decoy.
type plant struct {
	ID          string     `json:"id"`
	Kind        string     `json:"kind"`
	Tier        core.Tier  `json:"tier,omitempty"`
	MaxTier     core.Tier  `json:"max_tier,omitempty"`
	Domain      string     `json:"domain,omitempty"`
	Requirement string     `json:"requirement,omitempty"`
	Risk        string     `json:"risk,omitempty"`
	ContractRef *string    `json:"contract_ref,omitempty"`
	Paths       []string   `json:"paths"`
	Functions   []string   `json:"functions,omitempty"`
	Match       []string   `json:"match"`
	Oracle      []string   `json:"oracle"`
	Summary     string     `json:"summary,omitempty"`
	RootCause   *rootCause `json:"root_cause,omitempty"`
	Sites       []site     `json:"sites,omitempty"`

	patterns []*regexp.Regexp
}

type rootCause struct {
	Path     string `json:"path"`
	Function string `json:"function"`
	Summary  string `json:"summary"`
}

type site struct {
	Path     string `json:"path"`
	Function string `json:"function"`
	Oracle   string `json:"oracle"`
}

func (t truth) blockers() []plant {
	var out []plant
	for _, p := range t.Plants {
		if p.Kind == kindBlocker {
			out = append(out, p)
		}
	}
	return out
}

// runRecord is a trial's run.json.
type runRecord struct {
	Case     string `json:"case"`
	Arm      string `json:"arm"`
	Trial    string `json:"trial"`
	Started  string `json:"started"`
	Ended    string `json:"ended"`
	Seconds  int    `json:"seconds"`
	EndState string `json:"end_state"`
	Steps    []step `json:"steps"`
	Harness  string `json:"harness"`
}

type step struct {
	N            int    `json:"n"`
	Kind         string `json:"kind"`
	Role         string `json:"role"`
	Seconds      int    `json:"seconds"`
	Exit         int    `json:"exit"`
	Model        string `json:"model"`
	InputTokens  int    `json:"input_tokens"`
	OutputTokens int    `json:"output_tokens"`
}

// oracleRecord is a trial's oracle.json: the final commit judged by the
// fixture's own tests and by the hidden oracle, per test method.
type oracleRecord struct {
	FixtureTests string            `json:"fixture_tests"`
	Plants       map[string]string `json:"plants"`
	Decoys       map[string]string `json:"decoys"`
}

// diffRecord is a trial's diff.json: the seed against the final commit,
// without .metron/.
type diffRecord struct {
	Seed    string     `json:"seed"`
	Head    string     `json:"head"`
	Files   []fileDiff `json:"files"`
	Added   int        `json:"added"`
	Deleted int        `json:"deleted"`
}

type fileDiff struct {
	Path    string `json:"path"`
	Added   int    `json:"added"`
	Deleted int    `json:"deleted"`
}

// baselineRecord is a baseline trial's baseline.json. A round's review is
// the unrestricted agent's VERDICT.json or a bounded reviewer's REVIEW.json.
type baselineRecord struct {
	Arm    string          `json:"arm"`
	Rounds []baselineRound `json:"rounds"`
	Final  string          `json:"final"`
}

type baselineRound struct {
	Review review `json:"review"`
	Fixed  *bool  `json:"fixed,omitempty"`
}

type review struct {
	Ready    *bool             `json:"ready,omitempty"`
	Summary  *string           `json:"summary,omitempty"`
	Verdict  string            `json:"verdict,omitempty"`
	Findings []baselineFinding `json:"findings"`
}

type baselineFinding struct {
	Title       string   `json:"title"`
	Severity    string   `json:"severity"`
	Files       []string `json:"files"`
	Description string   `json:"description"`
}

// trial is everything recorded for one trial.
type trial struct {
	Name     string
	Run      runRecord
	Oracle   oracleRecord
	Diff     diffRecord
	Ledger   []core.Event    // the Metron arm
	Baseline *baselineRecord // the baselines
}

// decodeFile decodes a JSON file strictly: unknown and duplicate members are
// errors.
func decodeFile(path string, v any) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if err := jsonv2.Unmarshal(data, v, jsonv2.RejectUnknownMembers(true)); err != nil {
		return fmt.Errorf("%s: %v", path, err)
	}
	return nil
}

// loadTruth reads <cases>/<name>/truth.json, inherits from its base, and
// compiles the match expressions.
func loadTruth(cases, name string) (truth, error) {
	var t truth
	if err := decodeFile(filepath.Join(cases, name, "truth.json"), &t); err != nil {
		return t, err
	}
	if t.Case != name {
		return t, fmt.Errorf("%s/truth.json names case %q: it must name its own directory", name, t.Case)
	}
	if t.Base != "" {
		var b truth
		if err := decodeFile(filepath.Join(cases, t.Base, "truth.json"), &b); err != nil {
			return t, fmt.Errorf("case %s has base %s: %w", name, t.Base, err)
		}
		if b.Base != "" {
			return t, fmt.Errorf("case %s has base %s, which has a base of its own: a variant's base must hold the seed", name, t.Base)
		}
		if t.Plants == nil {
			t.Plants = b.Plants
		}
		if t.Decoys == nil {
			t.Decoys = b.Decoys
		}
	}
	if t.ExpectedEnd != "" && t.ExpectedEnd != expectApproved && t.ExpectedEnd != expectGovernance {
		return t, fmt.Errorf("case %s: expected_end %q is neither approved nor governance", name, t.ExpectedEnd)
	}
	if len(t.blockers()) == 0 {
		return t, fmt.Errorf("case %s has no blocker plant: nothing could make a GREEN unsafe", name)
	}
	for _, list := range [][]plant{t.Plants, t.Decoys} {
		for i := range list {
			p := &list[i]
			switch {
			case p.Kind != kindBlocker && p.Kind != kindDecoy:
				return t, fmt.Errorf("case %s: %s has kind %q, not blocker or decoy", name, p.ID, p.Kind)
			case len(p.Paths) == 0 || len(p.Match) == 0:
				return t, fmt.Errorf("case %s: %s needs at least one path and one match expression", name, p.ID)
			case p.Kind == kindBlocker && len(p.Oracle) == 0:
				return t, fmt.Errorf("case %s: blocker %s has no oracle test, so its fix cannot be judged", name, p.ID)
			}
			for _, expr := range p.Match {
				re, err := regexp.Compile(expr)
				if err != nil {
					return t, fmt.Errorf("case %s: %s: %v", name, p.ID, err)
				}
				p.patterns = append(p.patterns, re)
			}
		}
	}
	return t, nil
}

// loadTrial reads one trial directory and checks that it is what eval/run
// records for the case, arm and trial it is filed under.
func loadTrial(dir, caseName, arm, name string) (trial, error) {
	tr := trial{Name: name}
	if err := decodeFile(filepath.Join(dir, "run.json"), &tr.Run); err != nil {
		return tr, err
	}
	if r := tr.Run; r.Case != caseName || r.Arm != arm || r.Trial != name {
		return tr, fmt.Errorf("%s: run.json is for %s/%s/%s, but the directory is %s/%s/%s", dir, r.Case, r.Arm, r.Trial, caseName, arm, name)
	}
	if !validEnd(arm, tr.Run.EndState) {
		return tr, fmt.Errorf("%s: end state %q is not one the %s arm records", dir, tr.Run.EndState, arm)
	}
	if err := decodeFile(filepath.Join(dir, "oracle.json"), &tr.Oracle); err != nil {
		return tr, err
	}
	if err := checkOracle(tr.Oracle); err != nil {
		return tr, fmt.Errorf("%s: oracle.json: %v", dir, err)
	}
	if err := decodeFile(filepath.Join(dir, "diff.json"), &tr.Diff); err != nil {
		return tr, err
	}
	added, deleted := 0, 0
	for _, f := range tr.Diff.Files {
		added, deleted = added+f.Added, deleted+f.Deleted
	}
	if added != tr.Diff.Added || deleted != tr.Diff.Deleted {
		return tr, fmt.Errorf("%s: diff.json totals %d/%d do not match its files, %d/%d", dir, tr.Diff.Added, tr.Diff.Deleted, added, deleted)
	}
	if arm == armMetron {
		data, err := os.ReadFile(filepath.Join(dir, "ledger.jsonl"))
		if err != nil {
			return tr, err
		}
		if tr.Ledger, err = store.VerifyLedger(data); err != nil {
			return tr, fmt.Errorf("%s: ledger.jsonl: %v", dir, err)
		}
		return tr, nil
	}
	var b baselineRecord
	if err := decodeFile(filepath.Join(dir, "baseline.json"), &b); err != nil {
		return tr, err
	}
	if b.Arm != arm || b.Final != tr.Run.EndState {
		return tr, fmt.Errorf("%s: baseline.json says arm %s ending %s, run.json %s ending %s", dir, b.Arm, b.Final, arm, tr.Run.EndState)
	}
	for i, r := range b.Rounds {
		if err := checkReview(arm, r); err != nil {
			return tr, fmt.Errorf("%s: baseline.json, round %d: %v", dir, i+1, err)
		}
	}
	tr.Baseline = &b
	return tr, nil
}

func validEnd(arm, end string) bool {
	switch arm {
	case armMetron:
		if kinds, ok := strings.CutPrefix(end, governancePrefix); ok {
			return kinds != ""
		}
		return end == endApproved || end == endRoleFailure || end == endDriverCap
	case armUnrestricted:
		return end == endApproved || end == endNotReady || end == endRoleFailure
	case armBounded:
		return end == endApproved || end == endCap || end == endRoleFailure
	}
	return false
}

func checkOracle(o oracleRecord) error {
	if o.FixtureTests != outcomePass && o.FixtureTests != outcomeFail {
		return fmt.Errorf("fixture_tests is %q, not pass or fail", o.FixtureTests)
	}
	for _, m := range []map[string]string{o.Plants, o.Decoys} {
		for _, id := range sortedKeys(m) {
			if v := m[id]; v != outcomePass && v != outcomeFail && v != outcomeError {
				return fmt.Errorf("%s is %q, not pass, fail or error", id, v)
			}
		}
	}
	return nil
}

func checkReview(arm string, r baselineRound) error {
	rv := r.Review
	switch arm {
	case armUnrestricted:
		if rv.Ready == nil || rv.Summary == nil || rv.Verdict != "" || r.Fixed != nil {
			return errors.New("an unrestricted round holds a VERDICT.json: ready and summary, no verdict, no fixed")
		}
	case armBounded:
		if rv.Ready != nil || rv.Summary != nil || (rv.Verdict != verdictApprove && rv.Verdict != verdictChanges) {
			return errors.New("a bounded round holds a REVIEW.json: a verdict of approve or request_changes")
		}
	}
	for _, f := range rv.Findings {
		if f.Severity != severityBlocker && f.Severity != severityNonBlocker {
			return fmt.Errorf("finding %q has severity %q, not blocker or non-blocker", f.Title, f.Severity)
		}
	}
	return nil
}

// trialDirs lists the subdirectories of dir in a stable order: names that
// are numbers by value, then the others by name.
func trialDirs(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var names []string
	for _, e := range entries {
		if e.IsDir() {
			names = append(names, e.Name())
		}
	}
	slices.SortFunc(names, func(a, b string) int {
		x, errA := strconv.Atoi(a)
		y, errB := strconv.Atoi(b)
		switch {
		case errA == nil && errB == nil && x != y:
			return x - y
		case errA == nil && errB != nil:
			return -1
		case errA != nil && errB == nil:
			return 1
		}
		return strings.Compare(a, b)
	})
	return names, nil
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	return keys
}

// payload decodes an event's data strictly into its payload type.
func payload[T any](e core.Event) (T, error) {
	var v T
	if err := jsonv2.Unmarshal(bytes.TrimSpace(e.Data), &v, jsonv2.RejectUnknownMembers(true)); err != nil {
		return v, fmt.Errorf("event %d (%s): %v", e.Seq, e.Type, err)
	}
	return v, nil
}
