package core

import "encoding/json"

// EventType names a kind of ledger event.
type EventType string

const (
	EventLedgerInitialized   EventType = "ledger.initialized"
	EventContractPinned      EventType = "contract.pinned"
	EventEvidenceRecorded    EventType = "evidence.recorded"
	EventRoundOpened         EventType = "round.opened"
	EventPacketIssued        EventType = "packet.issued"
	EventPacketVoided        EventType = "packet.voided"
	EventReportAccepted      EventType = "report.accepted"
	EventVerdictRecorded     EventType = "verdict.recorded"
	EventFindingOpened       EventType = "finding.opened"
	EventFindingFixClaimed   EventType = "finding.fix-claimed"
	EventFindingFixConfirmed EventType = "finding.fix-confirmed"
	EventFindingFixRejected  EventType = "finding.fix-rejected"
	EventFindingReopened     EventType = "finding.reopened"
	EventFindingWithdrawn    EventType = "finding.withdrawn"
	EventFindingReclassified EventType = "finding.reclassified"
	EventEscalationRaised    EventType = "escalation.raised"
	EventEscalationResolved  EventType = "escalation.resolved"
	EventDirectiveIssued     EventType = "directive.issued"
)

// Event is one line of the ledger. Members are serialized in this order; the
// hash chain depends on it.
type Event struct {
	Seq  int       `json:"seq"`
	Time string    `json:"time"`
	Prev string    `json:"prev"`
	Type EventType `json:"type"`
	// More is the number of events of the same command that follow this one.
	// It counts down to 0 within a batch; the last line of a ledger has 0.
	More int             `json:"more"`
	Data json.RawMessage `json:"data"`
	Hash string          `json:"hash"`
}

// Draft is an event the domain has decided on and the store has not yet
// sealed. Sealing assigns the sequence number, the time and the hashes.
type Draft struct {
	Type EventType
	// Data is the payload type that corresponds to Type.
	Data any
}

// The payload of each event type follows. Payloads never contain a member
// named "hash".

// LedgerInitialized is the payload of EventLedgerInitialized.
type LedgerInitialized struct {
	Protocol string `json:"protocol"`
}

// ContractPinned is the payload of EventContractPinned.
type ContractPinned struct {
	// Digest is the contract digest: the binding, semantic identity.
	Digest string `json:"digest"`
	// RawDigest is the digest of the file's bytes, kept for audit.
	RawDigest string `json:"raw_digest"`
	Stage     Stage  `json:"stage"`
	Reason    string `json:"reason"`
	// By is the declared identity of whoever pinned it.
	By string `json:"by"`
}

// EvidenceRecorded is the payload of EventEvidenceRecorded.
type EvidenceRecorded = Evidence

// RoundOpened is the payload of EventRoundOpened.
type RoundOpened struct {
	Round     int       `json:"round"`
	Stage     Stage     `json:"stage"`
	Candidate Candidate `json:"candidate"`
	// Contract is the contract digest the round is conducted under.
	Contract string `json:"contract"`
	Protocol string `json:"protocol"`
}

// PacketIssued is the payload of EventPacketIssued. It repeats the bindings
// and the subject lists of the packet so that a report can be validated from
// the ledger alone.
type PacketIssued struct {
	Packet    string    `json:"packet"`
	Role      Role      `json:"role"`
	Action    Step      `json:"action"`
	Round     int       `json:"round"`
	Candidate Candidate `json:"candidate"`
	// Base is the candidate the round reviewed. It is set exactly when the
	// packet has a base: for fix and verify packets.
	Base     *Candidate `json:"base,omitempty"`
	Contract string     `json:"contract"`
	Protocol string     `json:"protocol"`
	// Digest is the digest of the packet's canonical JSON form.
	Digest            string   `json:"digest"`
	Subjects          []string `json:"subjects,omitempty"`
	Backlog           []string `json:"backlog,omitempty"`
	RootCauseRequired []string `json:"root_cause_required,omitempty"`
	Escalations       []string `json:"escalations,omitempty"`
	Adjudicable       []string `json:"adjudicable,omitempty"`
}

// PacketVoided is the payload of EventPacketVoided.
type PacketVoided struct {
	Packet string `json:"packet"`
	Reason string `json:"reason"`
}

// ReportAccepted is the payload of EventReportAccepted.
type ReportAccepted struct {
	Packet string `json:"packet"`
	Role   Role   `json:"role"`
	Author Author `json:"author"`
	// Digest is the digest of the report's canonical JSON form.
	Digest string `json:"digest"`
	// RawDigest is the digest of the bytes as submitted.
	RawDigest  string     `json:"raw_digest"`
	Provenance Provenance `json:"provenance"`
}

// VerdictRecorded is the payload of EventVerdictRecorded.
type VerdictRecorded struct {
	Round   int     `json:"round"`
	Verdict Verdict `json:"verdict"`
	// Candidate and Contract are the digests the verdict is bound to.
	Candidate string `json:"candidate"`
	Contract  string `json:"contract"`
	// Reviewer is the author identity that issued the verdict.
	Reviewer     string              `json:"reviewer"`
	Requirements []RequirementResult `json:"requirements"`
}

// RequirementResult is a requirement's status as of a verdict.
type RequirementResult struct {
	Requirement string `json:"requirement"`
	Status      string `json:"status"`
}

// FindingOpened is the payload of EventFindingOpened.
type FindingOpened struct {
	ID     string       `json:"id"`
	Round  int          `json:"round"`
	Source Role         `json:"source"`
	Packet string       `json:"packet"`
	Input  FindingInput `json:"finding"`
}

// FindingFixClaimed is the payload of EventFindingFixClaimed.
type FindingFixClaimed struct {
	ID     string `json:"id"`
	Round  int    `json:"round"`
	Packet string `json:"packet"`
	// Candidate is the digest of the candidate the fix was claimed on.
	Candidate string `json:"candidate"`
	Fix       Fix    `json:"fix"`
}

// FindingFixConfirmed is the payload of EventFindingFixConfirmed.
type FindingFixConfirmed struct {
	ID       string `json:"id"`
	Packet   string `json:"packet"`
	Evidence string `json:"evidence"`
}

// FindingFixRejected is the payload of EventFindingFixRejected.
type FindingFixRejected struct {
	ID       string `json:"id"`
	Packet   string `json:"packet"`
	Evidence string `json:"evidence"`
	// RootCauseRequired is true when this rejection brings the finding's
	// recurrence count to the contract's threshold.
	RootCauseRequired bool `json:"root_cause_required"`
}

// FindingReopened is the payload of EventFindingReopened.
type FindingReopened struct {
	ID            string       `json:"id"`
	Packet        string       `json:"packet"`
	Reason        ReopenReason `json:"reason"`
	Justification string       `json:"justification"`
	// RootCauseRequired is true when this reopening brings the finding's
	// recurrence count to the contract's threshold.
	RootCauseRequired bool `json:"root_cause_required"`
}

// FindingWithdrawn is the payload of EventFindingWithdrawn.
type FindingWithdrawn struct {
	ID        string `json:"id"`
	Packet    string `json:"packet"`
	Rationale string `json:"rationale"`
}

// FindingReclassified is the payload of EventFindingReclassified.
type FindingReclassified struct {
	ID        string `json:"id"`
	Packet    string `json:"packet"`
	From      Tier   `json:"from"`
	To        Tier   `json:"to"`
	Rationale string `json:"rationale"`
}

// EscalationRaised is the payload of EventEscalationRaised.
type EscalationRaised struct {
	ID    string         `json:"id"`
	Kind  EscalationKind `json:"kind"`
	Owner Role           `json:"owner"`
	Round int            `json:"round"`
	// Packet is the packet whose report raised it or, for a suspension that
	// the campaign budget raises, the packet whose issue used the budget up.
	// It is empty for an escalation raised by a failed check recording.
	Packet       string   `json:"packet,omitempty"`
	Reason       string   `json:"reason"`
	Findings     []string `json:"findings,omitempty"`
	Components   []string `json:"components,omitempty"`
	Requirements []string `json:"requirements,omitempty"`
}

// EscalationResolved is the payload of EventEscalationResolved.
type EscalationResolved struct {
	ID string `json:"id"`
	// By is the role that discharged it, and Actor that party's declared
	// identity.
	By    Role   `json:"by"`
	Actor string `json:"actor"`
	// Packet is the audit packet whose report discharged it. It is empty for
	// a governance resolution.
	Packet     string `json:"packet,omitempty"`
	Resolution string `json:"resolution"`
}

// DirectiveIssued is the payload of EventDirectiveIssued.
type DirectiveIssued struct {
	Packet string `json:"packet"`
	Text   string `json:"text"`
}
