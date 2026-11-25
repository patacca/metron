package core

import (
	"fmt"
	"strings"
)

// The failure streak, the campaign budget, and what they raise.
//
// A failed step is a verdict other than GREEN, a verification that rejects a
// fix or finds a blocking regression, or a check recording with a failure.
// The streak counts them since the last GREEN verdict or the last resumption
// by governance; nothing else ends it. The campaign budget counts every
// packet issued, whatever its outcome, so that steps which succeed cannot
// hide a campaign that never ends.

// stalls returns the escalations that one more failed step raises, without
// identifiers: a convergence analysis once per streak, from the moment the
// streak reaches its threshold, and a process audit each time the streak
// reaches a multiple of its own. what says which step failed.
func (s State) stalls(th Convergence, round int, packet, candidate, what string) []EscalationRaised {
	streak := s.streak + 1
	reason := fmt.Sprintf("%s since the last GREEN verdict or resumption, the latest being %s",
		count(streak, "step has failed", "steps have failed"), what)
	var out []EscalationRaised
	raise := func(kind EscalationKind) {
		out = append(out, EscalationRaised{Kind: kind, Owner: kind.Owner(), Round: round, Packet: packet, Candidate: candidate, Reason: reason})
	}
	if streak >= th.ConvergenceAuditAfter && !s.convergenceRaised {
		raise(EscalationNonConvergence)
	}
	if streak%th.ProcessAuditAfter == 0 {
		raise(EscalationProcessAudit)
	}
	return out
}

// stall queues the escalations that the failed step of the report under
// ingestion raises.
func (in *ingestion) stall(what string) {
	in.raised = append(in.raised, in.s.stalls(in.th, in.pk.Round, in.pk.Packet, in.f.Candidate.Digest, what)...)
}

// exhausted returns the suspension that issuing one more packet raises
// because the campaign has used up its budget, or false.
func (s State) exhausted(th Convergence, p Packet, round int) (EscalationRaised, bool) {
	used := s.steps
	if _, suspended := s.suspension(); suspended || used+1 < th.SuspendAfter {
		return EscalationRaised{}, false
	}
	return EscalationRaised{ID: seqID("E", len(s.escalations)+1), Kind: EscalationSuspension, Owner: RoleGovernance,
		Round: round, Packet: p.ID, Candidate: p.Candidate.Digest,
		Reason: fmt.Sprintf("packet %s is the %s of this campaign without an approval, which is its budget: the campaign is suspended once this packet is answered or void", p.ID, ordinal(used+1))}, true
}

// suspend queues the suspension an audit report raises: when it discharges a
// process audit, and when the auditor asks for one.
func (in *ingestion) suspend(b AuditorBody) {
	var why []string
	for _, id := range in.pk.Escalations {
		if i := in.s.escalationIndex(id); i >= 0 && in.s.escalations[i].Kind == EscalationProcessAudit {
			why = append(why, fmt.Sprintf("process audit %s is discharged", id))
		}
	}
	if b.Suspend != "" {
		why = append(why, "the auditor asks for the campaign to be suspended: "+b.Suspend)
	}
	if _, suspended := in.s.suspension(); suspended || len(why) == 0 {
		return
	}
	in.raise(EscalationSuspension, strings.Join(why, "; "), nil, nil, nil)
}

// ordinal writes n as an ordinal number.
func ordinal(n int) string {
	suffix := "th"
	if n%100 < 11 || n%100 > 13 {
		switch n % 10 {
		case 1:
			suffix = "st"
		case 2:
			suffix = "nd"
		case 3:
			suffix = "rd"
		}
	}
	return fmt.Sprintf("%d%s", n, suffix)
}
