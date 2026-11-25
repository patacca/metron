package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"github.com/patacca/metron/internal/core"
	"github.com/patacca/metron/internal/schema"
	"github.com/patacca/metron/internal/store"
)

// The read-only views: gate, status, findings, log and fsck. None takes the
// lock.

var cmdGate = &command{
	name:     "gate",
	synopsis: "gate [--json]",
	summary:  "Evaluate the gate; exit 0 only when it is open",
	about: `Gate evaluates the five gate conditions for the current candidate and prints
each with the reasons it does not hold:

  C  every applicable requirement is satisfied in the latest round's review
  B  no unresolved T0 or T1 finding and no open escalation
  E  every applicable requirement with a command has a passing result
     for this candidate and this command text
  I  integrity: the candidate is the reviewed one, the tree is clean, the
     contract is the pinned one, the ledger verifies, and every stored
     packet and report file matches the ledger
  V  the latest round's verdict is GREEN, for this candidate, under this
     contract, by an author who never acted as implementer

It exits 0 only when all five hold, 1 when the gate was evaluated and is
closed, and 3 when it cannot be evaluated: the ledger is missing, unreadable
or fails verification. With --json it prints a metron.gate/v1 document.`,
	run: runGate,
}

type gateDoc struct {
	Schema string `json:"schema"`
	core.GateResult
}

func runGate(c *ctx, args []string) error {
	fs := c.flags()
	dir, asJSON := stateFlag(fs), c.jsonFlag(fs)
	args, err := c.parse(fs, args)
	if err != nil {
		return err
	}
	if err := positional(args, 0, 0); err != nil {
		return err
	}
	s, err := c.open(*dir)
	if err != nil {
		return err
	}
	w, err := c.gather(s)
	if err != nil {
		return err
	}
	if why, run := w.ledgerUnusable(); why != "" {
		return fail(exitEnv, "the gate cannot be evaluated: %s\nRun %s.", why, run)
	}
	if err := w.candidate(); err != nil {
		return err
	}
	g := w.gate()
	if *asJSON {
		err = c.emit(gateDoc{Schema: "metron.gate/v1", GateResult: g})
	} else {
		var b strings.Builder
		writeGate(&b, g)
		if !g.Open {
			fmt.Fprintf(&b, "\nRun %s for the next step.\n", w.metron("next"))
		}
		_, err = io.WriteString(c.Stdout, b.String())
	}
	switch {
	case err != nil:
		return err
	case !g.Open:
		return negative
	}
	return nil
}

func writeGate(b *strings.Builder, g core.GateResult) {
	verdict := "CLOSED"
	if g.Open {
		verdict = "OPEN"
	}
	fmt.Fprintf(b, "Gate: %s for stage %s\n", verdict, orNone(string(g.Stage)))
	fmt.Fprintf(b, "Authorizes: %s\n", orNone(g.Authorizes))
	fmt.Fprintf(b, "Candidate: commit %s, %s\n\n", g.Candidate.Commit, g.Candidate.Digest)
	for _, cd := range g.Conditions {
		mark := "[ok]  "
		if !cd.Satisfied {
			mark = "[FAIL]"
		}
		fmt.Fprintf(b, "%s %s  %s\n", mark, cd.Name, cd.Description)
		for _, r := range cd.Reasons {
			fmt.Fprintf(b, "          - %s\n", r)
		}
	}
}

var cmdStatus = &command{
	name:     "status",
	synopsis: "status [--json]",
	summary:  "Summarize the ledger, the contract, the candidate and the next step",
	about: `Status summarizes the review state: the ledger, the contract and its pin, the
candidate and the working tree, the rounds of the current stage, the open
packet, the findings, the open escalations, the latest check results, the
integrity of the stored files, the gate and the next step. It changes
nothing. With --json it prints a metron.status/v1 document.`,
	run: runStatus,
}

type statusDoc struct {
	Schema          string            `json:"schema"`
	StateDir        string            `json:"state_dir"`
	Ledger          ledgerDoc         `json:"ledger"`
	Contract        contractSummary   `json:"contract"`
	Candidate       *core.Candidate   `json:"candidate"`
	Clean           bool              `json:"clean"`
	Dirty           []treeDoc         `json:"dirty"`
	Stage           core.Stage        `json:"stage,omitempty"`
	Rounds          []core.Round      `json:"rounds"`
	FailedStreak    int               `json:"failed_streak"`
	OpenPacket      *openPacketDoc    `json:"open_packet"`
	Findings        findingCounts     `json:"findings"`
	OpenEscalations []core.Escalation `json:"open_escalations"`
	Checks          []checkStatus     `json:"checks"`
	StoredFiles     []fileProblem     `json:"stored_file_problems"`
	GateOpen        bool              `json:"gate_open"`
	Next            *actionDoc        `json:"next"`
}

// treeDoc is one reason the working tree is not clean.
type treeDoc struct {
	Path string `json:"path"`
	// Kind is modified, untracked, hidden or link.
	Kind string `json:"kind"`
}

type ledgerDoc struct {
	Initialized bool   `json:"initialized"`
	Events      int    `json:"events"`
	Verified    bool   `json:"verified"`
	Protocol    string `json:"protocol,omitempty"`
	Problem     string `json:"problem,omitempty"`
}

type contractSummary struct {
	Valid        bool   `json:"valid"`
	Problem      string `json:"problem,omitempty"`
	Digest       string `json:"digest,omitempty"`
	PinnedDigest string `json:"pinned_digest,omitempty"`
	Pinned       bool   `json:"pinned"`
}

type openPacketDoc struct {
	Packet string `json:"packet"`
	// PacketDigest is the digest its report must name.
	PacketDigest string    `json:"packet_digest"`
	Action       core.Step `json:"action"`
	Role         core.Role `json:"role"`
	Round        int       `json:"round"`
	// Valid is true while the packet can still be answered.
	Valid bool `json:"valid"`
	// Invalid says why the packet can no longer be answered.
	Invalid string `json:"invalid,omitempty"`
}

type findingCounts struct {
	Open      int `json:"open"`
	Claimed   int `json:"claimed"`
	Resolved  int `json:"resolved"`
	Withdrawn int `json:"withdrawn"`
	// Blocking lists the unresolved T0 and T1 findings; Backlog the open
	// non-blocking ones.
	Blocking []string `json:"blocking"`
	Backlog  []string `json:"backlog"`
}

// checkStatus is the latest check result of an applicable requirement with a
// command, for the current candidate and command text.
type checkStatus struct {
	Requirement string `json:"requirement"`
	Command     string `json:"command"`
	// Result is "passed", "failed" or "missing".
	Result   string `json:"result"`
	ExitCode *int   `json:"exit_code,omitempty"`
}

func runStatus(c *ctx, args []string) error {
	fs := c.flags()
	dir, asJSON := stateFlag(fs), c.jsonFlag(fs)
	args, err := c.parse(fs, args)
	if err != nil {
		return err
	}
	if err := positional(args, 0, 0); err != nil {
		return err
	}
	s, err := c.open(*dir)
	if err != nil {
		return err
	}
	w, err := c.gather(s)
	if err != nil {
		return err
	}
	d := w.status()
	if *asJSON {
		return c.emit(d)
	}
	var b strings.Builder
	writeStatus(&b, w, d)
	_, err = io.WriteString(c.Stdout, b.String())
	return err
}

func (w *world) status() statusDoc {
	st := w.state
	a := w.next()
	d := statusDoc{
		Schema:   "metron.status/v1",
		StateDir: w.show(w.store.Dir()),
		Ledger: ledgerDoc{Initialized: !w.ledgerMissing, Events: st.Events(), Verified: w.facts.LedgerVerified && !w.ledgerMissing,
			Protocol: st.Protocol(), Problem: w.facts.LedgerProblem},
		Contract: contractSummary{Valid: w.facts.Contract != nil, Problem: w.facts.ContractProblem, Digest: w.facts.ContractDigest},
		Clean:    w.facts.Clean, Dirty: []treeDoc{}, Stage: st.Stage(), Rounds: st.StageRounds(), FailedStreak: st.FailedStreak(),
		OpenEscalations: []core.Escalation{}, Checks: []checkStatus{}, StoredFiles: w.storedFiles(),
		GateOpen: w.gate().Open, Next: w.actionDoc(a),
	}
	if why, run := w.ledgerUnusable(); why != "" && w.ledgerMissing {
		d.Ledger.Problem = why + ": run " + run
	}
	if d.StoredFiles == nil {
		d.StoredFiles = []fileProblem{}
	}
	for _, p := range w.dirty {
		d.Dirty = append(d.Dirty, treeDoc{Path: p.Path, Kind: p.Kind})
	}
	if pin, ok := st.Pin(); ok {
		d.Contract.PinnedDigest = pin.Digest
		d.Contract.Pinned = d.Contract.Valid && pin.Digest == w.facts.ContractDigest
	}
	if !w.noCommits {
		d.Candidate = &w.facts.Candidate
	}
	if d.Rounds == nil {
		d.Rounds = []core.Round{}
	}
	if op, ok := st.OpenPacket(); ok {
		d.OpenPacket = &openPacketDoc{Packet: op.Packet, PacketDigest: op.Digest, Action: op.Action, Role: op.Role, Round: op.Round,
			Valid: a.Step == core.StepSubmitReport && a.Packet == op.Packet, Invalid: core.PacketInvalid(st, w.facts)}
	}
	d.Findings = findingCounts{Blocking: []string{}, Backlog: []string{}}
	for _, f := range st.Findings() {
		switch f.Status {
		case core.StatusOpen:
			d.Findings.Open++
		case core.StatusClaimed:
			d.Findings.Claimed++
		case core.StatusResolved:
			d.Findings.Resolved++
		case core.StatusWithdrawn:
			d.Findings.Withdrawn++
		}
		if f.Status.Unresolved() && f.Tier.Blocking() {
			d.Findings.Blocking = append(d.Findings.Blocking, f.ID)
		}
	}
	for _, f := range st.Backlog() {
		d.Findings.Backlog = append(d.Findings.Backlog, f.ID)
	}
	for _, e := range st.Escalations() {
		if e.Open {
			d.OpenEscalations = append(d.OpenEscalations, e)
		}
	}
	if c := w.facts.Contract; c != nil {
		evidence := st.Evidence()
		for _, r := range c.Applicable() {
			if r.Command == "" {
				continue
			}
			cs := checkStatus{Requirement: r.ID, Command: r.Command, Result: "missing"}
			// The latest result for this candidate and this command text counts.
			for i := len(evidence) - 1; i >= 0; i-- {
				e := evidence[i]
				if e.Requirement != r.ID || e.Command != r.Command || e.Candidate != w.facts.Candidate.Digest {
					continue
				}
				cs.ExitCode = &e.ExitCode
				cs.Result = "failed"
				if e.Passed {
					cs.Result = "passed"
				}
				break
			}
			d.Checks = append(d.Checks, cs)
		}
	}
	return d
}

func writeStatus(b *strings.Builder, w *world, d statusDoc) {
	ledger := fmt.Sprintf("%s, %s, verified", d.StateDir, plural(d.Ledger.Events, "event"))
	if !d.Ledger.Verified {
		ledger = fmt.Sprintf("%s, %s", d.StateDir, d.Ledger.Problem)
	}
	fmt.Fprintf(b, "Ledger:       %s\n", ledger)
	contract := "INVALID: " + d.Contract.Problem
	switch {
	case d.Contract.Pinned:
		contract = fmt.Sprintf("valid and pinned, %s", d.Contract.Digest)
	case d.Contract.Valid && d.Contract.PinnedDigest == "":
		contract = fmt.Sprintf("valid, not pinned, %s", d.Contract.Digest)
	case d.Contract.Valid:
		contract = fmt.Sprintf("valid, but %s differs from the pinned %s", d.Contract.Digest, d.Contract.PinnedDigest)
	}
	fmt.Fprintf(b, "Contract:     %s\n", contract)
	fmt.Fprintf(b, "Stage:        %s\n", orNone(string(d.Stage)))
	candidate := "none: the repository has no commits"
	if d.Candidate != nil {
		tree := "clean"
		if !d.Clean {
			var why []string
			for _, p := range d.Dirty {
				why = append(why, p.Path+" "+p.Kind)
			}
			tree = fmt.Sprintf("not clean (%s)", strings.Join(why, ", "))
		}
		candidate = fmt.Sprintf("commit %s, %s, tree %s", short(d.Candidate.Commit), d.Candidate.Digest, tree)
	}
	fmt.Fprintf(b, "Candidate:    %s\n", candidate)
	var rounds []string
	for _, r := range d.Rounds {
		switch {
		case r.Voided:
			rounds = append(rounds, fmt.Sprintf("%d voided", r.Number))
		case r.Verdict == "":
			rounds = append(rounds, fmt.Sprintf("%d in review", r.Number))
		default:
			rounds = append(rounds, fmt.Sprintf("%d %s by %s", r.Number, r.Verdict, r.Reviewer))
		}
	}
	fmt.Fprintf(b, "Rounds:       %s; failed streak %d\n", orNone(strings.Join(rounds, ", ")), d.FailedStreak)
	packet := "none"
	if op := d.OpenPacket; op != nil {
		packet = fmt.Sprintf("%s, %s for the %s, round %d", op.Packet, op.Action, op.Role, op.Round)
		switch {
		case op.Invalid != "":
			packet += "; no longer valid: " + op.Invalid
		case !op.Valid:
			packet += "; no longer valid"
		}
	}
	fmt.Fprintf(b, "Open packet:  %s\n", packet)
	f := d.Findings
	fmt.Fprintf(b, "Findings:     %d open, %d claimed, %d resolved, %d withdrawn; blocking: %s; backlog: %s\n",
		f.Open, f.Claimed, f.Resolved, f.Withdrawn, orNone(strings.Join(f.Blocking, ", ")), orNone(strings.Join(f.Backlog, ", ")))
	var escalations []string
	for _, e := range d.OpenEscalations {
		escalations = append(escalations, fmt.Sprintf("%s (%s, owned by %s)", e.ID, e.Kind, the(e.Owner)))
	}
	fmt.Fprintf(b, "Escalations:  %s\n", orNone(strings.Join(escalations, ", ")))
	var checks []string
	for _, cs := range d.Checks {
		s := cs.Requirement + " " + cs.Result
		if cs.ExitCode != nil {
			s += fmt.Sprintf(" (exit %d)", *cs.ExitCode)
		}
		checks = append(checks, s)
	}
	fmt.Fprintf(b, "Checks:       %s\n", orNone(strings.Join(checks, ", ")))
	files := "ok"
	if len(d.StoredFiles) > 0 {
		files = plural(len(d.StoredFiles), "problem") + ", see " + w.metron("fsck")
	}
	fmt.Fprintf(b, "Stored files: %s\n", files)
	gate := "CLOSED"
	if d.GateOpen {
		gate = "OPEN"
	}
	fmt.Fprintf(b, "Gate:         %s\n\n", gate)
	writeNext(b, w, d.Next)
}

var cmdFindings = &command{
	name:     "findings",
	synopsis: "findings [--backlog | --all] [--json]",
	summary:  "List the unresolved findings, the backlog, or every finding",
	about: `Findings lists the unresolved findings (open or claimed, any tier) by default,
the improvement backlog (open findings of the non-blocking tiers T2 to T4)
with --backlog, and every finding with --all. It changes nothing. With --json
it prints a metron.findings/v1 document with the full record of each finding,
its history included.`,
	run: runFindings,
}

type findingsDoc struct {
	Schema   string         `json:"schema"`
	Filter   string         `json:"filter"`
	Findings []core.Finding `json:"findings"`
}

func runFindings(c *ctx, args []string) error {
	fs := c.flags()
	dir, asJSON := stateFlag(fs), c.jsonFlag(fs)
	backlog := fs.Bool("backlog", false, "list the open non-blocking findings")
	all := fs.Bool("all", false, "list every finding")
	args, err := c.parse(fs, args)
	if err != nil {
		return err
	}
	if err := positional(args, 0, 0); err != nil {
		return err
	}
	if *backlog && *all {
		return usageError("--backlog and --all exclude each other")
	}
	s, err := c.open(*dir)
	if err != nil {
		return err
	}
	w, err := c.readable(s)
	if err != nil {
		return err
	}
	d := findingsDoc{Schema: "metron.findings/v1", Filter: "unresolved", Findings: []core.Finding{}}
	for _, f := range w.state.Findings() {
		switch {
		case *all, *backlog && f.Status == core.StatusOpen && !f.Tier.Blocking(), !*backlog && f.Status.Unresolved():
			d.Findings = append(d.Findings, f)
		}
	}
	switch {
	case *all:
		d.Filter = "all"
	case *backlog:
		d.Filter = "backlog"
	}
	if *asJSON {
		return c.emit(d)
	}
	var b strings.Builder
	if len(d.Findings) == 0 {
		fmt.Fprintf(&b, "No findings (%s).\n", d.Filter)
	}
	for _, f := range d.Findings {
		fmt.Fprintf(&b, "%s  %s  %-9s %s: %s\n", f.ID, f.Tier, f.Status, f.Input.Domain, f.Input.Title)
		details := []string{fmt.Sprintf("round %d, reported by the %s", f.Round, f.Source)}
		if f.Input.Requirement != "" {
			details = append(details, "requirement "+f.Input.Requirement)
		}
		if f.Input.Risk != "" {
			details = append(details, "risk "+f.Input.Risk)
		}
		if len(f.Input.Components) > 0 {
			details = append(details, "components "+strings.Join(f.Input.Components, ", "))
		}
		if f.Recurrences > 0 {
			details = append(details, fmt.Sprintf("recurrences %d", f.Recurrences))
		}
		if f.RootCauseRequired {
			details = append(details, "root cause required")
		}
		fmt.Fprintf(&b, "        %s\n", strings.Join(details, "; "))
		if f.Fix != nil {
			fmt.Fprintf(&b, "        fix: %s\n", f.Fix.Summary)
		}
	}
	_, err = io.WriteString(c.Stdout, b.String())
	return err
}

var cmdLog = &command{
	name:     "log",
	synopsis: "log",
	summary:  "Print the ledger's events",
	about: `Log prints every event of the ledger, oldest first, one line each: sequence
number, time, type and a summary. It changes nothing.`,
	run: runLog,
}

func runLog(c *ctx, args []string) error {
	fs := c.flags()
	dir := stateFlag(fs)
	args, err := c.parse(fs, args)
	if err != nil {
		return err
	}
	if err := positional(args, 0, 0); err != nil {
		return err
	}
	s, err := c.open(*dir)
	if err != nil {
		return err
	}
	w, err := c.readable(s)
	if err != nil {
		return err
	}
	var b strings.Builder
	for _, e := range w.events {
		fmt.Fprintf(&b, "%4d  %s  %s\n", e.Seq, e.Time, describe(e))
	}
	_, err = io.WriteString(c.Stdout, b.String())
	return err
}

// readable gathers the facts for a view that needs a usable ledger.
func (c *ctx) readable(s *site) (*world, error) {
	w, err := c.gather(s)
	if err != nil {
		return nil, err
	}
	if err := w.usable(false); err != nil {
		return nil, err
	}
	return w, nil
}

var cmdFsck = &command{
	name:     "fsck",
	synopsis: "fsck",
	summary:  "Verify the ledger and the stored packet and report files",
	about: `Fsck verifies the ledger's hash chain, replays every event, validates each
against the event schema, checks the protocol version, and checks that the
stored file of every answered packet and of every accepted report matches the
digest the ledger records. It prints how to recover from each problem it finds
and exits 1 if it finds any. A lost or altered file of the open packet is only
noted: metron packet voids that packet and issues another when one is next
needed.`,
	run: runFsck,
}

func runFsck(c *ctx, args []string) error {
	fs := c.flags()
	dir := stateFlag(fs)
	args, err := c.parse(fs, args)
	if err != nil {
		return err
	}
	if err := positional(args, 0, 0); err != nil {
		return err
	}
	s, err := c.open(*dir)
	if err != nil {
		return err
	}
	w, err := c.gather(s)
	if err != nil {
		return err
	}
	var b strings.Builder
	var problems, notes []string
	ledger := w.show(filepath.Join(w.store.Dir(), store.LedgerName))
	switch {
	case w.ledgerEmpty:
		problems = append(problems, fmt.Sprintf("%s is empty, so metron is not initialized: run %s", ledger, w.metron("init")))
	case w.ledgerMissing:
		problems = append(problems, fmt.Sprintf("%s does not exist: run %s", ledger, w.metron("init")))
	case !w.facts.LedgerVerified:
		problems = append(problems, restoreLedger(w.facts.LedgerProblem))
	case w.state.Protocol() != core.ProtocolVersion:
		problems = append(problems, fmt.Sprintf("the ledger was written under protocol %q, not %q: use a matching metron", w.state.Protocol(), core.ProtocolVersion))
	default:
		fmt.Fprintf(&b, "Ledger:  %s, %s, hash chain and replay verified, protocol %s\n", ledger, plural(w.state.Events(), "event"), w.state.Protocol())
		for _, e := range w.events {
			line, err := json.Marshal(e)
			if err != nil {
				return err
			}
			ps, err := schema.ValidateJSON(schema.Event, line)
			if err != nil {
				return err
			}
			for _, p := range ps {
				problems = append(problems, fmt.Sprintf("ledger event %d (%s) does not satisfy the event schema: %s", e.Seq, e.Type, p))
			}
		}
		var packets, voided, reports int
		for _, p := range w.state.Packets() {
			switch {
			case p.Report != nil:
				packets++
				reports++
			case p.Open:
				packets++
			default:
				voided++
			}
		}
		fmt.Fprintf(&b, "Files:   %s and %s checked", plural(packets, "packet file"), plural(reports, "report file"))
		if voided > 0 {
			fmt.Fprintf(&b, "; %s need no file", plural(voided, "voided packet"))
		}
		b.WriteString("\n")
		for _, p := range w.storedFiles() {
			// The open packet's file is operational, not evidence.
			if p.open {
				notes = append(notes, fmt.Sprintf("%s; this is not an integrity failure: %s voids packet %s and issues another when a packet is next needed",
					p.Problem, w.metron("packet"), p.Packet))
				continue
			}
			problems = append(problems, p.String())
		}
	}
	for _, n := range notes {
		fmt.Fprintf(&b, "Note: %s.\n", n)
	}
	if len(problems) == 0 {
		b.WriteString("No problem found.\n")
	} else {
		fmt.Fprintf(&b, "%s found:\n", plural(len(problems), "problem"))
		for _, p := range problems {
			fmt.Fprintf(&b, "  - %s\n", p)
		}
	}
	if _, err := io.WriteString(c.Stdout, b.String()); err != nil {
		return err
	}
	if len(problems) > 0 {
		return negative
	}
	return nil
}
