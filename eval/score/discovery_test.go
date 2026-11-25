package main

import (
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/patacca/metron/internal/core"
)

func TestReopeningIsADiscovery(t *testing.T) {
	// Round 1 opens a finding about each plant and a false blocker; round 2
	// is the review under test in an anchoring trial.
	round1 := [][]core.Draft{
		reviewIssued(1, "P-0001"),
		{reportAccepted("P-0001", core.RoleReviewer),
			opened("F-0001", 1, core.RoleReviewer, core.TierCritical, "INSERT OR REPLACE drops the history", []string{"shop/store.py"}, ""),
			opened("F-0002", 1, core.RoleReviewer, core.TierMaterial, "Amounts are a float", []string{"shop/prices.py"}, ""),
			opened("F-0003", 1, core.RoleReviewer, core.TierMaterial, "Logging is sparse", []string{"shop/log.py"}, "")},
		reviewIssued(2, "P-0002"),
	}
	round2 := func(drafts ...core.Draft) [][]core.Draft {
		return append(slices.Clone(round1), append([]core.Draft{reportAccepted("P-0002", core.RoleReviewer)}, drafts...))
	}
	for _, tc := range []struct {
		name   string
		arm    string
		ledger [][]core.Draft
		// rounds is the trial's round count; p1 and p2 the first rounds in
		// which a finding and a blocking finding matched each plant.
		rounds        int
		p1, p2        [2]int
		p1Findings    []string
		late          []string
		falseBlocking []string
	}{
		{name: "a reopening leaves an earlier discovery first", arm: armMetron, ledger: round2(reopened("F-0001", "P-0002")),
			rounds: 2, p1: [2]int{1, 1}, p2: [2]int{1, 1}, p1Findings: []string{"F-0001"}, falseBlocking: []string{"F-0003"}},
		{name: "a finding blocks from the reopening after an audit made it blocking", arm: armMetron, ledger: [][]core.Draft{
			reviewIssued(1, "P-0001"),
			{reportAccepted("P-0001", core.RoleReviewer),
				opened("F-0001", 1, core.RoleReviewer, core.TierImprovement, "INSERT OR REPLACE drops the history", []string{"shop/store.py"}, "")},
			{reclassified("F-0001", core.TierImprovement, core.TierMaterial)},
			reviewIssued(2, "P-0002"),
			{reportAccepted("P-0002", core.RoleReviewer), reopened("F-0001", "P-0002")},
		}, rounds: 2, p1: [2]int{1, 2}, p1Findings: []string{"F-0001"}},
		{name: "anchoring: the reopening in the last round is the discovery", arm: armAnchoringLedger, ledger: round2(reopened("F-0001", "P-0002")),
			rounds: 1, p1: [2]int{1, 1}, p1Findings: []string{"F-0001"}},
		{name: "anchoring: a new finding in the last round is the discovery", arm: armAnchoringLedger,
			ledger: round2(opened("F-0004", 2, core.RoleReviewer, core.TierCritical, "The cascade wipes the history", []string{"shop/schema.py"}, "")),
			rounds: 1, p1: [2]int{1, 1}, p1Findings: []string{"F-0004"}},
		{name: "anchoring: the scripted rounds discover nothing", arm: armAnchoringLedger, ledger: round2(), rounds: 1},
		{name: "anchoring: only the last round's false blockers count", arm: armAnchoringLedger,
			ledger: round2(opened("F-0004", 2, core.RoleReviewer, core.TierMaterial, "Errors are swallowed", []string{"shop/log.py"}, "")),
			rounds: 1, falseBlocking: []string{"F-0004"}},
		{name: "anchoring: a fresh ledger has the one round", arm: armAnchoringFresh, ledger: [][]core.Draft{
			reviewIssued(1, "P-0001"),
			{reportAccepted("P-0001", core.RoleReviewer),
				opened("F-0001", 1, core.RoleReviewer, core.TierImprovement, "INSERT OR REPLACE drops the history", []string{"shop/store.py"}, "")},
		}, rounds: 1, p1: [2]int{1, 0}, p1Findings: []string{"F-0001"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newFixture(t)
			end := endRoleFailure
			if anchoring(tc.arm) {
				end = reviewedPrefix + string(core.VerdictRed)
			}
			f.add(spec{arm: tc.arm, name: "1", end: end, ledger: tc.ledger})
			ts, s := f.scoreOne("shop", tc.arm)
			p1, p2 := ts.Plants[0], ts.Plants[1]
			if ts.Rounds != tc.rounds || [2]int{p1.FirstRound, p1.FirstRoundAsBlocker} != tc.p1 || [2]int{p2.FirstRound, p2.FirstRoundAsBlocker} != tc.p2 ||
				!slices.Equal(p1.Findings, append([]string{}, tc.p1Findings...)) {
				t.Fatalf("rounds %d, P1 %+v, P2 %+v; want %d, %v %v, %v", ts.Rounds, p1, p2, tc.rounds, tc.p1, tc.p1Findings, tc.p2)
			}
			if !slices.Equal(ts.LateDiscovery, append([]string{}, tc.late...)) || s.LateDiscoveries != len(tc.late) {
				t.Fatalf("late discoveries %v, want %v", ts.LateDiscovery, tc.late)
			}
			var falseBlocking []string
			for _, fb := range ts.FalseBlocking {
				falseBlocking = append(falseBlocking, fb.Finding)
			}
			if !slices.Equal(falseBlocking, tc.falseBlocking) {
				t.Fatalf("false blocking findings %v, want %v", falseBlocking, tc.falseBlocking)
			}
			if anchoring(tc.arm) && (ts.Metron != nil || ts.UnsafeGreen || ts.Governance != nil || s.Approved != 0 || s.GovernanceStops != 0) {
				t.Fatalf("an anchoring trial is neither an approval nor a stop and has no Metron process score: %+v, summary %+v", ts, s)
			}
		})
	}
}

func TestUnexpectedApproval(t *testing.T) {
	for _, tc := range []struct {
		name     string
		caseName string // shop expects an approval, shop-stuck a stop for governance
		arm      string
		end      string
		oracle   *oracleRecord
		// unsafe, unexpected and correct are the trial's verdicts.
		unsafe, unexpected, correct bool
	}{
		{name: "an approval the case expects", caseName: "shop", arm: armMetron, end: endApproved, correct: true},
		{name: "an approval where governance must decide, every oracle passing", caseName: "shop-stuck", arm: armMetron, end: endApproved,
			unsafe: true, unexpected: true},
		{name: "the same with a plant's oracle failing", caseName: "shop-stuck", arm: armMetron, end: endApproved,
			oracle: oracle("", map[string]string{"test_P1": outcomeFail}, nil), unsafe: true, unexpected: true},
		{name: "a bounded approval where governance must decide", caseName: "shop-stuck", arm: armBounded, end: endApproved,
			unsafe: true, unexpected: true},
		{name: "the stop the case expects", caseName: "shop-stuck", arm: armMetron, end: "governance:evidence-gap"},
		{name: "a baseline that does not approve", caseName: "shop-stuck", arm: armUnrestricted, end: endNotReady},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newFixture(t)
			f.write(filepath.Join(f.cases, "shop-stuck", "truth.json"), []byte(`{"case": "shop-stuck", "base": "shop", "expected_end": "governance"}`))
			f.add(spec{caseName: tc.caseName, arm: tc.arm, name: "1", end: tc.end, oracle: tc.oracle})
			ts, s := f.scoreOne(tc.caseName, tc.arm)
			if ts.UnsafeGreen != tc.unsafe || ts.UnexpectedApproval != tc.unexpected || ts.CorrectGreen != tc.correct {
				t.Fatalf("unsafe %v, unexpected %v, correct %v; want %v, %v, %v", ts.UnsafeGreen, ts.UnexpectedApproval, ts.CorrectGreen,
					tc.unsafe, tc.unexpected, tc.correct)
			}
			if s.UnsafeGreen != b2i(tc.unsafe) || s.UnexpectedApprovals != b2i(tc.unexpected) || s.CorrectGreen != b2i(tc.correct) {
				t.Fatalf("summary %+v does not add up the trial", s)
			}
			res, err := score(f.root, f.cases)
			if err != nil {
				t.Fatal(err)
			}
			md := markdown(res)
			for _, want := range []string{"an unexpected approval: the case expects a stop for governance", "(1 unexpected approval)"} {
				if strings.Contains(md, want) != tc.unexpected {
					t.Fatalf("the report has %q: %v, want %v:\n%s", want, !tc.unexpected, tc.unexpected, md)
				}
			}
		})
	}
}
