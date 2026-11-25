package cli

// The integrity of the state directory: the stored packet and report files
// checked against the ledger, a ledger older than its files, a ledger lost
// beside them, and whether the ledger can be used at all.

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/patacca/metron/internal/canon"
	"github.com/patacca/metron/internal/core"
	"github.com/patacca/metron/internal/store"
)

// lostLedgerWays names the two ways out when a ledger was lost or rolled
// back while its packet or report files remain.
const lostLedgerWays = "restore the ledger from version control or a backup, or, as governance's decision, set the state directory aside to start a new history, which discards every verdict recorded so far"

// setAside is the template that sets a state directory aside and starts a
// new history. It is never a command: it is destructive, and the directory
// is the user's.
func (s *site) setAside() string {
	return "mv <state-directory> <somewhere-else> && " + s.metron("init")
}

// stranded lists the packet and report files in the state directory.
func (s *site) stranded() ([]string, error) {
	var out []string
	for _, sub := range []string{store.PacketsDir, store.ReportsDir} {
		entries, err := os.ReadDir(filepath.Join(s.store.Dir(), sub))
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err != nil {
			return nil, err
		}
		for _, e := range entries {
			if storedPacket.MatchString(e.Name()) {
				out = append(out, sub+"/"+e.Name())
			}
		}
	}
	return out, nil
}

// lostLedger says why a state directory without a usable ledger may not be
// initialized, or returns "".
func (s *site) lostLedger(stranded []string, empty bool) string {
	if len(stranded) == 0 {
		return ""
	}
	ledger := "has no ledger"
	if empty {
		ledger = "has an empty ledger"
	}
	files := strings.Join(stranded, ", ")
	if len(stranded) > 3 {
		files = strings.Join(stranded[:3], ", ") + fmt.Sprintf(" and %d more", len(stranded)-3)
	}
	return fmt.Sprintf("the state directory %s %s but holds packet or report files (%s): the ledger they belong to was lost, not never written; %s",
		s.show(s.store.Dir()), ledger, files, lostLedgerWays)
}

// storedPacket is the name of a stored packet file.
var storedPacket = regexp.MustCompile(`^P-[0-9]{4,}\.json$`)

// unissuedPackets fails the ledger when a stored packet file names a packet
// it never issued. A packet file is written after the ledger commit that
// issues it, so that can only mean the ledger was rolled back or replaced. A
// ledger rolled back together with its files is not detected.
func (w *world) unissuedPackets() error {
	entries, err := os.ReadDir(filepath.Join(w.store.Dir(), store.PacketsDir))
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	var unknown []string
	for _, e := range entries {
		if !storedPacket.MatchString(e.Name()) {
			continue
		}
		if _, ok := w.state.Packet(strings.TrimSuffix(e.Name(), ".json")); !ok {
			unknown = append(unknown, store.PacketsDir+"/"+e.Name())
		}
	}
	if len(unknown) > 0 {
		w.facts.LedgerVerified = false
		w.facts.LedgerProblem = fmt.Sprintf("the ledger is older than its stored files: it never issued the packet stored in %s, so it was rolled back or replaced; %s",
			strings.Join(unknown, ", "), lostLedgerWays)
	}
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
	// usage is the template of the command that recovers, when one does.
	usage string
	// report is set for a report file, and clear for a packet file; detail
	// is the problem without the file's owner. open is set for the file of the
	// open packet, which is operational rather than evidence: metron packet
	// voids the packet and issues another when one is next needed.
	report bool
	detail string
	open   bool
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
				fp.usage, fp.open = w.metron("packet"), true
				fp.Recovery = fmt.Sprintf("run %s to void it and issue a new packet", fp.usage)
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
		// The packet is named in prose, never inside the command.
		ingest := w.metron("ingest <report-file>")
		switch {
		case errors.Is(err, fs.ErrNotExist):
			w.files = append(w.files, fileProblem{Packet: p.Packet, usage: ingest, report: true,
				detail:   fmt.Sprintf("its stored file %s is missing", name),
				Recovery: "restore it from version control, or by running " + ingest + " with the identical bytes of the report accepted for packet " + p.Packet})
		case err != nil:
			w.files = append(w.files, fileProblem{Packet: p.Packet, report: true,
				detail:   fmt.Sprintf("its stored file %s cannot be read (%v)", name, err),
				Recovery: "make it readable"})
		case canon.Digest(data) != p.Report.RawDigest:
			w.files = append(w.files, fileProblem{Packet: p.Packet, usage: ingest, report: true,
				detail:   fmt.Sprintf("its stored file %s does not match the raw digest recorded when it was accepted", name),
				Recovery: "restore it from version control, or delete it and run " + ingest + " with the identical bytes of the report accepted for packet " + p.Packet})
		}
	}
	for i := range w.files {
		w.files[i] = w.files[i].withProblem()
	}
	return w.files
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
	case len(w.stranded) > 0:
		return w.lostLedger(w.stranded, w.ledgerEmpty), w.metron("fsck")
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

// restoreLedger adds the advice to restore the ledger from version control to
// a verification problem that does not already give it.
func restoreLedger(problem string) string {
	if strings.Contains(problem, "restore the ledger from version control") || strings.Contains(problem, lostLedgerWays) {
		return problem
	}
	return problem + ": restore the ledger from version control"
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
