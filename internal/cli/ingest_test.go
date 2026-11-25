package cli

import (
	"bytes"
	"os"
	"strings"
	"testing"

	"github.com/patacca/metron/internal/core"
)

func TestIngestRejections(t *testing.T) {
	for _, tc := range []struct {
		name   string
		report func(t *testing.T, sk core.Report) []byte
		layer  core.Layer
		want   string
	}{
		{"malformed json", func(*testing.T, core.Report) []byte { return []byte(`{"schema": `) }, reportStructural, "not valid JSON"},
		{"duplicate keys", func(t *testing.T, sk core.Report) []byte {
			return []byte(`{"schema":"metron.report/v1","schema":"metron.report/v1"}`)
		}, reportStructural, `duplicate member "schema"`},
		{"structural", func(t *testing.T, sk core.Report) []byte { return marshal(t, sk) }, reportStructural, "/author/id: must be 1 to 128 printable ASCII characters"},
		{"semantic", func(t *testing.T, sk core.Report) []byte {
			return review(t, sk, "rev-1", map[string]string{"REQ-2": core.RequirementViolated})
		}, core.LayerSemantic, "/body/requirements/1/status: REQ-2 is violated but no blocking finding"},
		{"transition", func(t *testing.T, sk core.Report) []byte {
			sk.Packet = "P-0009"
			return review(t, sk, "rev-1", nil)
		}, core.LayerTransition, `/packet: packet "P-0009" was never issued`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := pinned(t)
			r.must("check")
			_, sk := r.packet()
			before := r.read(".metron/state/ledger.jsonl")
			report := tc.report(t, sk)
			res := r.ingest(exitNegative, report)
			contains(t, "ingest error", res.err, "rejected by the "+string(tc.layer)+" layer; nothing was changed", tc.want)
			var d ingestDoc
			unmarshal(t, r.ingest(exitNegative, report, "--json").out, &d)
			if d.Schema != "metron.ingest/v1" || d.Accepted || d.Rejection == nil || d.Rejection.Layer != tc.layer {
				t.Fatalf("ingest document %+v", d)
			}
			if !bytes.Equal(before, r.read(".metron/state/ledger.jsonl")) {
				t.Fatal("a rejected report changed the ledger")
			}
			if _, err := os.Stat(r.path(".metron/state/reports/P-0001.json")); !os.IsNotExist(err) {
				t.Fatalf("a rejected report was stored: %v", err)
			}
			r.expectNext(core.StepSubmitReport)
		})
	}
}

func TestIngestAgainAndRestore(t *testing.T) {
	r := pinned(t)
	r.write("greeting.txt", "hello\n")
	r.commit("spell it right")
	r.must("check")
	_, sk := r.packet()
	report := review(t, sk, "rev-1", nil)
	r.stdin = string(report)
	var d ingestDoc
	unmarshal(t, r.must("ingest", "-", "--json").out, &d)
	if !d.Accepted || d.Duplicate || d.Verdict != core.VerdictGreen || d.Next.Step != core.StepNone || len(d.Events) != 2 {
		t.Fatalf("ingest document %+v", d)
	}
	ledger := r.read(".metron/state/ledger.jsonl")
	stored := ".metron/state/reports/P-0001.json"
	if !bytes.Equal(r.read(stored), report) {
		t.Fatal("the stored report differs from the bytes submitted")
	}

	// The same report, or the same content formatted differently, is
	// accepted again without any change.
	reformatted := bytes.ReplaceAll(report, []byte("\n  "), []byte("\n    "))
	for _, again := range [][]byte{report, reformatted} {
		contains(t, "ingest again", r.ingest(exitOK, again).out, "already accepted; nothing changed")
	}
	if !bytes.Equal(ledger, r.read(".metron/state/ledger.jsonl")) {
		t.Fatal("ingesting the same report again changed the ledger")
	}

	// Losing the report that carries the verdict closes the gate until the
	// identical bytes restore it.
	if err := os.Remove(r.path(stored)); err != nil {
		t.Fatal(err)
	}
	contains(t, "fsck", r.expect(exitNegative, "fsck").out, "reports/P-0001.json is missing", "metron ingest <report-file> with the identical bytes of the report accepted for packet P-0001")
	contains(t, "gate", r.expect(exitNegative, "gate").out, "[FAIL] I", "reports/P-0001.json is missing")
	a := r.expectNext(core.StepRepairLedger)
	contains(t, "next reason", a.Reason, "the gate relies on the GREEN verdict of round 1, carried by the report of packet P-0001, and its stored file reports/P-0001.json is missing",
		"restore the file from version control", "ingest the identical bytes again", "change the candidate so that a new review is held")
	if a.Command != "" || a.Usage != "metron ingest <report-file>" {
		t.Fatalf("next %+v", a)
	}
	contains(t, "ingest reformatted", r.ingest(exitNegative, reformatted).err,
		"the stored report reports/P-0001.json is missing, and this file is the same report in a different byte form", "restore the stored report from version control")
	r.expect(exitNegative, "fsck")
	contains(t, "ingest original", r.ingest(exitOK, report).out, "Note: restored the stored report file reports/P-0001.json.")
	if !bytes.Equal(r.read(stored), report) {
		t.Fatal("the restored report differs from the bytes submitted")
	}
	r.must("fsck")
	r.must("gate")
	if !bytes.Equal(ledger, r.read(".metron/state/ledger.jsonl")) {
		t.Fatal("restoring the report changed the ledger")
	}

	// An altered report file is never overwritten: it is deleted first.
	r.write(stored, strings.Replace(string(report), "rev-1", "rev-2", 1))
	contains(t, "fsck", r.expect(exitNegative, "fsck").out, "does not match the raw digest recorded")
	r.expect(exitNegative, "gate")
	contains(t, "ingest original", r.ingest(exitNegative, report).err, "never overwritten", "delete it and ingest this file again")
	if err := os.Remove(r.path(stored)); err != nil {
		t.Fatal(err)
	}
	r.ingest(exitOK, report)
	r.must("gate")
}

// TestStrayReportFile leaves a report file that the ledger does not record,
// as a crash between storing a report and accepting it would: fsck ignores
// it, and the next ingestion for the packet replaces it.
func TestStrayReportFile(t *testing.T) {
	r := pinned(t)
	r.write("greeting.txt", "hello\n")
	r.commit("spell it right")
	r.must("check")
	_, sk := r.packet()
	stored := ".metron/state/reports/P-0001.json"
	r.write(stored, "half a report")
	r.must("fsck")
	report := review(t, sk, "rev-1", nil)
	r.ingest(exitOK, report)
	if !bytes.Equal(r.read(stored), report) {
		t.Fatal("the stored report is not the one accepted")
	}
	r.must("fsck")
	r.must("gate")
}

// TestReportForAnotherLedger submits a report written for P-0001 of one
// repository to P-0001 of another: the packet digest tells them apart.
func TestReportForAnotherLedger(t *testing.T) {
	a, b := pinned(t), pinned(t)
	b.write("greeting.txt", "hello\n")
	b.commit("spell it right")
	for _, r := range []*repo{a, b} {
		r.must("check")
	}
	_, sk := a.packet()
	b.packet()
	res := b.ingest(exitNegative, review(t, sk, "rev-1", nil))
	contains(t, "ingest error", res.err, "rejected by the transition layer", "/packet_digest: the report was written for a different packet P-0001")
	a.ingest(exitOK, review(t, sk, "rev-1", nil))
}

func TestIngestFixWithoutChange(t *testing.T) {
	r := pinned(t)
	r.must("check")
	_, sk := r.packet()
	r.ingest(exitOK, review(t, sk, "rev-1", map[string]string{"REQ-2": core.RequirementViolated}, misspelled()))
	_, sk = r.packet()
	// A commit that touches only .metron/ fixes nothing.
	r.write(".metron/notes.txt", "thinking about it\n")
	commit := r.commit("notes")
	res := r.ingest(exitNegative, fix(t, sk, "impl-1", commit))
	contains(t, "ingest error", res.err, "rejected by the transition layer", "changes nothing outside .metron/")
	r.write("greeting.txt", "hello\n")
	commit = r.commit("fix the greeting")
	// The reviewer may not act as implementer.
	contains(t, "ingest error", r.ingest(exitNegative, fix(t, sk, "rev-1", commit)).err, `author "rev-1" acted as reviewer`)
	r.ingest(exitOK, fix(t, sk, "impl-1", commit))
	r.expectNext(core.StepVerify)
}
