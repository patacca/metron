// Package store keeps Metron's state directory: the hash-chained ledger, the
// lock that serializes writing commands, and the write-once packet and report
// files. It also reads the acceptance contract strictly.
//
// docs/architecture.md, sections 4, 6 and 7, is the specification.
package store

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/patacca/metron/internal/core"
)

// Names inside the state directory.
const (
	LedgerName  = "ledger.jsonl"
	LockName    = "lock"
	PacketsDir  = "packets"
	ReportsDir  = "reports"
	tempName    = "write.tmp"
	fileMode    = 0o666
	dirMode     = 0o777
	fileNameExt = ".json"
)

// ErrLocked is returned by Lock when another process holds the lock.
var ErrLocked = errors.New("another metron process is running")

// errLockHeld is what the platform lock functions return when the lock is
// held elsewhere.
var errLockHeld = errors.New("lock held")

// Store is a state directory. Reading needs no lock; writing goes through a
// Writer, which holds it.
type Store struct {
	dir string
}

// New returns the store for a state directory. It performs no I/O.
func New(dir string) *Store {
	return &Store{dir: dir}
}

// Dir returns the state directory.
func (s *Store) Dir() string { return s.dir }

// Load reads and verifies the ledger. A missing ledger is an error that
// wraps fs.ErrNotExist.
func (s *Store) Load() ([]core.Event, error) {
	data, err := os.ReadFile(filepath.Join(s.dir, LedgerName))
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, fmt.Errorf("no ledger in %s; run metron init: %w", s.dir, err)
		}
		return nil, fmt.Errorf("read ledger: %w", err)
	}
	events, err := VerifyLedger(data)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", filepath.Join(s.dir, LedgerName), err)
	}
	return events, nil
}

// ReadPacket returns the stored packet with the given identifier.
func (s *Store) ReadPacket(id string) ([]byte, error) {
	return s.readOnce(PacketsDir, id)
}

// ReadReport returns the report accepted for the given packet, exactly as it
// was submitted.
func (s *Store) ReadReport(packet string) ([]byte, error) {
	return s.readOnce(ReportsDir, packet)
}

func (s *Store) readOnce(sub, id string) ([]byte, error) {
	if !validPacketID(id) {
		return nil, fmt.Errorf("invalid packet identifier %q: expected P- followed by at least four digits", id)
	}
	return os.ReadFile(filepath.Join(s.dir, sub, id+fileNameExt))
}

// Writer holds the exclusive lock on a state directory. Every command that
// writes acquires it before reading the ledger and closes it after
// committing. The operating system releases it if the process dies.
type Writer struct {
	*Store
	lock *os.File
}

// Lock acquires the state directory's lock without waiting. If another
// process holds it, the error wraps ErrLocked. The state directory must
// exist. Acquiring the lock discards a temporary file left by an interrupted
// write.
func (s *Store) Lock() (*Writer, error) {
	path := filepath.Join(s.dir, LockName)
	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE, fileMode)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, fmt.Errorf("state directory %s does not exist; run metron init: %w", s.dir, err)
		}
		return nil, fmt.Errorf("open lock file: %w", err)
	}
	if err := lockFile(f); err != nil {
		f.Close()
		if errors.Is(err, errLockHeld) {
			return nil, fmt.Errorf("%s is locked: %w; wait for it to finish and retry", path, ErrLocked)
		}
		return nil, fmt.Errorf("lock %s: %w", path, err)
	}
	w := &Writer{Store: s, lock: f}
	if err := os.Remove(filepath.Join(s.dir, tempName)); err != nil && !errors.Is(err, fs.ErrNotExist) {
		w.Close()
		return nil, fmt.Errorf("discard leftover temporary file: %w", err)
	}
	return w, nil
}

// Close releases the lock. Calling it again does nothing.
func (w *Writer) Close() error {
	if w.lock == nil {
		return nil
	}
	f := w.lock
	w.lock = nil
	err := unlockFile(f)
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	return err
}

func (w *Writer) held() error {
	if w.lock == nil {
		return errors.New("store: the lock has been released")
	}
	return nil
}

// Append commits sealed events at the end of the ledger, all or none. The
// events must extend the ledger as it is on disk; see Seal. The complete new
// ledger is written to a temporary file, synced and renamed over the old one.
// A missing ledger is treated as empty.
func (w *Writer) Append(batch []core.Event) error {
	if err := w.held(); err != nil {
		return err
	}
	if len(batch) == 0 {
		return nil
	}
	path := filepath.Join(w.dir, LedgerName)
	old, err := os.ReadFile(path)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("read ledger: %w", err)
	}
	events, err := VerifyLedger(old)
	if err != nil {
		return fmt.Errorf("%s does not verify, nothing was written: %w", path, err)
	}
	content := bytes.Clone(old)
	for _, e := range batch {
		if err := checkEvent(e); err != nil {
			return fmt.Errorf("event %d: %w; nothing was written", e.Seq, err)
		}
		content = append(appendLine(content, e), '\n')
	}
	if _, err := VerifyLedger(content); err != nil {
		return fmt.Errorf("the events do not extend the ledger of %d events (seal them against its last event), nothing was written: %w", len(events), err)
	}
	return w.writeAtomic(path, content)
}

// WritePacket stores an issued packet under its identifier. Packets are
// immutable: writing the same bytes again succeeds, different bytes are an
// error.
func (w *Writer) WritePacket(id string, data []byte) error {
	return w.writeOnce(PacketsDir, "packet", id, data)
}

// WriteReport stores an accepted report, exactly as submitted, under the
// identifier of the packet it answers. Reports are immutable like packets.
func (w *Writer) WriteReport(packet string, data []byte) error {
	return w.writeOnce(ReportsDir, "report for packet", packet, data)
}

func (w *Writer) writeOnce(sub, what, id string, data []byte) error {
	if err := w.held(); err != nil {
		return err
	}
	if !validPacketID(id) {
		return fmt.Errorf("invalid packet identifier %q: expected P- followed by at least four digits", id)
	}
	dir := filepath.Join(w.dir, sub)
	if err := os.MkdirAll(dir, dirMode); err != nil {
		return err
	}
	path := filepath.Join(dir, id+fileNameExt)
	existing, err := os.ReadFile(path)
	switch {
	case err == nil && bytes.Equal(existing, data):
		return nil
	case err == nil:
		return fmt.Errorf("%s %s is already stored with different content at %s; stored files are immutable", what, id, path)
	case !errors.Is(err, fs.ErrNotExist):
		return err
	}
	return w.writeAtomic(path, data)
}

// writeAtomic replaces path with data through a synced temporary file in the
// state directory, then syncs the directory that holds path.
func (w *Writer) writeAtomic(path string, data []byte) error {
	tmp := filepath.Join(w.dir, tempName)
	f, err := os.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, fileMode)
	if err != nil {
		return err
	}
	_, err = f.Write(data)
	if err == nil {
		err = f.Sync()
	}
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err == nil {
		err = os.Rename(tmp, path)
	}
	if err != nil {
		os.Remove(tmp)
		return fmt.Errorf("write %s: %w", path, err)
	}
	if err := syncDir(filepath.Dir(path)); err != nil {
		return fmt.Errorf("sync %s: %w", filepath.Dir(path), err)
	}
	return nil
}

// validPacketID reports whether id is "P-" followed by a sequence number
// zero-padded to four digits, with no further leading zeros, other than zero.
// Only such identifiers become file names.
func validPacketID(id string) bool {
	digits, ok := strings.CutPrefix(id, "P-")
	if !ok || len(digits) < 4 || (len(digits) > 4 && digits[0] == '0') {
		return false
	}
	if strings.Trim(digits, "0123456789") != "" {
		return false
	}
	return strings.Trim(digits, "0") != ""
}
