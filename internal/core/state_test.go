package core

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func TestApplyRejects(t *testing.T) {
	h := newHarness(t)
	h.checks(true)
	h.accept(h.packet(), "rev-1", red(h.contract, "crash"))
	base := h.state
	for _, tc := range []struct {
		name  string
		state State
		event Event
		about string
	}{
		{"before genesis", State{}, Event{Type: EventContractPinned, Data: json.RawMessage(`{}`)}, "must begin"},
		{"second genesis", base, Event{Type: EventLedgerInitialized, Data: json.RawMessage(`{"protocol":"metron.protocol/v1"}`)}, "already initialized"},
		{"other protocol", State{}, Event{Type: EventLedgerInitialized, Data: json.RawMessage(`{"protocol":"metron.protocol/v2"}`)}, "protocol"},
		{"unknown type", base, Event{Type: "finding.forgotten", Data: json.RawMessage(`{}`)}, "unknown event type"},
		{"not an object", base, Event{Type: EventDirectiveIssued, Data: json.RawMessage(`null`)}, "not a JSON object"},
		{"unknown member", base, Event{Type: EventPacketVoided, Data: json.RawMessage(`{"packet":"P-0001","reason":"r","hash":"x"}`)}, "malformed"},
		{"member case differs", base, Event{Type: EventPacketVoided, Data: json.RawMessage(`{"Packet":"P-0001","reason":"r"}`)}, "malformed"},
		{"duplicate member", base, Event{Type: EventPacketVoided, Data: json.RawMessage(`{"packet":"P-0001","packet":"P-0001","reason":"r"}`)}, "malformed"},
		{"trailing data", base, Event{Type: EventPacketVoided, Data: json.RawMessage(`{"packet":"P-0001","reason":"r"} {}`)}, "malformed"},
		{"wrong member type", base, Event{Type: EventFindingOpened, Data: json.RawMessage(`{"id":1}`)}, "malformed"},
		{"finding out of sequence", base, Event{Type: EventFindingFixConfirmed, Data: json.RawMessage(`{"id":"F-0001","packet":"P-0001","evidence":"e"}`)}, "not an answered verify packet"},
		{"illegal transition", base, Event{Type: EventFindingReopened, Data: json.RawMessage(`{"id":"F-0001","packet":"P-0001","reason":"regression","justification":"j","root_cause_required":false}`)}, "is open, not resolved"},
		{"identifier skipped", base, Event{Type: EventEscalationRaised, Data: json.RawMessage(`{"id":"E-0002","kind":"dispute","owner":"auditor","round":1,"reason":"r","findings":["F-0001"]}`)}, "next escalation identifier"},
		{"escalation from the wrong report", base, Event{Type: EventEscalationRaised, Data: json.RawMessage(`{"id":"E-0001","kind":"dispute","owner":"auditor","round":1,"packet":"P-0001","reason":"r","findings":["F-0001"]}`)}, "not raised by the report"},
		{"owner mismatch", base, Event{Type: EventEscalationRaised, Data: json.RawMessage(`{"id":"E-0001","kind":"dispute","owner":"governance","round":1,"reason":"r","findings":["F-0001"]}`)}, "owned by auditor"},
		{"void a closed packet", base, Event{Type: EventPacketVoided, Data: json.RawMessage(`{"packet":"P-0001","reason":"r"}`)}, "not open"},
		{"evidence inconsistent", base, Event{Type: EventEvidenceRecorded, Data: json.RawMessage(`{"requirement":"REQ-1","command":"c","candidate":"` + digestOf("x") + `","exit_code":1,"passed":true,"output_digest":"` + digestOf("o") + `"}`)}, "exit code"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			before, _ := Replay(h.events)
			s, err := Apply(tc.state, tc.event)
			if err == nil || !strings.Contains(err.Error(), tc.about) {
				t.Fatalf("got %v, want an error about %q", err, tc.about)
			}
			if !reflect.DeepEqual(s, State{}) {
				t.Fatal("a failed Apply returned a non-zero state")
			}
			if !reflect.DeepEqual(base, before) {
				t.Fatal("Apply modified its argument")
			}
		})
	}
}

func TestReplayGenesis(t *testing.T) {
	genesis := func(protocol string) Event {
		return Event{Type: EventLedgerInitialized, Data: json.RawMessage(`{"protocol":"` + protocol + `"}`)}
	}
	pin := newHarness(t).events[1]
	for _, tc := range []struct {
		name   string
		events []Event
		about  []string // nil: replay succeeds
	}{
		{"empty ledger", nil, nil},
		{"genesis then pin", []Event{genesis(ProtocolVersion), pin}, nil},
		{"first event is not genesis", []Event{pin}, []string{"ledger event 1", "must begin with ledger.initialized"}},
		{"genesis repeated", []Event{genesis(ProtocolVersion), pin, genesis(ProtocolVersion)}, []string{"ledger event 3", "already initialized"}},
		{"other protocol version", []Event{genesis("metron.protocol/v2")}, []string{`"metron.protocol/v2"`, `"` + ProtocolVersion + `"`}},
		{"no protocol version", []Event{genesis("")}, []string{`""`, `"` + ProtocolVersion + `"`}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, err := Replay(tc.events)
			if tc.about == nil {
				if err != nil || s.initialized != (len(tc.events) > 0) {
					t.Fatalf("replay: initialized %t, %v", s.initialized, err)
				}
				return
			}
			if err == nil {
				t.Fatal("replay succeeded")
			}
			for _, a := range tc.about {
				if !strings.Contains(err.Error(), a) {
					t.Fatalf("error %q does not mention %s", err, a)
				}
			}
		})
	}
	if a := Next(State{}, Facts{LedgerVerified: true}); a.Step != StepRepairLedger {
		t.Fatalf("next on an empty ledger is %s", a.Step)
	}
}

func TestFindingHistory(t *testing.T) {
	h := newHarness(t)
	h.checks(true)
	x := h.candidate().Digest
	h.accept(h.packet(), "rev-1", red(h.contract, "crash"))
	h.accept(h.packet(), "impl-1", ImplementerBody{Disputes: []Dispute{{Finding: "F-0001", Rationale: "never empty"}}})
	ap := h.packet()
	h.accept(ap, "aud-1", AuditorBody{Resolutions: resolutions(ap, "it is critical"), Directives: []string{"test empty input"},
		Adjudications: []Adjudication{{Finding: "F-0001", Decision: DecisionReclassify, Tier: TierCritical, Rationale: "data loss"}}})
	h.accept(h.packet(), "impl-1", ImplementerBody{Fixes: []Fix{fix("F-0001", h.commit(true))}})
	y := h.candidate().Digest
	h.accept(h.verifying(), "ver-1", verify(map[string]string{"F-0001": VerificationRejected}, "F-0001"))
	h.accept(h.packet(), "impl-1", ImplementerBody{Fixes: []Fix{fix("F-0001", h.commit(true))}})
	z := h.candidate().Digest
	h.accept(h.verifying(), "ver-1", verify(map[string]string{"F-0001": VerificationConfirmed}, "F-0001"))
	h.checks(true)
	b := review(h.contract, VerdictRed, map[string]string{"REQ-1": RequirementViolated})
	b.Reopen = []Reopen{{Finding: "F-0001", Reason: ReopenInvalidVerification, Justification: "the repro still fails"}}
	rp := h.packet()
	if len(rp.Findings) != 1 || len(rp.Findings[0].History) != 8 {
		t.Fatalf("the review packet does not carry the history: %+v", rp.Findings)
	}
	h.accept(rp, "rev-1", b)

	// A dispute comes with an implementer report, which is bound to no
	// candidate; every other step names the one it was made on.
	want := []FindingEvent{
		{Event: EventFindingOpened, Round: 1, Packet: "P-0001", Candidate: x},
		{Event: EventEscalationRaised, Round: 1, Packet: "P-0002", Note: "the implementer disputes F-0001: never empty"},
		{Event: EventFindingReclassified, Round: 1, Packet: "P-0003", Candidate: x, Note: "T1 to T0: data loss"},
		{Event: EventEscalationResolved, Round: 1, Packet: "P-0003", Candidate: x, Note: "it is critical"},
		{Event: EventFindingFixClaimed, Round: 1, Packet: "P-0004", Candidate: y, Note: "guard the nil case"},
		{Event: EventFindingFixRejected, Round: 1, Packet: "P-0005", Candidate: y, Note: "ran the repro"},
		{Event: EventFindingFixClaimed, Round: 1, Packet: "P-0006", Candidate: z, Note: "guard the nil case"},
		{Event: EventFindingFixConfirmed, Round: 1, Packet: "P-0007", Candidate: z, Note: "ran the repro"},
		{Event: EventFindingReopened, Round: 2, Packet: "P-0008", Candidate: z, Note: "invalid-verification: the repro still fails"},
	}
	if f, _ := lookup(h.state, "F-0001"); !reflect.DeepEqual(f.History, want) {
		t.Fatalf("history\n got %+v\nwant %+v", f.History, want)
	}
	wantDecisions := []Decision{
		{Source: DecisionDirective, Packet: "P-0003", By: RoleAuditor, Text: "test empty input"},
		{Source: DecisionResolution, Escalation: "E-0001", Packet: "P-0003", By: RoleAuditor, Text: "it is critical"},
	}
	if d := h.state.decisions; !reflect.DeepEqual(d, wantDecisions) {
		t.Fatalf("decisions %+v", d)
	}
}

func TestApplyLeavesItsArgumentUnchanged(t *testing.T) {
	h := newHarness(t)
	h.checks(true)
	h.accept(h.packet(), "rev-1", red(h.contract, "crash"))
	h.accept(h.packet(), "impl-1", ImplementerBody{Fixes: []Fix{fix("F-0001", h.commit(true))}})
	for i := range h.events {
		prefix, err := Replay(h.events[:i])
		if err != nil {
			t.Fatal(err)
		}
		snapshot, _ := Replay(h.events[:i])
		if _, err := Apply(prefix, h.events[i]); err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(prefix, snapshot) {
			t.Fatalf("applying event %d (%s) modified its argument", i+1, h.events[i].Type)
		}
	}
}

func TestReplayReportsThePosition(t *testing.T) {
	h := newHarness(t)
	events := append(h.events, Event{Type: "bogus", Data: json.RawMessage(`{}`)})
	if _, err := Replay(events); err == nil || !strings.Contains(err.Error(), "ledger event 3 (bogus)") {
		t.Fatalf("got %v", err)
	}
}

func TestViews(t *testing.T) {
	h := newHarness(t)
	h.checks(true)
	h.accept(h.packet(), "rev-1", review(h.contract, VerdictRed, map[string]string{"REQ-1": RequirementViolated},
		blocker("crash", "REQ-1"), improvement("long")))
	if b := h.state.Backlog(); len(b) != 1 || b[0].ID != "F-0002" {
		t.Fatalf("backlog %+v", b)
	}
	if r, ok := h.state.latestRound(); !ok || r.Verdict != VerdictRed || r.Reviewer != "rev-1" {
		t.Fatalf("latest round %+v", r)
	}
	if p, ok := h.state.Packet("P-0001"); !ok || p.Open || p.Report == nil || p.Report.Author.ID != "rev-1" {
		t.Fatalf("packet %+v", p)
	}
	if !h.state.initialized || h.state.Protocol() != ProtocolVersion || h.state.Events() != len(h.events) || len(h.state.pins) != 1 {
		t.Fatal("ledger summary views")
	}
	fs := h.state.Findings()
	fs[0].Status = StatusWithdrawn
	if x, _ := lookup(h.state, "F-0001"); x.Status != StatusOpen {
		t.Fatal("a view shares its list with the state")
	}
}
