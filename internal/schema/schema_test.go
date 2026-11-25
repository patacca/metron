package schema

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/patacca/metron/internal/core"
)

func fixtures(t testing.TB, k Kind, sub string) map[string][]byte {
	t.Helper()
	files, err := filepath.Glob(filepath.Join("testdata", string(k), sub, "*.json"))
	if err != nil {
		t.Fatal(err)
	}
	if len(files) == 0 {
		t.Fatalf("no fixtures under testdata/%s/%s", k, sub)
	}
	out := make(map[string][]byte, len(files))
	for _, f := range files {
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		out[strings.TrimSuffix(filepath.Base(f), ".json")] = b
	}
	return out
}

func pointers(ps []Problem) []string {
	out := make([]string, len(ps))
	for i, p := range ps {
		out[i] = p.Pointer
	}
	return out
}

func TestValidFixtures(t *testing.T) {
	for _, k := range Kinds() {
		for name, data := range fixtures(t, k, "valid") {
			t.Run(string(k)+"/"+name, func(t *testing.T) {
				ps, err := ValidateJSON(k, data)
				if err != nil {
					t.Fatal(err)
				}
				if len(ps) != 0 {
					t.Fatalf("valid fixture rejected: %v", ps)
				}
			})
		}
	}
}

func TestInvalidFixtures(t *testing.T) {
	seen := map[string]bool{}
	for _, k := range Kinds() {
		for name, data := range fixtures(t, k, "invalid") {
			key := string(k) + "/" + name
			seen[key] = true
			t.Run(key, func(t *testing.T) {
				want, ok := invalidFixtures[key]
				if !ok {
					t.Fatalf("fixture has no entry in invalidFixtures")
				}
				ps, err := ValidateJSON(k, data)
				if err != nil {
					t.Fatal(err)
				}
				if got := pointers(ps); !reflect.DeepEqual(got, want) {
					t.Fatalf("pointers = %q, want %q\nproblems: %v", got, want, ps)
				}
				for _, p := range ps {
					if strings.TrimSpace(p.Message) == "" {
						t.Errorf("empty message at %q", p.Pointer)
					}
				}
			})
		}
	}
	for key := range invalidFixtures {
		if !seen[key] {
			t.Errorf("invalidFixtures lists %s, which has no file", key)
		}
	}
}

func TestProblemMessages(t *testing.T) {
	tests := []struct{ fixture, want string }{
		{"contract/bad-stage", `must be one of "prototype"`},
		{"contract/missing-stage", `missing required member "stage"`},
		{"contract/unknown-member", "unknown member"},
		{"contract/blank-statement", "must not be blank"},
		{"contract/empty-requirements", "at least 1 item"},
		{"packet/fractional-time", "RFC 3339 UTC"},
		{"packet/digest-without-prefix", `"sha256:" followed by 64`},
		{"event/zero-seq", "at least 1"},
		{"event/fractional-seq", "type integer"},
	}
	for _, tc := range tests {
		kind, name, _ := strings.Cut(tc.fixture, "/")
		data := fixtures(t, Kind(kind), "invalid")[name]
		ps, err := ValidateJSON(Kind(kind), data)
		if err != nil || len(ps) == 0 {
			t.Fatalf("%s: problems %v, error %v", tc.fixture, ps, err)
		}
		if !strings.Contains(ps[0].Message, tc.want) {
			t.Errorf("%s: message %q does not contain %q", tc.fixture, ps[0].Message, tc.want)
		}
	}
}

func TestDeterministic(t *testing.T) {
	data := fixtures(t, Report, "invalid")["reviewer-body-for-implementer"]
	first, err := ValidateJSON(Report, data)
	if err != nil || len(first) < 2 {
		t.Fatalf("problems %v, error %v", first, err)
	}
	for i := 0; i < 20; i++ {
		again, _ := ValidateJSON(Report, data)
		if !reflect.DeepEqual(first, again) {
			t.Fatalf("run %d differs:\n%v\n%v", i, first, again)
		}
	}
	for i := 1; i < len(first); i++ {
		a, b := first[i-1], first[i]
		if less(b, a) {
			t.Fatalf("problems are not ordered: %v before %v", a, b)
		}
	}
}

func TestValidateDecodedValue(t *testing.T) {
	var doc any
	if err := json.Unmarshal(fixtures(t, Contract, "valid")["full"], &doc); err != nil {
		t.Fatal(err)
	}
	ps, err := Validate(Contract, doc)
	if err != nil || len(ps) != 0 {
		t.Fatalf("float64 decoding: problems %v, error %v", ps, err)
	}
	ps, err = Validate(Contract, struct{}{})
	if err != nil || len(ps) != 1 {
		t.Fatalf("non-JSON value: problems %v, error %v", ps, err)
	}
}

func TestValidateErrors(t *testing.T) {
	if _, err := Validate("nope", map[string]any{}); err == nil {
		t.Error("unknown kind accepted")
	}
	if _, err := ValidateJSON("nope", []byte("{}")); err == nil {
		t.Error("unknown kind accepted by ValidateJSON")
	}
	for _, in := range []string{"", "{", `{"a":}`, "{} {}", "{} x", "nul"} {
		ps, err := ValidateJSON(Contract, []byte(in))
		if err != nil || len(ps) != 1 || ps[0].Pointer != "" || !strings.Contains(ps[0].Message, "not valid JSON") {
			t.Errorf("%q: problems %v, error %v", in, ps, err)
		}
	}
	ps, err := ValidateJSON(Event, []byte("null"))
	if err != nil || len(ps) != 1 || !strings.Contains(ps[0].Message, "type object, got null") {
		t.Errorf("null document: %v %v", ps, err)
	}
}

func TestNamesAndBytes(t *testing.T) {
	want := []string{"contract", "defs", "event", "packet", "report"}
	if got := Names(); !reflect.DeepEqual(got, want) {
		t.Fatalf("Names() = %v, want %v", got, want)
	}
	for _, n := range want {
		b, err := Bytes(n)
		if err != nil {
			t.Fatal(err)
		}
		var doc map[string]any
		if err := json.Unmarshal(b, &doc); err != nil {
			t.Fatalf("%s: %v", n, err)
		}
		if id := doc["$id"]; id != idPrefix+n+fileExt {
			t.Errorf("%s: $id = %v", n, id)
		}
	}
	if _, err := Bytes("../go.mod"); err == nil || !strings.Contains(err.Error(), "choose one of") {
		t.Errorf("Bytes of an unknown name: %v", err)
	}
	for _, k := range Kinds() {
		if _, err := Bytes(string(k)); err != nil {
			t.Errorf("no schema for kind %s", k)
		}
	}
}

func TestNothingIsFetched(t *testing.T) {
	if _, err := (refuseLoader{}).Load("https://example.com/x.json"); err == nil {
		t.Fatal("loader loaded a foreign URL")
	}
	// Compilation succeeded without any other loader: every reference was
	// resolved from the embedded resources.
	if _, err := Validate(Packet, map[string]any{}); err != nil {
		t.Fatal(err)
	}
}

// Every valid fixture decodes strictly into its core type, and what the core
// type encodes again still validates.
func TestRoundTrip(t *testing.T) {
	for _, k := range Kinds() {
		for name, data := range fixtures(t, k, "valid") {
			t.Run(string(k)+"/"+name, func(t *testing.T) {
				out := roundTrip(t, k, data)
				ps, err := ValidateJSON(k, out)
				if err != nil {
					t.Fatal(err)
				}
				if len(ps) != 0 {
					t.Fatalf("re-encoded document rejected: %v\n%s", ps, out)
				}
			})
		}
	}
}

func strict(t *testing.T, data []byte, v any) {
	t.Helper()
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		t.Fatalf("strict decoding into %T: %v", v, err)
	}
}

func marshal(t *testing.T, v any) []byte {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func roundTrip(t *testing.T, k Kind, data []byte) []byte {
	t.Helper()
	switch k {
	case Contract:
		var c core.Contract
		strict(t, data, &c)
		return marshal(t, c)
	case Packet:
		var p core.Packet
		strict(t, data, &p)
		return marshal(t, p)
	case Report:
		var r core.Report
		strict(t, data, &r)
		body := reflect.New(reportBodies[r.Role]).Interface()
		strict(t, r.Body, body)
		r.Body = marshal(t, body)
		return marshal(t, r)
	case Event:
		var e core.Event
		strict(t, data, &e)
		payload := reflect.New(eventPayloads[e.Type]).Interface()
		strict(t, e.Data, payload)
		e.Data = marshal(t, payload)
		return marshal(t, e)
	}
	t.Fatalf("kind %s", k)
	return nil
}

func TestEveryEventTypeHasValidFixture(t *testing.T) {
	seen := map[core.EventType]bool{}
	for _, data := range fixtures(t, Event, "valid") {
		var e core.Event
		strict(t, data, &e)
		seen[e.Type] = true
	}
	for typ := range eventPayloads {
		if !seen[typ] {
			t.Errorf("no valid fixture for event type %s", typ)
		}
	}
}

func FuzzValidate(f *testing.F) {
	for _, k := range Kinds() {
		for _, sub := range []string{"valid", "invalid"} {
			files, _ := filepath.Glob(filepath.Join("testdata", string(k), sub, "*.json"))
			for _, file := range files {
				if b, err := os.ReadFile(file); err == nil {
					f.Add(b)
				}
			}
		}
	}
	for _, s := range []string{"", "null", "[]", "{}", `""`, "0", "1e999", "-0", "[[[[]]]]", `{"type":"round.opened","data":null}`,
		`{"role":"auditor","body":[]}`, `{"seq":18446744073709551616}`, "\xff", `{"\u0000":1}`} {
		f.Add([]byte(s))
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		for _, k := range Kinds() {
			ps, err := ValidateJSON(k, data)
			if err != nil {
				t.Fatalf("%s: %v", k, err)
			}
			for i, p := range ps {
				if p.Message == "" {
					t.Fatalf("%s: empty message at %q", k, p.Pointer)
				}
				if p.Pointer != "" && p.Pointer[0] != '/' {
					t.Fatalf("%s: pointer %q is not a JSON Pointer", k, p.Pointer)
				}
				if i > 0 && less(p, ps[i-1]) {
					t.Fatalf("%s: problems out of order", k)
				}
			}
			again, _ := ValidateJSON(k, data)
			if !reflect.DeepEqual(ps, again) {
				t.Fatalf("%s: result is not deterministic", k)
			}
		}
	})
}

// Pointers are ordered by document position, not as text: item 2 comes before
// item 10.
func TestDocumentOrder(t *testing.T) {
	var doc map[string]any
	if err := json.Unmarshal(fixtures(t, Report, "valid")["reviewer-full"], &doc); err != nil {
		t.Fatal(err)
	}
	body := doc["body"].(map[string]any)
	var coverage []any
	for i := 0; i < 12; i++ {
		coverage = append(coverage, map[string]any{"domain": "stability", "status": "skipped", "summary": "s"})
	}
	body["coverage"] = coverage
	body["extra"] = true
	ps, err := Validate(Report, doc)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"/body/coverage/0/status"}
	for i := 1; i < 12; i++ {
		want = append(want, fmt.Sprintf("/body/coverage/%d/status", i))
	}
	want = append(want, "/body/extra")
	if got := pointers(ps); !reflect.DeepEqual(got, want) {
		t.Fatalf("pointers = %q, want %q", got, want)
	}
}

func TestComparePointers(t *testing.T) {
	ordered := []string{"", "/a", "/a/2", "/a/10", "/a/10/x", "/a/b", "/b", "/b/0", "/b/00", "/b/000", "/b/1", "/b/10", "/b/1x", "/b/x"}
	for i, a := range ordered {
		for j, b := range ordered {
			got := comparePointers(a, b)
			if (i < j && got >= 0) || (i > j && got <= 0) || (i == j && got != 0) {
				t.Errorf("comparePointers(%q, %q) = %d", a, b, got)
			}
		}
	}
}
