package core

import "slices"

// Read-only views of the state for status, findings and log displays. Each
// returns a copy of the top-level list in ledger order; the values it holds
// share nested lists with the state and must not be modified.

// Protocol returns the protocol version the ledger was written under.
func (s State) Protocol() string { return s.protocol }

// Events returns the number of events folded into the state.
func (s State) Events() int { return s.applied }

// Pin returns the contract pin in force, and false when nothing is pinned.
func (s State) Pin() (ContractPinned, bool) { return s.pin() }

// Stage returns the pinned stage, or "" when nothing is pinned.
func (s State) Stage() Stage {
	p, _ := s.pin()
	return p.Stage
}

// StageRounds returns the rounds of the current stage, numbered from 1.
func (s State) StageRounds() []Round { return slices.Clone(s.stageRounds()) }

// FailedStreak returns the number of consecutive rounds of the current stage
// that ended without GREEN.
func (s State) FailedStreak() int { return s.streak }

// Packets returns every issued packet in identifier order.
func (s State) Packets() []PacketRecord { return slices.Clone(s.packets) }

// Packet returns the packet with the given identifier.
func (s State) Packet(id string) (PacketRecord, bool) {
	if i := s.packetIndex(id); i >= 0 {
		return s.packets[i], true
	}
	return PacketRecord{}, false
}

// OpenPacket returns the open packet, valid or not, if there is one.
func (s State) OpenPacket() (PacketRecord, bool) { return s.openPacket() }

// Findings returns every finding in identifier order.
func (s State) Findings() []Finding { return slices.Clone(s.findings) }

// Backlog returns the open non-blocking findings in identifier order: the
// improvement backlog, which never blocks the gate.
func (s State) Backlog() []Finding {
	var out []Finding
	for _, f := range s.findings {
		if f.Status == StatusOpen && !f.Tier.Blocking() {
			out = append(out, f)
		}
	}
	return out
}

// Escalations returns every escalation in identifier order.
func (s State) Escalations() []Escalation { return slices.Clone(s.escalations) }

// Evidence returns every recorded check result in ledger order.
func (s State) Evidence() []Evidence { return slices.Clone(s.evidence) }
