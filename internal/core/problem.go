package core

import (
	"fmt"
	"strings"
	"time"
	"unicode"
)

// Layer names the validation layer that refused a document or a command.
type Layer string

const (
	// LayerSemantic refuses content that is inconsistent in itself, with the
	// contract, or with the records it cites.
	LayerSemantic Layer = "semantic"
	// LayerTransition refuses what the current state does not permit.
	LayerTransition Layer = "transition"
)

// Problem is one reason for a refusal.
type Problem struct {
	// Path is a JSON Pointer into the refused document. It is empty when the
	// problem concerns the document as a whole or the command itself.
	Path    string `json:"path"`
	Message string `json:"message"`
}

// Rejection is the error returned when a validation or a command refuses. It
// carries every problem found by one layer, in a deterministic order. Use
// errors.As to unpack it.
//
// Commands return a *Rejection when they refuse on the merits. Any other error
// means the caller passed malformed facts or arguments, which is an internal
// error rather than a refusal.
type Rejection struct {
	Layer    Layer     `json:"layer"`
	Problems []Problem `json:"problems"`
}

func (r *Rejection) Error() string {
	var b strings.Builder
	fmt.Fprintf(&b, "rejected by %s validation", r.Layer)
	for _, p := range r.Problems {
		b.WriteString("\n  ")
		if p.Path != "" {
			b.WriteString(p.Path)
			b.WriteString(": ")
		}
		b.WriteString(p.Message)
	}
	return b.String()
}

// problems collects the problems of one layer in the order they are found.
type problems []Problem

func (ps *problems) add(path, format string, args ...any) {
	*ps = append(*ps, Problem{Path: path, Message: fmt.Sprintf(format, args...)})
}

func (ps problems) err(layer Layer) error {
	if len(ps) == 0 {
		return nil
	}
	return &Rejection{Layer: layer, Problems: ps}
}

// refuse returns a rejection with a single problem that concerns the command
// as a whole.
func refuse(layer Layer, format string, args ...any) error {
	var ps problems
	ps.add("", format, args...)
	return ps.err(layer)
}

// ptr builds a JSON Pointer from member names and array indices. Metron's
// member names never contain "~" or "/", so no escaping is needed.
func ptr(parts ...any) string {
	var b strings.Builder
	for _, p := range parts {
		b.WriteByte('/')
		fmt.Fprint(&b, p)
	}
	return b.String()
}

// blank reports whether s has no content: no rune that is graphic and not
// a space. U+00A0 is a space; U+200B, U+FEFF and U+2060 are format
// characters, not graphic, so none of them is content either.
func blank(s string) bool {
	for _, r := range s {
		if unicode.IsGraphic(r) && !unicode.IsSpace(r) {
			return false
		}
	}
	return true
}

// count writes n with the singular or the plural noun, as English needs.
func count(n int, one, many string) string {
	return fmt.Sprintf("%d %s", n, pick(n, one, many))
}

func lowerHex(s string) bool {
	for i := 0; i < len(s); i++ {
		c := s[i]
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') {
			return false
		}
	}
	return true
}

// validDigest reports whether d is "sha256:" followed by 64 lowercase hex
// digits.
func validDigest(d string) bool {
	h, ok := strings.CutPrefix(d, "sha256:")
	return ok && len(h) == 64 && lowerHex(h)
}

// validCommit reports whether h is a full SHA-1 or SHA-256 commit hash.
func validCommit(h string) bool {
	return (len(h) == 40 || len(h) == 64) && lowerHex(h)
}

func validCandidate(c Candidate) bool {
	return validCommit(c.Commit) && validDigest(c.Digest)
}

// candidateProblem says in words what is missing or wrong in a candidate, or
// returns "" when it is well formed.
func candidateProblem(c Candidate) string {
	var ps []string
	switch {
	case c.Commit == "":
		ps = append(ps, "the commit is unknown")
	case !validCommit(c.Commit):
		ps = append(ps, fmt.Sprintf("commit %q is not a full commit hash", c.Commit))
	}
	switch {
	case c.Digest == "":
		ps = append(ps, "the candidate digest is unknown")
	case !validDigest(c.Digest):
		ps = append(ps, fmt.Sprintf("candidate digest %q is not \"sha256:\" followed by 64 lowercase hex digits", c.Digest))
	}
	return strings.Join(ps, " and ")
}

const timeLayout = "2006-01-02T15:04:05Z"

// validTime reports whether t is RFC 3339 UTC with second precision.
func validTime(t string) bool {
	v, err := time.Parse(timeLayout, t)
	return err == nil && v.Format(timeLayout) == t
}

// wellFormedID reports whether id is prefix followed by one or more digits.
func wellFormedID(id, prefix string) bool {
	n, ok := strings.CutPrefix(id, prefix)
	if !ok || n == "" {
		return false
	}
	for i := 0; i < len(n); i++ {
		if n[i] < '0' || n[i] > '9' {
			return false
		}
	}
	return true
}

// seqID formats a ledger identifier: a prefix and a sequence number
// zero-padded to at least four digits.
func seqID(prefix string, n int) string { return fmt.Sprintf("%s-%04d", prefix, n) }

// idLess orders ledger identifiers that share a prefix by sequence number.
// Padding to at least four digits makes length, then text, numeric order.
func idLess(a, b string) bool {
	if len(a) != len(b) {
		return len(a) < len(b)
	}
	return a < b
}
