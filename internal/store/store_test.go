package store

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/patacca/metron/internal/core"
)

// These variables make the test binary act as a second process on a state
// directory until it is killed: holding its lock, appending batches of two
// events in a loop, or replacing a report file in a loop.
const (
	lockHelperEnv    = "METRON_STORE_TEST_HOLD_LOCK"
	appendHelperEnv  = "METRON_STORE_TEST_APPEND_LOOP"
	replaceHelperEnv = "METRON_STORE_TEST_REPLACE_LOOP"
)

// Two report versions large enough that a write takes measurable time.
var (
	reportA = bytes.Repeat([]byte("a"), 1<<20)
	reportB = bytes.Repeat([]byte("b"), 1<<20+1)
)

func TestMain(m *testing.M) {
	if dir := os.Getenv(lockHelperEnv); dir != "" {
		w, err := New(dir).Lock()
		if err != nil {
			fmt.Println(err)
			os.Exit(1)
		}
		fmt.Println("locked")
		io.Copy(io.Discard, os.Stdin) // block until killed
		w.Close()
		os.Exit(0)
	}
	if dir := os.Getenv(appendHelperEnv); dir != "" {
		appendForever(dir)
	}
	if dir := os.Getenv(replaceHelperEnv); dir != "" {
		replaceForever(dir)
	}
	os.Exit(m.Run())
}

func appendForever(dir string) {
	w, err := New(dir).Lock()
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	fmt.Println("appending")
	for i := 0; ; i++ {
		events, err := w.Load()
		if err != nil && !errors.Is(err, fs.ErrNotExist) {
			fmt.Println(err)
			os.Exit(1)
		}
		var prev *core.Event
		if len(events) > 0 {
			prev = &events[len(events)-1]
		}
		void := core.PacketVoided{Packet: "P-0001", Reason: strings.Repeat("x", i%512)}
		batch, err := Seal(prev, t0, []core.Draft{{Type: core.EventPacketVoided, Data: void}, {Type: core.EventPacketVoided, Data: void}})
		if err == nil {
			err = w.Append(batch)
		}
		if err != nil {
			fmt.Println(err)
			os.Exit(1)
		}
	}
}

func replaceForever(dir string) {
	w, err := New(dir).Lock()
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	for i := 0; ; i++ {
		data := reportA
		if i%2 == 1 {
			data = reportB
		}
		if err := w.ReplaceReport("P-0001", data); err != nil {
			fmt.Println(err)
			os.Exit(1)
		}
		if i == 0 {
			fmt.Println("replacing")
		}
	}
}

// startHelper runs the test binary as a helper on dir and waits for its
// first line of output.
func startHelper(t *testing.T, env, dir, ready string) *exec.Cmd {
	t.Helper()
	cmd := exec.Command(os.Args[0])
	cmd.Env = append(os.Environ(), env+"="+dir)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	line, err := bufio.NewReader(stdout).ReadString('\n')
	if line != ready+"\n" {
		cmd.Process.Kill()
		cmd.Wait()
		t.Fatalf("helper did not start: %q, %v", line, err)
	}
	return cmd
}

func kill(t *testing.T, cmd *exec.Cmd) {
	t.Helper()
	if err := cmd.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	cmd.Wait()
}

func lockT(t *testing.T, s *Store) *Writer {
	t.Helper()
	w, err := s.Lock()
	if err != nil {
		t.Fatalf("Lock: %v", err)
	}
	t.Cleanup(func() { w.Close() })
	return w
}

func readFile(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func assertNoTemp(t *testing.T, dir string) {
	t.Helper()
	if _, err := os.Stat(filepath.Join(dir, tempName)); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("temporary file present: %v", err)
	}
}

func TestAppendAndLoad(t *testing.T) {
	s := New(t.TempDir())
	if _, err := s.Load(); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("Load of a missing ledger = %v, want fs.ErrNotExist", err)
	}
	w := lockT(t, s)
	drafts := sampleDrafts()
	first := seal(t, nil, drafts[:2]...)
	if err := w.Append(first); err != nil {
		t.Fatalf("Append: %v", err)
	}
	loaded, err := w.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	second := seal(t, &loaded[len(loaded)-1], drafts[2])
	if err := w.Append(second); err != nil {
		t.Fatalf("Append: %v", err)
	}
	if err := w.Append(nil); err != nil {
		t.Fatalf("Append of an empty batch: %v", err)
	}
	got, err := s.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if want := append(first, second...); !reflect.DeepEqual(got, want) {
		t.Fatalf("loaded events differ from appended ones")
	}
	assertNoTemp(t, s.Dir())
}

func TestAppendKeepsExistingBytes(t *testing.T) {
	s := New(t.TempDir())
	events, data := sampleLedger(t)
	crlf := bytes.ReplaceAll(data, []byte("\n"), []byte("\r\n"))
	path := filepath.Join(s.Dir(), LedgerName)
	if err := os.WriteFile(path, crlf, 0o666); err != nil {
		t.Fatal(err)
	}
	w := lockT(t, s)
	next := seal(t, &events[len(events)-1], core.Draft{Type: core.EventPacketVoided, Data: core.PacketVoided{Packet: "P-0001", Reason: "r"}})
	if err := w.Append(next); err != nil {
		t.Fatalf("Append: %v", err)
	}
	got := readFile(t, path)
	if !bytes.HasPrefix(got, crlf) || !bytes.Equal(got[len(crlf):], encode(next)) {
		t.Fatalf("existing bytes were not kept verbatim")
	}
}

// A failed Append writes nothing.
func TestAppendAllOrNothing(t *testing.T) {
	base, _ := sampleLedger(t)
	tampered := seal(t, &base[2],
		core.Draft{Type: core.EventPacketVoided, Data: core.PacketVoided{Packet: "P-0001", Reason: "a"}},
		core.Draft{Type: core.EventPacketVoided, Data: core.PacketVoided{Packet: "P-0002", Reason: "b"}})
	tampered[1].Data = []byte(`{"packet":"P-0002","reason":"c"}`)
	detached := seal(t, &base[1], core.Draft{Type: core.EventPacketVoided, Data: core.PacketVoided{Packet: "P-0001", Reason: "a"}})
	invalid := seal(t, &base[2], core.Draft{Type: core.EventPacketVoided, Data: core.PacketVoided{Packet: "P-0001", Reason: "a"}})
	invalid[0].Time = "yesterday"
	first := seal(t, &base[2], core.Draft{Type: core.EventPacketVoided, Data: core.PacketVoided{Packet: "P-0001", Reason: "a"}})
	twoAsOne := append(first, seal(t, &first[0], core.Draft{Type: core.EventPacketVoided, Data: core.PacketVoided{Packet: "P-0002", Reason: "b"}})...)
	truncated := seal(t, &base[2], sampleDrafts()...)[:2]
	tests := []struct {
		name  string
		batch []core.Event
	}{
		{"second event tampered", tampered},
		{"sealed against an older event", detached},
		{"genesis batch on a non-empty ledger", seal(t, nil, sampleDrafts()[0])},
		{"invalid member", invalid},
		{"two batches presented as one", twoAsOne},
		{"batch without its last event", truncated},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := New(t.TempDir())
			path := filepath.Join(s.Dir(), LedgerName)
			w := lockT(t, s)
			if err := w.Append(base); err != nil {
				t.Fatal(err)
			}
			before := readFile(t, path)
			if err := w.Append(tt.batch); err == nil {
				t.Fatal("Append succeeded")
			}
			if !bytes.Equal(readFile(t, path), before) {
				t.Fatal("the ledger changed")
			}
			assertNoTemp(t, s.Dir())
		})
	}
}

func TestAppendRefusesCorruptLedger(t *testing.T) {
	s := New(t.TempDir())
	events, data := sampleLedger(t)
	data[10] ^= 1
	path := filepath.Join(s.Dir(), LedgerName)
	if err := os.WriteFile(path, data, 0o666); err != nil {
		t.Fatal(err)
	}
	w := lockT(t, s)
	next := seal(t, &events[2], core.Draft{Type: core.EventPacketVoided, Data: core.PacketVoided{Packet: "P-0001", Reason: "r"}})
	err := w.Append(next)
	var le *LedgerError
	if !errors.As(err, &le) || le.Line != 1 {
		t.Fatalf("Append = %v, want a *LedgerError for line 1", err)
	}
	if !bytes.Equal(readFile(t, path), data) {
		t.Fatal("the ledger changed")
	}
}

// A crash after the temporary file is written and before it is renamed
// leaves the old ledger in force; the next writer discards the leftover.
func TestInterruptedWrite(t *testing.T) {
	s := New(t.TempDir())
	events, data := sampleLedger(t)
	path := filepath.Join(s.Dir(), LedgerName)
	if err := os.WriteFile(path, data, 0o666); err != nil {
		t.Fatal(err)
	}
	next := seal(t, &events[2], core.Draft{Type: core.EventPacketVoided, Data: core.PacketVoided{Packet: "P-0001", Reason: "r"}})
	complete := append(bytes.Clone(data), encode(next)...)
	for _, leftover := range [][]byte{complete, complete[:len(complete)-7], nil} {
		tmp := filepath.Join(s.Dir(), tempName)
		if err := os.WriteFile(tmp, leftover, 0o666); err != nil {
			t.Fatal(err)
		}
		got, err := s.Load()
		if err != nil || !reflect.DeepEqual(got, events) {
			t.Fatalf("Load with a leftover temporary file = %v, %v", got, err)
		}
		w := lockT(t, s)
		assertNoTemp(t, s.Dir())
		w.Close()
		if !bytes.Equal(readFile(t, path), data) {
			t.Fatal("the ledger changed")
		}
	}
	w := lockT(t, s)
	if err := w.Append(next); err != nil {
		t.Fatalf("Append after recovery: %v", err)
	}
	if !bytes.Equal(readFile(t, path), complete) {
		t.Fatal("unexpected ledger after recovery")
	}
}

// A writer killed at an arbitrary moment, possibly between writing the
// temporary file and renaming it, leaves a ledger that verifies and holds
// whole batches only.
func TestKilledWriter(t *testing.T) {
	s := New(t.TempDir())
	total := 0
	for round := range 8 {
		cmd := startHelper(t, appendHelperEnv, s.Dir(), "appending")
		time.Sleep(time.Duration(20+round*13) * time.Millisecond)
		kill(t, cmd)
		events, err := s.Load()
		if err != nil && !errors.Is(err, fs.ErrNotExist) {
			t.Fatalf("round %d: ledger does not verify after a kill: %v", round, err)
		}
		if len(events)%2 != 0 {
			t.Fatalf("round %d: %d events, a batch was split", round, len(events))
		}
		if len(events) < total {
			t.Fatalf("round %d: the ledger lost events", round)
		}
		total = len(events)
		w := lockT(t, s)
		assertNoTemp(t, s.Dir())
		w.Close()
	}
	if total == 0 {
		t.Fatal("the helper never committed a batch")
	}
}

func TestLockExclusive(t *testing.T) {
	s := New(t.TempDir())
	w := lockT(t, s)
	if _, err := New(s.Dir()).Lock(); !errors.Is(err, errLocked) {
		t.Fatalf("second Lock = %v, want errLocked", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("second Close: %v", err)
	}
	lockT(t, s)
}

// The lock excludes other processes and disappears when its holder dies.
func TestLockHeldByAnotherProcess(t *testing.T) {
	dir := t.TempDir()
	cmd := exec.Command(os.Args[0])
	cmd.Env = append(os.Environ(), lockHelperEnv+"="+dir)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	defer stdin.Close()
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	line, err := bufio.NewReader(stdout).ReadString('\n')
	if line != "locked\n" {
		cmd.Process.Kill()
		cmd.Wait()
		t.Fatalf("helper did not take the lock: %q, %v", line, err)
	}
	_, err = New(dir).Lock()
	if !errors.Is(err, errLocked) {
		t.Errorf("Lock while another process holds it = %v, want errLocked", err)
	}
	if err := cmd.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	cmd.Wait()
	w, err := New(dir).Lock()
	if err != nil {
		t.Fatalf("Lock after the holder died: %v", err)
	}
	w.Close()
}

func TestLockMissingDirectory(t *testing.T) {
	_, err := New(filepath.Join(t.TempDir(), "absent")).Lock()
	if !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("Lock = %v, want fs.ErrNotExist", err)
	}
}

func TestWriterClosed(t *testing.T) {
	s := New(t.TempDir())
	w := lockT(t, s)
	w.Close()
	if err := w.Append(seal(t, nil, sampleDrafts()[0])); err == nil {
		t.Error("Append after Close succeeded")
	}
	if err := w.WritePacket("P-0001", []byte("{}")); err == nil {
		t.Error("WritePacket after Close succeeded")
	}
}

func TestWriteOnce(t *testing.T) {
	type op struct {
		write func(*Writer, string, []byte) error
		read  func(*Store, string) ([]byte, error)
		sub   string
	}
	ops := map[string]op{
		"packet": {(*Writer).WritePacket, (*Store).ReadPacket, PacketsDir},
		"report": {(*Writer).WriteReport, (*Store).ReadReport, ReportsDir},
	}
	for _, name := range []string{"packet", "report"} {
		o := ops[name]
		t.Run(name, func(t *testing.T) {
			s := New(t.TempDir())
			w := lockT(t, s)
			first := []byte("{\"a\": 1}\r\n") // stored exactly, not normalized
			if err := o.write(w, "P-0001", first); err != nil {
				t.Fatalf("write: %v", err)
			}
			if err := o.write(w, "P-0001", bytes.Clone(first)); err != nil {
				t.Fatalf("writing the same bytes again: %v", err)
			}
			if err := o.write(w, "P-0001", []byte(`{"a":1}`)); err == nil {
				t.Fatal("writing different bytes succeeded")
			}
			got, err := o.read(s, "P-0001")
			if err != nil || !bytes.Equal(got, first) {
				t.Fatalf("read = %q, %v", got, err)
			}
			if !bytes.Equal(readFile(t, filepath.Join(s.Dir(), o.sub, "P-0001.json")), first) {
				t.Fatal("file content differs")
			}
			if _, err := o.read(s, "P-0002"); !errors.Is(err, fs.ErrNotExist) {
				t.Fatalf("read of a missing file = %v", err)
			}
			for _, bad := range []string{"", "P-1", "P-0000", "P-00001", "p-0001", "P-0001/../x", "../P-0001", "P-12a4", "P-0001.json", `P-0001\x`} {
				if err := o.write(w, bad, first); err == nil {
					t.Errorf("write with identifier %q succeeded", bad)
				}
				if _, err := o.read(s, bad); err == nil || errors.Is(err, fs.ErrNotExist) {
					t.Errorf("read with identifier %q = %v, want a validation error", bad, err)
				}
			}
			assertNoTemp(t, s.Dir())
		})
	}
}

func TestReplaceReport(t *testing.T) {
	s := New(t.TempDir())
	w := lockT(t, s)
	path := filepath.Join(s.Dir(), ReportsDir, "P-0001.json")
	for _, step := range []struct {
		name string
		data []byte
	}{
		{"over nothing", []byte("{\"v\": 1}\r\n")},
		{"over identical bytes", []byte("{\"v\": 1}\r\n")},
		{"over different bytes", []byte(`{"v":2}`)},
		{"with fewer bytes", []byte(`{}`)},
	} {
		if err := w.ReplaceReport("P-0001", step.data); err != nil {
			t.Fatalf("%s: ReplaceReport: %v", step.name, err)
		}
		if got := readFile(t, path); !bytes.Equal(got, step.data) {
			t.Fatalf("%s: file holds %q, want %q", step.name, got, step.data)
		}
		if got, err := s.ReadReport("P-0001"); err != nil || !bytes.Equal(got, step.data) {
			t.Fatalf("%s: ReadReport = %q, %v", step.name, got, err)
		}
	}
	// Once the report is accepted, the write-once path keeps it.
	if err := w.WriteReport("P-0001", []byte(`{}`)); err != nil {
		t.Fatalf("WriteReport of the same bytes: %v", err)
	}
	if err := w.WriteReport("P-0001", []byte(`{"v":3}`)); err == nil {
		t.Fatal("WriteReport over a different file succeeded")
	}
	for _, bad := range []string{"", "P-1", "../P-0001", "P-0001/x"} {
		if err := w.ReplaceReport(bad, []byte(`{}`)); err == nil {
			t.Errorf("ReplaceReport(%q) succeeded", bad)
		}
	}
	assertNoTemp(t, s.Dir())
	w.Close()
	if err := w.ReplaceReport("P-0001", []byte(`{}`)); err == nil {
		t.Fatal("ReplaceReport after Close succeeded")
	}
}

// An interrupted replacement leaves the old file or the new one, never a
// partial file.
func TestInterruptedReplaceReport(t *testing.T) {
	s := New(t.TempDir())
	w := lockT(t, s)
	if err := w.ReplaceReport("P-0001", reportA); err != nil {
		t.Fatal(err)
	}
	w.Close()
	// The state after a crash between writing the temporary file and
	// renaming it.
	if err := os.WriteFile(filepath.Join(s.Dir(), tempName), reportB[:1000], 0o666); err != nil {
		t.Fatal(err)
	}
	if got, err := s.ReadReport("P-0001"); err != nil || !bytes.Equal(got, reportA) {
		t.Fatalf("ReadReport after an interrupted replacement = %d bytes, %v", len(got), err)
	}
	lockT(t, s).Close()
	assertNoTemp(t, s.Dir())

	// A writer killed at arbitrary moments.
	for round := range 8 {
		cmd := startHelper(t, replaceHelperEnv, s.Dir(), "replacing")
		time.Sleep(time.Duration(10+round*7) * time.Millisecond)
		kill(t, cmd)
		got, err := s.ReadReport("P-0001")
		if err != nil {
			t.Fatalf("round %d: ReadReport: %v", round, err)
		}
		if !bytes.Equal(got, reportA) && !bytes.Equal(got, reportB) {
			t.Fatalf("round %d: the report file is partial: %d bytes", round, len(got))
		}
		lockT(t, s).Close()
		assertNoTemp(t, s.Dir())
	}
}

func TestValidPacketID(t *testing.T) {
	tests := map[string]bool{
		"P-0001": true, "P-9999": true, "P-10000": true, "P-123456789": true,
		"P-0000": false, "P-001": false, "P-01000": false, "P-": false,
		"F-0001": false, "P0001": false, "P--001": false, "P-+001": false, " P-0001": false,
	}
	for id, want := range tests {
		if got := validPacketID(id); got != want {
			t.Errorf("validPacketID(%q) = %v, want %v", id, got, want)
		}
	}
}
