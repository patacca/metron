package core

import (
	"errors"
	"reflect"
	"strings"
	"testing"
)

// rejection asserts that a command refused with layer and a problem at path,
// and produced no drafts.
func rejection(t *testing.T, drafts []Draft, err error, layer Layer, path string) {
	t.Helper()
	var r *Rejection
	if !errors.As(err, &r) || r.Layer != layer || len(drafts) != 0 {
		t.Fatalf("got %d drafts and %v, want a %s rejection", len(drafts), err, layer)
	}
	if !hasProblem(r.Problems, path) {
		t.Fatalf("no problem at %q: %v", path, err)
	}
}

func TestInitialize(t *testing.T) {
	d, err := Initialize(State{})
	if err != nil {
		t.Fatal(err)
	}
	eqTypes(t, d, EventLedgerInitialized)
	h := newHarness(t)
	d, err = Initialize(h.state)
	rejection(t, d, err, LayerTransition, "")
}

func TestPinRules(t *testing.T) {
	for _, tc := range []struct {
		name   string
		facts  func(f *Facts)
		reason string
		layer  Layer
		path   string
	}{
		{name: "ledger unverified", facts: func(f *Facts) { f.LedgerVerified, f.LedgerProblem = false, "hash mismatch at line 4" }, reason: "r", layer: LayerTransition},
		{name: "blank reason", facts: func(f *Facts) {}, reason: " ", layer: LayerSemantic},
		{name: "contract unavailable", facts: func(f *Facts) { f.Contract, f.ContractProblem = nil, "duplicate key" }, reason: "r", layer: LayerSemantic},
		{name: "contract invalid", facts: func(f *Facts) { f.Contract.Stage = "beta" }, reason: "r", layer: LayerSemantic, path: "/stage"},
		{name: "already pinned", facts: func(f *Facts) {}, reason: "r", layer: LayerTransition},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := newHarness(t)
			f := h.facts()
			c := *f.Contract
			f.Contract = &c
			tc.facts(&f)
			d, err := Pin(h.state, f, tc.reason, "gov")
			rejection(t, d, err, tc.layer, tc.path)
		})
	}
	h := newHarness(t)
	c := testContract()
	c.Authorizes = "other"
	h.contract = c
	f := h.facts()
	f.ContractRawDigest = "abc"
	var r *Rejection
	if d, err := Pin(h.state, f, "r", "gov"); err == nil || errors.As(err, &r) || d != nil {
		t.Fatalf("malformed digest facts: %v", err)
	}
}

func TestRecordEvidenceRules(t *testing.T) {
	ok := func(h *harness) Evidence {
		return Evidence{Requirement: "REQ-1", Command: "go test ./...", Candidate: h.candidate().Digest, ExitCode: 0, Passed: true, OutputDigest: digestOf("o")}
	}
	for _, tc := range []struct {
		name  string
		prep  func(h *harness)
		edit  func(h *harness, e []Evidence) []Evidence
		layer Layer
		path  string
	}{
		{name: "pass", edit: func(h *harness, e []Evidence) []Evidence { return e }},
		{name: "fail", edit: func(h *harness, e []Evidence) []Evidence { e[0].ExitCode, e[0].Passed = 2, false; return e }},
		{name: "nothing to record", edit: func(h *harness, e []Evidence) []Evidence { return nil }, layer: LayerSemantic},
		{name: "dirty tree", prep: func(h *harness) { h.clean = false }, edit: func(h *harness, e []Evidence) []Evidence { return e }, layer: LayerTransition},
		{name: "contract edited", prep: func(h *harness) { h.contract.Authorizes = "changed" }, edit: func(h *harness, e []Evidence) []Evidence { return e }, layer: LayerTransition},
		{name: "inapplicable requirement", edit: func(h *harness, e []Evidence) []Evidence { e[0].Requirement = "REQ-4"; return e }, layer: LayerSemantic, path: "/0/requirement"},
		{name: "requirement without command", edit: func(h *harness, e []Evidence) []Evidence { e[0].Requirement = "REQ-2"; return e }, layer: LayerSemantic, path: "/0/requirement"},
		{name: "different command", edit: func(h *harness, e []Evidence) []Evidence { e[0].Command = "go test ./internal/..."; return e }, layer: LayerSemantic, path: "/0/command"},
		{name: "recorded twice", edit: func(h *harness, e []Evidence) []Evidence { return append(e, e[0]) }, layer: LayerSemantic, path: "/1/requirement"},
		{name: "other candidate", edit: func(h *harness, e []Evidence) []Evidence { e[0].Candidate = digestOf("other"); return e }, layer: LayerSemantic, path: "/0/candidate"},
		{name: "passed with a failing exit code", edit: func(h *harness, e []Evidence) []Evidence { e[0].ExitCode = 1; return e }, layer: LayerSemantic, path: "/0/passed"},
		{name: "malformed output digest", edit: func(h *harness, e []Evidence) []Evidence { e[0].OutputDigest = "md5:x"; return e }, layer: LayerSemantic, path: "/0/output_digest"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := newHarness(t)
			if tc.prep != nil {
				tc.prep(h)
			}
			d, err := RecordEvidence(h.state, h.facts(), tc.edit(h, []Evidence{ok(h)}))
			if tc.layer == "" {
				h.must(d, err)
				return
			}
			rejection(t, d, err, tc.layer, tc.path)
		})
	}
}

func TestIssuePacketRules(t *testing.T) {
	h := newHarness(t)
	h.checks(true)
	p, _, err := h.planned()
	if err != nil {
		t.Fatal(err)
	}
	f := h.facts()
	digest := digestOf("packet")

	tampered := p
	tampered.Round = 7
	if _, err := IssuePacket(h.state, f, tampered, digest); err == nil {
		t.Fatal("a tampered packet was issued")
	}
	bare := p
	bare.Instructions = ""
	if _, err := IssuePacket(h.state, f, bare, digest); err == nil {
		t.Fatal("a packet without instructions was issued")
	}
	if _, err := IssuePacket(h.state, f, p, "sha256:short"); err == nil {
		t.Fatal("a malformed digest was accepted")
	}
	later := f
	later.Time = "2026-11-25T10:00:00+01:00"
	if _, err := PlanPacket(h.state, later); err == nil {
		t.Fatal("a non-UTC time was accepted")
	}
	d := h.must(IssuePacket(h.state, f, p, digest))
	eqTypes(t, d, EventRoundOpened, EventPacketIssued)
	h.packets[p.ID] = p
	d, err = IssuePacket(h.state, h.facts(), p, digest)
	rejection(t, d, err, LayerTransition, "")

	// A stale read-only packet is voided by the next issue.
	h.commit(true)
	h.expect(StepRunChecks)
	h.checks(true)
	np, _, err := h.planned()
	if err != nil {
		t.Fatal(err)
	}
	d = h.must(IssuePacket(h.state, h.facts(), np, digest))
	eqTypes(t, d, EventPacketVoided, EventRoundOpened, EventPacketIssued)
	if v := d[0].Data.(PacketVoided); v.Packet != p.ID || !strings.Contains(v.Reason, "candidate changed") {
		t.Fatalf("void %+v", v)
	}
	if rs := h.state.StageRounds(); len(rs) != 2 || !rs[0].Voided || rs[1].Number != 2 {
		t.Fatalf("rounds %+v", rs)
	}
}

func TestResolveRules(t *testing.T) {
	h, _ := auditorSetup(t)
	for _, tc := range []struct {
		name, id, decision string
		layer              Layer
	}{
		{"unknown escalation", "E-0042", "d", LayerTransition},
		{"auditor-owned escalation", "E-0001", "d", LayerTransition},
		{"blank decision", "E-0001", "", LayerSemantic},
	} {
		t.Run(tc.name, func(t *testing.T) {
			d, err := Resolve(h.state, h.facts(), tc.id, tc.decision, "gov")
			rejection(t, d, err, tc.layer, "")
		})
	}
	g := newHarness(t)
	g.checks(true)
	g.accept(g.packet(), "rev-1", review(g.contract, VerdictIndeterminate, map[string]string{"REQ-2": RequirementUnverified}))
	g.must(Resolve(g.state, g.facts(), "E-0001", "accepted as is", "gov"))
	d, err := Resolve(g.state, g.facts(), "E-0001", "again", "gov")
	rejection(t, d, err, LayerTransition, "")
}

func TestPacketProblem(t *testing.T) {
	h := newHarness(t)
	h.checks(true)
	lost := h.packet()
	f := h.facts()
	f.PacketProblem = "packets/P-0001.json is missing"
	if a := Next(h.state, f); a.Step != StepReview {
		t.Fatalf("next with an unusable packet is %s", a.Step)
	}
	plan, err := PlanPacket(h.state, f)
	if err != nil || plan.Open != "" || plan.Packet.ID != "P-0002" || plan.Packet.Round != 2 {
		t.Fatalf("plan %+v, %v", plan, err)
	}
	r := h.report(lost, "rev-1", green(h.contract))
	raw := mustJSON(t, r)
	d, dup, err := Ingest(h.state, f, r, digestOf(string(raw)), digestOf("raw"))
	rejection(t, d, err, LayerTransition, "/packet")
	if dup || !strings.Contains(err.Error(), "metron packet") {
		t.Fatalf("%v", err)
	}
	np := plan.Packet
	np.Instructions = "brief"
	d = h.must(IssuePacket(h.state, f, np, digestOf("p2")))
	eqTypes(t, d, EventPacketVoided, EventRoundOpened, EventPacketIssued)
	if v := d[0].Data.(PacketVoided); v.Packet != "P-0001" || v.Reason != f.PacketProblem {
		t.Fatalf("void %+v", v)
	}
	h.packets[np.ID] = np
	h.accept(np, "rev-1", green(h.contract))

	// With no packet open the member is ignored.
	f = h.facts()
	f.PacketProblem = "anything"
	if !reflect.DeepEqual(Next(h.state, f), h.next()) || !Gate(h.state, f).Open {
		t.Fatal("a packet problem changed the outcome while no packet is open")
	}
}
