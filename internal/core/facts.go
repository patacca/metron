package core

import (
	"errors"
	"fmt"
)

// Facts is everything the core needs that is observed outside it. The caller
// gathers it once per command and passes the same value to every core call
// of that command.
type Facts struct {
	// Time is the current time, RFC 3339 UTC with second precision.
	Time string
	// LedgerVerified is true when the ledger's hash chain verified and every
	// event replayed. LedgerProblem says why it did not.
	LedgerVerified bool
	LedgerProblem  string
	// Contract is the parsed contract when it loaded and passed validation.
	// It is nil otherwise, and ContractProblem says why. The core validates
	// it again and treats a contract that fails as missing.
	Contract        *Contract
	ContractProblem string
	// ContractDigest and ContractRawDigest are the contract digest and the
	// raw digest of the contract file.
	ContractDigest    string
	ContractRawDigest string
	// Candidate is HEAD: its commit and candidate digest.
	Candidate Candidate
	// PacketProblem is non-empty when the open packet's stored file is
	// missing or does not match its recorded digest. The open packet is then
	// unusable. When no packet is open it is ignored.
	PacketProblem string
	// Clean is true when the working tree has no change outside .metron/,
	// untracked files included.
	Clean bool
	// Commits lists the commits reachable from HEAD but not from the base of
	// the packet being answered (git rev-list <base>..HEAD), with the base
	// taken from PacketRecord.Base. Only an implementer report needs it.
	Commits []Commit
}

// Commit is a commit between a packet's base and HEAD.
type Commit struct {
	// Hash is the full commit hash.
	Hash string
	// Substantive is true when the commit changes a path outside .metron/.
	// A commit that changes nothing else fixes nothing.
	Substantive bool
}

// ledgerProblem says why the ledger cannot be relied on, or returns "".
func ledgerProblem(s State, f Facts) string {
	switch {
	case !f.LedgerVerified:
		reason := f.LedgerProblem
		if blank(reason) {
			reason = "no reason given"
		}
		return fmt.Sprintf("the ledger failed verification (%s): restore it from history or repair it", reason)
	case !s.initialized:
		return "the ledger is not initialized: initialize it"
	case s.protocol != ProtocolVersion:
		return fmt.Sprintf("the ledger was written under protocol %q, not %q: use a matching binary", s.protocol, ProtocolVersion)
	}
	return ""
}

// contractProblem says why the contract cannot be used, or returns "". The
// contract is usable when it is valid and its digest is the pinned one.
func contractProblem(s State, f Facts) string {
	if f.Contract == nil {
		reason := f.ContractProblem
		if blank(reason) {
			reason = "no reason given"
		}
		return fmt.Sprintf("the contract is unavailable (%s): fix it, then pin it", reason)
	}
	if err := ValidateContract(*f.Contract); err != nil {
		var r *Rejection
		errors.As(err, &r)
		return fmt.Sprintf("the contract is invalid (%d problems, first: %s %s): fix it, then pin it",
			len(r.Problems), r.Problems[0].Path, r.Problems[0].Message)
	}
	if !validDigest(f.ContractDigest) {
		return fmt.Sprintf("the contract digest %q is malformed", f.ContractDigest)
	}
	pin, ok := s.pin()
	if !ok {
		return "no contract is pinned: governance must pin the contract"
	}
	if pin.Digest != f.ContractDigest {
		return fmt.Sprintf("the contract changed: its digest %s differs from the pinned %s; governance must pin it", f.ContractDigest, pin.Digest)
	}
	return ""
}

// packetInvalid says why an open packet is no longer valid, or returns "".
// A packet is valid while its stored file is usable, the pinned contract
// digest is the one it was issued under and, for a read-only role, the
// candidate digest is unchanged and the tree is clean.
func packetInvalid(s State, f Facts, p PacketRecord) string {
	pin, ok := s.pin()
	switch {
	case f.PacketProblem != "" && blank(f.PacketProblem):
		return "the stored packet file is missing or damaged"
	case f.PacketProblem != "":
		return f.PacketProblem
	case !ok || pin.Digest != p.Contract:
		return "the pinned contract changed"
	case !p.Role.ReadOnly():
		return ""
	case f.Candidate.Digest != p.Candidate.Digest:
		return fmt.Sprintf("the candidate changed from %s to %s", p.Candidate.Digest, f.Candidate.Digest)
	case !f.Clean:
		return "the working tree is not clean"
	}
	return ""
}

// ready refuses a command when the ledger or the contract cannot be relied on.
func ready(s State, f Facts) error {
	if p := ledgerProblem(s, f); p != "" {
		return refuse(LayerTransition, "%s", p)
	}
	if p := contractProblem(s, f); p != "" {
		return refuse(LayerTransition, "%s", p)
	}
	return nil
}

// checkCandidate rejects malformed candidate facts, which are an internal
// error of the caller.
func checkCandidate(f Facts) error {
	if !validCandidate(f.Candidate) {
		return fmt.Errorf("facts: candidate %+v is malformed", f.Candidate)
	}
	return nil
}
