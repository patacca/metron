package cli

import (
	"bytes"
	"strings"
	"testing"

	"github.com/patacca/metron/internal/core"
)

// TestWorkDirectory writes the skeleton and the report of a read-only role
// under .metron/work/, which keeps the tree clean through the whole review.
func TestWorkDirectory(t *testing.T) {
	r := pinned(t)
	r.write("greeting.txt", "hello\n")
	r.commit("spell it right")
	r.must("check")
	res := r.must("packet", "--format", "skeleton", "-o", ".metron/work/review.json")
	if strings.Contains(res.err, "warning") {
		t.Fatalf("packet warns about .metron/work/: %s", res.err)
	}
	var sk core.Report
	unmarshal(t, string(r.read(".metron/work/review.json")), &sk)
	r.write(".metron/work/review.json", string(review(t, sk, "rev-1", nil)))
	if status := r.git("status", "--porcelain", "--untracked-files=all"); strings.Contains(status, ".metron/work") {
		t.Fatalf("git sees the scratch space:\n%s", status)
	}
	r.expectNext(core.StepSubmitReport)
	contains(t, "ingest output", r.must("ingest", ".metron/work/review.json").out, "Accepted the reviewer report for packet P-0001")
	r.expectNext(core.StepNone)
	r.must("gate")
}

// TestInitCompletesIgnoreFiles runs init where .metron/.gitignore exists
// without the line for the scratch space.
func TestInitCompletesIgnoreFiles(t *testing.T) {
	r := newRepo(t)
	r.write(".metron/.gitignore", "# ours\n/state/lock\n/state/*.tmp")
	contains(t, "init output", r.must("init").out, "Added /work/ to .metron/.gitignore.", "Created .metron/.gitattributes.")
	if got := string(r.read(".metron/.gitignore")); got != "# ours\n/state/lock\n/state/*.tmp\n/work/\n" {
		t.Fatalf(".metron/.gitignore is %q", got)
	}
	contains(t, "second init", r.must("init").out, "Found .metron/.gitignore; left it as it is.", "Found .metron/.gitattributes; left it as it is.")
}

// TestReportStoredBeforeCommit makes storing a report fail: the ledger must
// not record a report whose bytes were not kept.
func TestReportStoredBeforeCommit(t *testing.T) {
	r := pinned(t)
	r.must("check")
	_, sk := r.packet()
	// A file where the reports directory belongs makes the write fail on
	// every platform, where permissions would not on Windows.
	r.write(".metron/state/reports", "not a directory")
	ledger := r.read(".metron/state/ledger.jsonl")
	res := r.ingest(exitEnv, review(t, sk, "rev-1", map[string]string{"REQ-2": core.RequirementViolated}, misspelled()))
	contains(t, "ingest error", res.err, "store the report before accepting it, so nothing was changed")
	if !bytes.Equal(ledger, r.read(".metron/state/ledger.jsonl")) {
		t.Fatal("the ledger recorded a report that was not stored")
	}
	r.expectNext(core.StepSubmitReport)
}
