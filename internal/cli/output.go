package cli

import (
	jsonv2 "encoding/json/v2"
	"fmt"
	"slices"
	"strings"

	"github.com/patacca/metron/internal/core"
	"github.com/patacca/metron/internal/vcs"
)

// actionDoc is the next action as printed by metron next --json and embedded
// in other documents.
type actionDoc struct {
	Schema string `json:"schema,omitempty"`
	core.Action
	// PacketDigest is the digest of the open packet the step concerns, which
	// its report must name.
	PacketDigest string `json:"packet_digest,omitempty"`
	// Command is the command that performs or continues the step. It is
	// empty when no command does, and then hint says what to do.
	Command string `json:"command"`
	hint    string
	// leftovers marks a tree made unclean by the checks: what they left is
	// removed, never committed.
	leftovers bool
}

func (w *world) actionDoc(a core.Action) *actionDoc {
	d := &actionDoc{Action: a, Command: w.command(a), hint: thenHint(w, a)}
	if rec, ok := w.state.Packet(a.Packet); ok && a.Packet != "" {
		d.PacketDigest = rec.Digest
	}
	return d
}

// writeNext prints the next action for a person.
func writeNext(b *strings.Builder, w *world, d *actionDoc) {
	a := d.Action
	line := func(label, value string) { fmt.Fprintf(b, "%-14s%s\n", label+":", value) }
	if a.Step == core.StepNone {
		line("Next step", "none, the gate is open")
	} else {
		line("Next step", fmt.Sprintf("%s (owner: %s)", a.Step, a.Owner))
	}
	line("Reason", a.Reason)
	if a.Packet != "" {
		line("Packet", a.Packet+", digest "+d.PacketDigest)
	}
	for _, l := range []struct {
		label string
		ids   []string
	}{{"Findings", a.Findings}, {"Escalations", a.Escalations}, {"Requirements", a.Requirements}} {
		if len(l.ids) > 0 {
			line(l.label, strings.Join(l.ids, ", "))
		}
	}
	if op, why, ok := w.staleOpenPacket(a); ok {
		line("Note", fmt.Sprintf("packet %s (%s) is open but no longer valid (%s); %s voids it", op.Packet, op.Action, why, w.metron("packet")))
	}
	if a.Step == core.StepCommitChanges {
		for _, l := range treeLines(w.dirty, d.leftovers) {
			line("Tree", l)
		}
	}
	switch {
	case d.Command == "":
		line("Do", d.hint)
	default:
		line("Run", d.Command)
		if d.hint != "" {
			line("Then", d.hint)
		}
	}
}

// thenHint says what follows the command, for the steps that take more than
// one command.
func thenHint(w *world, a core.Action) string {
	switch a.Step {
	case core.StepReview, core.StepFix, core.StepVerify, core.StepAudit:
		role, _ := a.Step.PacketRole()
		return fmt.Sprintf("give the packet to the %s (--format prompt renders it for reading) and submit the report with %s",
			role, w.metron("ingest <report-file>"))
	case core.StepSubmitReport:
		return fmt.Sprintf("%s prints the packet again if it was lost", w.metron("packet"))
	case core.StepPinContract:
		if w.facts.Contract == nil {
			return fmt.Sprintf(`fix every problem it lists, then pin the contract with %s`, w.metron(`contract pin -m "<reason>"`))
		}
	case core.StepCommitChanges:
		switch {
		case w.facts.Clean && !w.noCommits:
			return "the candidate itself must change: commit a change to something outside .metron/, then run " + w.metron("next")
		case w.command(a) == "":
			return "replace each symbolic link into .metron/ with what it points to, or remove it, commit, then run " + w.metron("next")
		}
		return fmt.Sprintf("run %s; discarding the changes also makes the tree clean", w.metron("next"))
	case core.StepRunChecks:
		return "if a check fails, change the code, commit it and run the checks again"
	}
	return ""
}

// treeLines says, for each reason the working tree is not clean, what it is
// and how to fix it. leftovers selects the fixes for what a check left
// behind, which must be removed, not committed.
func treeLines(ps []vcs.TreeProblem, leftovers bool) []string {
	out := make([]string, 0, len(ps))
	for _, p := range ps {
		var what, fix string
		switch p.Kind {
		case vcs.TreeModified:
			what, fix = "modified", "commit or discard the change"
			if leftovers {
				fix = "restore it with git checkout -- " + quoteArg(p.Path)
			}
		case vcs.TreeUntracked:
			what, fix = "untracked", "commit it, delete it, or ignore it"
			if leftovers {
				fix = "delete it"
			}
		case vcs.TreeHidden:
			what = "hidden by an assume-unchanged or skip-worktree flag"
			fix = fmt.Sprintf("clear the flag with git update-index --no-skip-worktree -- %[1]s and git update-index --no-assume-unchanged -- %[1]s, then commit or discard what it hid", quoteArg(p.Path))
		case vcs.TreeLink:
			what, fix = "a symbolic link into .metron/", "replace the link with what it points to, or remove it, and commit"
		default:
			what, fix = p.Kind, "make the tree clean"
		}
		out = append(out, fmt.Sprintf("%s is %s: %s", p.Path, what, fix))
	}
	if slices.ContainsFunc(ps, func(p vcs.TreeProblem) bool { return p.Kind == vcs.TreeHidden }) {
		out = append(out, "if a sparse checkout or core.ignoreStat=true sets these flags, clearing them alone does not last: disable the sparse checkout with git sparse-checkout disable, and set git config core.ignoreStat false, first")
	}
	return out
}

// the names a role in a sentence: "governance", or "the reviewer".
func the(r core.Role) string {
	if r == core.RoleGovernance {
		return string(r)
	}
	return "the " + string(r)
}

// short abbreviates a digest or a commit hash for summaries.
func short(s string) string {
	h, ok := strings.CutPrefix(s, "sha256:")
	if len(h) > 12 {
		h = h[:12]
	}
	if ok {
		return "sha256:" + h
	}
	return h
}

// decode decodes an event's payload for display. Every event shown has
// replayed, so its payload decodes; a zero value is shown otherwise.
func decode[T any](e core.Event) T {
	var v T
	_ = jsonv2.Unmarshal(e.Data, &v)
	return v
}

// describe summarizes an event in one line.
func describe(e core.Event) string {
	return fmt.Sprintf("%-22s %s", e.Type, summarize(e))
}

func summarize(e core.Event) string {
	switch e.Type {
	case core.EventLedgerInitialized:
		d := decode[core.LedgerInitialized](e)
		return "protocol " + d.Protocol
	case core.EventContractPinned:
		d := decode[core.ContractPinned](e)
		return fmt.Sprintf("contract %s, stage %s, pinned by %s: %s", short(d.Digest), d.Stage, d.By, d.Reason)
	case core.EventEvidenceRecorded:
		d := decode[core.Evidence](e)
		result := "passed"
		if !d.Passed {
			result = "FAILED"
		}
		return fmt.Sprintf("%s %s (exit %d) on candidate %s: %s", d.Requirement, result, d.ExitCode, short(d.Candidate), d.Command)
	case core.EventRoundOpened:
		d := decode[core.RoundOpened](e)
		return fmt.Sprintf("round %d of stage %s on commit %s", d.Round, d.Stage, short(d.Candidate.Commit))
	case core.EventPacketIssued:
		d := decode[core.PacketIssued](e)
		s := fmt.Sprintf("%s: %s for the %s, round %d, commit %s", d.Packet, d.Action, d.Role, d.Round, short(d.Candidate.Commit))
		if len(d.Subjects) > 0 {
			s += ", subjects " + strings.Join(d.Subjects, ", ")
		}
		if len(d.Escalations) > 0 {
			s += ", escalations " + strings.Join(d.Escalations, ", ")
		}
		return s
	case core.EventPacketVoided:
		d := decode[core.PacketVoided](e)
		return fmt.Sprintf("%s: %s", d.Packet, d.Reason)
	case core.EventReportAccepted:
		d := decode[core.ReportAccepted](e)
		return fmt.Sprintf("%s: %s report by %s (%s)", d.Packet, d.Role, d.Author.ID, d.Author.Kind)
	case core.EventVerdictRecorded:
		d := decode[core.VerdictRecorded](e)
		return fmt.Sprintf("round %d: %s by %s", d.Round, d.Verdict, d.Reviewer)
	case core.EventFindingOpened:
		d := decode[core.FindingOpened](e)
		return fmt.Sprintf("%s (%s, %s): %s", d.ID, d.Input.Tier, d.Input.Domain, d.Input.Title)
	case core.EventFindingFixClaimed:
		d := decode[core.FindingFixClaimed](e)
		commits := make([]string, len(d.Fix.Commits))
		for i, c := range d.Fix.Commits {
			commits[i] = short(c)
		}
		return fmt.Sprintf("%s: %s (commits %s)", d.ID, d.Fix.Summary, strings.Join(commits, ", "))
	case core.EventFindingFixConfirmed:
		d := decode[core.FindingFixConfirmed](e)
		return fmt.Sprintf("%s: %s", d.ID, d.Evidence)
	case core.EventFindingFixRejected:
		d := decode[core.FindingFixRejected](e)
		return fmt.Sprintf("%s: %s", d.ID, d.Evidence)
	case core.EventFindingReopened:
		d := decode[core.FindingReopened](e)
		return fmt.Sprintf("%s (%s): %s", d.ID, d.Reason, d.Justification)
	case core.EventFindingWithdrawn:
		d := decode[core.FindingWithdrawn](e)
		return fmt.Sprintf("%s: %s", d.ID, d.Rationale)
	case core.EventFindingReclassified:
		d := decode[core.FindingReclassified](e)
		return fmt.Sprintf("%s %s -> %s: %s", d.ID, d.From, d.To, d.Rationale)
	case core.EventEscalationRaised:
		d := decode[core.EscalationRaised](e)
		return fmt.Sprintf("%s (%s, owned by %s): %s", d.ID, d.Kind, the(d.Owner), d.Reason)
	case core.EventEscalationResolved:
		d := decode[core.EscalationResolved](e)
		return fmt.Sprintf("%s by %s (%s): %s", d.ID, the(d.By), d.Actor, d.Resolution)
	case core.EventDirectiveIssued:
		d := decode[core.DirectiveIssued](e)
		return fmt.Sprintf("from %s: %s", d.Packet, d.Text)
	}
	return string(e.Data)
}
