package store

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"go.yaml.in/yaml/v3"

	"github.com/patacca/metron/internal/canon"
)

func TestParseContractValid(t *testing.T) {
	path := filepath.Join("testdata", "contract", "valid.yaml")
	c, err := ReadContract(path)
	if err != nil {
		t.Fatalf("ReadContract: %v", err)
	}
	want := map[string]any{
		"schema": "metron.contract/v1",
		"project": map[string]any{
			"name":       "Metron: example",
			"objectives": []any{"Decide whether a commit is approved", "Keep review loops finite"},
		},
		"stage":              "prototype",
		"authorizes":         "Internal use by the team",
		"scope":              []any{"cmd/", "internal/"},
		"operating_envelope": []any{"Single user,\nlocal repository\n"},
		"requirements": []any{
			map[string]any{
				"id": "REQ-1", "domain": "functional-correctness", "statement": "The gate fails closed",
				"criterion": "Every error path exits non-zero", "command": "go test ./...",
			},
			map[string]any{"id": "REQ-2", "domain": "architecture", "statement": "yes", "criterion": "on", "from": "prototype"},
		},
		"risks": map[string]any{
			"unacceptable": []any{map[string]any{"id": "RISK-1", "statement": "A gate opens on a violated requirement"}},
		},
		"not_applicable": []any{},
		"convergence": map[string]any{
			"root_cause_after": int64(2), "convergence_audit_after": int64(3), "process_audit_after": int64(-5),
			"quoted_date": "2026-01-01", "flag": true, "off": false, "unicode": "naïve é 😀",
		},
	}
	if !reflect.DeepEqual(c.Value, want) {
		t.Fatalf("value:\n got %#v\nwant %#v", c.Value, want)
	}
	golden := `{"authorizes":"Internal use by the team","convergence":{"convergence_audit_after":3,"flag":true,"off":false,` +
		`"process_audit_after":-5,"quoted_date":"2026-01-01","root_cause_after":2,"unicode":"naïve é 😀"},"not_applicable":[],` +
		`"operating_envelope":["Single user,\nlocal repository\n"],"project":{"name":"Metron: example","objectives":` +
		`["Decide whether a commit is approved","Keep review loops finite"]},"requirements":[{"command":"go test ./...",` +
		`"criterion":"Every error path exits non-zero","domain":"functional-correctness","id":"REQ-1","statement":` +
		`"The gate fails closed"},{"criterion":"on","domain":"architecture","from":"prototype","id":"REQ-2","statement":"yes"}],` +
		`"risks":{"unacceptable":[{"id":"RISK-1","statement":"A gate opens on a violated requirement"}]},` +
		`"schema":"metron.contract/v1","scope":["cmd/","internal/"],"stage":"prototype"}`
	if string(c.Canonical) != golden {
		t.Fatalf("canonical form:\n got %s\nwant %s", c.Canonical, golden)
	}
	sum := sha256.Sum256([]byte(golden))
	if want := "sha256:" + hex.EncodeToString(sum[:]); c.Digest != want {
		t.Fatalf("Digest = %s, want %s", c.Digest, want)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	sum = sha256.Sum256(raw)
	if want := "sha256:" + hex.EncodeToString(sum[:]); c.RawDigest != want {
		t.Fatalf("RawDigest = %s, want %s", c.RawDigest, want)
	}

	// Formatting, comments, quoting and member order do not change the
	// contract digest; they change the raw digest.
	other, err := ReadContract(filepath.Join("testdata", "contract", "valid-reformatted.yaml"))
	if err != nil {
		t.Fatalf("ReadContract: %v", err)
	}
	if other.Digest != c.Digest || !bytes.Equal(other.Canonical, c.Canonical) {
		t.Fatalf("reformatted contract has another digest:\n%s", other.Canonical)
	}
	if other.RawDigest == c.RawDigest {
		t.Fatal("reformatted contract has the same raw digest")
	}
}

func TestReadContractMissing(t *testing.T) {
	if _, err := ReadContract(filepath.Join(t.TempDir(), "contract.yaml")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("ReadContract = %v, want os.ErrNotExist", err)
	}
}

// Every fixture in testdata/contract/reject is rejected at the expected
// position with the expected explanation.
func TestParseContractRejectsFixtures(t *testing.T) {
	tests := map[string]struct {
		line, column int // zero when the error has no position of its own
		want         string
	}{
		"empty.yaml":                {0, 0, "the contract is empty"},
		"multiple-documents.yaml":   {2, 1, "single document"},
		"duplicate-key.yaml":        {3, 1, `duplicate key "stage", first defined at line 1, column 1`},
		"duplicate-key-nested.yaml": {4, 3, `duplicate key "name", first defined at line 2, column 3`},
		"duplicate-key-quoted.yaml": {2, 1, `duplicate key "name"`},
		"key-integer.yaml":          {2, 3, "key 1 is not a string; quote it"},
		"key-boolean.yaml":          {1, 1, "key true is not a string"},
		"key-null.yaml":             {1, 1, "key ~ is not a string"},
		"key-collection.yaml":       {1, 3, "must be a string, not a collection"},
		"anchor.yaml":               {1, 7, "anchor &shared is not allowed"},
		"merge-key.yaml":            {2, 3, "merge key << is not allowed"},
		"tag-custom.yaml":           {1, 8, "tag !custom is not allowed"},
		"tag-standard.yaml":         {1, 8, "tag !!str is not allowed"},
		"tag-collection.yaml":       {1, 10, "tag !!map is not allowed"},
		"null-value.yaml":           {1, 7, "missing value (null)"},
		"float.yaml":                {2, 21, "1.5 is read as a floating-point number"},
		"date.yaml":                 {2, 20, "2026-01-01 is read as a date; quote it"},
		"integer-hex.yaml":          {2, 21, "integer 0x10 must be written in plain decimal"},
		"integer-range.yaml":        {2, 21, "outside the range"},
		"syntax.yaml":               {0, 0, "invalid YAML: yaml: line"},
	}
	dir := filepath.Join("testdata", "contract", "reject")
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var files []string
	for _, e := range entries {
		files = append(files, e.Name())
	}
	for name := range tests {
		if !slices.Contains(files, name) {
			t.Errorf("fixture %s is missing", name)
		}
	}
	for _, name := range files {
		t.Run(name, func(t *testing.T) {
			tt, ok := tests[name]
			if !ok {
				t.Fatalf("fixture %s has no expectation", name)
			}
			path := filepath.Join(dir, name)
			_, err := ReadContract(path)
			if err == nil {
				t.Fatal("ReadContract succeeded")
			}
			if !strings.HasPrefix(err.Error(), path+": ") || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error %q does not name the file and contain %q", err, tt.want)
			}
			var ce *ContractError
			if hasPosition := errors.As(err, &ce); hasPosition != (tt.line != 0) {
				t.Fatalf("error %q: has a position = %v, want %v", err, hasPosition, tt.line != 0)
			}
			if ce != nil && (ce.Line != tt.line || ce.Column != tt.column) {
				t.Fatalf("position %d:%d, want %d:%d (%v)", ce.Line, ce.Column, tt.line, tt.column, err)
			}
		})
	}
}

// Variants of the fixture rules.
func TestParseContractRejectsVariants(t *testing.T) {
	tests := []struct{ in, want string }{
		{"a: ~\n", "missing value"},
		{"a: null\n", "missing value"},
		{"a: .inf\n", "floating-point"},
		{"a: -.inf\n", "floating-point"},
		{"a: .nan\n", "floating-point"},
		{"a: 1e3\n", "floating-point"},
		{"a: -1.0\n", "floating-point"},
		{"a: 99999999999999999999\n", "floating-point"},
		{"a: 2026-01-01T10:00:00Z\n", "read as a date"},
		{"a: 2026-1-2\n", "read as a date"},
		{"a: 0o17\n", "plain decimal"},
		{"a: 017\n", "plain decimal"},
		{"a: 1_000\n", "plain decimal"},
		{"a: +1\n", "plain decimal"},
		{"a: 0b101\n", "plain decimal"},
		{"a: -9007199254740992\n", "outside the range"},
		{"a: <<\n", "merge key"},
		{"a: !!binary aGk=\n", "tag !!binary"},
		{"a: *x\n", "unknown anchor"},
		{"- &a x\n- *a\n", "anchor &a"},
		{"a: \"\xff\"\n", "invalid YAML"},
		{"a: \"\\ud800\"\n", "invalid YAML"},
		{"a: b\n...\n---\nc: d\n", "single document"},
		{"a:\n  - b\n  - b: c\n    b: d\n", `duplicate key "b"`},
	}
	for _, tt := range tests {
		if _, err := ParseContract([]byte(tt.in)); err == nil || !strings.Contains(err.Error(), tt.want) {
			t.Errorf("ParseContract(%q) = %v, want an error containing %q", tt.in, err, tt.want)
		}
	}
}

// An alias is always preceded by its anchor, which is rejected first; the
// alias rule stands on its own all the same.
func TestAliasRejected(t *testing.T) {
	_, err := jsonValue(&yaml.Node{Kind: yaml.AliasNode, Value: "x", Line: 3, Column: 4})
	var ce *ContractError
	if !errors.As(err, &ce) || ce.Line != 3 || ce.Column != 4 || !strings.Contains(ce.Msg, "alias *x") {
		t.Fatalf("got %v", err)
	}
}

func TestParseContractAccepts(t *testing.T) {
	tests := []struct {
		in, want string
	}{
		{"a: yes\nb: on\nc: 'null'\nd: \"1.5\"\n", `{"a":"yes","b":"on","c":"null","d":"1.5"}`},
		{"a: True\nb: FALSE\nc: -0\nd: 9007199254740991\n", `{"a":true,"b":false,"c":0,"d":9007199254740991}`},
		{"---\na: 1\n...\n", `{"a":1}`},
		{"[]\n", `[]`},
		{"\"x\"\n", `"x"`},
		{"a: ! 12\n", `{"a":12}`},
	}
	for _, tt := range tests {
		c, err := ParseContract([]byte(tt.in))
		if err != nil {
			t.Errorf("ParseContract(%q): %v", tt.in, err)
			continue
		}
		if string(c.Canonical) != tt.want {
			t.Errorf("ParseContract(%q) = %s, want %s", tt.in, c.Canonical, tt.want)
		}
	}
}

func FuzzParseContract(f *testing.F) {
	for _, dir := range []string{"testdata/contract", "testdata/contract/reject"} {
		entries, err := os.ReadDir(dir)
		if err != nil {
			f.Fatal(err)
		}
		for _, e := range entries {
			if e.IsDir() {
				continue
			}
			data, err := os.ReadFile(filepath.Join(dir, e.Name()))
			if err != nil {
				f.Fatal(err)
			}
			f.Add(data)
		}
	}
	f.Add([]byte("a: &x [*x]\n"))
	f.Add([]byte("{? a: b}\n"))
	f.Fuzz(func(t *testing.T, data []byte) {
		c, err := ParseContract(data)
		if err != nil {
			return
		}
		again, err := canon.Canonicalize(c.Canonical)
		if err != nil || !bytes.Equal(again, c.Canonical) {
			t.Fatalf("canonical form is not canonical: %q, %v", c.Canonical, err)
		}
		if c.Digest != canon.Digest(c.Canonical) || c.RawDigest != canon.Digest(data) {
			t.Fatal("digests do not match")
		}
	})
}
