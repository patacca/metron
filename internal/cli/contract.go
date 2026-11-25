package cli

import (
	"cmp"
	_ "embed"
	jsonv2 "encoding/json/v2"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"slices"
	"strconv"
	"strings"

	"go.yaml.in/yaml/v3"

	"github.com/patacca/metron/internal/canon"
	"github.com/patacca/metron/internal/core"
	"github.com/patacca/metron/internal/schema"
	"github.com/patacca/metron/internal/store"
)

// contractTemplate is what metron init writes to .metron/contract.yaml.
//
//go:embed contract.yaml
var contractTemplate []byte

// Contract validation layers, in the order they run. Each stops the next.
const (
	layerFile       = "file"
	layerYAML       = "yaml"
	layerStructural = "structural"
)

// contractProblem is one reason the contract cannot be used.
type contractProblem struct {
	Layer string `json:"layer"`
	// Path is a JSON Pointer into the contract, empty for the whole file.
	Path string `json:"path"`
	// Line is the line of .metron/contract.yaml it concerns, when known.
	Line    int    `json:"line,omitempty"`
	Message string `json:"message"`
}

func (p contractProblem) String() string {
	var b strings.Builder
	if p.Line > 0 {
		fmt.Fprintf(&b, "line %d: ", p.Line)
	}
	if p.Path != "" {
		b.WriteString(p.Path + ": ")
	}
	b.WriteString(p.Message)
	return b.String()
}

// contractCheck is the contract file read strictly, validated structurally,
// decoded strictly and validated semantically, stopping at the first layer
// that finds problems.
type contractCheck struct {
	path     string
	problems []contractProblem
	// contract is set when every layer passed.
	contract *core.Contract
	// digest is set once the file parsed; rawDigest once it was read.
	digest, rawDigest string
}

func checkContract(path string) *contractCheck {
	cc := &contractCheck{path: path}
	data, err := os.ReadFile(path)
	if err != nil {
		msg := fmt.Sprintf("the contract file cannot be read: %v", err)
		if errors.Is(err, fs.ErrNotExist) {
			msg = "the contract file does not exist: metron init creates it from the template"
		}
		cc.problems = append(cc.problems, contractProblem{Layer: layerFile, Message: msg})
		return cc
	}
	cc.rawDigest = canon.Digest(data)
	cf, err := store.ParseContract(data)
	if err != nil {
		p := contractProblem{Layer: layerYAML, Message: err.Error()}
		var ce *store.ContractError
		if errors.As(err, &ce) {
			p.Line, p.Message = ce.Line, ce.Msg
		}
		cc.problems = append(cc.problems, p)
		return cc
	}
	cc.digest = cf.Digest
	line := yamlLines(data)
	ps, err := schema.ValidateJSON(schema.Contract, cf.Canonical)
	if err != nil {
		ps = []schema.Problem{{Message: "the contract schema is unusable: " + err.Error()}}
	}
	for _, p := range ps {
		cc.problems = append(cc.problems, contractProblem{Layer: layerStructural, Path: p.Pointer, Line: line(p.Pointer), Message: p.Message})
	}
	if len(cc.problems) > 0 {
		sortByLine(cc.problems)
		return cc
	}
	var c core.Contract
	if err := jsonv2.Unmarshal(cf.Canonical, &c, jsonv2.RejectUnknownMembers(true)); err != nil {
		cc.problems = append(cc.problems, contractProblem{Layer: layerStructural, Message: "the contract does not decode: " + err.Error()})
		return cc
	}
	if err := core.ValidateContract(c); err != nil {
		var r *core.Rejection
		if !errors.As(err, &r) {
			r = &core.Rejection{Layer: core.LayerSemantic, Problems: []core.Problem{{Message: err.Error()}}}
		}
		for _, p := range r.Problems {
			cc.problems = append(cc.problems, contractProblem{Layer: string(r.Layer), Path: p.Path, Line: line(p.Path), Message: p.Message})
		}
		sortByLine(cc.problems)
		return cc
	}
	cc.contract = &c
	return cc
}

// sortByLine orders problems as they appear in the file, keeping the order
// of the problems on one line.
func sortByLine(ps []contractProblem) {
	slices.SortStableFunc(ps, func(a, b contractProblem) int { return cmp.Compare(a.Line, b.Line) })
}

// summary says in one sentence why the contract is unusable, or returns "".
func (cc *contractCheck) summary(s *site) string {
	if len(cc.problems) == 0 {
		return ""
	}
	first := cc.problems[0]
	if len(cc.problems) == 1 {
		return fmt.Sprintf("%s: %s", s.show(cc.path), first)
	}
	at := ""
	if first.Line > 0 {
		at = fmt.Sprintf(" at line %d,", first.Line)
	}
	if first.Path != "" {
		at += " " + first.Path + ":"
	}
	return fmt.Sprintf("%s has %d %s problems, the first%s %s; %s lists them all",
		s.show(cc.path), len(cc.problems), first.Layer, at, first.Message, s.metron("contract"))
}

// yamlLines returns a function that finds the line of .metron/contract.yaml a
// JSON Pointer refers to: the line of the member's key, of the list item, or
// of the nearest enclosing one that exists. It returns 0 for the document.
func yamlLines(data []byte) func(string) int {
	var doc yaml.Node
	if yaml.Unmarshal(data, &doc) != nil || len(doc.Content) != 1 {
		return func(string) int { return 0 }
	}
	return func(pointer string) int {
		if pointer == "" {
			return 0
		}
		n, line := doc.Content[0], 0
		for _, tok := range strings.Split(pointer, "/")[1:] {
			tok = strings.NewReplacer("~1", "/", "~0", "~").Replace(tok)
			var next *yaml.Node
			switch n.Kind {
			case yaml.MappingNode:
				for i := 0; i+1 < len(n.Content); i += 2 {
					if n.Content[i].Value == tok {
						next, line = n.Content[i+1], n.Content[i].Line
					}
				}
			case yaml.SequenceNode:
				if i, err := strconv.Atoi(tok); err == nil && i >= 0 && i < len(n.Content) {
					next, line = n.Content[i], n.Content[i].Line
				}
			}
			if next == nil {
				return max(line, n.Line)
			}
			n = next
		}
		return line
	}
}

const contractAbout = `Contract reads .metron/contract.yaml in three layers (strict YAML, the
contract schema, the semantic rules), prints every problem the first failing
layer finds with its location, then the contract digest, the raw digest of the
file, and the pinned digest. It exits 0 only when the contract is valid and
pinned.

"metron contract pin -m <reason>" is governance's act of authorizing the
current contract. It records the contract digest, the reason, and the git
identity (user.name and user.email) of whoever pins it. Pinning voids the open
packet. Pinning the digest that is already pinned is refused.`

var cmdContract = &command{
	name:     "contract",
	synopsis: "contract [--json] | contract pin -m <reason>",
	summary:  "Validate the contract; 'contract pin' authorizes it (governance)",
	about:    contractAbout,
	run:      runContract,
}

type contractDoc struct {
	Schema   string            `json:"schema"`
	Path     string            `json:"path"`
	Valid    bool              `json:"valid"`
	Problems []contractProblem `json:"problems"`
	Stage    core.Stage        `json:"stage,omitempty"`
	// Digest and RawDigest are empty when the file could not be parsed or read.
	Digest    string `json:"digest,omitempty"`
	RawDigest string `json:"raw_digest,omitempty"`
	// PinnedDigest is empty when nothing is pinned or the ledger is unusable.
	PinnedDigest string `json:"pinned_digest,omitempty"`
	// Pinned is true when the contract is valid and its digest is pinned.
	Pinned        bool   `json:"pinned"`
	LedgerProblem string `json:"ledger_problem,omitempty"`
}

func runContract(c *ctx, args []string) error {
	if len(args) > 0 && args[0] == "pin" {
		return runPin(c, args[1:])
	}
	fs := c.flags()
	dir, asJSON := stateFlag(fs), jsonFlag(fs)
	args, err := c.parse(fs, args)
	if err != nil {
		return err
	}
	if err := positional(args, 0, 0); err != nil {
		return usageError("unexpected argument %q: the only subcommand is pin", args[0])
	}
	s, err := c.open(*dir)
	if err != nil {
		return err
	}
	w, err := c.gather(s, false)
	if err != nil {
		return err
	}
	cc := w.contract
	doc := contractDoc{Schema: "metron.contract-status/v1", Path: s.show(cc.path), Valid: cc.contract != nil,
		Problems: cc.problems, Digest: cc.digest, RawDigest: cc.rawDigest}
	if doc.Problems == nil {
		doc.Problems = []contractProblem{}
	}
	if cc.contract != nil {
		doc.Stage = cc.contract.Stage
	}
	switch pin, ok := w.state.Pin(); {
	case w.ledgerMissing:
		doc.LedgerProblem = "the ledger does not exist: run " + s.metron("init")
	case !w.facts.LedgerVerified:
		doc.LedgerProblem = "the ledger fails verification: " + w.facts.LedgerProblem
	case ok:
		doc.PinnedDigest = pin.Digest
	}
	doc.Pinned = doc.Valid && doc.PinnedDigest != "" && doc.PinnedDigest == doc.Digest
	if *asJSON {
		err = writeJSON(c.Stdout, doc)
	} else {
		err = printContract(c.Stdout, s, doc)
	}
	switch {
	case err != nil:
		return err
	case !doc.Pinned:
		return negative
	}
	return nil
}

func printContract(out io.Writer, s *site, d contractDoc) error {
	var b strings.Builder
	fmt.Fprintf(&b, "Contract: %s\n", d.Path)
	if len(d.Problems) > 0 {
		fmt.Fprintf(&b, "%s found by the %s layer:\n", plural(len(d.Problems), "problem"), d.Problems[0].Layer)
		for _, p := range d.Problems {
			fmt.Fprintf(&b, "  %s\n", p)
		}
	}
	fmt.Fprintf(&b, "Contract digest: %s\n", orNone(d.Digest))
	fmt.Fprintf(&b, "Raw digest:      %s\n", orNone(d.RawDigest))
	fmt.Fprintf(&b, "Pinned digest:   %s\n", orNone(d.PinnedDigest))
	if d.LedgerProblem != "" {
		fmt.Fprintf(&b, "Ledger:          %s\n", d.LedgerProblem)
	}
	pin := s.metron(`contract pin -m "<reason>"`)
	switch {
	case !d.Valid:
		fmt.Fprintf(&b, "Result: INVALID. Fix every problem listed, then run %s again.\n", s.metron("contract"))
	case d.Pinned:
		fmt.Fprintf(&b, "Result: valid and pinned, stage %s.\n", d.Stage)
	case d.PinnedDigest == "":
		fmt.Fprintf(&b, "Result: valid, not pinned. Governance authorizes it with: %s\n", pin)
	default:
		fmt.Fprintf(&b, "Result: valid, but its meaning differs from the pinned contract. Work waits until governance pins it: %s\n", pin)
	}
	_, err := io.WriteString(out, b.String())
	return err
}

func runPin(c *ctx, args []string) error {
	c.cmd = &command{name: "contract pin", synopsis: "contract pin -m <reason>", about: contractAbout}
	fs := c.flags()
	dir := stateFlag(fs)
	reason := fs.String("m", "", "why governance authorizes this contract (required)")
	args, err := c.parse(fs, args)
	if err != nil {
		return err
	}
	if err := positional(args, 0, 0); err != nil {
		return err
	}
	if strings.TrimSpace(*reason) == "" {
		return usageError(`missing -m "<reason>": say why governance authorizes this contract`)
	}
	s, err := c.open(*dir)
	if err != nil {
		return err
	}
	wr, w, err := c.lock(s, false)
	if err != nil {
		return err
	}
	defer wr.Close()
	if ps := w.contract.problems; len(ps) > 0 {
		var b strings.Builder
		fmt.Fprintf(&b, "%s cannot be pinned: %s found by the %s layer:\n", s.show(w.contract.path), plural(len(ps), "problem"), ps[0].Layer)
		for _, p := range ps {
			fmt.Fprintf(&b, "  %s\n", p)
		}
		b.WriteString("Fix them, then pin again.")
		return fail(exitNegative, "%s", b.String())
	}
	name, email, err := s.repo.Identity()
	if err != nil {
		return fail(exitEnv, "%v\nThe pin records who authorized the contract.", err)
	}
	drafts, err := core.Pin(w.state, w.facts, *reason, fmt.Sprintf("%s <%s>", name, email))
	if err != nil {
		return err
	}
	events, err := w.commit(wr, drafts)
	if err != nil {
		return err
	}
	var b strings.Builder
	fmt.Fprintf(&b, "Pinned the contract for stage %s.\n", w.facts.Contract.Stage)
	for _, e := range events {
		fmt.Fprintf(&b, "  %s\n", describe(e))
	}
	b.WriteString("\n")
	writeNext(&b, w, w.next())
	_, err = io.WriteString(c.Stdout, b.String())
	return err
}

func orNone(s string) string {
	if s == "" {
		return "none"
	}
	return s
}

func plural(n int, noun string) string {
	if n == 1 {
		return "1 " + noun
	}
	return fmt.Sprintf("%d %ss", n, noun)
}
