package core

import (
	"bytes"
	jsonv2 "encoding/json/v2"
	"errors"
	"fmt"
	"slices"
)

// State is the fold of the ledger's events. It is derived from events alone
// and never needs the contract. It is a value: Apply never modifies its
// argument, and the accessors return copies. The zero State is an empty,
// uninitialized ledger.
type State struct {
	initialized bool
	protocol    string
	// applied counts the events folded so far.
	applied int
	pins    []ContractPinned
	// epochStart is the index in rounds of the first round of the current
	// stage. Round numbers run across stages; the stage only bounds the
	// streak, churn and the history shown to roles.
	epochStart  int
	rounds      []Round
	packets     []PacketRecord
	findings    []Finding
	escalations []Escalation
	evidence    []Evidence
	decisions   []Decision
	actors      []actor
	// streak counts the failed steps since the last GREEN verdict or the last
	// resumption by governance, and convergenceRaised whether a
	// non-convergence escalation was raised during it.
	streak            int
	convergenceRaised bool
	// steps counts the packets issued in the current campaign: since the
	// ledger began, the last approval or the last resumption by governance.
	steps int
	// churn tracks, for the current campaign, the rounds in which each
	// component was named in a fix.
	churn []churn
	// counted is set once the batch of events being folded has been counted
	// as a failed step, and midBatch while more events of that batch follow.
	counted, midBatch bool
}

// Round is one review round.
type Round struct {
	// Number counts rounds from 1 across the whole ledger.
	Number    int       `json:"number"`
	Stage     Stage     `json:"stage"`
	Candidate Candidate `json:"candidate"`
	// Contract is the contract digest the round is conducted under.
	Contract string `json:"contract"`
	// Packet is the review packet of the round.
	Packet string `json:"packet,omitempty"`
	// Verdict, Reviewer and Requirements are empty until the verdict is
	// recorded.
	Verdict      Verdict             `json:"verdict,omitempty"`
	Reviewer     string              `json:"reviewer,omitempty"`
	Requirements []RequirementResult `json:"requirements,omitempty"`
	// Voided is set when the round's packet was voided; the round then never
	// has a verdict.
	Voided bool `json:"voided"`
}

// PacketRecord is an issued packet as tracked by the ledger.
type PacketRecord struct {
	PacketIssued
	// Open is true until a report is accepted for the packet or it is voided.
	Open       bool   `json:"open"`
	VoidReason string `json:"void_reason,omitempty"`
	// Report is the accepted report, once there is one.
	Report *ReportAccepted `json:"report,omitempty"`
}

type actor struct {
	id   string
	role Role
}

type churn struct {
	component string
	rounds    []int
	raised    bool
}

// Replay folds events, in order, into a new State. Like Apply it ignores
// Seq, Prev and Hash; verifying the chain is the store's job. It reads More
// to tell where one command's events end.
func Replay(events []Event) (State, error) {
	var s State
	for i, e := range events {
		if err := s.apply(e); err != nil {
			return State{}, fmt.Errorf("ledger event %d (%s): %w", i+1, e.Type, err)
		}
	}
	return s, nil
}

// Apply folds one event into s and returns the new state. It decodes
// Event.Data strictly according to Event.Type and rejects an unknown type, a
// malformed payload, or an event the state machine does not permit. On error
// it returns the zero State.
func Apply(s State, e Event) (State, error) {
	n := s.clone()
	if err := n.apply(e); err != nil {
		return State{}, err
	}
	return n, nil
}

// clone copies every slice that apply may modify in place. Nested slices are
// never modified in place, so they may stay shared.
func (s State) clone() State {
	s.pins = slices.Clone(s.pins)
	s.rounds = slices.Clone(s.rounds)
	s.packets = slices.Clone(s.packets)
	s.findings = slices.Clone(s.findings)
	s.escalations = slices.Clone(s.escalations)
	s.evidence = slices.Clone(s.evidence)
	s.decisions = slices.Clone(s.decisions)
	s.actors = slices.Clone(s.actors)
	s.churn = slices.Clone(s.churn)
	return s
}

func (s *State) apply(e Event) error {
	if !s.initialized && e.Type != EventLedgerInitialized {
		return fmt.Errorf("the ledger must begin with %s", EventLedgerInitialized)
	}
	s.applied++
	// A failed step is one command, however many of its events record a
	// failure: one verification that rejects three fixes fails once.
	if !s.midBatch {
		s.counted = false
	}
	s.midBatch = e.More > 0
	switch e.Type {
	case EventLedgerInitialized:
		return s.onInitialized(e.Data)
	case EventContractPinned:
		return s.onPinned(e.Data)
	case EventEvidenceRecorded:
		return s.onEvidence(e.Data)
	case EventRoundOpened:
		return s.onRoundOpened(e.Data)
	case EventPacketIssued:
		return s.onPacketIssued(e.Data)
	case EventPacketVoided:
		return s.onPacketVoided(e.Data)
	case EventReportAccepted:
		return s.onReportAccepted(e.Data)
	case EventVerdictRecorded:
		return s.onVerdict(e.Data)
	case EventFindingOpened:
		return s.onFindingOpened(e.Data)
	case EventFindingFixClaimed:
		return s.onFixClaimed(e.Data)
	case EventFindingFixConfirmed:
		return s.onFixConfirmed(e.Data)
	case EventFindingFixRejected:
		return s.onFixRejected(e.Data)
	case EventFindingReopened:
		return s.onReopened(e.Data)
	case EventFindingWithdrawn:
		return s.onWithdrawn(e.Data)
	case EventFindingReclassified:
		return s.onReclassified(e.Data)
	case EventEscalationRaised:
		return s.onEscalationRaised(e.Data)
	case EventEscalationResolved:
		return s.onEscalationResolved(e.Data)
	case EventDirectiveIssued:
		return s.onDirective(e.Data)
	}
	return fmt.Errorf("unknown event type %q", e.Type)
}

// payload decodes an event payload strictly: a single JSON object, member
// names matched exactly, no unknown or duplicate members.
func payload[T any](data []byte) (T, error) {
	var v T
	d := bytes.TrimLeft(data, " \t\r\n")
	if len(d) == 0 || d[0] != '{' {
		return v, errors.New("the payload is not a JSON object")
	}
	if err := jsonv2.Unmarshal(d, &v, jsonv2.RejectUnknownMembers(true)); err != nil {
		return v, fmt.Errorf("malformed payload: %v", err)
	}
	return v, nil
}

// answered checks that a packet exists, was issued for action, and has an
// accepted report.
func (s *State) answered(id string, action Step) error {
	i := s.packetIndex(id)
	if i < 0 {
		return fmt.Errorf("packet %q does not exist", id)
	}
	if pk := s.packets[i]; pk.Action != action || pk.Report == nil {
		return fmt.Errorf("packet %s is not an answered %s packet", id, action)
	}
	return nil
}

// danglingRound reports a round opened without its review packet.
func (s *State) danglingRound() error {
	if n := len(s.rounds); n > 0 && s.rounds[n-1].Packet == "" {
		return fmt.Errorf("round %d was opened without a review packet", s.rounds[n-1].Number)
	}
	return nil
}

func (s State) pin() (ContractPinned, bool) {
	if len(s.pins) == 0 {
		return ContractPinned{}, false
	}
	return s.pins[len(s.pins)-1], true
}

func (s State) stageRounds() []Round { return s.rounds[s.epochStart:] }

// latestRound returns the most recent round of the current stage that has a
// verdict.
func (s State) latestRound() (Round, bool) {
	rs := s.stageRounds()
	for i := len(rs) - 1; i >= 0; i-- {
		if rs[i].Verdict != "" {
			return rs[i], true
		}
	}
	return Round{}, false
}

func (s State) openPacket() (PacketRecord, bool) {
	for i := len(s.packets) - 1; i >= 0; i-- {
		if s.packets[i].Open {
			return s.packets[i], true
		}
	}
	return PacketRecord{}, false
}

func (s State) packetIndex(id string) int {
	return slices.IndexFunc(s.packets, func(p PacketRecord) bool { return p.Packet == id })
}

func (s State) findingIndex(id string) int {
	return slices.IndexFunc(s.findings, func(f Finding) bool { return f.ID == id })
}

func (s State) escalationIndex(id string) int {
	return slices.IndexFunc(s.escalations, func(e Escalation) bool { return e.ID == id })
}

// actedAs reports whether id was the author of an accepted report in role.
func (s State) actedAs(id string, role Role) bool {
	return slices.Contains(s.actors, actor{id: id, role: role})
}
