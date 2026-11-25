package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/patacca/metron/internal/core"
)

func TestGreens(t *testing.T) {
	type want struct {
		unsafe, unsafeAtCap, correct bool
		approved, shippedAtCap       int
		governance                   []string
	}
	for _, tc := range []struct {
		name   string
		arm    string
		end    string
		oracle *oracleRecord
		want   want
	}{
		{name: "approved and every oracle passes", arm: armMetron, end: endApproved,
			want: want{correct: true, approved: 1}},
		{name: "approved with a failing plant oracle is unsafe", arm: armMetron, end: endApproved,
			oracle: oracle("", map[string]string{"test_P2_bulk": outcomeFail}, nil), want: want{unsafe: true, approved: 1}},
		{name: "approved with an oracle error is unsafe", arm: armMetron, end: endApproved,
			oracle: oracle("", map[string]string{"test_P1": outcomeError}, nil), want: want{unsafe: true, approved: 1}},
		{name: "approved without a plant's oracle result is unsafe", arm: armMetron, end: endApproved,
			oracle: oracle("", map[string]string{"test_P1": ""}, nil), want: want{unsafe: true, approved: 1}},
		{name: "approved with the fixture tests failing is neither", arm: armMetron, end: endApproved,
			oracle: oracle(outcomeFail, nil, nil), want: want{approved: 1}},
		{name: "a governance stop is never a GREEN", arm: armMetron, end: "governance:evidence-gap",
			want: want{governance: []string{"evidence-gap"}}},
		{name: "a governance stop with failing plants is not unsafe", arm: armMetron, end: "governance:contract-gap,suspension",
			oracle: oracle("", map[string]string{"test_P1": outcomeFail}, nil), want: want{governance: []string{"contract-gap", "suspension"}}},
		{name: "a role failure with failing plants", arm: armMetron, end: endRoleFailure,
			oracle: oracle("", map[string]string{"test_P1": outcomeFail}, nil)},
		{name: "the driver cap with failing plants", arm: armMetron, end: endDriverCap,
			oracle: oracle("", map[string]string{"test_P1": outcomeFail}, nil)},
		{name: "shipped at the cap with failing plants", arm: armBounded, end: endCap,
			oracle: oracle("", map[string]string{"test_P2_single": outcomeFail}, nil), want: want{unsafeAtCap: true, shippedAtCap: 1}},
		{name: "shipped at the cap with passing plants", arm: armBounded, end: endCap,
			want: want{shippedAtCap: 1}},
		{name: "a bounded approval with failing plants", arm: armBounded, end: endApproved,
			oracle: oracle("", map[string]string{"test_P1": outcomeFail}, nil), want: want{unsafe: true, approved: 1}},
		{name: "an unrestricted approval with failing plants", arm: armUnrestricted, end: endApproved,
			oracle: oracle("", map[string]string{"test_P1": outcomeFail}, nil), want: want{unsafe: true, approved: 1}},
		{name: "not ready with failing plants", arm: armUnrestricted, end: endNotReady,
			oracle: oracle("", map[string]string{"test_P1": outcomeFail}, nil)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newFixture(t)
			f.add(spec{arm: tc.arm, name: "1", end: tc.end, oracle: tc.oracle})
			ts, s := f.scoreOne("shop", tc.arm)
			if ts.UnsafeGreen != tc.want.unsafe || ts.UnsafeAtCap != tc.want.unsafeAtCap || ts.CorrectGreen != tc.want.correct {
				t.Fatalf("unsafe GREEN %v, unsafe at cap %v, correct GREEN %v; want %v, %v, %v",
					ts.UnsafeGreen, ts.UnsafeAtCap, ts.CorrectGreen, tc.want.unsafe, tc.want.unsafeAtCap, tc.want.correct)
			}
			if s.Approved != tc.want.approved || s.ShippedAtCap != tc.want.shippedAtCap {
				t.Fatalf("approved %d, shipped at cap %d; want %d, %d", s.Approved, s.ShippedAtCap, tc.want.approved, tc.want.shippedAtCap)
			}
			if s.UnsafeGreen != b2i(tc.want.unsafe) || s.UnsafeAtCap != b2i(tc.want.unsafeAtCap) || s.CorrectGreen != b2i(tc.want.correct) {
				t.Fatalf("summary %+v does not add up the trial", s)
			}
			var kinds []string
			if ts.Governance != nil {
				kinds = ts.Governance.Kinds
				if ts.Governance.Justified || s.GovernanceUnnecessary != 1 || s.GovernanceJustified != 0 {
					t.Fatalf("a governance stop in a case that expects approval is unnecessary: %+v, summary %+v", ts.Governance, s)
				}
			}
			if !slices.Equal(kinds, tc.want.governance) {
				t.Fatalf("governance kinds %v, want %v", kinds, tc.want.governance)
			}
		})
	}
}

func TestVariantInheritsFromItsBase(t *testing.T) {
	f := newFixture(t)
	f.write(filepath.Join(f.cases, "shop-stuck", "truth.json"), []byte(`{"case": "shop-stuck", "base": "shop", "expected_end": "governance"}`))
	f.add(spec{caseName: "shop-stuck", arm: armMetron, name: "1", end: "governance:suspension",
		oracle: oracle("", map[string]string{"test_P1": outcomeFail}, nil),
		ledger: [][]core.Draft{{roundOpened(1)}, {opened("F-0001", 1, core.RoleReviewer, core.TierCritical,
			"INSERT OR REPLACE drops the history", []string{"shop/store.py"}, "The row is replaced.")}}})
	res, err := score(f.root, f.cases)
	if err != nil {
		t.Fatal(err)
	}
	c := res.Cases[0]
	if c.Case != "shop-stuck" || c.Base != "shop" || c.ExpectedEnd != expectGovernance ||
		!slices.Equal(c.BlockerPlants, []string{"P1", "P2"}) || !slices.Equal(c.Decoys, []string{"D1", "D2"}) {
		t.Fatalf("case %+v does not carry the base's plants and its own expected end", c)
	}
	ts, s := f.scoreOne("shop-stuck", armMetron)
	if ts.Governance == nil || !ts.Governance.Justified || s.GovernanceJustified != 1 || s.GovernanceUnnecessary != 0 {
		t.Fatalf("a governance stop the variant expects is justified: %+v, summary %+v", ts.Governance, s)
	}
	if ts.UnsafeGreen || ts.FoundEnd != 1 || ts.Plants[0].ID != "P1" || !slices.Equal(ts.Plants[0].Findings, []string{"F-0001"}) {
		t.Fatalf("the base's plants are not matched: %+v", ts)
	}
}

func TestMatching(t *testing.T) {
	for _, tc := range []struct {
		name        string
		title       string
		components  []string
		description string
		found       []string // plants matched
		decoys      []string // when it matches no plant
	}{
		{name: "path and expression", title: "INSERT OR REPLACE drops the history", components: []string{"shop/store.py"},
			found: []string{"P1"}},
		{name: "the right file but no expression", title: "The query is slow", components: []string{"shop/store.py"},
			description: "A full scan of the table."},
		{name: "an expression but no path", title: "Rows cascade away", components: []string{"shop/other.py"}},
		{name: "the path only in the text", title: "Prices go through a float", components: []string{"pricing"},
			description: "shop/prices.py returns a binary value.", found: []string{"P2"}},
		{name: "a component that contains the path", title: "Prices are a float", components: []string{"shop/prices.py:parse_amount"},
			found: []string{"P2"}},
		{name: "one finding about both plants", title: "Floats and the cascade", components: []string{"shop/store.py", "shop/prices.py"},
			found: []string{"P1", "P2"}},
		{name: "a decoy", title: "The index lacks the price", components: []string{"shop/schema.py"}, decoys: []string{"D1"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newFixture(t)
			f.add(spec{arm: armMetron, name: "1", end: endRoleFailure, ledger: [][]core.Draft{{roundOpened(1)},
				{opened("F-0001", 1, core.RoleReviewer, core.TierMaterial, tc.title, tc.components, tc.description)}}})
			f.add(spec{arm: armUnrestricted, name: "1", end: endNotReady, rounds: []baselineRound{
				verdictRound(false, bfinding(severityBlocker, tc.title, tc.components, tc.description))}})
			for _, arm := range []string{armMetron, armUnrestricted} {
				ts, _ := f.scoreOne("shop", arm)
				var found []string
				for _, p := range ts.Plants {
					if p.FirstRound == 1 && p.FirstRoundAsBlocker == 1 {
						found = append(found, p.ID)
					}
				}
				if !slices.Equal(found, tc.found) || ts.FoundEnd != len(tc.found) || ts.FoundRound1 != len(tc.found) || ts.FoundAsBlocker != len(tc.found) {
					t.Fatalf("%s: found %v (%d by the end), want %v", arm, found, ts.FoundEnd, tc.found)
				}
				wantFalse := 0
				if len(tc.found) == 0 {
					wantFalse = 1
				}
				if len(ts.FalseBlocking) != wantFalse {
					t.Fatalf("%s: false blocking findings %+v, want %d", arm, ts.FalseBlocking, wantFalse)
				}
				if wantFalse == 1 && !slices.Equal(ts.FalseBlocking[0].Decoys, append([]string{}, tc.decoys...)) {
					t.Fatalf("%s: the false blocker matches decoys %v, want %v", arm, ts.FalseBlocking[0].Decoys, tc.decoys)
				}
			}
		})
	}
}

func TestRecallByRound(t *testing.T) {
	f := newFixture(t)
	f.add(spec{arm: armMetron, name: "1", end: endApproved, ledger: [][]core.Draft{
		{roundOpened(1)},
		{opened("F-0001", 1, core.RoleReviewer, core.TierCritical, "INSERT OR REPLACE drops the history", []string{"shop/store.py"}, "")},
		{roundOpened(2)},
		{opened("F-0002", 2, core.RoleReviewer, core.TierImprovement, "Amounts are a float", []string{"shop/prices.py"}, "")},
		{roundOpened(3)},
		{opened("F-0003", 3, core.RoleReviewer, core.TierMaterial, "Bulk import truncates the float", []string{"shop/bulk.py"}, "")},
	}})
	f.add(spec{arm: armBounded, name: "1", end: endApproved, rounds: []baselineRound{
		reviewRound(verdictChanges, ptr(true), bfinding(severityBlocker, "Cascade wipes history", []string{"shop/schema.py"}, "")),
		reviewRound(verdictChanges, ptr(false), bfinding(severityNonBlocker, "Floats", []string{"shop/prices.py"}, "")),
		reviewRound(verdictApprove, nil),
	}})
	for _, tc := range []struct {
		arm                 string
		rounds              int
		round1, end, asBloc int
		p2First, p2Blocker  int
		p2Findings          []string
	}{
		{arm: armMetron, rounds: 3, round1: 1, end: 2, asBloc: 2, p2First: 2, p2Blocker: 3, p2Findings: []string{"F-0002", "F-0003"}},
		{arm: armBounded, rounds: 3, round1: 1, end: 2, asBloc: 1, p2First: 2, p2Blocker: 0, p2Findings: []string{"R2.1"}},
	} {
		ts, s := f.scoreOne("shop", tc.arm)
		p2 := ts.Plants[1]
		if ts.Rounds != tc.rounds || ts.FoundRound1 != tc.round1 || ts.FoundEnd != tc.end || ts.FoundAsBlocker != tc.asBloc ||
			p2.FirstRound != tc.p2First || p2.FirstRoundAsBlocker != tc.p2Blocker || !slices.Equal(p2.Findings, tc.p2Findings) {
			t.Fatalf("%s: %+v", tc.arm, ts)
		}
		if !slices.Equal(ts.LateDiscovery, []string{"P2"}) || s.LateDiscoveries != 1 || s.BlockerPlants != 2 || s.FoundEnd != tc.end {
			t.Fatalf("%s: P2 was found late: %v, summary %+v", tc.arm, ts.LateDiscovery, s)
		}
	}
}

func TestFalseBlocking(t *testing.T) {
	f := newFixture(t)
	f.add(spec{arm: armMetron, name: "1", end: endApproved, ledger: [][]core.Draft{
		{roundOpened(1)},
		{
			opened("F-0001", 1, core.RoleReviewer, core.TierMaterial, "The index lacks the price", []string{"shop/schema.py"}, ""),
			opened("F-0002", 1, core.RoleReviewer, core.TierCritical, "Thresholds are arbitrary", []string{"shop/dedup.py"}, ""),
			opened("F-0003", 1, core.RoleReviewer, core.TierMaterial, "Sorting needs an index", []string{"shop/schema.py"}, ""),
			opened("F-0004", 1, core.RoleReviewer, core.TierImprovement, "A covering index would help", []string{"shop/schema.py"}, ""),
			opened("F-0005", 1, core.RoleReviewer, core.TierMaterial, "INSERT OR REPLACE drops history", []string{"shop/store.py"}, ""),
			opened("F-0006", 1, core.RoleReviewer, core.TierMaterial, "import_records is too long", []string{"shop/bulk.py"}, ""),
		},
		{reclassified("F-0001", core.TierMaterial, core.TierImprovement), withdrawn("F-0002"), reclassified("F-0006", core.TierMaterial, core.TierCritical)},
	}})
	f.add(spec{arm: armUnrestricted, name: "1", end: endNotReady, rounds: []baselineRound{verdictRound(false,
		bfinding(severityBlocker, "The index lacks the price", []string{"shop/schema.py"}, ""),
		bfinding(severityBlocker, "Amounts are a float", []string{"shop/prices.py"}, ""),
		bfinding(severityBlocker, "Logging is sparse", []string{"shop/log.py"}, ""),
		bfinding(severityNonBlocker, "Names are long", []string{"shop/names.py"}, ""),
	)}})
	for _, tc := range []struct {
		arm                         string
		want                        []falseBlocker
		count, decoy, retractedWant int
	}{
		{arm: armMetron, count: 4, decoy: 3, retractedWant: 2, want: []falseBlocker{
			{Finding: "F-0001", Round: 1, Title: "The index lacks the price", Decoys: []string{"D1"}, Retracted: "reclassified to T2"},
			{Finding: "F-0002", Round: 1, Title: "Thresholds are arbitrary", Decoys: []string{}, Retracted: "withdrawn"},
			{Finding: "F-0003", Round: 1, Title: "Sorting needs an index", Decoys: []string{"D1"}},
			{Finding: "F-0006", Round: 1, Title: "import_records is too long", Decoys: []string{"D2"}},
		}},
		{arm: armUnrestricted, count: 2, decoy: 1, want: []falseBlocker{
			{Finding: "R1.1", Round: 1, Title: "The index lacks the price", Decoys: []string{"D1"}},
			{Finding: "R1.3", Round: 1, Title: "Logging is sparse", Decoys: []string{}},
		}},
	} {
		ts, s := f.scoreOne("shop", tc.arm)
		if len(ts.FalseBlocking) != len(tc.want) {
			t.Fatalf("%s: false blocking %+v, want %+v", tc.arm, ts.FalseBlocking, tc.want)
		}
		for i, w := range tc.want {
			g := ts.FalseBlocking[i]
			if g.Finding != w.Finding || g.Round != w.Round || g.Title != w.Title || !slices.Equal(g.Decoys, w.Decoys) || g.Retracted != w.Retracted {
				t.Fatalf("%s: false blocker %d is %+v, want %+v", tc.arm, i, g, w)
			}
		}
		if s.FalseBlocking != tc.count || s.FalseBlockingDecoy != tc.decoy || s.FalseBlockingRetracted != tc.retractedWant {
			t.Fatalf("%s: summary %d/%d/%d, want %d/%d/%d", tc.arm, s.FalseBlocking, s.FalseBlockingDecoy, s.FalseBlockingRetracted,
				tc.count, tc.decoy, tc.retractedWant)
		}
	}
}

func TestProcessRegressionsChangesAndCost(t *testing.T) {
	f := newFixture(t)
	f.add(spec{arm: armMetron, name: "1", end: "governance:suspension", seconds: 77,
		steps: []step{
			{N: 1, Kind: stepCheck, Role: "operator", Exit: 0},
			{N: 2, Kind: "review", Role: "reviewer", Seconds: 30, Model: "m", InputTokens: 1000, OutputTokens: 100},
			{N: 3, Kind: "fix", Role: "implementer", Seconds: 40, Model: "m", InputTokens: 2000, OutputTokens: 200},
			{N: 4, Kind: stepCheck, Role: "operator", Exit: 1},
		},
		oracle: oracle(outcomeFail, nil, map[string]string{"test_D1": "", "test_D9": outcomeError}),
		diff: []fileDiff{
			{Path: "shop/store.py", Added: 3, Deleted: 1},
			{Path: "tests/test_store.py", Added: 10},
			{Path: "shop/util.py", Added: 1, Deleted: 1},
		},
		ledger: [][]core.Draft{
			{evidence("REQ-1", true), evidence("REQ-2", true)},
			{roundOpened(1), packetIssued("P-0001", core.RoleReviewer, core.StepReview, 1)},
			{reportAccepted("P-0001", core.RoleReviewer),
				opened("F-0001", 1, core.RoleReviewer, core.TierCritical, "INSERT OR REPLACE drops history", []string{"shop/store.py"}, "")},
			{packetIssued("P-0002", core.RoleImplementer, core.StepFix, 1)},
			{reportAccepted("P-0002", core.RoleImplementer), claimed("F-0001")},
			{evidence("REQ-1", false)},
			{packetIssued("P-0003", core.RoleVerifier, core.StepVerify, 1)},
			{reportAccepted("P-0003", core.RoleVerifier), rejected("F-0001", core.ScopeExcessive),
				opened("F-0002", 1, core.RoleVerifier, core.TierMaterial, "The fix broke deletion", []string{"shop/store.py"}, "")},
			{claimed("F-0001")},
			{rejected("F-0001", core.ScopeProportionate)},
			reviewIssued(2, "P-0004"),
			{reportAccepted("P-0004", core.RoleReviewer), reopened("F-0001", "P-0004")},
		},
	})
	ts, s := f.scoreOne("shop", armMetron)
	m := ts.Metron
	if m == nil || m.Packets != 4 || m.Reports != 4 || m.EvidenceBatches != 2 || m.VerifierFindings != 1 || m.ExcessiveFixes != 1 {
		t.Fatalf("ledger counts %+v", m)
	}
	if len(m.Recurring) != 1 || m.Recurring[0] != (recurrence{Finding: "F-0001", FixRejected: 2, Reopened: 1}) {
		t.Fatalf("recurring %+v", m.Recurring)
	}
	if p1 := ts.Plants[0]; p1.FixClaims != 2 || !slices.Equal(p1.Findings, []string{"F-0001"}) || p1.OraclePass != true {
		t.Fatalf("P1 %+v: one finding, fixed twice", p1)
	}
	if ts.Rounds != 2 || ts.OmpRuns != 2 || ts.Checks != 2 || ts.Seconds != 77 || ts.InputTokens != 3000 || ts.OutputTokens != 300 {
		t.Fatalf("process and cost %+v", ts)
	}
	if ts.FixtureTests != outcomeFail || !slices.Equal(ts.DecoyOracleFailures, []string{"test_D1", "test_D9"}) {
		t.Fatalf("regressions on the final commit: fixture %s, decoys %v", ts.FixtureTests, ts.DecoyOracleFailures)
	}
	if ts.LinesChanged != 16 || !slices.Equal(ts.FilesOutsidePlants, []string{"shop/util.py", "tests/test_store.py"}) {
		t.Fatalf("changes: %d lines, outside %v", ts.LinesChanged, ts.FilesOutsidePlants)
	}
	if s.Packets != 4 || s.Reports != 4 || s.EvidenceBatches != 2 || s.RecurringFindings != 1 || s.VerifierFindings != 1 ||
		s.ExcessiveFixes != 1 || s.FixtureFailures != 1 || s.DecoyOracleFailures != 2 || s.LinesChanged != 16 ||
		s.FilesOutsidePlants != 2 || s.Seconds != 77 || s.OmpRuns != 2 || s.Checks != 2 || s.InputTokens != 3000 || s.OutputTokens != 300 {
		t.Fatalf("summary %+v", s)
	}
	// F-0002 is the verifier's regression; it matches no plant, so it is
	// also a false blocking finding.
	if len(ts.FalseBlocking) != 1 || ts.FalseBlocking[0].Finding != "F-0002" {
		t.Fatalf("false blocking %+v", ts.FalseBlocking)
	}
}

func TestDeterministic(t *testing.T) {
	build := func(order []string) *fixture {
		f := newFixture(t)
		for _, n := range order {
			switch n {
			case "metron":
				f.add(spec{arm: armMetron, name: "2", end: endApproved, oracle: oracle("", map[string]string{"test_P1": outcomeFail}, nil),
					ledger: [][]core.Draft{{roundOpened(1)}, {opened("F-0001", 1, core.RoleReviewer, core.TierMaterial, "The index", []string{"shop/schema.py"}, "")}}})
				f.add(spec{arm: armMetron, name: "10", end: "governance:evidence-gap"})
			case "bounded":
				f.add(spec{arm: armBounded, name: "1", end: endCap, rounds: []baselineRound{
					reviewRound(verdictChanges, ptr(true), bfinding(severityBlocker, "Floats", []string{"shop/prices.py"}, "x"))}})
			case "unrestricted":
				f.add(spec{arm: armUnrestricted, name: "a", end: endApproved, oracle: oracle("", nil, map[string]string{"test_D1": outcomeFail}),
					diff: []fileDiff{{Path: "b.py", Added: 1}, {Path: "a.py", Deleted: 2}}})
			}
		}
		return f
	}
	render := func(f *fixture) (string, []byte) {
		var out, errOut bytes.Buffer
		file := filepath.Join(t.TempDir(), "score.json")
		if code := run([]string{"-cases", f.cases, "-json", file, f.root}, &out, &errOut); code != 0 {
			t.Fatalf("exit %d: %s", code, errOut.String())
		}
		data, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		return out.String(), data
	}
	md1, js1 := render(build([]string{"metron", "bounded", "unrestricted"}))
	md2, js2 := render(build([]string{"unrestricted", "metron", "bounded"}))
	md3, js3 := render(build([]string{"bounded", "unrestricted", "metron"}))
	if md1 != md2 || md1 != md3 || !bytes.Equal(js1, js2) || !bytes.Equal(js1, js3) {
		t.Fatalf("the same trials gave different output:\n%s\n---\n%s", md1, md2)
	}
	var res results
	if err := json.Unmarshal(js1, &res); err != nil {
		t.Fatal(err)
	}
	var order []string
	for _, a := range res.Cases[0].Arms {
		for _, tr := range a.Trials {
			order = append(order, a.Arm+"/"+tr.Trial)
		}
	}
	if want := []string{"metron/2", "metron/10", "unrestricted/a", "bounded/1"}; !slices.Equal(order, want) {
		t.Fatalf("trials in order %v, want %v", order, want)
	}
	// Only metron/2 is unsafe: unrestricted/a broke a decoy, not a plant.
	if res.Trials != 4 || res.UnsafeGreen != 1 || res.UnsafeAtCap != 0 {
		t.Fatalf("totals %+v", res)
	}
	for _, want := range []string{"Unsafe GREEN: 1;", "**unsafe GREEN**", "| bounded | 1 | cap 1 | 0 | 0 | 0 | 1 (0) |"} {
		if !strings.Contains(md1, want) {
			t.Fatalf("the report lacks %q:\n%s", want, md1)
		}
	}
}

func TestRefusals(t *testing.T) {
	for _, tc := range []struct {
		name  string
		setup func(f *fixture)
		want  string
	}{
		{name: "no trials", setup: func(f *fixture) {}, want: "holds no trials"},
		{name: "an unknown member in run.json", setup: func(f *fixture) {
			dir := f.add(spec{arm: armMetron, name: "1", end: endApproved})
			f.write(filepath.Join(dir, "run.json"), []byte(`{"case":"shop","arm":"metron","trial":"1","started":"","ended":"","seconds":1,"end_state":"approved","steps":[],"harness":"omp","extra":1}`))
		}, want: "run.json"},
		{name: "an unknown arm", setup: func(f *fixture) { f.add(spec{arm: "solo", name: "1", end: endApproved}) }, want: `"solo" is not an arm`},
		{name: "an end state the arm does not record", setup: func(f *fixture) { f.add(spec{arm: armMetron, name: "1", end: endCap}) }, want: "not one the metron arm records"},
		{name: "an empty governance kind", setup: func(f *fixture) { f.add(spec{arm: armMetron, name: "1", end: "governance:"}) }, want: "not one the metron arm records"},
		{name: "an anchoring review without a verdict", setup: func(f *fixture) { f.add(spec{arm: armAnchoringFresh, name: "1", end: "reviewed:MAYBE"}) },
			want: "not one the anchoring-fresh arm records"},
		{name: "an anchoring trial that claims an approval", setup: func(f *fixture) { f.add(spec{arm: armAnchoringLedger, name: "1", end: endApproved}) },
			want: "not one the anchoring-ledger arm records"},
		{name: "a reopening by a packet never issued", setup: func(f *fixture) {
			f.add(spec{arm: armMetron, name: "1", end: endRoleFailure, ledger: [][]core.Draft{reviewIssued(1, "P-0001"),
				{opened("F-0001", 1, core.RoleReviewer, core.TierCritical, "The cascade", []string{"shop/store.py"}, "")},
				{reopened("F-0001", "P-0009")}}})
		}, want: "no event opened that finding or issued that packet"},
		{name: "a trial filed under another name", setup: func(f *fixture) {
			dir := f.add(spec{arm: armMetron, name: "1", end: endApproved})
			if err := os.Rename(dir, filepath.Join(filepath.Dir(dir), "2")); err != nil {
				t.Fatal(err)
			}
		}, want: "but the directory is shop/metron/2"},
		{name: "no oracle.json", setup: func(f *fixture) {
			if err := os.Remove(filepath.Join(f.add(spec{arm: armMetron, name: "1", end: endApproved}), "oracle.json")); err != nil {
				t.Fatal(err)
			}
		}, want: "oracle.json"},
		{name: "an oracle outcome that is not one", setup: func(f *fixture) {
			f.add(spec{arm: armMetron, name: "1", end: endApproved, oracle: oracle("", map[string]string{"test_P1": "maybe"}, nil)})
		}, want: `test_P1 is "maybe"`},
		{name: "diff totals that do not add up", setup: func(f *fixture) {
			dir := f.add(spec{arm: armMetron, name: "1", end: endApproved})
			f.write(filepath.Join(dir, "diff.json"), []byte(`{"seed":"a","head":"b","files":[{"path":"x","added":1,"deleted":0}],"added":0,"deleted":0}`))
		}, want: "do not match its files"},
		{name: "a tampered ledger", setup: func(f *fixture) {
			dir := f.add(spec{arm: armMetron, name: "1", end: endApproved, ledger: [][]core.Draft{{roundOpened(1)}}})
			path := filepath.Join(dir, "ledger.jsonl")
			data, _ := os.ReadFile(path)
			f.write(path, bytes.Replace(data, []byte(`"round":1`), []byte(`"round":2`), 1))
		}, want: "ledger.jsonl"},
		{name: "a baseline that ends otherwise than its run", setup: func(f *fixture) {
			dir := f.add(spec{arm: armBounded, name: "1", end: endCap})
			f.write(filepath.Join(dir, "baseline.json"), []byte(`{"arm":"bounded","rounds":[],"final":"approved"}`))
		}, want: "baseline.json says"},
		{name: "a bounded round without a verdict", setup: func(f *fixture) {
			f.add(spec{arm: armBounded, name: "1", end: endCap, rounds: []baselineRound{verdictRound(false)}})
		}, want: "a bounded round holds a REVIEW.json"},
		{name: "a variant whose base is a variant", setup: func(f *fixture) {
			f.write(filepath.Join(f.cases, "v1", "truth.json"), []byte(`{"case": "v1", "base": "shop"}`))
			f.write(filepath.Join(f.cases, "v2", "truth.json"), []byte(`{"case": "v2", "base": "v1"}`))
			f.add(spec{caseName: "v2", arm: armMetron, name: "1", end: endApproved})
		}, want: "which has a base of its own"},
		{name: "a blocker without an oracle", setup: func(f *fixture) {
			f.write(filepath.Join(f.cases, "bare", "truth.json"), []byte(`{"case": "bare", "plants": [{"id": "P1", "kind": "blocker", "paths": ["a"], "match": ["a"], "oracle": []}]}`))
			f.add(spec{caseName: "bare", arm: armMetron, name: "1", end: endApproved})
		}, want: "has no oracle test"},
		{name: "a match expression Go cannot compile", setup: func(f *fixture) {
			f.write(filepath.Join(f.cases, "bad", "truth.json"), []byte(`{"case": "bad", "plants": [{"id": "P1", "kind": "blocker", "paths": ["a"], "match": ["(?<=x)"], "oracle": ["t"]}]}`))
			f.add(spec{caseName: "bad", arm: armMetron, name: "1", end: endApproved})
		}, want: "case bad: P1"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newFixture(t)
			tc.setup(f)
			var out, errOut bytes.Buffer
			if code := run([]string{"-cases", f.cases, f.root}, &out, &errOut); code != 1 || !strings.Contains(errOut.String(), tc.want) || out.Len() != 0 {
				t.Fatalf("exit %d, stderr %q, stdout %d bytes; want exit 1 and an error about %q", code, errOut.String(), out.Len(), tc.want)
			}
		})
	}
}

func TestUsage(t *testing.T) {
	for _, args := range [][]string{{}, {"a", "b"}, {"-unknown", "a"}} {
		var out, errOut bytes.Buffer
		if code := run(args, &out, &errOut); code != 2 || !strings.Contains(errOut.String(), "usage: score") {
			t.Fatalf("%v: exit %d, %q", args, code, errOut.String())
		}
	}
}
