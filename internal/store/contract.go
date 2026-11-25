package store

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"unicode/utf8"

	"go.yaml.in/yaml/v3"

	"github.com/patacca/metron/internal/canon"
)

// ContractFile is an acceptance contract read strictly from YAML. It has not
// been validated against the contract schema or the semantic rules.
type ContractFile struct {
	// Canonical is the canonical JSON form of the document.
	Canonical []byte
	// Digest is the contract digest: the digest of Canonical, the binding
	// identity of the contract.
	Digest string
}

// ContractError reports where a contract breaks the strict YAML rules.
type ContractError struct {
	// Line and Column are 1-based.
	Line, Column int
	Msg          string
}

func (e *ContractError) Error() string {
	return fmt.Sprintf("line %d, column %d: %s", e.Line, e.Column, e.Msg)
}

// ParseContract parses a contract as strict YAML, as docs/architecture.md
// section 6 step 1 defines it: a single document; mappings with unique
// string keys; no anchors, aliases, merge keys or tags; scalars limited to
// strings, decimal integers and booleans. A plain scalar that YAML reads as
// null, a float or a date is rejected rather than converted.
func ParseContract(data []byte) (*ContractFile, error) {
	dec := yaml.NewDecoder(bytes.NewReader(data))
	var doc yaml.Node
	if err := dec.Decode(&doc); err != nil {
		if errors.Is(err, io.EOF) {
			return nil, errors.New("the contract is empty; start from the template that metron init writes")
		}
		return nil, fmt.Errorf("invalid YAML: %w", err)
	}
	var next yaml.Node
	switch err := dec.Decode(&next); {
	case err == nil:
		return nil, errorAt(&next, "a second YAML document starts here; the contract must be a single document")
	case !errors.Is(err, io.EOF):
		return nil, fmt.Errorf("invalid YAML: %w", err)
	}
	if doc.Kind != yaml.DocumentNode || len(doc.Content) != 1 {
		return nil, errorAt(&doc, "expected a single YAML document")
	}
	value, err := jsonValue(doc.Content[0])
	if err != nil {
		return nil, err
	}
	canonical, err := canon.Encode(value)
	if err != nil {
		return nil, err
	}
	return &ContractFile{Canonical: canonical, Digest: canon.Digest(canonical)}, nil
}

func errorAt(n *yaml.Node, format string, args ...any) error {
	return &ContractError{Line: n.Line, Column: n.Column, Msg: fmt.Sprintf(format, args...)}
}

// jsonValue converts a node to its JSON value, enforcing the strict rules.
func jsonValue(n *yaml.Node) (any, error) {
	if err := plainNode(n); err != nil {
		return nil, err
	}
	switch n.Kind {
	case yaml.MappingNode:
		m := make(map[string]any, len(n.Content)/2)
		first := make(map[string]*yaml.Node, len(n.Content)/2)
		for i := 0; i+1 < len(n.Content); i += 2 {
			k, v := n.Content[i], n.Content[i+1]
			key, err := mappingKey(k)
			if err != nil {
				return nil, err
			}
			if f, dup := first[key]; dup {
				return nil, errorAt(k, "duplicate key %q, first defined at line %d, column %d; remove one of them", key, f.Line, f.Column)
			}
			first[key] = k
			if m[key], err = jsonValue(v); err != nil {
				return nil, err
			}
		}
		return m, nil
	case yaml.SequenceNode:
		a := make([]any, 0, len(n.Content))
		for _, e := range n.Content {
			v, err := jsonValue(e)
			if err != nil {
				return nil, err
			}
			a = append(a, v)
		}
		return a, nil
	case yaml.ScalarNode:
		return scalar(n)
	default:
		return nil, errorAt(n, "unexpected YAML node")
	}
}

// plainNode rejects aliases, anchors and explicit tags.
func plainNode(n *yaml.Node) error {
	switch {
	case n.Kind == yaml.AliasNode:
		return errorAt(n, "alias *%s is not allowed; write the value out in full", n.Value)
	case n.Anchor != "":
		return errorAt(n, "anchor &%s is not allowed; remove it and write repeated values out in full", n.Anchor)
	case n.Style&yaml.TaggedStyle != 0:
		return errorAt(n, "tag %s is not allowed; remove it, and quote the value if it must be a string", n.Tag)
	}
	return nil
}

func mappingKey(k *yaml.Node) (string, error) {
	if err := plainNode(k); err != nil {
		return "", err
	}
	switch {
	case k.Kind != yaml.ScalarNode:
		return "", errorAt(k, "a mapping key must be a string, not a collection")
	case k.Tag == "!!merge":
		return "", errorAt(k, "merge key << is not allowed; write the members out in full")
	case k.Tag != "!!str":
		return "", errorAt(k, "key %s is not a string; quote it", k.Value)
	case !utf8.ValidString(k.Value):
		return "", errorAt(k, "key is not valid UTF-8")
	}
	return k.Value, nil
}

func scalar(n *yaml.Node) (any, error) {
	switch n.Tag {
	case "!!str":
		if !utf8.ValidString(n.Value) {
			return nil, errorAt(n, "string is not valid UTF-8")
		}
		return n.Value, nil
	case "!!bool":
		return strings.EqualFold(n.Value, "true"), nil
	case "!!int":
		return integer(n)
	case "!!null":
		return nil, errorAt(n, "missing value (null); give a value, or remove the key")
	case "!!float":
		return nil, errorAt(n, "%s is read as a floating-point number, and only integers are allowed; quote it if it is a string", n.Value)
	case "!!timestamp":
		return nil, errorAt(n, "%s is read as a date; quote it", n.Value)
	case "!!merge":
		return nil, errorAt(n, "<< is read as a merge key; quote it if it is a string")
	default:
		return nil, errorAt(n, "unsupported value of type %s", n.Tag)
	}
}

// integer accepts only plain decimal integers in the canonical range, so that
// what the author reads is what is digested.
func integer(n *yaml.Node) (any, error) {
	digits := strings.TrimPrefix(n.Value, "-")
	if digits == "" || strings.Trim(digits, "0123456789") != "" || (len(digits) > 1 && digits[0] == '0') {
		return nil, errorAt(n, "integer %s must be written in plain decimal, without a plus sign, base prefix, underscores or leading zeros; quote it if it is a string", n.Value)
	}
	v, err := strconv.ParseInt(n.Value, 10, 64)
	if err != nil || v > canon.MaxInt || v < -canon.MaxInt {
		return nil, errorAt(n, "integer %s is outside the range -(2^53-1) to 2^53-1", n.Value)
	}
	return v, nil
}
