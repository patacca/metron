package store

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/patacca/metron/internal/canon"
	"github.com/patacca/metron/internal/core"
)

// TimeFormat is the layout of an event's time: RFC 3339 UTC with second
// precision.
const TimeFormat = "2006-01-02T15:04:05Z"

// GenesisPrev is the prev value of the first event.
const GenesisPrev = "0000000000000000000000000000000000000000000000000000000000000000"

// hashMarker introduces the hash member. The hash covers a line up to its
// last occurrence.
const hashMarker = `,"hash":"`

// LedgerError reports the first line of a ledger that fails verification.
type LedgerError struct {
	// Line is 1-based.
	Line int
	Msg  string
}

func (e *LedgerError) Error() string {
	return fmt.Sprintf("ledger line %d: %s", e.Line, e.Msg)
}

// VerifyLedger parses ledger bytes and verifies every line: the envelope
// form, sequence numbers, prev links and hashes, as docs/architecture.md
// section 7 defines them. Lines may end in CRLF. Empty data is a ledger with
// no events. The error, if any, is a *LedgerError for the first line that
// fails.
func VerifyLedger(data []byte) ([]core.Event, error) {
	if len(data) == 0 {
		return nil, nil
	}
	// The last element is what follows the final newline: nothing, in a
	// complete ledger.
	lines := bytes.Split(data, []byte("\n"))
	events := make([]core.Event, 0, len(lines)-1)
	prev := GenesisPrev
	for i, line := range lines {
		if i == len(lines)-1 {
			if len(line) != 0 {
				return nil, &LedgerError{Line: i + 1, Msg: "no newline at the end of the line; the ledger is truncated or was edited"}
			}
			break
		}
		line = bytes.TrimSuffix(line, []byte("\r"))
		e, err := parseLine(line)
		if err != nil {
			return nil, &LedgerError{Line: i + 1, Msg: err.Error()}
		}
		if e.Seq != i+1 {
			return nil, &LedgerError{Line: i + 1, Msg: fmt.Sprintf("seq is %d, want %d", e.Seq, i+1)}
		}
		if e.Prev != prev {
			return nil, &LedgerError{Line: i + 1, Msg: "prev does not match the hash of the previous event"}
		}
		prev = e.Hash
		events = append(events, e)
	}
	return events, nil
}

// parseLine decodes one ledger line without its line terminator and checks
// that it is exactly what the writer produces for the decoded event, and that
// its hash is correct.
func parseLine(line []byte) (core.Event, error) {
	if !utf8.Valid(line) {
		return core.Event{}, fmt.Errorf("invalid UTF-8")
	}
	var e core.Event
	if err := json.Unmarshal(line, &e); err != nil {
		return core.Event{}, fmt.Errorf("not a valid event: %v", err)
	}
	if err := checkEvent(e); err != nil {
		return core.Event{}, err
	}
	if !isHex64(e.Hash) {
		return core.Event{}, fmt.Errorf("hash is not 64 lowercase hexadecimal digits")
	}
	if !bytes.Equal(appendLine(nil, e), line) {
		return core.Event{}, fmt.Errorf("not in envelope form: members seq, time, prev, type, data, hash in this order, without whitespace")
	}
	// The hash covers the line up to the last occurrence of the marker,
	// followed by a closing brace.
	cut := bytes.LastIndex(line, []byte(hashMarker))
	sum := sha256.Sum256(append(bytes.Clone(line[:cut]), '}'))
	if hex.EncodeToString(sum[:]) != e.Hash {
		return core.Event{}, fmt.Errorf("hash does not match the content of the line")
	}
	return e, nil
}

// checkEvent validates the members of an event other than its hash, so that
// appendLine can write them without escaping.
func checkEvent(e core.Event) error {
	if e.Seq < 1 {
		return fmt.Errorf("seq %d is not positive", e.Seq)
	}
	if t, err := time.Parse(TimeFormat, e.Time); err != nil || t.Format(TimeFormat) != e.Time {
		return fmt.Errorf("time %q is not RFC 3339 UTC with second precision", e.Time)
	}
	if !isHex64(e.Prev) {
		return fmt.Errorf("prev is not 64 lowercase hexadecimal digits")
	}
	if e.Type == "" || strings.Trim(string(e.Type), "abcdefghijklmnopqrstuvwxyz0123456789.-") != "" {
		return fmt.Errorf("type %q is not made of lowercase letters, digits, '.' and '-'", e.Type)
	}
	if len(e.Data) == 0 || e.Data[0] != '{' {
		return fmt.Errorf("data is not a JSON object")
	}
	var compact bytes.Buffer
	if err := json.Compact(&compact, e.Data); err != nil || !bytes.Equal(compact.Bytes(), e.Data) {
		return fmt.Errorf("data is not compact JSON")
	}
	return nil
}

// appendLine writes an event in envelope form, without a line terminator.
// The members of e must have passed checkEvent.
func appendLine(b []byte, e core.Event) []byte {
	b = appendBody(b, e)
	b = b[:len(b)-1] // the closing brace of the body
	b = append(b, hashMarker...)
	b = append(b, e.Hash...)
	return append(b, '"', '}')
}

// appendBody writes the part of an event's line its hash covers: the line up
// to the hash member, closed with a brace.
func appendBody(b []byte, e core.Event) []byte {
	b = append(b, `{"seq":`...)
	b = strconv.AppendInt(b, int64(e.Seq), 10)
	b = append(b, `,"time":"`...)
	b = append(b, e.Time...)
	b = append(b, `","prev":"`...)
	b = append(b, e.Prev...)
	b = append(b, `","type":"`...)
	b = append(b, e.Type...)
	b = append(b, `","data":`...)
	b = append(b, e.Data...)
	return append(b, '}')
}

// Seal turns drafts into events that extend the ledger whose last event is
// prev, or start a ledger when prev is nil. Every event gets the given time,
// which must be RFC 3339 UTC with second precision (TimeFormat). A payload is
// written in its canonical JSON form.
func Seal(prev *core.Event, time string, drafts []core.Draft) ([]core.Event, error) {
	seq, hash := 0, GenesisPrev
	if prev != nil {
		seq, hash = prev.Seq, prev.Hash
	}
	events := make([]core.Event, 0, len(drafts))
	for _, d := range drafts {
		data, err := canon.Marshal(d.Data)
		if err != nil {
			return nil, fmt.Errorf("seal %s event: payload: %w", d.Type, err)
		}
		e := core.Event{Seq: seq + 1, Time: time, Prev: hash, Type: d.Type, Data: data}
		if err := checkEvent(e); err != nil {
			return nil, fmt.Errorf("seal %s event: %w", d.Type, err)
		}
		sum := sha256.Sum256(appendBody(nil, e))
		e.Hash = hex.EncodeToString(sum[:])
		events = append(events, e)
		seq, hash = e.Seq, e.Hash
	}
	return events, nil
}

func isHex64(s string) bool {
	if len(s) != 64 {
		return false
	}
	for i := 0; i < len(s); i++ {
		if c := s[i]; !('0' <= c && c <= '9' || 'a' <= c && c <= 'f') {
			return false
		}
	}
	return true
}
