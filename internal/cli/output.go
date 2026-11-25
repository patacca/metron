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
	// Tree lists why the working tree is not clean, for commit-changes.
	Tree []treeDoc `json:"tree,omitempty"`
	// StateDir is the state directory a template to set it aside applies to.
	StateDir string `json:"state_dir,omitempty"`
	// Command is a complete command, safe to run as it stands in a POSIX
	// shell. Usage is a template with <placeholders> for a step that needs a
	// decision, a message or a file, whose subjects are in their own members.
	// At most one is set; with neither, hint says what to do.
	Command string `json:"command,omitempty"`
	Usage   string `json:"usage,omitempty"`
	hint    string
	// leftovers marks a tree made unclean by the checks: what they left is
	// removed, never committed.
	leftovers bool
}

func (w *world) actionDoc(a core.Action) *actionDoc {
	d := &actionDoc{Action: a, hint: thenHint(w, a)}
	d.Command, d.Usage = w.instruction(a)
	if rec, ok := w.state.Packet(a.Packet); ok && a.Packet != "" {
		d.PacketDigest = rec.Digest
	}
	if a.Step == core.StepCommitChanges {
		for _, p := range w.dirty {
			d.Tree = append(d.Tree, treeDoc{Path: p.Path, Kind: p.Kind})
		}
	}
	if a.Step == core.StepRepairLedger && len(w.stranded) > 0 {
		d.StateDir = w.show(w.store.Dir())
	}
	return d
}

// writeNext prints the next action for a person. A file name is printed only
// as data, never inside a command.
func writeNext(b *strings.Builder, w *world, d *actionDoc) {
	a := d.Action
	line := func(label, value string) { fmt.Fprintf(b, "%-14s%s\n", label+":", value) }
	if a.Step == core.StepNone {
		line("Next step", "none, the gate is open")
	} else {
		line("Next step", fmt.Sprintf("%s (owner: %s)", a.Step, a.Owner))
	}
	line("Reason", printable(a.Reason))
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
		writeTree(line, w, w.dirty, d.leftovers)
	}
	switch {
	case d.Command != "":
		line("Run", d.Command)
	case d.Usage != "":
		line("Usage", d.Usage)
	default:
		line("Do", d.hint)
		return
	}
	if d.hint != "" {
		line("Then", d.hint)
	}
}

// howTo is one line that says how to perform an action: "Run:" with a
// complete command, "Usage:" with a template, or "Do:" with what to do.
func (w *world) howTo(a core.Action) string {
	d := w.actionDoc(a)
	switch {
	case d.Command != "":
		return "Run: " + d.Command
	case d.Usage != "":
		return "Usage: " + d.Usage
	}
	return "Do: " + d.hint
}

// thenHint says what follows the command, for the steps that take more than
// one command, or what to do when no command does it.
func thenHint(w *world, a core.Action) string {
	switch a.Step {
	case core.StepReview, core.StepFix, core.StepVerify, core.StepAudit:
		role, _ := a.Step.PacketRole()
		return fmt.Sprintf("give the packet to the %s (--format prompt renders it for reading; --format skeleton -o .metron/work/report.json writes the report to fill in) and submit the report with %s",
			role, w.metron("ingest <report-file>"))
	case core.StepSubmitReport:
		return fmt.Sprintf("%s prints the packet again if it was lost", w.metron("packet"))
	case core.StepPinContract:
		if w.facts.Contract == nil {
			return fmt.Sprintf("fix every problem it lists, then pin the contract with %s", w.metron("contract pin -m <reason>"))
		}
	case core.StepCommitChanges:
		if cmd, usage := w.instruction(a); cmd == "" && usage == "" {
			if w.facts.Clean && !w.noCommits {
				return "the candidate itself must change: commit a change to something outside .metron/, then run " + w.metron("next")
			}
			return "make the working tree clean as listed, then run " + w.metron("next")
		}
		if cmd, _ := w.instruction(a); cmd != "" {
			return "run " + w.metron("next")
		}
		return "run " + w.metron("next") + "; discarding the changes also makes the tree clean"
	case core.StepRunChecks:
		return "if a check fails, change the code, commit it and run the checks again"
	}
	return ""
}

// treeKinds says, for each kind of reason the working tree is not clean,
// what it is and how to fix it; leftovers holds the fix for what a check left
// behind, which is removed, never committed. A fix names no file: the files
// are listed apart, as data.
var treeKinds = []struct {
	kind, what, fix, leftovers string
}{
	{vcs.TreeModified, "modified", "commit or discard each modified file",
		"restore each modified file with git checkout -- <path>"},
	{vcs.TreeDeleted, "deleted", "commit each deletion with git rm -- <path>, or restore the file with git checkout -- <path>",
		"restore each deleted file with git checkout -- <path>"},
	{vcs.TreeUntracked, "untracked", "commit, delete or ignore each untracked file",
		"delete each untracked file"},
	{vcs.TreeHidden, "hidden by an assume-unchanged or skip-worktree flag",
		"clear the flags of each hidden file with git update-index --no-skip-worktree -- <path> and git update-index --no-assume-unchanged -- <path>, then commit or discard what they hid",
		"clear the flags of each hidden file with git update-index --no-skip-worktree -- <path> and git update-index --no-assume-unchanged -- <path>, then restore it with git checkout -- <path>"},
	{vcs.TreeLink, "a symbolic link into .metron/ or .git/",
		"replace each such link with what it points to, or remove it, and commit", "remove each such link"},
}

// writeTree prints why the working tree is not clean: each path with what is
// wrong with it, escaped, then one fix per kind.
func writeTree(line func(label, value string), w *world, ps []vcs.TreeProblem, leftovers bool) {
	if w.ignoreCase {
		line("Tree", "core.ignoreCase is true on a file system that distinguishes case")
		line("Fix", "set it to false with git config core.ignoreCase false, then look again: git may have overlooked files")
	}
	for _, p := range ps {
		what := p.Kind
		for _, k := range treeKinds {
			if k.kind == p.Kind {
				what = k.what
			}
		}
		line("Tree", printable(p.Path)+": "+what)
	}
	for _, k := range treeKinds {
		if !slices.ContainsFunc(ps, func(p vcs.TreeProblem) bool { return p.Kind == k.kind }) {
			continue
		}
		fix := k.fix
		if leftovers {
			fix = k.leftovers
		}
		if k.kind == vcs.TreeHidden && (w.sparse || w.ignoreStat) {
			// Clearing the flags would not last: git sets them again.
			if w.sparse {
				line("Fix", "this is a sparse checkout, which sets the flag on every file it leaves out, so it is never clean: end it with git sparse-checkout disable")
			}
			if w.ignoreStat {
				line("Fix", "core.ignoreStat is true, under which git flags every file it writes: set it to false with git config core.ignoreStat false, then commit or discard what the flags hid")
			}
			continue
		}
		line("Fix", fix)
	}
}

// shellQuote quotes s for a POSIX shell: as it is when every byte is plainly
// safe, else in single quotes, each single quote written '\”. Text from the
// invocation that metron repeats in a command goes through it.
func shellQuote(s string) string {
	safe := s != ""
	for i := 0; i < len(s) && safe; i++ {
		c := s[i]
		safe = 'a' <= c && c <= 'z' || 'A' <= c && c <= 'Z' || '0' <= c && c <= '9' || strings.IndexByte("_./:@%+=,-", c) >= 0
	}
	if safe {
		return s
	}
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
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
		if d.Scope == core.ScopeExcessive {
			return fmt.Sprintf("%s: the fix changes more than the finding needs (%s); %s", d.ID, d.ScopeEvidence, d.Evidence)
		}
		return fmt.Sprintf("%s: %s", d.ID, d.Evidence)
	case core.EventFindingReopened:
		d := decode[core.FindingReopened](e)
		return fmt.Sprintf("%s (%s): %s", d.ID, d.Reason, d.Justification)
	case core.EventFindingAddressed:
		d := decode[core.FindingAddressed](e)
		return fmt.Sprintf("%s: %s", d.ID, d.Evidence)
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
	case core.EventDecisionSuperseded:
		d := decode[core.DecisionSuperseded](e)
		return fmt.Sprintf("%s by %s: %s", d.Decision, d.Packet, d.Rationale)
	}
	return string(e.Data)
}
