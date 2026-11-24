// Package canon implements the canonical JSON form Metron digests, RFC 8785
// (JCS), for the subset of JSON that Metron documents use: objects, arrays,
// strings, booleans, null and integers.
//
// Values are represented as map[string]any, []any, string, bool, nil and
// int64. Integers are limited to the range an IEEE 754 double represents
// exactly, ±(2^53−1), so that the canonical form is the one RFC 8785 gives.
// Numbers with a fraction or an exponent are rejected, as are invalid UTF-8,
// unpaired surrogate escapes and duplicate object members.
//
// The package performs no I/O.
package canon

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"slices"
	"strconv"
	"unicode/utf16"
	"unicode/utf8"
)

// MaxInt is the largest integer magnitude the canonical form accepts.
const MaxInt = 1<<53 - 1

// maxDepth bounds the nesting of arrays and objects, so that hostile input
// cannot exhaust the stack.
const maxDepth = 1000

// SyntaxError reports where and why input is not acceptable strict JSON.
type SyntaxError struct {
	// Line and Column are 1-based; Column counts bytes.
	Line, Column int
	Msg          string
}

func (e *SyntaxError) Error() string {
	return fmt.Sprintf("line %d, column %d: %s", e.Line, e.Column, e.Msg)
}

// Parse parses untrusted JSON strictly. It rejects duplicate object members
// at any depth, data after the value, numbers that are not integers in range,
// invalid UTF-8, unpaired surrogate escapes and a byte order mark.
func Parse(data []byte) (any, error) {
	p := parser{data: data}
	p.space()
	v, err := p.value(0)
	if err != nil {
		return nil, err
	}
	p.space()
	if p.pos != len(p.data) {
		return nil, p.errorf("unexpected data after the JSON value")
	}
	return v, nil
}

// Encode returns the canonical form of a value built from map[string]any,
// []any, string, bool, nil and int64.
func Encode(v any) ([]byte, error) {
	return appendValue(nil, v, 0)
}

// Canonicalize parses data strictly and returns its canonical form.
func Canonicalize(data []byte) ([]byte, error) {
	v, err := Parse(data)
	if err != nil {
		return nil, err
	}
	return Encode(v)
}

// Marshal returns the canonical form of a Go value as encoding/json would
// represent it.
func Marshal(v any) ([]byte, error) {
	data, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	return Canonicalize(data)
}

// Digest returns the SHA-256 of data written "sha256:" followed by lowercase
// hex.
func Digest(data []byte) string {
	sum := sha256.Sum256(data)
	return "sha256:" + hex.EncodeToString(sum[:])
}

type parser struct {
	data []byte
	pos  int
}

func (p *parser) errorf(format string, args ...any) error {
	line, col := 1, 1
	for _, c := range p.data[:min(p.pos, len(p.data))] {
		if c == '\n' {
			line++
			col = 1
		} else {
			col++
		}
	}
	return &SyntaxError{Line: line, Column: col, Msg: fmt.Sprintf(format, args...)}
}

func (p *parser) space() {
	for p.pos < len(p.data) {
		switch p.data[p.pos] {
		case ' ', '\t', '\n', '\r':
			p.pos++
		default:
			return
		}
	}
}

func (p *parser) value(depth int) (any, error) {
	if p.pos >= len(p.data) {
		return nil, p.errorf("unexpected end of input, expected a value")
	}
	switch c := p.data[p.pos]; {
	case c == '{':
		return p.object(depth + 1)
	case c == '[':
		return p.array(depth + 1)
	case c == '"':
		return p.string()
	case c == '-' || ('0' <= c && c <= '9'):
		return p.number()
	case c == 't':
		return true, p.literal("true")
	case c == 'f':
		return false, p.literal("false")
	case c == 'n':
		return nil, p.literal("null")
	default:
		return nil, p.errorf("unexpected character %q, expected a value", rune(c))
	}
}

func (p *parser) literal(word string) error {
	if !bytes.HasPrefix(p.data[p.pos:], []byte(word)) {
		return p.errorf("invalid literal, expected %s", word)
	}
	p.pos += len(word)
	return nil
}

func (p *parser) object(depth int) (any, error) {
	if depth > maxDepth {
		return nil, p.errorf("nesting deeper than %d levels", maxDepth)
	}
	p.pos++ // {
	m := map[string]any{}
	p.space()
	if p.pos < len(p.data) && p.data[p.pos] == '}' {
		p.pos++
		return m, nil
	}
	for {
		if p.pos >= len(p.data) || p.data[p.pos] != '"' {
			return nil, p.errorf("expected a member name in double quotes")
		}
		keyPos := p.pos
		key, err := p.string()
		if err != nil {
			return nil, err
		}
		if _, dup := m[key]; dup {
			p.pos = keyPos
			return nil, p.errorf("duplicate member %q", key)
		}
		p.space()
		if p.pos >= len(p.data) || p.data[p.pos] != ':' {
			return nil, p.errorf("expected ':' after member name")
		}
		p.pos++
		p.space()
		v, err := p.value(depth)
		if err != nil {
			return nil, err
		}
		m[key] = v
		p.space()
		if p.pos >= len(p.data) {
			return nil, p.errorf("unexpected end of input in object")
		}
		switch p.data[p.pos] {
		case ',':
			p.pos++
			p.space()
		case '}':
			p.pos++
			return m, nil
		default:
			return nil, p.errorf("expected ',' or '}' in object")
		}
	}
}

func (p *parser) array(depth int) (any, error) {
	if depth > maxDepth {
		return nil, p.errorf("nesting deeper than %d levels", maxDepth)
	}
	p.pos++ // [
	a := []any{}
	p.space()
	if p.pos < len(p.data) && p.data[p.pos] == ']' {
		p.pos++
		return a, nil
	}
	for {
		p.space()
		v, err := p.value(depth)
		if err != nil {
			return nil, err
		}
		a = append(a, v)
		p.space()
		if p.pos >= len(p.data) {
			return nil, p.errorf("unexpected end of input in array")
		}
		switch p.data[p.pos] {
		case ',':
			p.pos++
		case ']':
			p.pos++
			return a, nil
		default:
			return nil, p.errorf("expected ',' or ']' in array")
		}
	}
}

func (p *parser) number() (any, error) {
	start := p.pos
	if p.data[p.pos] == '-' {
		p.pos++
	}
	digits := p.pos
	for p.pos < len(p.data) && '0' <= p.data[p.pos] && p.data[p.pos] <= '9' {
		p.pos++
	}
	switch n := p.pos - digits; {
	case n == 0:
		return nil, p.errorf("invalid number: expected a digit")
	case n > 1 && p.data[digits] == '0':
		p.pos = start
		return nil, p.errorf("invalid number: leading zero")
	}
	if p.pos < len(p.data) {
		switch p.data[p.pos] {
		case '.', 'e', 'E':
			p.pos = start
			return nil, p.errorf("number with a fraction or exponent: only integers are allowed")
		}
	}
	text := string(p.data[start:p.pos])
	n, err := strconv.ParseInt(text, 10, 64)
	if err != nil || n > MaxInt || n < -MaxInt {
		p.pos = start
		return nil, p.errorf("integer %s is outside the range -(2^53-1) to 2^53-1", text)
	}
	return n, nil
}

// string parses a quoted string at p.pos.
func (p *parser) string() (string, error) {
	p.pos++ // opening quote
	var b []byte
	for {
		if p.pos >= len(p.data) {
			return "", p.errorf("unterminated string")
		}
		c := p.data[p.pos]
		switch {
		case c == '"':
			p.pos++
			return string(b), nil
		case c == '\\':
			if err := p.escape(&b); err != nil {
				return "", err
			}
		case c < 0x20:
			return "", p.errorf("control character %U in string must be escaped", rune(c))
		case c < utf8.RuneSelf:
			b = append(b, c)
			p.pos++
		default:
			r, size := utf8.DecodeRune(p.data[p.pos:])
			if r == utf8.RuneError && size == 1 {
				return "", p.errorf("invalid UTF-8 in string")
			}
			b = append(b, p.data[p.pos:p.pos+size]...)
			p.pos += size
		}
	}
}

func (p *parser) escape(b *[]byte) error {
	if p.pos+1 >= len(p.data) {
		return p.errorf("unterminated escape sequence")
	}
	var c byte
	switch e := p.data[p.pos+1]; e {
	case '"', '\\', '/':
		c = e
	case 'b':
		c = '\b'
	case 'f':
		c = '\f'
	case 'n':
		c = '\n'
	case 'r':
		c = '\r'
	case 't':
		c = '\t'
	case 'u':
		return p.unicodeEscape(b)
	default:
		return p.errorf("invalid escape sequence \\%c", rune(e))
	}
	*b = append(*b, c)
	p.pos += 2
	return nil
}

// unicodeEscape decodes a \uXXXX escape at p.pos, or a surrogate pair of two.
func (p *parser) unicodeEscape(b *[]byte) error {
	r, err := p.hex4(p.pos + 2)
	if err != nil {
		return err
	}
	switch {
	case utf16.IsSurrogate(r) && r < 0xDC00:
		// A high surrogate must be followed by an escaped low surrogate.
		if p.pos+8 <= len(p.data) && p.data[p.pos+6] == '\\' && p.data[p.pos+7] == 'u' {
			low, err := p.hex4(p.pos + 8)
			if err != nil {
				return err
			}
			if pair := utf16.DecodeRune(r, low); pair != utf8.RuneError {
				*b = utf8.AppendRune(*b, pair)
				p.pos += 12
				return nil
			}
		}
		return p.errorf("unpaired surrogate \\u%04x in string", r)
	case utf16.IsSurrogate(r):
		return p.errorf("unpaired surrogate \\u%04x in string", r)
	}
	*b = utf8.AppendRune(*b, r)
	p.pos += 6
	return nil
}

// hex4 decodes the four hex digits at data[at:at+4].
func (p *parser) hex4(at int) (rune, error) {
	if at+4 > len(p.data) {
		return 0, p.errorf("truncated \\u escape")
	}
	v, err := strconv.ParseUint(string(p.data[at:at+4]), 16, 16)
	if err != nil {
		return 0, p.errorf("invalid \\u escape")
	}
	return rune(v), nil
}

func appendValue(b []byte, v any, depth int) ([]byte, error) {
	if depth > maxDepth {
		return nil, fmt.Errorf("canon: nesting deeper than %d levels", maxDepth)
	}
	switch v := v.(type) {
	case nil:
		return append(b, "null"...), nil
	case bool:
		return strconv.AppendBool(b, v), nil
	case int64:
		if v > MaxInt || v < -MaxInt {
			return nil, fmt.Errorf("canon: integer %d is outside the range -(2^53-1) to 2^53-1", v)
		}
		return strconv.AppendInt(b, v, 10), nil
	case string:
		return appendString(b, v)
	case []any:
		b = append(b, '[')
		for i, e := range v {
			if i > 0 {
				b = append(b, ',')
			}
			var err error
			if b, err = appendValue(b, e, depth+1); err != nil {
				return nil, err
			}
		}
		return append(b, ']'), nil
	case map[string]any:
		type member struct {
			name  string
			units []uint16
		}
		members := make([]member, 0, len(v))
		for k := range v {
			members = append(members, member{k, utf16.Encode([]rune(k))})
		}
		// RFC 8785 section 3.2.3: sort by the UTF-16 code units of the names.
		slices.SortFunc(members, func(a, b member) int { return slices.Compare(a.units, b.units) })
		b = append(b, '{')
		for i, m := range members {
			if i > 0 {
				b = append(b, ',')
			}
			var err error
			if b, err = appendString(b, m.name); err != nil {
				return nil, err
			}
			b = append(b, ':')
			if b, err = appendValue(b, v[m.name], depth+1); err != nil {
				return nil, err
			}
		}
		return append(b, '}'), nil
	default:
		return nil, fmt.Errorf("canon: unsupported type %T", v)
	}
}

// appendString writes s as RFC 8785 section 3.2.2.2 requires: the two-letter
// escapes for '"', '\\', \b, \f, \n, \r and \t, \u00xx in lowercase hex for
// the other control characters, and everything else as UTF-8.
func appendString(b []byte, s string) ([]byte, error) {
	if !utf8.ValidString(s) {
		return nil, fmt.Errorf("canon: invalid UTF-8 in string %q", s)
	}
	const hexDigits = "0123456789abcdef"
	b = append(b, '"')
	for i := 0; i < len(s); i++ {
		switch c := s[i]; c {
		case '"', '\\':
			b = append(b, '\\', c)
		case '\b':
			b = append(b, '\\', 'b')
		case '\f':
			b = append(b, '\\', 'f')
		case '\n':
			b = append(b, '\\', 'n')
		case '\r':
			b = append(b, '\\', 'r')
		case '\t':
			b = append(b, '\\', 't')
		default:
			if c < 0x20 {
				b = append(b, '\\', 'u', '0', '0', hexDigits[c>>4], hexDigits[c&0xf])
			} else {
				b = append(b, c)
			}
		}
	}
	return append(b, '"'), nil
}
