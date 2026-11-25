package cli

import (
	"bytes"
	"encoding/json"
	jsonv2 "encoding/json/v2"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"github.com/patacca/metron/internal/brief"
	"github.com/patacca/metron/internal/canon"
	"github.com/patacca/metron/internal/core"
	"github.com/patacca/metron/internal/schema"
	"github.com/patacca/metron/internal/store"
)

var cmdNext = &command{
	name:     "next",
	synopsis: "next [--json]",
	summary:  "Print the next permitted action and the command that performs it",
	about: `Next prints the single next permitted action: the step, the role that owns it,
the reason, the packet, findings, escalations or requirements it concerns, and
the command that performs or continues it. It changes nothing and takes no
lock.

With --json it prints a metron.next/v1 document: step, owner, reason, packet,
findings, escalations, requirements and command.`,
	run: runNext,
}

func runNext(c *ctx, args []string) error {
	fs := c.flags()
	dir, asJSON := stateFlag(fs), c.jsonFlag(fs)
	args, err := c.parse(fs, args)
	if err != nil {
		return err
	}
	if err := positional(args, 0, 0); err != nil {
		return err
	}
	s, err := c.open(*dir)
	if err != nil {
		return err
	}
	w, err := c.gather(s)
	if err != nil {
		return err
	}
	a := w.next()
	if *asJSON {
		doc := w.actionDoc(a)
		doc.Schema = "metron.next/v1"
		return c.emit(doc)
	}
	var b strings.Builder
	writeNext(&b, w, w.actionDoc(a))
	_, err = io.WriteString(c.Stdout, b.String())
	return err
}

// Packet output formats.
const (
	formatJSON     = "json"
	formatPrompt   = "prompt"
	formatSkeleton = "skeleton"
)

var cmdPacket = &command{
	name:     "packet",
	synopsis: "packet [-o <file>] [--format json|prompt|skeleton]",
	summary:  "Issue, or return again, the packet for the next action",
	about: `Packet issues the packet for the next action (review, fix, verify or audit),
records it in the ledger and stores it under the state directory. While that
packet is open and still valid, it returns the same packet again, byte for
byte, so a crashed agent can simply be retried. An open packet that is no
longer valid, or whose stored file is missing or altered, is voided and a new
one is issued.

--format json (the default) prints the packet itself; prompt renders it as a
self-contained prompt for a person or an agent; skeleton prints a report
skeleton shaped for the packet, every judgment left empty. Write it under
.metron/work/, which -o creates and git ignores, or outside the repository: a
new file elsewhere in the working tree makes the tree unclean, which
invalidates a read-only role's packet.`,
	run: runPacket,
}

func runPacket(c *ctx, args []string) error {
	fs := c.flags()
	dir := stateFlag(fs)
	out := fs.String("o", "", "write to `file` instead of standard output")
	format := fs.String("format", formatJSON, "what to write: json, prompt or skeleton")
	args, err := c.parse(fs, args)
	if err != nil {
		return err
	}
	if err := positional(args, 0, 0); err != nil {
		return err
	}
	if !slices.Contains([]string{formatJSON, formatPrompt, formatSkeleton}, *format) {
		return usageError("unknown format %q: use json, prompt or skeleton", *format)
	}
	s, err := c.open(*dir)
	if err != nil {
		return err
	}
	wr, w, err := c.lock(s)
	if err != nil {
		return err
	}
	defer wr.Close()
	// Only a packet step, or the open packet itself, takes a packet; anything
	// else is refused with what next says instead.
	if a := w.next(); a.Step != core.StepSubmitReport {
		if _, ok := a.Step.PacketRole(); !ok {
			return fail(exitNegative, "the next action is %s, which takes no packet: %s\n%s", a.Step, a.Reason, w.howTo(a))
		}
	}
	if err := w.candidate(); err != nil {
		return err
	}
	plan, err := core.PlanPacket(w.state, w.facts)
	var r *core.Rejection
	if errors.As(err, &r) {
		return fail(exitNegative, "%s%s", rejectionText(r), w.howTo(w.next()))
	}
	if err != nil {
		return err
	}
	var data []byte
	note := ""
	if plan.Open != "" {
		if data, err = s.store.ReadPacket(plan.Open); err != nil {
			return err
		}
		rec, _ := w.state.Packet(plan.Open)
		note = fmt.Sprintf("packet %s, digest %s, is open and still valid; returning it again", plan.Open, rec.Digest)
	} else {
		if data, err = issue(w, wr, plan.Packet); err != nil {
			return err
		}
		p := plan.Packet
		note = fmt.Sprintf("issued packet %s, digest %s: %s for the %s, round %d", p.ID, canon.Digest(data), p.Action, p.Role, p.Round)
	}
	var p core.Packet
	if err := jsonv2.Unmarshal(data, &p, jsonv2.RejectUnknownMembers(true)); err != nil {
		return fmt.Errorf("decode the stored packet: %w", err)
	}
	// A report names the digest recorded when its packet was issued.
	rec, ok := w.state.Packet(p.ID)
	if !ok {
		return fmt.Errorf("packet %s is not in the ledger", p.ID)
	}
	var rendered []byte
	switch *format {
	case formatJSON:
		var b bytes.Buffer
		if err := json.Indent(&b, data, "", "  "); err != nil {
			return err
		}
		rendered = append(b.Bytes(), '\n')
	case formatPrompt:
		text, err := brief.Prompt(p, rec.Digest)
		if err != nil {
			return err
		}
		rendered = []byte(text)
	case formatSkeleton:
		if rendered, err = brief.Skeleton(p, rec.Digest); err != nil {
			return err
		}
	}
	if *out == "" {
		fmt.Fprintf(c.Stderr, "metron packet: %s\n", note)
		_, err = c.Stdout.Write(rendered)
		return err
	}
	path := *out
	if !filepath.IsAbs(path) {
		path = filepath.Join(c.Dir, path)
	}
	// .metron/work/ is the scratch space for skeletons and reports.
	if rel, ok := s.relative(filepath.Dir(path)); ok && rel == ".metron/work" {
		if err := os.MkdirAll(filepath.Dir(path), 0o777); err != nil {
			return err
		}
	}
	if err := os.WriteFile(path, rendered, 0o666); err != nil {
		return err
	}
	fmt.Fprintf(c.Stderr, "metron packet: %s; wrote the %s to %s\n", note, *format, *out)
	if dirty, err := s.repo.TreeProblems(); err == nil && len(dirty) > len(w.dirty) {
		fmt.Fprintf(c.Stderr, "metron packet: warning: %s is in the working tree and makes it unclean, which invalidates a packet for a read-only role and blocks the loop; write it under .metron/work/ instead, or outside the repository\n", *out)
	}
	return nil
}

// issue renders a planned packet's instructions, commits its issue and stores
// its canonical JSON, whose digest the ledger records. It returns the stored
// bytes.
func issue(w *world, wr *store.Writer, p core.Packet) ([]byte, error) {
	var err error
	if p.Instructions, err = brief.Instructions(p); err != nil {
		return nil, err
	}
	data, err := canon.Marshal(p)
	if err != nil {
		return nil, fmt.Errorf("encode packet %s: %w", p.ID, err)
	}
	ps, err := schema.ValidateJSON(schema.Packet, data)
	if err != nil {
		return nil, err
	}
	if len(ps) > 0 {
		return nil, fmt.Errorf("packet %s does not satisfy the packet schema, so it was not issued: %s", p.ID, ps[0])
	}
	drafts, err := core.IssuePacket(w.state, w.facts, p, canon.Digest(data))
	if err != nil {
		return nil, err
	}
	if _, err := w.commit(wr, drafts); err != nil {
		return nil, err
	}
	if err := wr.WritePacket(p.ID, data); err != nil {
		return nil, fmt.Errorf("the ledger records packet %s, but its file could not be stored (%w): run %s again to void it and issue a new one",
			p.ID, err, w.metron("packet"))
	}
	w.refresh()
	return data, nil
}

var cmdIngest = &command{
	name:     "ingest",
	synopsis: "ingest [--json] <file|->",
	summary:  "Validate and ingest a role report",
	about: `Ingest reads a report from a file, or from standard input when the argument is
"-", and validates it in three layers: structural (strict JSON, the report
schema), semantic (consistent in itself, with the contract and the packet) and
transition (permitted by the state machine now). A rejected report changes
nothing; every problem is listed with its layer and location, and the status
is 1.

An accepted report is recorded with the digest of its canonical JSON form and
the raw digest of the bytes as submitted, and stored exactly as submitted.
Submitting the report that was already accepted succeeds and changes nothing;
if its stored file was lost, the same bytes restore it.

With --json it prints a metron.ingest/v1 document.`,
	run: runIngest,
}

// reportStructural names the structural layer of report validation, which
// the CLI applies before the core's semantic and transition layers.
const reportStructural core.Layer = layerStructural

type rejectionDoc struct {
	Layer    core.Layer     `json:"layer"`
	Problems []core.Problem `json:"problems"`
}

type ingestDoc struct {
	Schema    string    `json:"schema"`
	Accepted  bool      `json:"accepted"`
	Duplicate bool      `json:"duplicate"`
	Packet    string    `json:"packet,omitempty"`
	Role      core.Role `json:"role,omitempty"`
	// Digest is empty when the report is not JSON.
	Digest    string `json:"digest,omitempty"`
	RawDigest string `json:"raw_digest"`
	// Restored is set when a duplicate restored the stored report file.
	Restored          bool             `json:"restored"`
	Events            []core.EventType `json:"events"`
	FindingsOpened    []string         `json:"findings_opened"`
	Verdict           core.Verdict     `json:"verdict,omitempty"`
	EscalationsRaised []string         `json:"escalations_raised"`
	Notes             []string         `json:"notes,omitempty"`
	// Problem says why a report that was already accepted could not restore
	// its lost or damaged stored file.
	Problem   string        `json:"problem,omitempty"`
	Rejection *rejectionDoc `json:"rejection,omitempty"`
	Next      *actionDoc    `json:"next,omitempty"`
}

func runIngest(c *ctx, args []string) error {
	fs := c.flags()
	dir, asJSON := stateFlag(fs), c.jsonFlag(fs)
	args, err := c.parse(fs, args)
	if err != nil {
		return err
	}
	if len(args) == 0 {
		return usageError(`missing the report: give a file, or "-" for standard input`)
	}
	if err := positional(args, 1, 1); err != nil {
		return err
	}
	var raw []byte
	if args[0] == "-" {
		raw, err = io.ReadAll(c.Stdin)
	} else {
		path := args[0]
		if !filepath.IsAbs(path) {
			path = filepath.Join(c.Dir, path)
		}
		raw, err = os.ReadFile(path)
	}
	if err != nil {
		return fail(exitEnv, "cannot read the report: %v", err)
	}
	d := &ingestDoc{Schema: "metron.ingest/v1", RawDigest: canon.Digest(raw), Events: []core.EventType{},
		FindingsOpened: []string{}, EscalationsRaised: []string{}}
	reject := func(layer core.Layer, ps []core.Problem) error {
		d.Rejection = &rejectionDoc{Layer: layer, Problems: ps}
		return c.ingested(d, *asJSON, nil)
	}
	value, err := canon.Parse(raw)
	if err != nil {
		return reject(reportStructural, []core.Problem{{Message: "not valid JSON: " + err.Error()}})
	}
	canonical, err := canon.Encode(value)
	if err != nil {
		return reject(reportStructural, []core.Problem{{Message: err.Error()}})
	}
	d.Digest = canon.Digest(canonical)
	sps, err := schema.ValidateJSON(schema.Report, canonical)
	if err != nil {
		return err
	}
	if len(sps) > 0 {
		ps := make([]core.Problem, len(sps))
		for i, p := range sps {
			ps[i] = core.Problem{Path: p.Pointer, Message: p.Message}
		}
		return reject(reportStructural, ps)
	}
	var r core.Report
	if err := jsonv2.Unmarshal(canonical, &r, jsonv2.RejectUnknownMembers(true)); err != nil {
		return reject(reportStructural, []core.Problem{{Message: "the report does not decode: " + err.Error()}})
	}
	d.Packet, d.Role = r.Packet, r.Role

	s, err := c.open(*dir)
	if err != nil {
		return err
	}
	wr, w, err := c.lock(s)
	if err != nil {
		return err
	}
	defer wr.Close()
	if err := w.candidate(); err != nil {
		return err
	}
	if err := w.describe(r); err != nil {
		return err
	}
	drafts, dup, err := core.Ingest(w.state, w.facts, r, d.Digest, d.RawDigest)
	var rej *core.Rejection
	if errors.As(err, &rej) {
		return reject(rej.Layer, rej.Problems)
	}
	if err != nil {
		return err
	}
	d.Accepted, d.Duplicate = true, dup
	if dup {
		if err := restore(w, wr, r.Packet, raw, d); err != nil {
			return err
		}
		return c.ingested(d, *asJSON, w)
	}
	// The bytes submitted cannot be recreated later, so they are stored
	// before the ledger commit that accepts them. Until that commit the file
	// belongs to no accepted report and may be replaced.
	if err := wr.ReplaceReport(r.Packet, raw); err != nil {
		return fmt.Errorf("store the report before accepting it, so nothing was changed: %w", err)
	}
	events, err := w.commit(wr, drafts)
	if err != nil {
		return err
	}
	for _, e := range events {
		d.Events = append(d.Events, e.Type)
		switch e.Type {
		case core.EventFindingOpened:
			d.FindingsOpened = append(d.FindingsOpened, decode[core.FindingOpened](e).ID)
		case core.EventVerdictRecorded:
			d.Verdict = decode[core.VerdictRecorded](e).Verdict
		case core.EventEscalationRaised:
			d.EscalationsRaised = append(d.EscalationsRaised, decode[core.EscalationRaised](e).ID)
		}
	}
	return c.ingested(d, *asJSON, w, events...)
}

// restore handles a report that was already accepted: it restores the stored
// file when it is missing and the bytes are the ones accepted. When the file
// cannot be restored from these bytes, d.Problem says what to do instead.
func restore(w *world, wr *store.Writer, packet string, raw []byte, d *ingestDoc) error {
	rec, _ := w.state.Packet(packet)
	stored, err := w.store.ReadReport(packet)
	identical := d.RawDigest == rec.Report.RawDigest
	name := store.ReportsDir + "/" + packet + ".json"
	switch {
	case err == nil && canon.Digest(stored) == rec.Report.RawDigest:
	case errors.Is(err, fs.ErrNotExist) && identical:
		if err := wr.WriteReport(packet, raw); err != nil {
			return err
		}
		d.Restored = true
		d.Notes = append(d.Notes, fmt.Sprintf("restored the stored report file %s", name))
	case errors.Is(err, fs.ErrNotExist):
		d.Problem = fmt.Sprintf("the stored report %s is missing, and this file is the same report in a different byte form (raw digest %s, while %s was accepted), so it cannot restore it: restore the stored report from version control",
			name, d.RawDigest, rec.Report.RawDigest)
	case err != nil:
		return err
	case identical:
		d.Problem = fmt.Sprintf("the stored report %s does not match the raw digest accepted, and stored reports are never overwritten: restore it from version control, or delete it and ingest this file again",
			name)
	default:
		d.Problem = fmt.Sprintf("the stored report %s does not match the raw digest accepted, and this file is the same report in a different byte form: restore the stored report from version control",
			name)
	}
	return nil
}

// ingested prints the outcome of ingest. A rejection is a negative outcome.
func (c *ctx) ingested(d *ingestDoc, asJSON bool, w *world, events ...core.Event) error {
	if w != nil {
		d.Next = w.actionDoc(w.next())
	}
	if asJSON {
		if err := c.emit(d); err != nil {
			return err
		}
		if d.Rejection != nil || d.Problem != "" {
			return negative
		}
		return nil
	}
	var b strings.Builder
	if rj := d.Rejection; rj != nil {
		fmt.Fprintf(&b, "report rejected by the %s layer; nothing was changed:\n", rj.Layer)
		for _, p := range rj.Problems {
			b.WriteString("  ")
			if p.Path != "" {
				b.WriteString(p.Path + ": ")
			}
			b.WriteString(p.Message + "\n")
		}
		b.WriteString("Correct the report and submit it again.")
		return fail(exitNegative, "%s", b.String())
	}
	if d.Duplicate {
		fmt.Fprintf(&b, "The report for packet %s was already accepted; nothing changed.\n", d.Packet)
	} else {
		fmt.Fprintf(&b, "Accepted the %s report for packet %s.\n", d.Role, d.Packet)
	}
	for _, e := range events {
		fmt.Fprintf(&b, "  %s\n", describe(e))
	}
	for _, n := range d.Notes {
		fmt.Fprintf(&b, "Note: %s.\n", n)
	}
	if d.Problem != "" {
		if _, err := io.WriteString(c.Stdout, b.String()); err != nil {
			return err
		}
		return fail(exitNegative, "%s", d.Problem)
	}
	b.WriteString("\n")
	writeNext(&b, w, d.Next)
	_, err := io.WriteString(c.Stdout, b.String())
	return err
}

// escalationID is the form of an escalation identifier.
var escalationID = regexp.MustCompile(`^E-[0-9]{4,}$`)

var cmdResolve = &command{
	name:     "resolve",
	synopsis: "resolve <E-id> -m <decision>",
	summary:  "Discharge a governance-owned escalation (governance)",
	about: `Resolve records governance's decision on a governance-owned escalation
(contract-gap or evidence-gap) and discharges it: after amending and pinning the
contract, after supplying the missing evidence, or when dismissing it with a
rationale. It records the git identity (user.name and user.email) of whoever
resolves it. Auditor-owned escalations are discharged by an audit report.`,
	run: runResolve,
}

func runResolve(c *ctx, args []string) error {
	fs := c.flags()
	dir := stateFlag(fs)
	decision := fs.String("m", "", "governance's decision and its rationale (required)")
	args, err := c.parse(fs, args)
	if err != nil {
		return err
	}
	if len(args) == 0 {
		return usageError("missing the escalation identifier, such as E-0001")
	}
	if err := positional(args, 1, 1); err != nil {
		return err
	}
	if !escalationID.MatchString(args[0]) {
		return usageError("%q is not an escalation identifier: write E- followed by at least four digits, such as E-0001", args[0])
	}
	if strings.TrimSpace(*decision) == "" {
		return usageError(`missing -m "<decision>": state what governance decided and why`)
	}
	s, err := c.open(*dir)
	if err != nil {
		return err
	}
	wr, w, err := c.lock(s)
	if err != nil {
		return err
	}
	defer wr.Close()
	name, email, err := s.repo.Identity()
	if err != nil {
		return fail(exitEnv, "%v\nThe resolution records who decided.", err)
	}
	drafts, err := core.Resolve(w.state, w.facts, args[0], *decision, fmt.Sprintf("%s <%s>", name, email))
	if err != nil {
		return err
	}
	events, err := w.commit(wr, drafts)
	if err != nil {
		return err
	}
	var b strings.Builder
	fmt.Fprintf(&b, "Resolved escalation %s.\n", args[0])
	for _, e := range events {
		fmt.Fprintf(&b, "  %s\n", describe(e))
	}
	b.WriteString("\n")
	writeNext(&b, w, w.actionDoc(w.next()))
	_, err = io.WriteString(c.Stdout, b.String())
	return err
}
