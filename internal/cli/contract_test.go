package cli

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/patacca/metron/internal/core"
	"github.com/patacca/metron/internal/store"
)

func TestContractTemplate(t *testing.T) {
	if _, err := store.ParseContract(contractTemplate); err != nil {
		t.Fatalf("the template does not parse strictly: %v", err)
	}
	path := filepath.Join(t.TempDir(), "contract.yaml")
	if err := os.WriteFile(path, contractTemplate, 0o666); err != nil {
		t.Fatal(err)
	}
	cc := checkContract(path)
	want := []string{
		"/project/name", "/project/objectives/0", "/authorizes", "/scope/0", "/operating_envelope/0",
		"/requirements/0/statement", "/requirements/0/criterion",
		"/requirements/1/statement", "/requirements/1/criterion",
		"/requirements/2/statement", "/requirements/2/criterion",
		"/risks/unacceptable/0/statement",
	}
	var got []string
	lines := strings.Split(string(contractTemplate), "\n")
	for _, p := range cc.problems {
		got = append(got, p.Path)
		if p.Layer != layerStructural || p.Message != "must not be blank" {
			t.Errorf("problem %+v, want a structural blank value", p)
		}
		// The line shows the empty value: the member's key, or a list item.
		key := p.Path[strings.LastIndex(p.Path, "/")+1:]
		if p.Line < 1 || p.Line > len(lines) {
			t.Errorf("%s has line %d", p.Path, p.Line)
		} else if text := strings.TrimSpace(lines[p.Line-1]); !strings.HasSuffix(text, `""`) ||
			!(strings.Contains(text, key+":") || strings.HasPrefix(text, "- ")) {
			t.Errorf("%s points at line %d, %q", p.Path, p.Line, text)
		}
	}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("template problems\n got %v\nwant %v", got, want)
	}
	if cc.contract != nil || cc.digest == "" || cc.rawDigest == "" {
		t.Fatalf("check %+v", cc)
	}

	// Filling in every empty value is all a prototype contract needs.
	filled := bytes.ReplaceAll(contractTemplate, []byte(`""`), []byte(`"filled in"`))
	if err := os.WriteFile(path, filled, 0o666); err != nil {
		t.Fatal(err)
	}
	if cc := checkContract(path); len(cc.problems) > 0 || cc.contract == nil || cc.contract.Stage != core.StagePrototype {
		t.Fatalf("the filled-in template has problems: %+v", cc.problems)
	}
}

func TestContractLayers(t *testing.T) {
	for _, tc := range []struct {
		name, text, layer, want string
	}{
		{"yaml", "schema: [unclosed\n", layerYAML, "invalid YAML"},
		{"strict yaml", "schema: metron.contract/v1\nschema: again\n", layerYAML, `line 2: duplicate key "schema"`},
		{"structural", "schema: metron.contract/v1\nextra: 1\n", layerStructural, "/extra: unknown member"},
		{"semantic", strings.Replace(testContract, "REQ-3", "REQ-1", 1), string(core.LayerSemantic), `identifier "REQ-1" is already used`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := newRepo(t)
			r.must("init")
			r.write(".metron/contract.yaml", tc.text)
			res := r.expect(exitNegative, "contract")
			contains(t, "contract output", res.out, "found by the "+tc.layer+" layer", tc.want, "Result: INVALID")
			var d contractDoc
			unmarshal(t, r.expect(exitNegative, "contract", "--json").out, &d)
			if d.Schema != "metron.contract-status/v1" || d.Valid || d.Pinned || len(d.Problems) == 0 || d.Problems[0].Layer != tc.layer {
				t.Fatalf("contract document %+v", d)
			}
			res = r.expect(exitNegative, "contract", "pin", "-m", "try")
			contains(t, "pin error", res.err, "cannot be pinned", tc.want)
		})
	}
}

func TestUntouchedTemplate(t *testing.T) {
	r := newRepo(t)
	contains(t, "init output", r.must("init").out, "Created .metron/contract.yaml", "Created .metron/.gitignore",
		"Created .metron/.gitattributes", "Commit .metron/, state included",
		"Created .metron/state/ledger.jsonl", "Next step:    pin-contract (owner: governance)", "Run:          metron contract")
	res := r.expect(exitNegative, "contract")
	contains(t, "contract output", res.out, "12 problems found by the structural layer:", ": /project/name: must not be blank",
		"/risks/unacceptable/0/statement: must not be blank", "Pinned digest:   none", "Result: INVALID")
	before := r.read(".metron/state/ledger.jsonl")
	res = r.expect(exitNegative, "contract", "pin", "-m", "too early")
	contains(t, "pin error", res.err, "cannot be pinned: 12 problems", "/authorizes: must not be blank")
	if !bytes.Equal(before, r.read(".metron/state/ledger.jsonl")) {
		t.Fatal("a refused pin changed the ledger")
	}
	r.expect(exitNegative, "packet")
	if a := r.expectNext(core.StepPinContract); a.Command != "metron contract" {
		t.Fatalf("next %+v", a)
	}

	// init again finds everything and changes nothing.
	r.write(".metron/contract.yaml", testContract)
	res = r.must("init")
	contains(t, "second init", res.out, "Found .metron/contract.yaml; left it as it is.", "Found .metron/state/ledger.jsonl with 1 event")
	if string(r.read(".metron/contract.yaml")) != testContract || !bytes.Equal(before, r.read(".metron/state/ledger.jsonl")) {
		t.Fatal("a second init changed a file")
	}
}

func TestContractBlocksPacket(t *testing.T) {
	r := newRepo(t)
	r.must("init")
	r.write(".metron/contract.yaml", testContract)
	res := r.expect(exitNegative, "packet")
	contains(t, "packet error", res.err, "no contract is pinned", "Usage: metron contract pin -m <reason>")

	r.must("contract", "pin", "-m", "first")
	// A comment or a reformatting does not change the contract's meaning.
	r.write(".metron/contract.yaml", "# reviewed by governance\n"+testContract)
	r.must("contract")
	r.must("check")
	r.must("packet")

	r.write(".metron/contract.yaml", strings.Replace(testContract, "An internal demonstration", "A public launch", 1))
	res = r.expect(exitNegative, "contract")
	contains(t, "contract output", res.out, "its meaning differs from the pinned contract", "metron contract pin -m <reason>")
	res = r.expect(exitNegative, "packet")
	contains(t, "packet error", res.err, "the contract changed", "Usage: metron contract pin -m <reason>")
	if a := r.expectNext(core.StepPinContract); a.Command != "" || a.Usage != "metron contract pin -m <reason>" {
		t.Fatalf("next %+v", a)
	}
	r.expect(exitNegative, "check")
	r.expect(exitNegative, "gate")
}

func TestPinVoidsOpenPacket(t *testing.T) {
	r := pinned(t)
	r.must("check")
	p, sk := r.packet()
	r.write(".metron/contract.yaml", strings.Replace(testContract, "An internal demonstration", "A demonstration to the team", 1))
	res := r.must("contract", "pin", "-m", "widen the audience")
	contains(t, "pin output", res.out, "packet.voided          P-0001: the contract was re-pinned", "contract.pinned", "widen the audience",
		"Grace Governance <grace@example.com>")
	res = r.expect(exitNegative, "contract", "pin", "-m", "again")
	contains(t, "second pin", res.err, "is already pinned")
	r.ingest(exitNegative, review(t, sk, "rev-1", nil))
	r.expectNext(core.StepReview)
	q, _ := r.packet()
	var st statusDoc
	unmarshal(t, r.must("status", "--json").out, &st)
	if q.ID != "P-0002" || p.Round != 0 || q.Round != 0 || len(st.Rounds) != 2 || q.Contract.Authorizes != "A demonstration to the team" {
		t.Fatalf("packet after the pin: %s, %d rounds, authorizes %q", q.ID, len(st.Rounds), q.Contract.Authorizes)
	}
}
