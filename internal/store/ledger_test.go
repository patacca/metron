package store

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/patacca/metron/internal/core"
)

const t0 = "2026-01-02T03:04:05Z"

// sampleDrafts is a small but varied batch: nested payloads, optional
// members, non-ASCII text and characters that need escaping.
func sampleDrafts() []core.Draft {
	return []core.Draft{
		{Type: core.EventLedgerInitialized, Data: core.LedgerInitialized{Protocol: core.ProtocolVersion}},
		{Type: core.EventContractPinned, Data: core.ContractPinned{
			Digest: "sha256:" + strings.Repeat("ab", 32), RawDigest: "sha256:" + strings.Repeat("cd", 32),
			Stage: core.StagePrototype, Reason: "first pin: \"quoted\", naïve, <tag>", By: "Ada <ada@example.com>",
		}},
		{Type: core.EventFindingOpened, Data: core.FindingOpened{
			ID: "F-0001", Round: 1, Source: core.RoleReviewer, Packet: "P-0001",
			Input: core.FindingInput{Key: "k1", Tier: core.TierMaterial, Domain: core.DomainFunctionalCorrectness,
				Title: "line\nbreak", Components: []string{"a/b.go"}, Description: "d", Evidence: "e", Impact: "i"},
		}},
	}
}

func seal(t *testing.T, prev *core.Event, drafts ...core.Draft) []core.Event {
	t.Helper()
	events, err := Seal(prev, t0, drafts)
	if err != nil {
		t.Fatalf("Seal: %v", err)
	}
	return events
}

func encode(events []core.Event) []byte {
	var b []byte
	for _, e := range events {
		b = append(appendLine(b, e), '\n')
	}
	return b
}

func sampleLedger(t *testing.T) ([]core.Event, []byte) {
	t.Helper()
	events := seal(t, nil, sampleDrafts()...)
	return events, encode(events)
}

// withHash completes a hand-written line prefix, which ends just before the
// hash member, with a hash that is correct under the last-occurrence rule.
func withHash(prefix string) string {
	sum := sha256.Sum256([]byte(prefix + "}"))
	return prefix + `,"hash":"` + hex.EncodeToString(sum[:]) + `"}`
}

func TestSealGolden(t *testing.T) {
	events := seal(t, nil, core.Draft{Type: core.EventLedgerInitialized, Data: core.LedgerInitialized{Protocol: core.ProtocolVersion}})
	body := `{"seq":1,"time":"2026-01-02T03:04:05Z","prev":"` + strings.Repeat("0", 64) +
		`","type":"ledger.initialized","data":{"protocol":"metron.protocol/v1"}}`
	want := withHash(strings.TrimSuffix(body, "}"))
	if got := string(appendLine(nil, events[0])); got != want {
		t.Fatalf("line:\n got %s\nwant %s", got, want)
	}
}

func TestSealChainsBatches(t *testing.T) {
	first := seal(t, nil, sampleDrafts()[:2]...)
	second := seal(t, &first[1], sampleDrafts()[2])
	all := append(append([]core.Event(nil), first...), second...)
	got, err := VerifyLedger(encode(all))
	if err != nil {
		t.Fatalf("VerifyLedger: %v", err)
	}
	if !reflect.DeepEqual(got, all) {
		t.Fatalf("verified events differ from sealed ones")
	}
	for i, e := range all {
		if e.Seq != i+1 {
			t.Errorf("event %d has seq %d", i, e.Seq)
		}
	}
	if all[0].Prev != GenesisPrev || all[2].Prev != all[1].Hash {
		t.Error("prev links are wrong")
	}
}

func TestSealRejects(t *testing.T) {
	ok := core.LedgerInitialized{Protocol: core.ProtocolVersion}
	tests := []struct {
		name  string
		time  string
		draft core.Draft
	}{
		{"time with offset", "2026-01-02T03:04:05+00:00", core.Draft{Type: core.EventLedgerInitialized, Data: ok}},
		{"time with fraction", "2026-01-02T03:04:05.5Z", core.Draft{Type: core.EventLedgerInitialized, Data: ok}},
		{"time out of range", "2026-13-02T03:04:05Z", core.Draft{Type: core.EventLedgerInitialized, Data: ok}},
		{"empty type", t0, core.Draft{Type: "", Data: ok}},
		{"type needing escapes", t0, core.Draft{Type: `a"b`, Data: ok}},
		{"null payload", t0, core.Draft{Type: core.EventLedgerInitialized, Data: nil}},
		{"array payload", t0, core.Draft{Type: core.EventLedgerInitialized, Data: []string{"x"}}},
		{"non-integer payload", t0, core.Draft{Type: core.EventLedgerInitialized, Data: map[string]float64{"x": 1.5}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := Seal(nil, tt.time, []core.Draft{tt.draft}); err == nil {
				t.Fatal("Seal succeeded")
			}
		})
	}
}

func TestVerifyEmpty(t *testing.T) {
	events, err := VerifyLedger(nil)
	if err != nil || len(events) != 0 {
		t.Fatalf("VerifyLedger(nil) = %v, %v", events, err)
	}
}

func TestVerifyCRLF(t *testing.T) {
	events, data := sampleLedger(t)
	got, err := VerifyLedger(bytes.ReplaceAll(data, []byte("\n"), []byte("\r\n")))
	if err != nil {
		t.Fatalf("VerifyLedger: %v", err)
	}
	if !reflect.DeepEqual(got, events) {
		t.Fatal("events differ")
	}
}

// Every single-byte change to a valid ledger must be detected.
func TestVerifyRejectsEverySingleByteChange(t *testing.T) {
	_, data := sampleLedger(t)
	replacements := []byte{'\n', '\r', ' ', '"', ',', '{', '}', '\\', '0', 'a', 'A', 0x00, 0x80, 0xff}
	for i := range data {
		var candidates []byte
		for bit := 0; bit < 8; bit++ {
			candidates = append(candidates, data[i]^(1<<bit))
		}
		candidates = append(candidates, replacements...)
		for _, c := range candidates {
			if c == data[i] {
				continue
			}
			mutated := bytes.Clone(data)
			mutated[i] = c
			if _, err := VerifyLedger(mutated); err == nil {
				t.Fatalf("change of byte %d from %q to %q was accepted", i, data[i], c)
			}
		}
	}
}

func TestVerifyHashInPayload(t *testing.T) {
	tricky := map[string]any{
		"a":    1,
		"hash": strings.Repeat("f", 64),
		"note": `x","hash":"` + strings.Repeat("0", 64) + `"}`,
		"z":    map[string]any{"hash": `,"hash":"`},
	}
	events := seal(t, nil, core.Draft{Type: core.EventLedgerInitialized, Data: core.LedgerInitialized{Protocol: core.ProtocolVersion}})
	events = append(events, seal(t, &events[0], core.Draft{Type: "test.tricky", Data: tricky})...)
	data := encode(events)
	// One marker per envelope and one for the payload member "hash",
	// which follows "a". The string's quotes are escaped, and "z"'s member
	// is preceded by a brace.
	if n := bytes.Count(data, []byte(hashMarker)); n != 3 {
		t.Fatalf("found %d hash markers, want 3", n)
	}
	got, err := VerifyLedger(data)
	if err != nil {
		t.Fatalf("VerifyLedger: %v", err)
	}
	if !reflect.DeepEqual(got, events) {
		t.Fatal("events differ")
	}
	var decoded map[string]any
	if err := json.Unmarshal(got[1].Data, &decoded); err != nil || decoded["note"] != tricky["note"] {
		t.Fatalf("payload did not round-trip: %v", err)
	}
}

func TestVerifyRejects(t *testing.T) {
	_, valid := sampleLedger(t)
	lines := strings.SplitAfter(string(valid), "\n")
	zeros := strings.Repeat("0", 64)
	prefix := `{"seq":1,"time":"` + t0 + `","prev":"` + zeros + `","type":"ledger.initialized","data":`
	tests := []struct {
		name string
		data string
		line int
		want string
	}{
		{"missing final newline", strings.TrimSuffix(string(valid), "\n"), 3, "no newline"},
		{"corrupt line before a missing final newline", lines[0][:20] + "\n" + strings.TrimSuffix(lines[1], "\n"), 1, "not a valid event"},
		{"blank line", lines[0] + "\n" + lines[1], 2, "not a valid event"},
		{"line removed", lines[0] + lines[2], 2, "seq is 3, want 2"},
		{"lines swapped", lines[1] + lines[0], 1, "seq is 2, want 1"},
		{"truncated line", lines[0] + lines[1][:40] + "\n", 2, "not a valid event"},
		{"two CRs", strings.TrimSuffix(lines[0], "\n") + "\r\r\n", 1, "envelope form"},
		{"seq does not start at 1", withHash(`{"seq":2,"time":"`+t0+`","prev":"`+zeros+`","type":"x","data":{}`) + "\n", 1, "seq is 2"},
		{"genesis prev not zero", withHash(`{"seq":1,"time":"`+t0+`","prev":"`+strings.Repeat("1", 64)+`","type":"x","data":{}`) + "\n", 1, "prev does not match"},
		{"wrong hash", lines[0][:len(lines[0])-67] + zeros + "\"}\n", 1, "hash does not match"},
		{"space after colon", withHash(`{"seq": 1,"time":"`+t0+`","prev":"`+zeros+`","type":"x","data":{}`) + "\n", 1, "envelope form"},
		{"members reordered", withHash(`{"time":"`+t0+`","seq":1,"prev":"`+zeros+`","type":"x","data":{}`) + "\n", 1, "envelope form"},
		{"extra member", withHash(prefix+`{},"extra":1`) + "\n", 1, "envelope form"},
		{"duplicate member", withHash(prefix+`{},"seq":1`) + "\n", 1, "envelope form"},
		{"upper-case member", withHash(`{"SEQ":1,"time":"`+t0+`","prev":"`+zeros+`","type":"x","data":{}`) + "\n", 1, "envelope form"},
		{"escaped type", withHash(`{"seq":1,"time":"`+t0+`","prev":"`+zeros+`","type":"\u0078","data":{}`) + "\n", 1, "envelope form"},
		{"whitespace in data", withHash(prefix+`{"a": 1}`) + "\n", 1, "not compact"},
		{"data not an object", withHash(prefix+`[]`) + "\n", 1, "not a JSON object"},
		{"data missing", withHash(`{"seq":1,"time":"`+t0+`","prev":"`+zeros+`","type":"x"`) + "\n", 1, "not a JSON object"},
		{"time with offset", withHash(`{"seq":1,"time":"2026-01-02T03:04:05+00:00","prev":"`+zeros+`","type":"x","data":{}`) + "\n", 1, "RFC 3339"},
		{"seq not an integer", withHash(`{"seq":1.0,"time":"`+t0+`","prev":"`+zeros+`","type":"x","data":{}`) + "\n", 1, "not a valid event"},
		{"upper-case hash", prefix + `{},"hash":"` + strings.Repeat("A", 64) + "\"}\n", 1, "hash is not 64 lowercase"},
		{"invalid UTF-8", withHash(prefix+"{\"a\":\"\xff\"}") + "\n", 1, "invalid UTF-8"},
		{
			// The last marker is inside the payload and the envelope has no
			// hash member of its own.
			"hash only inside data",
			withHash(prefix+`{"a":1`) + "}\n", 1, "hash is not 64 lowercase",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := VerifyLedger([]byte(tt.data))
			var le *LedgerError
			if !errors.As(err, &le) {
				t.Fatalf("got %v, want a *LedgerError", err)
			}
			if le.Line != tt.line || !strings.Contains(le.Msg, tt.want) {
				t.Fatalf("got %v, want line %d containing %q", err, tt.line, tt.want)
			}
		})
	}
}

func FuzzVerifyLedger(f *testing.F) {
	events, err := Seal(nil, t0, sampleDrafts())
	if err != nil {
		f.Fatal(err)
	}
	data := encode(events)
	f.Add(data)
	f.Add(bytes.ReplaceAll(data, []byte("\n"), []byte("\r\n")))
	f.Add(data[:len(data)/2])
	f.Add([]byte(withHash(`{"seq":1,"time":"`+t0+`","prev":"`+GenesisPrev+`","type":"x","data":{"hash":"y"}`) + "\n"))
	f.Add([]byte("\n"))
	f.Fuzz(func(t *testing.T, data []byte) {
		events, err := VerifyLedger(data)
		if err != nil {
			var le *LedgerError
			if !errors.As(err, &le) || le.Line < 1 {
				t.Fatalf("error %v is not a *LedgerError with a line", err)
			}
			return
		}
		// A ledger that verifies is exactly what the writer produces for
		// its events, up to line endings.
		if want := bytes.ReplaceAll(data, []byte("\r\n"), []byte("\n")); !bytes.Equal(encode(events), want) {
			t.Fatalf("verified ledger does not re-encode to its bytes:\n%q\n%q", encode(events), want)
		}
	})
}
