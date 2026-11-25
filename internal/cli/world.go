package cli

import (
	"encoding/json"
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
	// inRepo is set when the state directory lies under .metron/, where it
	// is committed together with the work.
	inRepo bool
}

// open locates the repository that contains the working directory and the
// state directory: --state-dir, else $METRON_STATE_DIR, else .metron/state
// under the repository root. A state directory elsewhere in the working tree
// is refused: every write to it would make the tree unclean.
func (c *ctx) open(flagDir string) (*site, error) {
	repo, err := vcs.Open(c.Dir)
	if err != nil {
		return nil, fail(exitEnv, "%v\nmetron works inside a git repository: run it there, or create one with git init", err)
	}
	s := &site{repo: repo, flagDir: flagDir}
	dir, from, status := flagDir, "--state-dir", exitUsage
	if dir == "" {
		dir, from, status = c.Getenv(envStateDir), "$"+envStateDir, exitEnv
	}
	if dir == "" {
		s.store, s.inRepo = store.New(filepath.Join(repo.Root(), ".metron", "state")), true
		return s, nil
	}
	if !filepath.IsAbs(dir) {
		dir = filepath.Join(c.Dir, dir)
	}
	dir = filepath.Clean(dir)
	if rel, err := filepath.Rel(realPath(repo.Root()), realPath(dir)); err == nil && !filepath.IsAbs(rel) &&
		rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		if rel = filepath.ToSlash(rel); rel != ".metron" && !strings.HasPrefix(rel, ".metron/") {
			return nil, fail(status, "the state directory %s, set by %s, lies inside the working tree but outside .metron/, so every write to it would make the tree unclean: use a directory outside the repository, or leave the default .metron/state",
				dir, from)
		}
		s.inRepo = true
	}
	s.store = store.New(dir)
	return s, nil
}

// realPath resolves the symbolic links in the longest prefix of path that
// exists, so that paths can be compared with the repository root git reports.
func realPath(path string) string {
	rest := ""
	for p := path; ; {
		if r, err := filepath.EvalSymlinks(p); err == nil {
			return filepath.Join(r, rest)
		}
		parent := filepath.Dir(p)
		if parent == p {
			return path
		}
		rest = filepath.Join(filepath.Base(p), rest)
		p = parent
	}
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
	// ledgerMissing is set when the state directory has no ledger yet, or an
	// empty one: either way it is not initialized. ledgerEmpty tells which.
	ledgerMissing, ledgerEmpty bool
	contract                   *contractCheck
	// dirty lists why the working tree is not clean.
	dirty []vcs.TreeProblem
	// noCommits is set when HEAD has no commit; Facts.Candidate is then empty.
	noCommits bool
	// files caches storedFiles.
	files    []fileProblem
	filesSet bool
}

// gather observes the time, the ledger, the contract, the candidate, the
// working tree and the open packet's stored file. The commits an implementer
// report lists are described separately, by describe.
func (c *ctx) gather(s *site) (*world, error) {
	w := &world{site: s}
	w.facts.Time = c.Now().UTC().Format(store.TimeFormat)

	events, err := s.store.Load()
	switch {
	case errors.Is(err, fs.ErrNotExist):
		w.ledgerMissing = true
		w.facts.LedgerVerified = true
	case err != nil:
		w.facts.LedgerProblem = err.Error()
	case len(events) == 0:
		w.ledgerMissing, w.ledgerEmpty = true, true
		w.facts.LedgerVerified = true
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

	if w.dirty, err = s.repo.TreeProblems(); err != nil {
		return nil, err
	}
	w.facts.Clean = len(w.dirty) == 0

	if op, ok := w.state.OpenPacket(); ok {
		w.facts.PacketProblem = w.packetFile(op)
	}
	return w, nil
}

// describe observes the commits that the fixes of a report list, relative to
// HEAD and to the base of the report's packet. A report that lists none, or
// whose packet has no base, needs no description: the core judges it without.
func (w *world) describe(r core.Report) error {
	var body struct {
		Fixes []struct {
			Commits []string `json:"commits"`
		} `json:"fixes"`
	}
	if json.Unmarshal(r.Body, &body) != nil {
		return nil
	}
	var hashes []string
	for _, f := range body.Fixes {
		hashes = append(hashes, f.Commits...)
	}
	rec, ok := w.state.Packet(r.Packet)
	if len(hashes) == 0 || !ok || rec.Base == nil {
		return nil
	}
	commits, err := w.repo.Describe(rec.Base.Commit, hashes)
	if err != nil {
		return fmt.Errorf("describe the commits the report lists: %w", err)
	}
	w.facts.Commits = commits
	return nil
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
	// report is set for a report file, and clear for a packet file; detail
	// is the problem without the file's owner.
	report bool
	detail string
}

func (p fileProblem) String() string { return p.Problem + ": " + p.Recovery }

// withProblem fills in Problem from the file's owner and the detail.
func (p fileProblem) withProblem() fileProblem {
	owner := "packet " + p.Packet
	if p.report {
		owner = "report of packet " + p.Packet
	}
	p.Problem = owner + ": " + p.detail
	return p
}

// storedFiles checks the files the ledger refers to: the file of every packet
// that is open or answered, and the file of every accepted report. A voided
// packet binds nothing, so its file is not checked: voiding is how a packet
// whose file was lost is recovered. A report file whose packet has no
// accepted report is not the ledger's, and is ignored.
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
			fp := fileProblem{Packet: p.Packet, detail: why}
			if p.Open {
				fp.command = w.metron("packet")
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
			w.files = append(w.files, fileProblem{Packet: p.Packet, command: ingest, report: true,
				detail:   fmt.Sprintf("its stored file %s is missing", name),
				Recovery: "restore it from version control, or by running " + ingest + " with the identical bytes"})
		case err != nil:
			w.files = append(w.files, fileProblem{Packet: p.Packet, report: true,
				detail:   fmt.Sprintf("its stored file %s cannot be read (%v)", name, err),
				Recovery: "make it readable"})
		case canon.Digest(data) != p.Report.RawDigest:
			w.files = append(w.files, fileProblem{Packet: p.Packet, command: ingest, report: true,
				detail:   fmt.Sprintf("its stored file %s does not match the raw digest recorded when it was accepted", name),
				Recovery: "restore it from version control, or delete it and run " + ingest + " with the identical bytes"})
		}
	}
	for i := range w.files {
		w.files[i] = w.files[i].withProblem()
	}
	return w.files
}

// next is the single next action: the core's, except that a repository
// without commits has no candidate, and that a gate the core finds open still
// needs the report carrying its verdict stored intact.
func (w *world) next() core.Action {
	a := core.Next(w.state, w.facts)
	switch {
	case a.Step == core.StepRepairLedger:
		return a
	case w.noCommits && a.Step != core.StepPinContract:
		return core.Action{Step: core.StepCommitChanges, Owner: core.RoleImplementer,
			Reason: "the repository has no commits: commit the work to review"}
	case a.Step == core.StepNone:
		if p, r, lost := w.verdictReport(); lost {
			return core.Action{Step: core.StepRepairLedger, Owner: core.RoleGovernance,
				Reason: fmt.Sprintf("the gate relies on the %s verdict of round %d, carried by the report of packet %s, and %s: restore the file from version control, ingest the identical bytes again with %s, or change the candidate so that a new review is held",
					r.Verdict, r.Number, p.Packet, p.detail, p.command)}
		}
	}
	return a
}

// gate is the core's gate with one stored file added to integrity: the
// report carrying the verdict it relies on must be stored intact. Other
// stored-file problems are for fsck and status; they do not close it.
func (w *world) gate() core.GateResult {
	g := core.Gate(w.state, w.facts)
	p, _, lost := w.verdictReport()
	for i := range g.Conditions {
		cd := &g.Conditions[i]
		if cd.Name != core.ConditionIntegrity {
			continue
		}
		cd.Description += "; the report carrying the verdict is stored intact"
		if lost {
			cd.Reasons = append(cd.Reasons, p.String())
			cd.Satisfied, g.Open = false, false
		}
	}
	return g
}

// verdictReport returns the problem with the stored file of the reviewer
// report of the latest round of the current stage that has a verdict, and
// that round, if its file is not intact.
func (w *world) verdictReport() (fileProblem, core.Round, bool) {
	rounds := w.state.StageRounds()
	for i := len(rounds) - 1; i >= 0; i-- {
		if rounds[i].Verdict == "" {
			continue
		}
		for _, p := range w.storedFiles() {
			if p.report && p.Packet == rounds[i].Packet {
				return p, rounds[i], true
			}
		}
		break
	}
	return fileProblem{}, core.Round{}, false
}

// ledgerUnusable says why the ledger cannot be evaluated at all, with the
// command that starts the recovery, or returns "".
func (w *world) ledgerUnusable() (string, string) {
	switch {
	case w.ledgerEmpty:
		return fmt.Sprintf("metron is not initialized here: the ledger in %s is empty", w.show(w.store.Dir())), w.metron("init")
	case w.ledgerMissing:
		return fmt.Sprintf("metron is not initialized here: %s has no ledger", w.show(w.store.Dir())), w.metron("init")
	case !w.facts.LedgerVerified:
		return "the ledger fails verification: " + w.facts.LedgerProblem, w.metron("fsck")
	case w.state.Protocol() != core.ProtocolVersion:
		return fmt.Sprintf("the ledger was written under protocol %q, which this metron does not implement (%s)", w.state.Protocol(), core.ProtocolVersion), w.metron("version")
	}
	return "", ""
}

// command renders the command that performs or continues an action.
func (w *world) command(a core.Action) string {
	switch a.Step {
	case core.StepRepairLedger:
		switch {
		case w.ledgerMissing:
			return w.metron("init")
		case w.facts.LedgerVerified:
			if p, _, lost := w.verdictReport(); lost && p.command != "" {
				return p.command
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
		return w.commitCommand()
	case core.StepRunChecks:
		return w.metron("check")
	case core.StepReview, core.StepFix, core.StepVerify, core.StepAudit:
		return w.metron("packet")
	}
	return w.metron("gate")
}

// commitCommand is the command that makes the working tree clean: none when
// it is clean already and only a new commit of the candidate helps, or when
// a symbolic link must be replaced by hand.
func (w *world) commitCommand() string {
	if w.facts.Clean && !w.noCommits {
		return ""
	}
	var hidden []string
	link := false
	for _, p := range w.dirty {
		switch p.Kind {
		case vcs.TreeHidden:
			hidden = append(hidden, quoteArg(p.Path))
		case vcs.TreeLink:
			link = true
		}
	}
	switch {
	case len(hidden) > 0:
		paths := strings.Join(hidden, " ")
		return fmt.Sprintf("git update-index --no-skip-worktree -- %s && git update-index --no-assume-unchanged -- %s", paths, paths)
	case link:
		return ""
	}
	return "git add -A && git commit"
}

// staleOpenPacket returns the open packet and why the core no longer
// considers it valid, so that output can say that metron packet voids it.
func (w *world) staleOpenPacket(a core.Action) (core.PacketRecord, string, bool) {
	op, ok := w.state.OpenPacket()
	if !ok || a.Step == core.StepSubmitReport || a.Step == core.StepRepairLedger || a.Step == core.StepPinContract {
		return core.PacketRecord{}, "", false
	}
	return op, core.PacketInvalid(w.state, w.facts), true
}

// lock takes the state directory's lock and gathers the facts under it. The
// caller closes the writer after committing.
func (c *ctx) lock(s *site) (*store.Writer, *world, error) {
	wr, err := s.store.Lock()
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil, fail(exitEnv, "metron is not initialized here: the state directory %s does not exist; run %s",
			s.show(s.store.Dir()), s.metron("init"))
	}
	if err != nil {
		return nil, nil, err
	}
	w, err := c.gather(s)
	if err != nil {
		wr.Close()
		return nil, nil, err
	}
	if err := w.usable(true); err != nil {
		wr.Close()
		return nil, nil, err
	}
	return wr, w, nil
}

// usable refuses a command that needs a ledger when there is none, or when
// it cannot be evaluated. writing says the command came to change it.
func (w *world) usable(writing bool) error {
	why, run := w.ledgerUnusable()
	if why == "" {
		return nil
	}
	if writing && !w.facts.LedgerVerified {
		why += ", so nothing was changed"
	}
	return fail(exitEnv, "%s\nRun %s.", why, run)
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
	// Fold first: an event the state machine would not replay must never
	// reach the ledger, or every later command would fail to load it.
	folded := w.state
	for _, e := range events {
		if folded, err = core.Apply(folded, e); err != nil {
			return nil, fmt.Errorf("the %s event the core decided on does not fold, so nothing was written: %w", e.Type, err)
		}
	}
	if err := wr.Append(events); err != nil {
		return nil, err
	}
	w.state = folded
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
