package cli

import (
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"strings"

	"github.com/patacca/metron/internal/canon"
	"github.com/patacca/metron/internal/core"
	"github.com/patacca/metron/internal/store"
	"github.com/patacca/metron/internal/vcs"
)

// envStateDir names the environment variable that sets the state directory.
const envStateDir = "METRON_STATE_DIR"

// site is the repository and state directory a command works on.
type site struct {
	repo  *vcs.Repo
	store *store.Store
	// flagDir is --state-dir as given, repeated in the commands metron prints
	// so that following them reaches the same state directory.
	flagDir string
}

// open locates the repository that contains the working directory and the
// state directory: --state-dir, else $METRON_STATE_DIR, else .metron/state
// under the repository root.
func (c *ctx) open(flagDir string) (*site, error) {
	repo, err := vcs.Open(c.Dir)
	if err != nil {
		return nil, fail(exitEnv, "%v\nmetron works inside a git repository: run it there, or create one with git init", err)
	}
	dir := flagDir
	if dir == "" {
		dir = c.Getenv(envStateDir)
	}
	switch {
	case dir == "":
		dir = filepath.Join(repo.Root(), ".metron", "state")
	case !filepath.IsAbs(dir):
		dir = filepath.Join(c.Dir, dir)
	}
	return &site{repo: repo, store: store.New(filepath.Clean(dir)), flagDir: flagDir}, nil
}

// show renders a path for output: relative to the repository root, with
// forward slashes, when it lies inside the repository.
func (s *site) show(path string) string {
	if rel, err := filepath.Rel(s.repo.Root(), path); err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return filepath.ToSlash(rel)
	}
	return filepath.ToSlash(path)
}

// metron renders a metron command line that reaches the same state directory.
func (s *site) metron(args string) string {
	if s.flagDir == "" {
		return "metron " + args
	}
	return fmt.Sprintf("metron %s --state-dir %s", args, quoteArg(s.flagDir))
}

func quoteArg(s string) string {
	if s != "" && !strings.ContainsAny(s, " \t\"'$`\\") {
		return s
	}
	return `"` + strings.ReplaceAll(s, `"`, `\"`) + `"`
}

// world is everything one command observed, gathered once: the facts the core
// decides on, the ledger they were folded from, and what the CLI needs to
// explain them.
type world struct {
	*site
	facts  core.Facts
	state  core.State
	events []core.Event
	// ledgerMissing is set when the state directory has no ledger yet.
	ledgerMissing bool
	contract      *contractCheck
	dirty         []string
	// noCommits is set when HEAD has no commit; Facts.Candidate is then empty.
	noCommits bool
	// files caches storedFiles.
	files    []fileProblem
	filesSet bool
}

// gather observes the time, the ledger, the contract, the candidate, the
// working tree and the open packet's stored file. With commits set it also
// lists the commits since the base of an open fix packet, which only an
// implementer report needs.
func (c *ctx) gather(s *site, commits bool) (*world, error) {
	w := &world{site: s}
	w.facts.Time = c.Now().UTC().Format(store.TimeFormat)

	events, err := s.store.Load()
	switch {
	case errors.Is(err, fs.ErrNotExist):
		w.ledgerMissing = true
		w.facts.LedgerVerified = true
	case err != nil:
		w.facts.LedgerProblem = err.Error()
	default:
		st, err := core.Replay(events)
		if err != nil {
			w.facts.LedgerProblem = err.Error()
			break
		}
		w.state, w.events = st, events
		w.facts.LedgerVerified = true
	}

	w.contract = checkContract(filepath.Join(s.repo.Root(), ".metron", "contract.yaml"))
	w.facts.Contract = w.contract.contract
	w.facts.ContractProblem = w.contract.summary(s)
	w.facts.ContractDigest = w.contract.digest
	w.facts.ContractRawDigest = w.contract.rawDigest

	head, err := s.repo.Head()
	switch {
	case errors.Is(err, vcs.ErrNoCommits):
		w.noCommits = true
	case err != nil:
		return nil, err
	default:
		digest, err := s.repo.CandidateDigest(head)
		if err != nil {
			return nil, err
		}
		w.facts.Candidate = core.Candidate{Commit: head, Digest: digest}
	}

	if w.dirty, err = s.repo.DirtyPaths(); err != nil {
		return nil, err
	}
	w.facts.Clean = len(w.dirty) == 0

	if op, ok := w.state.OpenPacket(); ok {
		w.facts.PacketProblem = w.packetFile(op)
		if commits && !w.noCommits && op.Role == core.RoleImplementer && op.Base != nil {
			if w.facts.Commits, err = s.repo.CommitsSince(op.Base.Commit); err != nil {
				return nil, fmt.Errorf("list the commits since the base of packet %s: %w", op.Packet, err)
			}
		}
	}
	return w, nil
}

// packetFile says what is wrong with an issued packet's stored file, or
// returns "".
func (w *world) packetFile(p core.PacketRecord) string {
	name := store.PacketsDir + "/" + p.Packet + ".json"
	data, err := w.store.ReadPacket(p.Packet)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return fmt.Sprintf("its stored file %s is missing", name)
	case err != nil:
		return fmt.Sprintf("its stored file %s cannot be read (%v)", name, err)
	case canon.Digest(data) != p.Digest:
		return fmt.Sprintf("its stored file %s does not match the digest recorded when it was issued", name)
	}
	return ""
}

// fileProblem is a stored packet or report file that does not match the
// ledger, with the way to recover.
type fileProblem struct {
	Packet   string `json:"packet"`
	Problem  string `json:"problem"`
	Recovery string `json:"recovery"`
	// command is the command that recovers, when one does.
	command string
	// open is set for the open packet, which metron packet voids and issues
	// again.
	open bool
}

func (p fileProblem) String() string { return p.Problem + ": " + p.Recovery }

// storedFiles checks the files the ledger refers to: the file of every packet
// that is open or answered, and the file of every accepted report. A voided
// packet binds nothing, so its file is not checked: voiding is how a packet
// whose file was lost is recovered.
func (w *world) storedFiles() []fileProblem {
	if w.filesSet {
		return w.files
	}
	w.filesSet = true
	for _, p := range w.state.Packets() {
		if !p.Open && p.Report == nil {
			continue
		}
		if why := w.packetFile(p); why != "" {
			fp := fileProblem{Packet: p.Packet, Problem: fmt.Sprintf("packet %s: %s", p.Packet, why)}
			if p.Open {
				fp.open, fp.command = true, w.metron("packet")
				fp.Recovery = fmt.Sprintf("run %s to void it and issue a new packet", fp.command)
			} else {
				fp.Recovery = "restore the file from version control"
			}
			w.files = append(w.files, fp)
		}
		if p.Report == nil {
			continue
		}
		name := store.ReportsDir + "/" + p.Packet + ".json"
		data, err := w.store.ReadReport(p.Packet)
		ingest := w.metron("ingest <the report accepted for " + p.Packet + ">")
		switch {
		case errors.Is(err, fs.ErrNotExist):
			w.files = append(w.files, fileProblem{Packet: p.Packet, command: ingest,
				Problem:  fmt.Sprintf("report of packet %s: its stored file %s is missing", p.Packet, name),
				Recovery: "restore it by running " + ingest})
		case err != nil:
			w.files = append(w.files, fileProblem{Packet: p.Packet,
				Problem:  fmt.Sprintf("report of packet %s: its stored file %s cannot be read (%v)", p.Packet, name, err),
				Recovery: "make it readable"})
		case canon.Digest(data) != p.Report.RawDigest:
			w.files = append(w.files, fileProblem{Packet: p.Packet,
				Problem:  fmt.Sprintf("report of packet %s: its stored file %s does not match the raw digest recorded when it was accepted", p.Packet, name),
				Recovery: "delete the file and run " + ingest + ", or restore it from version control"})
		}
	}
	return w.files
}

// next is the single next action: the core's, except that a stored file that
// does not match the ledger is an integrity failure to repair first (the open
// packet's file excepted: the core ignores that packet and metron packet
// voids it), and that a repository without commits has no candidate.
func (w *world) next() core.Action {
	a := core.Next(w.state, w.facts)
	if a.Step == core.StepRepairLedger {
		return a
	}
	var why []string
	for _, p := range w.storedFiles() {
		if !p.open {
			why = append(why, p.String())
		}
	}
	if len(why) > 0 {
		return core.Action{Step: core.StepRepairLedger, Owner: core.RoleGovernance,
			Reason: "stored files do not match the ledger: " + strings.Join(why, "; ")}
	}
	if w.noCommits && a.Step != core.StepPinContract {
		return core.Action{Step: core.StepCommitChanges, Owner: core.RoleImplementer,
			Reason: "the repository has no commits: commit the work to review"}
	}
	return a
}

// gate is the core's gate with the stored files added to integrity: a file
// that does not match the ledger keeps the gate closed.
func (w *world) gate() core.GateResult {
	g := core.Gate(w.state, w.facts)
	files := w.storedFiles()
	for i := range g.Conditions {
		cd := &g.Conditions[i]
		if cd.Name != core.ConditionIntegrity {
			continue
		}
		cd.Description += "; every stored packet and report file matches the ledger"
		for _, p := range files {
			cd.Reasons = append(cd.Reasons, p.String())
			cd.Satisfied, g.Open = false, false
		}
	}
	return g
}

// command renders the command that performs or continues an action.
func (w *world) command(a core.Action) string {
	switch a.Step {
	case core.StepRepairLedger:
		switch {
		case w.ledgerMissing:
			return w.metron("init")
		case w.facts.LedgerVerified:
			for _, p := range w.storedFiles() {
				if !p.open && p.command != "" {
					return p.command
				}
			}
		}
		return w.metron("fsck")
	case core.StepPinContract:
		if w.facts.Contract == nil {
			return w.metron("contract")
		}
		return w.metron(`contract pin -m "<reason>"`)
	case core.StepSubmitReport:
		return w.metron("ingest <report-file>")
	case core.StepAdjudicate:
		return w.metron(fmt.Sprintf(`resolve %s -m "<decision>"`, a.Escalations[0]))
	case core.StepCommitChanges:
		return "git add -A && git commit"
	case core.StepRunChecks:
		return w.metron("check")
	case core.StepReview, core.StepFix, core.StepVerify, core.StepAudit:
		return w.metron("packet")
	}
	return w.metron("gate")
}

// staleOpenPacket returns the open packet when the core no longer considers
// it valid, so that output can say that metron packet will void it.
func (w *world) staleOpenPacket(a core.Action) (core.PacketRecord, bool) {
	op, ok := w.state.OpenPacket()
	if !ok || a.Step == core.StepSubmitReport || a.Step == core.StepRepairLedger || a.Step == core.StepPinContract {
		return core.PacketRecord{}, false
	}
	return op, true
}

// lock takes the state directory's lock and gathers the facts under it. The
// caller closes the writer after committing.
func (c *ctx) lock(s *site, commits bool) (*store.Writer, *world, error) {
	wr, err := s.store.Lock()
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil, fail(exitEnv, "metron is not initialized here: the state directory %s does not exist; run %s",
			s.show(s.store.Dir()), s.metron("init"))
	}
	if err != nil {
		return nil, nil, err
	}
	w, err := c.gather(s, commits)
	if err != nil {
		wr.Close()
		return nil, nil, err
	}
	if err := w.writable(); err != nil {
		wr.Close()
		return nil, nil, err
	}
	return wr, w, nil
}

// writable refuses to change a ledger that is missing or does not verify.
func (w *world) writable() error {
	switch {
	case w.ledgerMissing:
		return fail(exitEnv, "metron is not initialized here: %s has no ledger; run %s",
			w.show(w.store.Dir()), w.metron("init"))
	case !w.facts.LedgerVerified:
		return fail(exitEnv, "the ledger fails verification, so nothing was changed: %s\nRun %s, then restore the ledger from version control.",
			w.facts.LedgerProblem, w.metron("fsck"))
	}
	return nil
}

// candidate refuses a command that binds something to the candidate in a
// repository without commits.
func (w *world) candidate() error {
	if w.noCommits {
		return fail(exitEnv, "%v", vcs.ErrNoCommits)
	}
	return nil
}

// commit seals drafts after the ledger's last event, appends them, and folds
// them into the state.
func (w *world) commit(wr *store.Writer, drafts []core.Draft) ([]core.Event, error) {
	if len(drafts) == 0 {
		return nil, nil
	}
	var prev *core.Event
	if n := len(w.events); n > 0 {
		prev = &w.events[n-1]
	}
	events, err := store.Seal(prev, w.facts.Time, drafts)
	if err != nil {
		return nil, err
	}
	if err := wr.Append(events); err != nil {
		return nil, err
	}
	for _, e := range events {
		s, err := core.Apply(w.state, e)
		if err != nil {
			return nil, fmt.Errorf("fold the committed %s event: %w", e.Type, err)
		}
		w.state = s
	}
	w.events = append(w.events, events...)
	w.refresh()
	return events, nil
}

// refresh observes the stored files again after a commit or a file write.
func (w *world) refresh() {
	w.filesSet = false
	w.facts.PacketProblem = ""
	if op, ok := w.state.OpenPacket(); ok {
		w.facts.PacketProblem = w.packetFile(op)
	}
}
