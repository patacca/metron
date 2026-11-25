// Command score reads what eval/run recorded for evaluation trials and reports
// what they show. It makes no model calls and runs no commands.
//
//	go run ./eval/score [-cases eval/cases] [-json file] <results-root>
//
// <results-root>/<case>/<arm>/<trial>/ holds one trial: run.json, oracle.json
// and diff.json, with ledger.jsonl for the metron and anchoring arms and
// baseline.json for the unrestricted and bounded arms. The ground truth is
// <cases>/<case>/truth.json. The report goes to standard output as Markdown;
// -json writes the same numbers to a file.
package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("score", flag.ContinueOnError)
	fs.SetOutput(stderr)
	cases := fs.String("cases", filepath.Join("eval", "cases"), "the `directory` that holds the cases and their truth.json")
	jsonOut := fs.String("json", "", "also write the numbers as JSON to `file`")
	fs.Usage = func() {
		fmt.Fprintln(stderr, "usage: score [-cases dir] [-json file] <results-root>")
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if fs.NArg() != 1 {
		fs.Usage()
		return 2
	}
	res, err := score(fs.Arg(0), *cases)
	if err != nil {
		fmt.Fprintf(stderr, "score: %v\n", err)
		return 1
	}
	if *jsonOut != "" {
		var b bytes.Buffer
		enc := json.NewEncoder(&b)
		enc.SetEscapeHTML(false)
		enc.SetIndent("", "  ")
		if err := enc.Encode(res); err != nil {
			fmt.Fprintf(stderr, "score: %v\n", err)
			return 1
		}
		if err := os.WriteFile(*jsonOut, b.Bytes(), 0o644); err != nil {
			fmt.Fprintf(stderr, "score: %v\n", err)
			return 1
		}
	}
	if _, err := io.WriteString(stdout, markdown(res)); err != nil {
		fmt.Fprintf(stderr, "score: %v\n", err)
		return 1
	}
	return 0
}

// results is everything the scorer reports.
type results struct {
	Trials      int          `json:"trials"`
	UnsafeGreen int          `json:"unsafe_green"`
	UnsafeAtCap int          `json:"unsafe_at_cap"`
	Cases       []caseResult `json:"cases"`
}

type caseResult struct {
	Case          string      `json:"case"`
	Base          string      `json:"base,omitempty"`
	ExpectedEnd   string      `json:"expected_end"`
	BlockerPlants []string    `json:"blocker_plants"`
	Decoys        []string    `json:"decoys"`
	Arms          []armResult `json:"arms"`
}

type armResult struct {
	Arm     string       `json:"arm"`
	Summary summary      `json:"summary"`
	Trials  []trialScore `json:"trials"`
}

// summary adds up an arm's trials. Found* count plants over all trials, out
// of BlockerPlants, which is the number of trials times the case's blocker
// plants.
type summary struct {
	Trials                 int     `json:"trials"`
	EndStates              []count `json:"end_states"`
	Approved               int     `json:"approved"`
	UnsafeGreen            int     `json:"unsafe_green"`
	UnexpectedApprovals    int     `json:"unexpected_approvals"`
	CorrectGreen           int     `json:"correct_green"`
	ShippedAtCap           int     `json:"shipped_at_cap"`
	UnsafeAtCap            int     `json:"unsafe_at_cap"`
	GovernanceStops        int     `json:"governance_stops"`
	GovernanceJustified    int     `json:"governance_justified"`
	GovernanceUnnecessary  int     `json:"governance_unnecessary"`
	GovernanceKinds        []count `json:"governance_kinds"`
	BlockerPlants          int     `json:"blocker_plants"`
	FoundRound1            int     `json:"found_round_1"`
	FoundEnd               int     `json:"found_end"`
	FoundAsBlocker         int     `json:"found_as_blocker"`
	LateDiscoveries        int     `json:"late_discoveries"`
	FalseBlocking          int     `json:"false_blocking"`
	FalseBlockingDecoy     int     `json:"false_blocking_decoy"`
	FalseBlockingRetracted int     `json:"false_blocking_retracted"`
	Rounds                 int     `json:"rounds"`
	OmpRuns                int     `json:"omp_runs"`
	Checks                 int     `json:"checks"`
	Packets                int     `json:"packets"`
	Reports                int     `json:"reports"`
	EvidenceBatches        int     `json:"evidence_batches"`
	RecurringFindings      int     `json:"recurring_findings"`
	VerifierFindings       int     `json:"verifier_findings"`
	ExcessiveFixes         int     `json:"excessive_fixes"`
	FixtureFailures        int     `json:"fixture_failures"`
	DecoyOracleFailures    int     `json:"decoy_oracle_failures"`
	LinesChanged           int     `json:"lines_changed"`
	FilesOutsidePlants     int     `json:"files_outside_plants"`
	Seconds                int     `json:"seconds"`
	InputTokens            int     `json:"input_tokens"`
	OutputTokens           int     `json:"output_tokens"`
}

type count struct {
	Name string `json:"name"`
	N    int    `json:"n"`
}

// score reads every trial under root.
func score(root, cases string) (results, error) {
	var res results
	caseNames, err := trialDirs(root)
	if err != nil {
		return res, err
	}
	for _, c := range caseNames {
		t, err := loadTruth(cases, c)
		if err != nil {
			return res, err
		}
		cr := caseResult{Case: c, Base: t.Base, ExpectedEnd: t.ExpectedEnd, BlockerPlants: []string{}, Decoys: []string{}}
		if cr.ExpectedEnd == "" {
			cr.ExpectedEnd = expectApproved
		}
		for _, p := range t.blockers() {
			cr.BlockerPlants = append(cr.BlockerPlants, p.ID)
		}
		for _, d := range t.Decoys {
			cr.Decoys = append(cr.Decoys, d.ID)
		}
		armNames, err := trialDirs(filepath.Join(root, c))
		if err != nil {
			return res, err
		}
		for _, a := range armNames {
			if !slices.Contains(arms, a) {
				return res, fmt.Errorf("%s: %q is not an arm: the arms are %s", filepath.Join(root, c), a, strings.Join(arms, ", "))
			}
		}
		for _, a := range arms {
			if !slices.Contains(armNames, a) {
				continue
			}
			ar, err := scoreArm(t, filepath.Join(root, c, a), c, a)
			if err != nil {
				return res, err
			}
			if len(ar.Trials) == 0 {
				continue
			}
			res.Trials += ar.Summary.Trials
			res.UnsafeGreen += ar.Summary.UnsafeGreen
			res.UnsafeAtCap += ar.Summary.UnsafeAtCap
			cr.Arms = append(cr.Arms, ar)
		}
		if len(cr.Arms) > 0 {
			res.Cases = append(res.Cases, cr)
		}
	}
	if res.Trials == 0 {
		return res, errors.New(root + " holds no trials: expected <results-root>/<case>/<arm>/<trial>/run.json")
	}
	return res, nil
}

func scoreArm(t truth, dir, caseName, arm string) (armResult, error) {
	ar := armResult{Arm: arm}
	names, err := trialDirs(dir)
	if err != nil {
		return ar, err
	}
	ends, kinds := map[string]int{}, map[string]int{}
	s := &ar.Summary
	for _, n := range names {
		tr, err := loadTrial(filepath.Join(dir, n), caseName, arm, n)
		if err != nil {
			return ar, err
		}
		ts, err := scoreTrial(t, arm, tr)
		if err != nil {
			return ar, fmt.Errorf("%s: %v", filepath.Join(dir, n), err)
		}
		ar.Trials = append(ar.Trials, ts)

		s.Trials++
		ends[ts.EndState]++
		s.Approved += b2i(ts.EndState == endApproved)
		s.ShippedAtCap += b2i(ts.EndState == endCap)
		s.UnsafeGreen += b2i(ts.UnsafeGreen)
		s.UnexpectedApprovals += b2i(ts.UnexpectedApproval)
		s.UnsafeAtCap += b2i(ts.UnsafeAtCap)
		s.CorrectGreen += b2i(ts.CorrectGreen)
		if g := ts.Governance; g != nil {
			s.GovernanceStops++
			s.GovernanceJustified += b2i(g.Justified)
			s.GovernanceUnnecessary += b2i(!g.Justified)
			for _, k := range g.Kinds {
				kinds[k]++
			}
		}
		s.BlockerPlants += len(ts.Plants)
		s.FoundRound1 += ts.FoundRound1
		s.FoundEnd += ts.FoundEnd
		s.FoundAsBlocker += ts.FoundAsBlocker
		s.LateDiscoveries += len(ts.LateDiscovery)
		for _, fb := range ts.FalseBlocking {
			s.FalseBlocking++
			s.FalseBlockingDecoy += b2i(len(fb.Decoys) > 0)
			s.FalseBlockingRetracted += b2i(fb.Retracted != "")
		}
		s.Rounds += ts.Rounds
		s.OmpRuns += ts.OmpRuns
		s.Checks += ts.Checks
		if m := ts.Metron; m != nil {
			s.Packets += m.Packets
			s.Reports += m.Reports
			s.EvidenceBatches += m.EvidenceBatches
			s.RecurringFindings += len(m.Recurring)
			s.VerifierFindings += m.VerifierFindings
			s.ExcessiveFixes += m.ExcessiveFixes
		}
		s.FixtureFailures += b2i(ts.FixtureTests != outcomePass)
		s.DecoyOracleFailures += len(ts.DecoyOracleFailures)
		s.LinesChanged += ts.LinesChanged
		s.FilesOutsidePlants += len(ts.FilesOutsidePlants)
		s.Seconds += ts.Seconds
		s.InputTokens += ts.InputTokens
		s.OutputTokens += ts.OutputTokens
	}
	s.EndStates, s.GovernanceKinds = counts(ends), counts(kinds)
	return ar, nil
}

func counts(m map[string]int) []count {
	out := []count{}
	for _, k := range sortedKeys(m) {
		out = append(out, count{Name: k, N: m[k]})
	}
	return out
}

func b2i(b bool) int {
	if b {
		return 1
	}
	return 0
}
