package core

import (
	"errors"
	"fmt"
	"reflect"
	"slices"
)

// Commands are pure functions from state, facts and arguments to the drafts
// of the events they decide on. A command that refuses returns no drafts and
// a *Rejection; any other error means malformed facts or arguments.

// Initialize starts a ledger.
func Initialize(s State) ([]Draft, error) {
	if s.initialized {
		return nil, refuse(LayerTransition, "the ledger is already initialized: there is nothing to do")
	}
	return []Draft{{Type: EventLedgerInitialized, Data: LedgerInitialized{Protocol: ProtocolVersion}}}, nil
}

// Pin authorizes the contract in f on behalf of governance. by is the
// declared identity of whoever pins it. The open packet, if any, is voided.
func Pin(s State, f Facts, reason, by string) ([]Draft, error) {
	if p := ledgerProblem(s, f); p != "" {
		return nil, refuse(LayerTransition, "%s", p)
	}
	var ps problems
	if blank(reason) {
		ps.add("", "the reason is blank: say why governance authorizes this contract")
	}
	if blank(by) {
		ps.add("", "the pinning identity is blank: declare who pins the contract")
	}
	if f.Contract == nil {
		why := f.ContractProblem
		if blank(why) {
			why = "no reason given"
		}
		ps.add("", "the contract is unavailable (%s): fix it, then pin it", why)
	} else if err := ValidateContract(*f.Contract); err != nil {
		var r *Rejection
		errors.As(err, &r)
		ps = append(ps, r.Problems...)
	}
	if err := ps.err(LayerSemantic); err != nil {
		return nil, err
	}
	if !validDigest(f.ContractDigest) || !validDigest(f.ContractRawDigest) {
		return nil, fmt.Errorf("facts: contract digests %q and %q must be sha256 digests", f.ContractDigest, f.ContractRawDigest)
	}
	if cur, ok := s.pin(); ok && cur.Digest == f.ContractDigest {
		return nil, refuse(LayerTransition, "contract %s is already pinned: there is nothing to pin", f.ContractDigest)
	}
	var drafts []Draft
	if op, ok := s.openPacket(); ok {
		drafts = append(drafts, Draft{Type: EventPacketVoided, Data: PacketVoided{Packet: op.Packet, Reason: "the contract was re-pinned"}})
	}
	return append(drafts, Draft{Type: EventContractPinned, Data: ContractPinned{
		Digest: f.ContractDigest, RawDigest: f.ContractRawDigest, Stage: f.Contract.Stage, Reason: reason, By: by,
	}}), nil
}

// RecordEvidence records check results for the current candidate. Each
// result is for an applicable requirement with a command, ran that exact
// command on the current candidate, and passed exactly when it exited 0. The
// paths of problems index into results.
func RecordEvidence(s State, f Facts, results []Evidence) ([]Draft, error) {
	if err := ready(s, f); err != nil {
		return nil, err
	}
	if err := checkCandidate(f); err != nil {
		return nil, err
	}
	if !f.Clean {
		return nil, refuse(LayerTransition, "the working tree has changes outside .metron/: commit or discard them, then run the checks")
	}
	var ps problems
	if len(results) == 0 {
		ps.add("", "there is no check result to record")
	}
	applicable := f.Contract.Applicable()
	seen := make(map[string]int)
	for i, ev := range results {
		p := ptr(i)
		j := slices.IndexFunc(applicable, func(r Requirement) bool { return r.ID == ev.Requirement })
		switch {
		case j < 0:
			ps.add(p+"/requirement", "requirement %q does not apply at stage %s: record only applicable requirements", ev.Requirement, f.Contract.Stage)
		case applicable[j].Command == "":
			ps.add(p+"/requirement", "requirement %s has no command to check", ev.Requirement)
		case ev.Command != applicable[j].Command:
			ps.add(p+"/command", "the command differs from the one requirement %s names: run %q", ev.Requirement, applicable[j].Command)
		}
		if k, dup := seen[ev.Requirement]; dup {
			ps.add(p+"/requirement", "requirement %q already has a result at %s: record one result per requirement", ev.Requirement, ptr(k))
		} else {
			seen[ev.Requirement] = i
		}
		if ev.Candidate != f.Candidate.Digest {
			ps.add(p+"/candidate", "the result is for candidate %s, not the current %s: run the check again", ev.Candidate, f.Candidate.Digest)
		}
		if ev.Passed != (ev.ExitCode == 0) {
			ps.add(p+"/passed", "passed is %t but the exit code is %d: a check passes exactly when it exits 0", ev.Passed, ev.ExitCode)
		}
		if !validDigest(ev.OutputDigest) {
			ps.add(p+"/output_digest", "output digest %q is not a sha256 digest", ev.OutputDigest)
		}
	}
	if err := ps.err(LayerSemantic); err != nil {
		return nil, err
	}
	drafts := make([]Draft, len(results))
	for i, ev := range results {
		drafts[i] = Draft{Type: EventEvidenceRecorded, Data: ev}
	}
	return drafts, nil
}

// PacketPlan is the outcome of planning a packet.
type PacketPlan struct {
	// Open names the packet that is already open and still valid. When it is
	// set, Packet is the zero value and the caller returns the stored packet.
	Open string
	// Packet is the packet for the next action, complete except for its
	// Instructions.
	Packet Packet
}

// PlanPacket plans the packet for the next action. It refuses when the next
// action is not performed through a packet.
func PlanPacket(s State, f Facts) (PacketPlan, error) {
	a := Next(s, f)
	if a.Step == StepSubmitReport {
		return PacketPlan{Open: a.Packet}, nil
	}
	role, ok := a.Step.PacketRole()
	if !ok {
		return PacketPlan{}, refuse(LayerTransition, "the next action is %s, which takes no packet: %s", a.Step, a.Reason)
	}
	if !validTime(f.Time) {
		return PacketPlan{}, fmt.Errorf("facts: time %q is not RFC 3339 UTC with second precision", f.Time)
	}
	if err := checkCandidate(f); err != nil {
		return PacketPlan{}, err
	}
	pin, _ := s.pin()
	c := *f.Contract
	p := Packet{
		Schema:                 SchemaPacket,
		ID:                     seqID("P", len(s.packets)+1),
		Protocol:               ProtocolVersion,
		Role:                   role,
		Action:                 a.Step,
		IssuedAt:               f.Time,
		Candidate:              f.Candidate,
		ReadOnly:               role.ReadOnly(),
		ContractDigest:         pin.Digest,
		Contract:               c,
		ApplicableRequirements: requirementIDs(c.Applicable()),
		History:                s.history(),
		Decisions:              slices.Clone(s.decisions),
		ReportSchema:           SchemaReport,
	}
	for _, r := range c.Applicable() {
		if e, ok := s.latestEvidence(r, f.Candidate.Digest); ok && r.Command != "" {
			p.Evidence = append(p.Evidence, e)
		}
	}
	switch a.Step {
	case StepReview:
		p.Round = len(s.rounds) + 1
		p.Findings = slices.Clone(s.findings)
	case StepFix, StepVerify:
		base, ok := s.base()
		if !ok {
			return PacketPlan{}, refuse(LayerTransition, "no round has a verdict, so there is no base candidate for a %s packet", a.Step)
		}
		p.Base, p.Round = &base.Candidate, base.Number
		if a.Step == StepFix {
			p.Subjects = s.findingIDs(func(x Finding) bool { return x.Status == StatusOpen && x.Tier.Blocking() })
			p.Backlog = s.findingIDs(func(x Finding) bool { return x.Status == StatusOpen && !x.Tier.Blocking() })
			p.RootCauseRequired = s.findingIDs(func(x Finding) bool { return x.Status == StatusOpen && x.RootCauseRequired })
		} else {
			p.Subjects = s.findingIDs(func(x Finding) bool { return x.Status == StatusClaimed })
		}
		for _, x := range s.findings {
			if slices.Contains(p.Subjects, x.ID) || slices.Contains(p.Backlog, x.ID) {
				p.Findings = append(p.Findings, x)
			}
		}
	case StepAudit:
		r, ok := s.base()
		if !ok {
			return PacketPlan{}, refuse(LayerTransition, "no round has a verdict, so no escalation can be open for an audit")
		}
		p.Round = r.Number
		for _, e := range s.escalations {
			if e.Open && e.Owner == RoleAuditor {
				p.Escalations = append(p.Escalations, e)
			}
		}
		p.Adjudicable = s.findingIDs(func(x Finding) bool {
			return slices.ContainsFunc(p.Escalations, func(e Escalation) bool {
				return adjudicable(e.Kind) && slices.Contains(e.Findings, x.ID)
			})
		})
		p.Findings = slices.Clone(s.findings)
	}
	return PacketPlan{Packet: p}, nil
}

// IssuePacket turns a planned packet, with its Instructions filled in, and the
// digest of its canonical JSON form into drafts: packet.voided for a stale
// open packet, round.opened for a review, and packet.issued. The packet must
// be the one PlanPacket returns for the same state and facts, changed only in
// its Instructions.
func IssuePacket(s State, f Facts, p Packet, digest string) ([]Draft, error) {
	plan, err := PlanPacket(s, f)
	if err != nil {
		return nil, err
	}
	if plan.Open != "" {
		return nil, refuse(LayerTransition, "packet %s is open and still valid: return it instead of issuing another", plan.Open)
	}
	if blank(p.Instructions) {
		return nil, errors.New("the packet has no instructions: render the role's brief into it")
	}
	bare := p
	bare.Instructions = ""
	if !reflect.DeepEqual(bare, plan.Packet) {
		return nil, errors.New("the packet differs from the planned one: plan it again from the same state and facts and change only its instructions")
	}
	if !validDigest(digest) {
		return nil, fmt.Errorf("packet digest %q is not a sha256 digest", digest)
	}
	var drafts []Draft
	if op, ok := s.openPacket(); ok {
		drafts = append(drafts, Draft{Type: EventPacketVoided, Data: PacketVoided{Packet: op.Packet, Reason: packetInvalid(s, f, op)}})
	}
	if p.Action == StepReview {
		pin, _ := s.pin()
		drafts = append(drafts, Draft{Type: EventRoundOpened, Data: RoundOpened{
			Round: p.Round, Stage: pin.Stage, Candidate: p.Candidate, Contract: p.ContractDigest, Protocol: ProtocolVersion,
		}})
	}
	var escalations []string
	for _, e := range p.Escalations {
		escalations = append(escalations, e.ID)
	}
	return append(drafts, Draft{Type: EventPacketIssued, Data: PacketIssued{
		Packet: p.ID, Role: p.Role, Action: p.Action, Round: p.Round, Candidate: p.Candidate, Base: p.Base,
		Contract: p.ContractDigest, Protocol: ProtocolVersion, Digest: digest,
		Subjects: p.Subjects, Backlog: p.Backlog, RootCauseRequired: p.RootCauseRequired,
		Escalations: escalations, Adjudicable: p.Adjudicable,
	}}), nil
}

// Resolve discharges a governance-owned escalation with governance's decision.
// by is the declared identity of whoever resolves it.
func Resolve(s State, f Facts, id, decision, by string) ([]Draft, error) {
	if p := ledgerProblem(s, f); p != "" {
		return nil, refuse(LayerTransition, "%s", p)
	}
	var ps problems
	if blank(decision) {
		ps.add("", "the decision is blank: state what governance decided and why")
	}
	if blank(by) {
		ps.add("", "the resolving identity is blank: declare who resolves the escalation")
	}
	if err := ps.err(LayerSemantic); err != nil {
		return nil, err
	}
	i := s.escalationIndex(id)
	switch {
	case i < 0:
		return nil, refuse(LayerTransition, "escalation %q does not exist", id)
	case !s.escalations[i].Open:
		return nil, refuse(LayerTransition, "escalation %s is already resolved", id)
	case s.escalations[i].Owner != RoleGovernance:
		return nil, refuse(LayerTransition, "escalation %s (%s) is owned by the %s: it is discharged by an audit report",
			id, s.escalations[i].Kind, s.escalations[i].Owner)
	}
	return []Draft{{Type: EventEscalationResolved, Data: EscalationResolved{
		ID: id, By: RoleGovernance, Actor: by, Resolution: decision,
	}}}, nil
}

// base returns the most recent round with a verdict, in any stage: the round
// whose candidate fix and verify packets work from.
func (s State) base() (Round, bool) {
	for i := len(s.rounds) - 1; i >= 0; i-- {
		if s.rounds[i].Verdict != "" {
			return s.rounds[i], true
		}
	}
	return Round{}, false
}

// history summarizes the rounds of the current stage.
func (s State) history() []RoundSummary {
	var out []RoundSummary
	for _, r := range s.stageRounds() {
		out = append(out, RoundSummary{Round: r.Number, Candidate: r.Candidate, Verdict: r.Verdict, Reviewer: r.Reviewer})
	}
	return out
}

func requirementIDs(rs []Requirement) []string {
	ids := make([]string, len(rs))
	for i, r := range rs {
		ids[i] = r.ID
	}
	return ids
}
