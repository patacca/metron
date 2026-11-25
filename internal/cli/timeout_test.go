package cli

import (
	"bytes"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/patacca/metron/internal/canon"
	"github.com/patacca/metron/internal/core"
	"github.com/patacca/metron/internal/store"
)

// stopsWithin bounds how long metron check may take when its one check is
// stopped at a limit of a few seconds at most: the limit, waitDelay for an
// output that a leftover process holds open, and a wide margin. Every command
// that times out sleeps for longer than this, so that a check over in time
// was stopped.
const stopsWithin = 20 * time.Second

// timedCheck runs metron check, which must exit 1 within stopsWithin.
func (r *repo) timedCheck(args ...string) result {
	r.t.Helper()
	start := time.Now()
	res := r.expect(exitNegative, append([]string{"check"}, args...)...)
	if took := time.Since(start); took > stopsWithin {
		r.t.Fatalf("metron check %s took %s, more than %s", strings.Join(args, " "), took, stopsWithin)
	}
	return res
}

// recorded returns the check results in the ledger, oldest first.
func (r *repo) recorded() []core.Evidence {
	r.t.Helper()
	events, err := store.New(r.path(".metron/state")).Load()
	if err != nil {
		r.t.Fatal(err)
	}
	var out []core.Evidence
	for _, e := range events {
		if e.Type == core.EventEvidenceRecorded {
			var ev core.Evidence
			unmarshal(r.t, string(e.Data), &ev)
			out = append(out, ev)
		}
	}
	return out
}

// TestCheckWithinLimit runs a passing check under the default limit: what it
// prints is what check printed before it had a limit, byte for byte.
func TestCheckWithinLimit(t *testing.T) {
	r := pinned(t)
	golden(t, "pinned.check.txt", r.must("check").out)
	golden(t, "pinned.check.json", r.must("check", "--json").out)
}

func TestCheckTimeout(t *testing.T) {
	r := pinnedWith(t, "sleep 30")
	contains(t, "check output", r.timedCheck("--timeout", "300ms").out,
		"REQ-1    FAILED  exit 124 [timed out: stopped after the 300ms limit] sleep 30\n", "Recorded 1 result")
	out := r.timedCheck("--timeout", "300ms", "--json").out
	contains(t, "check document", out, `"timed_out": true`)
	var d checkDoc
	unmarshal(t, out, &d)
	if got := d.Results; len(got) != 1 || !got[0].TimedOut || got[0].ExitCode != timeoutExit || got[0].Passed || d.Passed || !d.Recorded {
		t.Fatalf("check document %+v", d)
	}
	ev := r.recorded()
	if len(ev) != 2 {
		t.Fatalf("the ledger holds %d results, want 2: %+v", len(ev), ev)
	}
	for _, e := range ev {
		if e.Requirement != "REQ-1" || e.ExitCode != timeoutExit || e.Passed {
			t.Errorf("recorded %+v", e)
		}
	}
	contains(t, "next reason", r.expectNext(core.StepRunChecks).Reason, "REQ-1 failed on this candidate (exit 124)")
}

// TestCheckTimeoutThenPass gives two requirements a check each: the first
// times out after printing a line, the second passes under the same limit.
// The limit leaves the shell seconds to start, print and finish.
func TestCheckTimeoutThenPass(t *testing.T) {
	r := newRepo(t)
	r.must("init")
	contract := strings.Replace(testContract, `"git ls-files --error-unmatch greeting.txt"`, `"echo started; sleep 30"`, 1)
	contract = strings.Replace(contract, `criterion: "greeting.txt reads hello"`,
		`criterion: "greeting.txt reads hello"`+"\n    command: \"git ls-files --error-unmatch greeting.txt\"", 1)
	r.write(".metron/contract.yaml", contract)
	r.must("contract", "pin", "-m", "first contract")
	r.commit("pin the contract")

	contains(t, "check output", r.timedCheck("--timeout", "3s").out,
		"REQ-1    FAILED  exit 124 [timed out: stopped after the 3s limit] echo started; sleep 30\n         | started\n",
		"REQ-2    passed  exit 0   git ls-files --error-unmatch greeting.txt\n", "Recorded 2 results")
	ev := r.recorded()
	if len(ev) != 2 {
		t.Fatalf("the ledger holds %d results, want 2: %+v", len(ev), ev)
	}
	if e := ev[0]; e.Requirement != "REQ-1" || e.ExitCode != timeoutExit || e.Passed || e.OutputDigest != canon.Digest([]byte("started\n")) {
		t.Errorf("recorded %+v for the check that timed out", e)
	}
	if e := ev[1]; e.Requirement != "REQ-2" || e.ExitCode != 0 || !e.Passed {
		t.Errorf("recorded %+v for the check that passed", e)
	}
}

// TestCheckTimeoutLeftover times out a check whose shell left a process in
// the background that holds its output open.
func TestCheckTimeoutLeftover(t *testing.T) {
	r := pinnedWith(t, "sleep 30 & sleep 30")
	var d checkDoc
	unmarshal(t, r.timedCheck("--timeout", "300ms", "--json").out, &d)
	if got := d.Results; len(got) != 1 || !got[0].TimedOut || got[0].ExitCode != timeoutExit || !d.Recorded {
		t.Fatalf("check document %+v", d)
	}
}

func TestCheckTimeoutUsage(t *testing.T) {
	r := pinned(t)
	before := r.read(".metron/state/ledger.jsonl")
	for _, args := range [][]string{
		{"--timeout", "0"}, {"--timeout", "0s"}, {"--timeout", "-5s"}, {"--timeout=-5s"}, {"--timeout", "nonsense"}, {"--timeout", ""},
	} {
		res := r.expect(exitUsage, append([]string{"check"}, args...)...)
		value := strings.TrimPrefix(args[len(args)-1], "--timeout=")
		contains(t, "metron check "+strings.Join(args, " "), res.err,
			fmt.Sprintf("invalid --timeout %q: give a positive duration, such as 90s or 10m", value), "Run 'metron help check' for usage.")
		if res.out != "" {
			t.Errorf("metron check %s printed %q", strings.Join(args, " "), res.out)
		}
	}
	if !bytes.Equal(before, r.read(".metron/state/ledger.jsonl")) {
		t.Fatal("a usage error recorded something")
	}
}
