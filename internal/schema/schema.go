// Package schema performs the structural validation of Metron's public
// documents against the JSON Schemas embedded from package schemas.
//
// It is the first of the three layers a document passes: it checks types,
// required members, unknown members, enumerations and identifier formats, and
// nothing else. Rules that relate members to each other or to the ledger
// belong to the semantic and transition layers.
//
// Every schema is loaded from the embedded files. Nothing is ever fetched from
// the network or read from the file system.
package schema

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"math/big"
	"sort"
	"strings"
	"sync"

	"github.com/patacca/metron/schemas"
	"github.com/santhosh-tekuri/jsonschema/v6"
	"github.com/santhosh-tekuri/jsonschema/v6/kind"
)

// Kind names a document type that has a schema.
type Kind string

// The document kinds.
const (
	Contract Kind = "contract"
	Report   Kind = "report"
	Packet   Kind = "packet"
	Event    Kind = "event"
)

// Kinds returns the document kinds in a fixed order.
func Kinds() []Kind { return []Kind{Contract, Report, Packet, Event} }

// Problem is one structural defect of a document.
type Problem struct {
	// Pointer is the RFC 6901 JSON Pointer to the offending location. The
	// empty string is the document itself. For a missing member it points to
	// the object that lacks it; for an unknown member, to the member.
	Pointer string
	// Message says what is wrong and what to do about it.
	Message string
}

// String renders the problem as "pointer: message".
func (p Problem) String() string {
	ptr := p.Pointer
	if ptr == "" {
		ptr = "(document)"
	}
	return ptr + ": " + p.Message
}

const (
	idPrefix = "https://github.com/patacca/metron/schemas/v1/"
	fileExt  = ".schema.json"
	dir      = "v1/"
)

var (
	once     sync.Once
	compiled map[Kind]*jsonschema.Schema
	loadErr  error
)

// refuseLoader makes sure nothing outside the embedded files is ever loaded:
// every resource is registered up front, so any other URL is an error.
type refuseLoader struct{}

func (refuseLoader) Load(url string) (any, error) {
	return nil, fmt.Errorf("schema %q is not one of the embedded Metron schemas", url)
}

func load() {
	c := jsonschema.NewCompiler()
	c.UseLoader(refuseLoader{})
	for _, n := range Names() {
		raw, err := Bytes(n)
		if err != nil {
			loadErr = err
			return
		}
		doc, err := jsonschema.UnmarshalJSON(bytes.NewReader(raw))
		if err != nil {
			loadErr = fmt.Errorf("schema %s: %w", n, err)
			return
		}
		if err := c.AddResource(idPrefix+n+fileExt, doc); err != nil {
			loadErr = fmt.Errorf("schema %s: %w", n, err)
			return
		}
	}
	compiled = make(map[Kind]*jsonschema.Schema, len(Kinds()))
	for _, k := range Kinds() {
		s, err := c.Compile(idPrefix + string(k) + fileExt)
		if err != nil {
			loadErr = fmt.Errorf("schema %s: %w", k, err)
			return
		}
		compiled[k] = s
	}
}

// Names returns the names of the embedded schemas, sorted: the four document
// kinds and "defs", the definitions they share.
func Names() []string {
	entries, err := fs.ReadDir(schemas.FS, strings.TrimSuffix(dir, "/"))
	if err != nil {
		return nil
	}
	var names []string
	for _, e := range entries {
		if n, ok := strings.CutSuffix(e.Name(), fileExt); ok && !e.IsDir() {
			names = append(names, n)
		}
	}
	sort.Strings(names)
	return names
}

// Bytes returns the bytes of the named schema. It fails for a name that
// Names does not list.
func Bytes(name string) ([]byte, error) {
	for _, n := range Names() {
		if n == name {
			return schemas.FS.ReadFile(dir + name + fileExt)
		}
	}
	return nil, fmt.Errorf("unknown schema %q: choose one of %s", name, strings.Join(Names(), ", "))
}

func schemaFor(k Kind) (*jsonschema.Schema, error) {
	once.Do(load)
	if loadErr != nil {
		return nil, loadErr
	}
	s, ok := compiled[k]
	if !ok {
		return nil, fmt.Errorf("unknown document kind %q: choose one of contract, report, packet, event", k)
	}
	return s, nil
}

// Validate checks a decoded JSON value against the schema of kind k and
// returns every problem found, ordered by pointer and then by message. It
// returns no problems exactly when the value is structurally valid.
//
// The value is what encoding/json yields for an empty interface, with
// numbers as float64 or json.Number. The error is non-nil only if k is not a
// kind or the embedded schemas cannot be compiled; in both cases the document
// must be treated as invalid.
func Validate(k Kind, doc any) ([]Problem, error) {
	s, err := schemaFor(k)
	if err != nil {
		return nil, err
	}
	err = s.Validate(doc)
	if err == nil {
		return nil, nil
	}
	var verr *jsonschema.ValidationError
	if !errors.As(err, &verr) {
		return nil, err
	}
	return flatten(verr), nil
}

// ValidateJSON decodes data, which must hold exactly one JSON value, and
// validates it as Validate does. Text that is not JSON is reported as a
// problem, not as an error.
func ValidateJSON(k Kind, data []byte) ([]Problem, error) {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	var doc any
	if err := dec.Decode(&doc); err != nil {
		return syntax(k, err)
	}
	if _, err := dec.Token(); !errors.Is(err, io.EOF) {
		return syntax(k, errors.New("unexpected data after the first JSON value"))
	}
	return Validate(k, doc)
}

func syntax(k Kind, err error) ([]Problem, error) {
	if _, e := schemaFor(k); e != nil {
		return nil, e
	}
	return []Problem{{Message: "not valid JSON: " + err.Error() + "; fix the syntax and submit again"}}, nil
}

// flatten collects the leaf errors of the tree, one problem each.
func flatten(e *jsonschema.ValidationError) []Problem {
	var out []Problem
	var walk func(*jsonschema.ValidationError)
	walk = func(e *jsonschema.ValidationError) {
		if len(e.Causes) > 0 {
			for _, c := range e.Causes {
				walk(c)
			}
			return
		}
		out = append(out, problems(e)...)
	}
	walk(e)
	sort.SliceStable(out, func(i, j int) bool { return less(out[i], out[j]) })
	uniq := out[:0]
	for i, p := range out {
		if i == 0 || p != out[i-1] {
			uniq = append(uniq, p)
		}
	}
	return uniq
}

// less orders problems in document order: pointers are compared segment by
// segment, two segments numerically when both are array indices (indices come first)
// and as text otherwise, a parent before its members; equal pointers by message.
func less(a, b Problem) bool {
	if c := comparePointers(a.Pointer, b.Pointer); c != 0 {
		return c < 0
	}
	return a.Message < b.Message
}

func comparePointers(a, b string) int {
	as, bs := strings.Split(a, "/"), strings.Split(b, "/")
	for i := 0; i < len(as) && i < len(bs); i++ {
		if c := compareSegments(as[i], bs[i]); c != 0 {
			return c
		}
	}
	return len(as) - len(bs)
}

// compareSegments is a total order: array indices first, numerically, then
// everything else as text. Mixing the two orders on one level would not be
// transitive.
func compareSegments(a, b string) int {
	ia, ib := isIndex(a), isIndex(b)
	switch {
	case ia && ib:
		ta, tb := strings.TrimLeft(a, "0"), strings.TrimLeft(b, "0")
		if len(ta) != len(tb) {
			return len(ta) - len(tb)
		}
		if c := strings.Compare(ta, tb); c != 0 {
			return c
		}
	case ia:
		return -1
	case ib:
		return 1
	}
	return strings.Compare(a, b)
}

func isIndex(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

func pointer(tokens []string) string {
	var b strings.Builder
	for _, t := range tokens {
		b.WriteByte('/')
		b.WriteString(escape(t))
	}
	return b.String()
}

var escaper = strings.NewReplacer("~", "~0", "/", "~1")

func escape(t string) string { return escaper.Replace(t) }

// patternHints says in words what each pattern of the schemas requires. A
// test keeps it complete.
var patternHints = map[string]string{
	`^[!-~]{1,128}$`:                "must be 1 to 128 printable ASCII characters without spaces (0x21 to 0x7E)",
	`\S`:                            "must not be blank",
	`^[0-9a-f]{64}$`:                "must be 64 lowercase hexadecimal digits",
	`^sha256:[0-9a-f]{64}$`:         `must be "sha256:" followed by 64 lowercase hexadecimal digits`,
	`^([0-9a-f]{40}|[0-9a-f]{64})$`: "must be a full commit hash: 40 or 64 lowercase hexadecimal digits",
	`^F-[0-9]{4,}$`:                 `must be a finding identifier such as "F-0001"`,
	`^P-[0-9]{4,}$`:                 `must be a packet identifier such as "P-0001"`,
	`^E-[0-9]{4,}$`:                 `must be an escalation identifier such as "E-0001"`,
	`^REQ-[0-9]+$`:                  `must be a requirement identifier such as "REQ-1"`,
	`^RISK-[0-9]+$`:                 `must be an unacceptable-risk identifier such as "RISK-1"`,
	`^ACC-[0-9]+$`:                  `must be an accepted-risk identifier such as "ACC-1"`,
	`^CAL-[0-9]+$`:                  `must be a calibration identifier such as "CAL-1"`,
	timePattern:                     `must be an RFC 3339 UTC time with second precision, such as "2026-01-31T09:30:00Z"`,
}

const timePattern = `^[0-9]{4}-(0[1-9]|1[0-2])-(0[1-9]|[12][0-9]|3[01])T([01][0-9]|2[0-3]):[0-5][0-9]:[0-5][0-9]Z$`

func quoteList(vals []any) string {
	parts := make([]string, len(vals))
	for i, v := range vals {
		parts[i] = fmt.Sprintf("%q", fmt.Sprint(v))
	}
	return strings.Join(parts, ", ")
}

func ratString(r *big.Rat) string { return r.RatString() }

func plural(n int) string {
	if n == 1 {
		return "1 item"
	}
	return fmt.Sprintf("%d items", n)
}

// problems turns one leaf error into the problems it stands for.
func problems(e *jsonschema.ValidationError) []Problem {
	at := pointer(e.InstanceLocation)
	one := func(msg string) []Problem { return []Problem{{Pointer: at, Message: msg}} }
	switch k := e.ErrorKind.(type) {
	case *kind.Type:
		return one(fmt.Sprintf("must be of type %s, got %s", strings.Join(k.Want, " or "), k.Got))
	case *kind.Enum:
		return one("must be one of " + quoteList(k.Want))
	case *kind.Const:
		return one(fmt.Sprintf("must be %q", fmt.Sprint(k.Want)))
	case *kind.Required:
		out := make([]Problem, 0, len(k.Missing))
		for _, m := range k.Missing {
			out = append(out, Problem{Pointer: at, Message: fmt.Sprintf("missing required member %q; add it", m)})
		}
		return out
	case *kind.AdditionalProperties:
		out := make([]Problem, 0, len(k.Properties))
		for _, p := range k.Properties {
			out = append(out, Problem{Pointer: at + "/" + escape(p), Message: "unknown member; remove it or check its spelling"})
		}
		return out
	case *kind.Pattern:
		if h, ok := patternHints[k.Want]; ok {
			return one(h)
		}
		return one(fmt.Sprintf("must match the pattern %s", k.Want))
	case *kind.MinItems:
		return one(fmt.Sprintf("must hold at least %s, got %d", plural(k.Want), k.Got))
	case *kind.Minimum:
		return one(fmt.Sprintf("must be at least %s, got %s", ratString(k.Want), ratString(k.Got)))
	case *kind.InvalidJsonValue:
		return one(fmt.Sprintf("is not a JSON value: %v", k.Value))
	}
	return one("violates the schema keyword " + strings.Join(e.ErrorKind.KeywordPath(), "/"))
}
