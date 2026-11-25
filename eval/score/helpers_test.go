package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/patacca/metron/internal/core"
	"github.com/patacca/metron/internal/store"
)

// The case the tests score: two blocker plants and two decoys.
const testTruth = `{
  "case": "shop",
  "synthetic": true,
  "test_command": "python3 -m unittest",
  "plants": [
    {"id": "P1", "kind": "blocker", "tier": "T0", "paths": ["shop/store.py", "shop/schema.py"],
     "match": ["(?i)cascad", "(?i)insert\\s+or\\s+replace"], "oracle": ["test_P1"]},
    {"id": "P2", "kind": "blocker", "tier": "T1", "paths": ["shop/prices.py", "shop/bulk.py"],
     "match": ["(?i)\\bfloat"], "oracle": ["test_P2_single", "test_P2_bulk"]}
  ],
  "decoys": [
    {"id": "D1", "kind": "decoy", "max_tier": "T2", "contract_ref": "REQ-8", "paths": ["shop/schema.py"],
     "match": ["(?i)\\bindex\\b"], "oracle": ["test_D1"]},
    {"id": "D2", "kind": "decoy", "max_tier": "T4", "contract_ref": null, "paths": ["shop/bulk.py"],
     "match": ["(?i)too long"], "oracle": []}
  ]
}`

// fixture is a results root and a cases directory in temporary directories.
type fixture struct {
	t     *testing.T
	root  string
	cases string
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	f := &fixture{t: t, root: t.TempDir(), cases: t.TempDir()}
	f.write(filepath.Join(f.cases, "shop", "truth.json"), []byte(testTruth))
	return f
}

func (f *fixture) write(path string, data []byte) {
	f.t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		f.t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		f.t.Fatal(err)
	}
}

func (f *fixture) writeJSON(path string, v any) {
	f.t.Helper()
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		f.t.Fatal(err)
	}
	f.write(path, data)
}

// spec describes one trial. Zero values mean: every oracle test passes,
// nothing changed, no steps.
type spec struct {
	caseName string // default shop
	arm      string
	name     string
	end      string
	oracle   *oracleRecord
	diff     []fileDiff
	steps    []step
	seconds  int
	ledger   [][]core.Draft  // the metron arm: one batch per command
	rounds   []baselineRound // the baselines
}

func passing() *oracleRecord {
	return &oracleRecord{
		FixtureTests: outcomePass,
		Plants:       map[string]string{"test_P1": outcomePass, "test_P2_single": outcomePass, "test_P2_bulk": outcomePass},
		Decoys:       map[string]string{"test_D1": outcomePass},
	}
}

// oracle returns the outcomes of passing() with the given ones changed; a
// value "" removes the test. An empty fixture keeps "pass".
func oracle(fixture string, plants, decoys map[string]string) *oracleRecord {
	o := passing()
	if fixture != "" {
		o.FixtureTests = fixture
	}
	apply := func(dst, src map[string]string) {
		for k, v := range src {
			if v == "" {
				delete(dst, k)
			} else {
				dst[k] = v
			}
		}
	}
	apply(o.Plants, plants)
	apply(o.Decoys, decoys)
	return o
}

// add writes the trial's files and returns its directory.
func (f *fixture) add(s spec) string {
	f.t.Helper()
	if s.caseName == "" {
		s.caseName = "shop"
	}
	dir := filepath.Join(f.root, s.caseName, s.arm, s.name)
	if s.oracle == nil {
		s.oracle = passing()
	}
	if s.steps == nil {
		s.steps = []step{}
	}
	f.writeJSON(filepath.Join(dir, "run.json"), runRecord{
		Case: s.caseName, Arm: s.arm, Trial: s.name, Started: "2026-11-25T10:00:00Z", Ended: "2026-11-25T11:00:00Z",
		Seconds: s.seconds, EndState: s.end, Steps: s.steps, Harness: "omp 19.1.1",
	})
	f.writeJSON(filepath.Join(dir, "oracle.json"), s.oracle)
	d := diffRecord{Seed: "a", Head: "b", Files: s.diff}
	if d.Files == nil {
		d.Files = []fileDiff{}
	}
	for _, fd := range d.Files {
		d.Added += fd.Added
		d.Deleted += fd.Deleted
	}
	f.writeJSON(filepath.Join(dir, "diff.json"), d)
	if s.arm == armMetron {
		f.write(filepath.Join(dir, "ledger.jsonl"), ledger(f.t, s.ledger))
		return dir
	}
	if s.rounds == nil {
		s.rounds = []baselineRound{}
	}
	f.writeJSON(filepath.Join(dir, "baseline.json"), baselineRecord{Arm: s.arm, Rounds: s.rounds, Final: s.end})
	return dir
}

// ledger seals the batches, after a genesis event, into ledger bytes.
func ledger(t *testing.T, batches [][]core.Draft) []byte {
	t.Helper()
	batches = append([][]core.Draft{{{Type: core.EventLedgerInitialized, Data: core.LedgerInitialized{Protocol: core.ProtocolVersion}}}}, batches...)
	var out []byte
	var prev *core.Event
	for _, b := range batches {
		events, err := store.Seal(prev, "2026-11-25T10:00:00Z", b)
		if err != nil {
			t.Fatal(err)
		}
		for i := range events {
			line, err := json.Marshal(events[i])
			if err != nil {
				t.Fatal(err)
			}
			out = append(append(out, line...), '\n')
			prev = &events[i]
		}
	}
	if _, err := store.VerifyLedger(out); err != nil {
		t.Fatalf("the test ledger does not verify: %v", err)
	}
	return out
}

// Drafts of the events the scorer reads.

var candidate = core.Candidate{Commit: "0123456789abcdef0123456789abcdef01234567", Digest: "sha256:" + hex64}

const hex64 = "0000000000000000000000000000000000000000000000000000000000000000"

func roundOpened(n int) core.Draft {
	return core.Draft{Type: core.EventRoundOpened, Data: core.RoundOpened{Round: n, Stage: core.StagePrototype, Candidate: candidate,
		Contract: "sha256:" + hex64, Protocol: core.ProtocolVersion}}
}

func packetIssued(id string, role core.Role, action core.Step, round int) core.Draft {
	return core.Draft{Type: core.EventPacketIssued, Data: core.PacketIssued{Packet: id, Role: role, Action: action, Round: round,
		Candidate: candidate, Contract: "sha256:" + hex64, Protocol: core.ProtocolVersion, Digest: "sha256:" + hex64}}
}

func reportAccepted(packet string, role core.Role) core.Draft {
	return core.Draft{Type: core.EventReportAccepted, Data: core.ReportAccepted{Packet: packet, Role: role,
		Author: core.Author{ID: string(role) + "-1", Kind: core.AuthorAgent}, Digest: "sha256:" + hex64, RawDigest: "sha256:" + hex64,
		Provenance: core.Provenance{Mode: core.ProvenanceDeclared}}}
}

func evidence(req string, passed bool) core.Draft {
	return core.Draft{Type: core.EventEvidenceRecorded, Data: core.Evidence{Requirement: req, Command: "make test",
		Candidate: "sha256:" + hex64, ExitCode: b2i(!passed), Passed: passed, OutputDigest: "sha256:" + hex64}}
}

func opened(id string, round int, source core.Role, tier core.Tier, title string, components []string, description string) core.Draft {
	return core.Draft{Type: core.EventFindingOpened, Data: core.FindingOpened{ID: id, Round: round, Source: source, Packet: "P-0001",
		Input: core.FindingInput{Key: id, Tier: tier, Domain: core.DomainDataIntegrity, Title: title, Components: components,
			Description: description, Evidence: "seen in a reproduction", Impact: "wrong results"}}}
}

func claimed(id string) core.Draft {
	return core.Draft{Type: core.EventFindingFixClaimed, Data: core.FindingFixClaimed{ID: id, Round: 1, Packet: "P-0002",
		Candidate: "sha256:" + hex64, Fix: core.Fix{Finding: id, Commits: []string{candidate.Commit}, Summary: "fixed", Components: []string{"shop"}}}}
}

func rejected(id, scope string) core.Draft {
	return core.Draft{Type: core.EventFindingFixRejected, Data: core.FindingFixRejected{ID: id, Packet: "P-0003",
		Evidence: "still broken", Scope: scope, ScopeEvidence: "looked at the diff"}}
}

func reopened(id string) core.Draft {
	return core.Draft{Type: core.EventFindingReopened, Data: core.FindingReopened{ID: id, Packet: "P-0004",
		Reason: core.ReopenRegression, Justification: "broke again"}}
}

func withdrawn(id string) core.Draft {
	return core.Draft{Type: core.EventFindingWithdrawn, Data: core.FindingWithdrawn{ID: id, Packet: "P-0005", Rationale: "not a defect"}}
}

func reclassified(id string, from, to core.Tier) core.Draft {
	return core.Draft{Type: core.EventFindingReclassified, Data: core.FindingReclassified{ID: id, Packet: "P-0005", From: from, To: to, Rationale: "outside the envelope"}}
}

func bfinding(severity, title string, files []string, description string) baselineFinding {
	return baselineFinding{Title: title, Severity: severity, Files: files, Description: description}
}

func verdictRound(ready bool, findings ...baselineFinding) baselineRound {
	summary := "done"
	if findings == nil {
		findings = []baselineFinding{}
	}
	return baselineRound{Review: review{Ready: &ready, Summary: &summary, Findings: findings}}
}

func reviewRound(verdict string, fixed *bool, findings ...baselineFinding) baselineRound {
	if findings == nil {
		findings = []baselineFinding{}
	}
	return baselineRound{Review: review{Verdict: verdict, Findings: findings}, Fixed: fixed}
}

func ptr[T any](v T) *T { return &v }

// scoreOne scores the fixture and returns the only trial of case and arm.
func (f *fixture) scoreOne(caseName, arm string) (trialScore, summary) {
	f.t.Helper()
	res, err := score(f.root, f.cases)
	if err != nil {
		f.t.Fatal(err)
	}
	for _, c := range res.Cases {
		for _, a := range c.Arms {
			if c.Case == caseName && a.Arm == arm && len(a.Trials) == 1 {
				return a.Trials[0], a.Summary
			}
		}
	}
	f.t.Fatalf("no single %s trial of case %s", arm, caseName)
	return trialScore{}, summary{}
}
