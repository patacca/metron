package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/patacca/metron/internal/core"
)

// TestLostLedger loses the ledger of a state directory that holds packets and
// reports: init refuses, next names the two ways out, and the gate cannot be
// evaluated. Setting the directory aside starts a new history.
func TestLostLedger(t *testing.T) {
	for _, empty := range []bool{false, true} {
		t.Run(map[bool]string{false: "missing", true: "empty"}[empty], func(t *testing.T) {
			r := green(t)
			ledger := r.path(".metron/state/ledger.jsonl")
			if empty {
				r.write(".metron/state/ledger.jsonl", "")
			} else if err := os.Remove(ledger); err != nil {
				t.Fatal(err)
			}
			ways := []string{"packets/P-0001.json", "reports/P-0001.json", "the ledger they belong to was lost, not never written",
				"restore the ledger from version control or a backup", "set the state directory aside to start a new history, which discards every verdict recorded so far"}
			setAside := "mv <state-directory> <somewhere-else> && metron init"

			res := r.expect(exitNegative, "init")
			contains(t, "init error", res.err, append(ways, "Usage: "+setAside)...)
			if _, err := os.Stat(ledger); empty == os.IsNotExist(err) {
				t.Fatalf("init touched the ledger: %v", err)
			}
			if data, _ := os.ReadFile(ledger); len(data) != 0 {
				t.Fatal("init wrote a ledger beside the files of a lost one")
			}

			a := r.expectNext(core.StepRepairLedger)
			contains(t, "next reason", a.Reason, ways...)
			if a.Command != "" || a.Usage != setAside || a.StateDir != ".metron/state" {
				t.Fatalf("next %+v", a)
			}
			contains(t, "next", r.must("next").out, "Usage:        "+setAside)
			contains(t, "gate", r.expect(exitEnv, "gate").err, "the gate cannot be evaluated", "lost, not never written")
			contains(t, "fsck", r.expect(exitNegative, "fsck").out, ways...)
			contains(t, "packet", r.expect(exitEnv, "packet").err, "lost, not never written")
			var st statusDoc
			unmarshal(t, r.must("status", "--json").out, &st)
			if !strings.Contains(st.Ledger.Problem, "lost, not never written") {
				t.Fatalf("status ledger %+v", st.Ledger)
			}

			// Governance sets the old history aside and starts a new one.
			if err := os.Rename(r.path(".metron/state"), filepath.Join(t.TempDir(), "old-state")); err != nil {
				t.Fatal(err)
			}
			contains(t, "init", r.must("init").out, "Created .metron/state/ledger.jsonl")
			r.expectNext(core.StepPinContract)
			r.must("contract", "pin", "-m", "a new history")
			r.must("check")
			r.expectNext(core.StepReview)
		})
	}
}

// TestInitEmptyStateDirectory initializes a state directory that exists but
// holds nothing of a ledger.
func TestInitEmptyStateDirectory(t *testing.T) {
	r := newRepo(t)
	for _, d := range []string{".metron/state/packets", ".metron/state/reports"} {
		if err := os.MkdirAll(r.path(d), 0o777); err != nil {
			t.Fatal(err)
		}
	}
	r.write(".metron/state/reports/notes.txt", "not a report\n")
	contains(t, "init", r.must("init").out, "Created .metron/state/ledger.jsonl")
	r.expectNext(core.StepPinContract)
}
