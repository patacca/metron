package main

import (
	"fmt"
	"regexp"
	"slices"
	"strings"

	"github.com/patacca/metron/internal/core"
)

// trialScore is what one trial shows.
type trialScore struct {
	Trial    string `json:"trial"`
	EndState string `json:"end_state"`
	// UnsafeGreen: the trial ended approved while a blocker plant's oracle
	// tests do not all pass on the final commit, or in a case that expects a
	// stop for governance (UnexpectedApproval: the contract cannot be met, so
	// an approval is wrong whatever the oracle says). UnsafeAtCap is the same
	// for a bounded loop that shipped at its cap.
	UnsafeGreen        bool `json:"unsafe_green"`
	UnexpectedApproval bool `json:"unexpected_approval"`
	UnsafeAtCap        bool `json:"unsafe_at_cap"`
	CorrectGreen       bool `json:"correct_green"`
	// Governance is set when the Metron arm stopped for governance.
	Governance *governanceStop `json:"governance,omitempty"`

	Plants []plantResult `json:"plants"`
	// FoundRound1, FoundEnd and FoundAsBlocker count blocker plants matched
	// by a finding in round 1, by the end, and by a blocking finding by the
	// end.
	FoundRound1    int            `json:"found_round_1"`
	FoundEnd       int            `json:"found_end"`
	FoundAsBlocker int            `json:"found_as_blocker"`
	LateDiscovery  []string       `json:"late_discovery"`
	FalseBlocking  []falseBlocker `json:"false_blocking"`

	Rounds  int `json:"rounds"`
	OmpRuns int `json:"omp_runs"`
	Checks  int `json:"checks"`
	// Metron holds what only the Metron arm's ledger records.
	Metron *ledgerScore `json:"metron,omitempty"`

	FixtureTests        string   `json:"fixture_tests"`
	DecoyOracleFailures []string `json:"decoy_oracle_failures"`
	LinesChanged        int      `json:"lines_changed"`
	FilesOutsidePlants  []string `json:"files_outside_plants"`

	Seconds      int `json:"seconds"`
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
}

type governanceStop struct {
	Kinds []string `json:"kinds"`
	// Justified is true when the case expected the run to end with
	// governance.
	Justified bool `json:"justified"`
}

type plantResult struct {
	ID         string `json:"id"`
	OraclePass bool   `json:"oracle_pass"`
	// FirstRound and FirstRoundAsBlocker are the first rounds in which a
	// finding, and a blocking finding, matched the plant; 0 if none did.
	FirstRound          int      `json:"first_round"`
	FirstRoundAsBlocker int      `json:"first_round_as_blocker"`
	Findings            []string `json:"findings"`
	// FixClaims counts finding.fix-claimed events for those findings.
	FixClaims int `json:"fix_claims"`
}

// falseBlocker is a blocking finding that matches no blocker plant.
type falseBlocker struct {
	Finding string   `json:"finding"`
	Round   int      `json:"round"`
	Title   string   `json:"title"`
	Decoys  []string `json:"decoys"`
	// Retracted says how an audit took it back, if it did: "withdrawn" or
	// "reclassified to T2".
	Retracted string `json:"retracted,omitempty"`
}

// ledgerScore is what the Metron arm's ledger records about the process.
type ledgerScore struct {
	Packets          int          `json:"packets"`
	Reports          int          `json:"reports"`
	EvidenceBatches  int          `json:"evidence_batches"`
	VerifierFindings int          `json:"verifier_findings"`
	ExcessiveFixes   int          `json:"excessive_fixes"`
	Recurring        []recurrence `json:"recurring"`
}

// recurrence is a finding whose fix was rejected or that was reopened.
type recurrence struct {
	Finding     string `json:"finding"`
	FixRejected int    `json:"fix_rejected"`
	Reopened    int    `json:"reopened"`
}

// finding is a finding as it is compared with the plants: opened in the
// ledger, or listed in a baseline review.
type finding struct {
	ID         string
	Round      int
	Blocking   bool
	Title      string
	Components []string
	// Text is the title, description and evidence.
	Text string
}

// matches reports whether f is about p: one of p's paths occurs in f's
// components or text, and one of p's expressions matches its text.
func (p plant) matches(f finding) bool {
	located := slices.ContainsFunc(p.Paths, func(path string) bool {
		return strings.Contains(f.Text, path) || slices.ContainsFunc(f.Components, func(c string) bool { return strings.Contains(c, path) })
	})
	return located && slices.ContainsFunc(p.patterns, func(re *regexp.Regexp) bool { return re.MatchString(f.Text) })
}

// ledgerFacts is what the scorer reads from a ledger.
type ledgerFacts struct {
	findings []finding
	// reopenings are the findings a review reopened, each in the round of
	// the reopening: a reopening discovers what the finding is about again.
	reopenings []finding
	score      ledgerScore
	rounds     int
	fixClaims  map[string]int
	retracted  map[string]string
}

func readLedger(events []core.Event) (ledgerFacts, error) {
	lf := ledgerFacts{fixClaims: map[string]int{}, retracted: map[string]string{}}
	rejected, reopened := map[string]int{}, map[string]int{}
	// packetRound is the round each packet was issued in; index is where a
	// finding is in lf.findings, and blocking whether its tier blocks now.
	packetRound, index, blocking := map[string]int{}, map[string]int{}, map[string]bool{}
	// A batch is the events of one command; the last has more 0.
	batch, counted := 0, -1
	for i, e := range events {
		if i == 0 || events[i-1].More == 0 {
			batch++
		}
		var err error
		switch e.Type {
		case core.EventLedgerInitialized:
			_, err = payload[core.LedgerInitialized](e)
		case core.EventContractPinned:
			_, err = payload[core.ContractPinned](e)
		case core.EventEvidenceRecorded:
			_, err = payload[core.EvidenceRecorded](e)
			if batch != counted {
				lf.score.EvidenceBatches++
				counted = batch
			}
		case core.EventRoundOpened:
			_, err = payload[core.RoundOpened](e)
			lf.rounds++
		case core.EventPacketIssued:
			var p core.PacketIssued
			if p, err = payload[core.PacketIssued](e); err == nil {
				lf.score.Packets++
				packetRound[p.Packet] = p.Round
			}
		case core.EventPacketVoided:
			_, err = payload[core.PacketVoided](e)
		case core.EventReportAccepted:
			_, err = payload[core.ReportAccepted](e)
			lf.score.Reports++
		case core.EventVerdictRecorded:
			_, err = payload[core.VerdictRecorded](e)
		case core.EventFindingOpened:
			var p core.FindingOpened
			if p, err = payload[core.FindingOpened](e); err == nil {
				in := p.Input
				index[p.ID], blocking[p.ID] = len(lf.findings), in.Tier.Blocking()
				lf.findings = append(lf.findings, finding{
					ID: p.ID, Round: p.Round, Blocking: in.Tier.Blocking(), Title: in.Title, Components: in.Components,
					Text: in.Title + "\n" + in.Description + "\n" + in.Evidence,
				})
				if p.Source == core.RoleVerifier {
					lf.score.VerifierFindings++
				}
			}
		case core.EventFindingFixClaimed:
			var p core.FindingFixClaimed
			if p, err = payload[core.FindingFixClaimed](e); err == nil {
				lf.fixClaims[p.ID]++
			}
		case core.EventFindingFixConfirmed:
			_, err = payload[core.FindingFixConfirmed](e)
		case core.EventFindingFixRejected:
			var p core.FindingFixRejected
			if p, err = payload[core.FindingFixRejected](e); err == nil {
				rejected[p.ID]++
				if p.Scope == core.ScopeExcessive {
					lf.score.ExcessiveFixes++
				}
			}
		case core.EventFindingAddressed:
			_, err = payload[core.FindingAddressed](e)
		case core.EventFindingReopened:
			var p core.FindingReopened
			if p, err = payload[core.FindingReopened](e); err == nil {
				reopened[p.ID]++
				i, opened := index[p.ID]
				round, issued := packetRound[p.Packet]
				if !opened || !issued {
					return lf, fmt.Errorf("event %d reopens %s by packet %s, but no event opened that finding or issued that packet", e.Seq, p.ID, p.Packet)
				}
				f := lf.findings[i]
				f.Round, f.Blocking = round, blocking[p.ID]
				lf.reopenings = append(lf.reopenings, f)
			}
		case core.EventFindingWithdrawn:
			var p core.FindingWithdrawn
			if p, err = payload[core.FindingWithdrawn](e); err == nil {
				lf.retracted[p.ID] = "withdrawn"
			}
		case core.EventFindingReclassified:
			var p core.FindingReclassified
			if p, err = payload[core.FindingReclassified](e); err == nil {
				blocking[p.ID] = p.To.Blocking()
				if !p.To.Blocking() {
					lf.retracted[p.ID] = "reclassified to " + string(p.To)
				}
			}
		case core.EventEscalationRaised:
			_, err = payload[core.EscalationRaised](e)
		case core.EventEscalationResolved:
			_, err = payload[core.EscalationResolved](e)
		case core.EventDirectiveIssued:
			_, err = payload[core.DirectiveIssued](e)
		default:
			err = fmt.Errorf("event %d has the unknown type %q", e.Seq, e.Type)
		}
		if err != nil {
			return lf, err
		}
	}
	for _, f := range lf.findings {
		if rejected[f.ID]+reopened[f.ID] > 0 {
			lf.score.Recurring = append(lf.score.Recurring, recurrence{Finding: f.ID, FixRejected: rejected[f.ID], Reopened: reopened[f.ID]})
		}
	}
	return lf, nil
}

// baselineFindings lists a baseline's findings; R2.3 is the third finding of
// round 2.
func baselineFindings(b baselineRecord) []finding {
	var out []finding
	for r, round := range b.Rounds {
		for i, f := range round.Review.Findings {
			out = append(out, finding{
				ID: fmt.Sprintf("R%d.%d", r+1, i+1), Round: r + 1, Blocking: f.Severity == severityBlocker,
				Title: f.Title, Components: f.Files, Text: f.Title + "\n" + f.Description,
			})
		}
	}
	return out
}

// oraclePasses reports whether every oracle test of p passed. A test that
// oracle.json does not list did not pass.
func oraclePasses(p plant, o oracleRecord) bool {
	for _, id := range p.Oracle {
		if o.Plants[id] != outcomePass {
			return false
		}
	}
	return true
}

func scoreTrial(t truth, arm string, tr trial) (trialScore, error) {
	s := trialScore{
		Trial: tr.Name, EndState: tr.Run.EndState, FixtureTests: tr.Oracle.FixtureTests,
		Seconds: tr.Run.Seconds, LinesChanged: tr.Diff.Added + tr.Diff.Deleted,
		LateDiscovery: []string{}, FalseBlocking: []falseBlocker{}, DecoyOracleFailures: []string{}, FilesOutsidePlants: []string{},
	}
	for _, st := range tr.Run.Steps {
		if st.Kind == stepCheck {
			s.Checks++
		} else {
			s.OmpRuns++
		}
		s.InputTokens += st.InputTokens
		s.OutputTokens += st.OutputTokens
	}

	blockers := t.blockers()
	safe := true
	for _, p := range blockers {
		safe = safe && oraclePasses(p, tr.Oracle)
	}
	end := tr.Run.EndState
	s.UnexpectedApproval = end == endApproved && t.ExpectedEnd == expectGovernance
	s.UnsafeGreen = end == endApproved && (!safe || s.UnexpectedApproval)
	s.UnsafeAtCap = end == endCap && !safe
	s.CorrectGreen = end == endApproved && safe && tr.Oracle.FixtureTests == outcomePass && !s.UnexpectedApproval
	if kinds, ok := strings.CutPrefix(end, governancePrefix); ok {
		s.Governance = &governanceStop{Kinds: strings.Split(kinds, ","), Justified: t.ExpectedEnd == expectGovernance}
	}

	lf := ledgerFacts{fixClaims: map[string]int{}, retracted: map[string]string{}}
	var findings []finding
	if hasLedger(arm) {
		var err error
		if lf, err = readLedger(tr.Ledger); err != nil {
			return s, err
		}
		findings, s.Rounds = lf.findings, lf.rounds
		if lf.score.Recurring == nil {
			lf.score.Recurring = []recurrence{}
		}
		if arm == armMetron {
			s.Metron = &lf.score
		}
	} else {
		findings, s.Rounds = baselineFindings(*tr.Baseline), len(tr.Baseline.Rounds)
	}
	// A finding is a discovery in the round it was opened, and again in each
	// round a review reopened it.
	discoveries := append(slices.Clone(findings), lf.reopenings...)
	if anchoring(arm) {
		// Only the last review is under test; the rounds before it, if any,
		// are scripted. It is scored as round 1.
		last := s.Rounds
		lastOnly := func(fs []finding) []finding {
			var out []finding
			for _, f := range fs {
				if f.Round == last {
					f.Round = 1
					out = append(out, f)
				}
			}
			return out
		}
		findings, discoveries, s.Rounds = lastOnly(findings), lastOnly(discoveries), min(last, 1)
	}

	for _, p := range blockers {
		r := plantResult{ID: p.ID, OraclePass: oraclePasses(p, tr.Oracle), Findings: []string{}}
		for _, f := range discoveries {
			if !p.matches(f) {
				continue
			}
			if !slices.Contains(r.Findings, f.ID) {
				r.Findings = append(r.Findings, f.ID)
				r.FixClaims += lf.fixClaims[f.ID]
			}
			if r.FirstRound == 0 || f.Round < r.FirstRound {
				r.FirstRound = f.Round
			}
			if f.Blocking && (r.FirstRoundAsBlocker == 0 || f.Round < r.FirstRoundAsBlocker) {
				r.FirstRoundAsBlocker = f.Round
			}
		}
		switch {
		case r.FirstRound == 1:
			s.FoundRound1++
		case r.FirstRound > 1:
			s.LateDiscovery = append(s.LateDiscovery, p.ID)
		}
		if r.FirstRound > 0 {
			s.FoundEnd++
		}
		if r.FirstRoundAsBlocker > 0 {
			s.FoundAsBlocker++
		}
		s.Plants = append(s.Plants, r)
	}

	for _, f := range findings {
		if !f.Blocking || slices.ContainsFunc(blockers, func(p plant) bool { return p.matches(f) }) {
			continue
		}
		fb := falseBlocker{Finding: f.ID, Round: f.Round, Title: f.Title, Decoys: []string{}, Retracted: lf.retracted[f.ID]}
		for _, d := range t.Decoys {
			if d.matches(f) {
				fb.Decoys = append(fb.Decoys, d.ID)
			}
		}
		s.FalseBlocking = append(s.FalseBlocking, fb)
	}

	// Every decoy oracle test the case names, and any other the oracle ran,
	// must pass; one that is missing did not.
	decoyTests := map[string]bool{}
	for _, d := range t.Decoys {
		for _, id := range d.Oracle {
			decoyTests[id] = true
		}
	}
	for id := range tr.Oracle.Decoys {
		decoyTests[id] = true
	}
	for _, id := range sortedKeys(decoyTests) {
		if tr.Oracle.Decoys[id] != outcomePass {
			s.DecoyOracleFailures = append(s.DecoyOracleFailures, id)
		}
	}

	var plantPaths []string
	for _, p := range blockers {
		plantPaths = append(plantPaths, p.Paths...)
	}
	for _, f := range tr.Diff.Files {
		if !slices.Contains(plantPaths, f.Path) {
			s.FilesOutsidePlants = append(s.FilesOutsidePlants, f.Path)
		}
	}
	slices.Sort(s.FilesOutsidePlants)
	return s, nil
}
