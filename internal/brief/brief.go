// Package brief renders what a role reads: the brief that goes into a
// packet's instructions, a self-contained prompt for manual use, and a report
// skeleton shaped for the packet. All text comes from the protocol templates
// embedded in package protocol; this package only prepares the data they are
// executed with.
//
// Rendering is deterministic. A packet that is inconsistent with itself, or
// that was issued under another protocol version, is refused rather than
// briefed, because a brief that misdescribes the packet misleads the role.
package brief

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"text/template"

	"github.com/patacca/metron/internal/core"
	"github.com/patacca/metron/protocol"
)

// dir is the directory of protocol version core.ProtocolVersion inside
// protocol.FS.
const dir = "v1"

// Protocol returns the text of the Metron Engineering Review Protocol, as
// printed by `metron protocol`.
func Protocol() ([]byte, error) {
	return protocol.FS.ReadFile(dir + "/protocol.md")
}

// Instructions renders the brief for p's role and action: the text Metron
// stores in p.Instructions. It ignores p.Instructions itself.
func Instructions(p core.Packet) (string, error) {
	v, err := newView(p)
	if err != nil {
		return "", err
	}
	return instructions(v)
}

// Prompt renders a self-contained prompt for p: the brief, the packet's
// content laid out for reading, the report skeleton and how to submit the
// report. digest is the digest of the stored packet, which the skeleton
// carries. The brief is p.Instructions when set, so that a prompt for an
// issued packet shows exactly what the packet says; otherwise it is rendered.
func Prompt(p core.Packet, digest string) (string, error) {
	v, err := newDigestView(p, digest)
	if err != nil {
		return "", err
	}
	v.Brief = p.Instructions
	if v.Brief == "" {
		if v.Brief, err = instructions(v); err != nil {
			return "", err
		}
	}
	skeleton, err := skeleton(v)
	if err != nil {
		return "", err
	}
	v.Skeleton = strings.TrimSuffix(string(skeleton), "\n")
	return execute("prompt.md.tmpl", v)
}

// Skeleton returns an indented JSON report for p with the envelope filled in
// and the body shaped for the action: one coverage entry per domain and one
// entry per applicable requirement for a review, one fix per subject for a
// fix, one verification per claimed fix for a verification, and one
// resolution per escalation and one adjudication per adjudicable finding for
// an audit. digest is the digest of the stored packet; the skeleton carries
// it so that the report is bound to this exact packet. Every judgment is left
// empty, so the skeleton fails validation until the role completes it.
func Skeleton(p core.Packet, digest string) ([]byte, error) {
	v, err := newDigestView(p, digest)
	if err != nil {
		return nil, err
	}
	return skeleton(v)
}

// newDigestView is newView for the entry points that bind a report to the
// stored packet, which must be identified by a well-formed digest.
func newDigestView(p core.Packet, digest string) (*view, error) {
	if !core.ValidDigest(digest) {
		return nil, fmt.Errorf("packet digest %q for packet %s is not \"sha256:\" followed by 64 lowercase hex digits; pass the digest of the stored packet", digest, p.ID)
	}
	v, err := newView(p)
	if err != nil {
		return nil, err
	}
	v.Digest = digest
	return v, nil
}

func instructions(v *view) (string, error) {
	return execute(string(v.Role)+".md.tmpl", v)
}

func execute(name string, v *view) (string, error) {
	t, err := template.New(name).
		Option("missingkey=error").
		Funcs(template.FuncMap{"codes": codes, "candidate": v.candidate, "contract": v.contract}).
		ParseFS(protocol.FS, dir+"/*.tmpl", dir+"/roles/*.tmpl")
	if err != nil {
		return "", fmt.Errorf("parse the embedded protocol templates: %w", err)
	}
	var b strings.Builder
	if err := t.ExecuteTemplate(&b, name, v); err != nil {
		return "", fmt.Errorf("render %s for packet %s: %w", name, v.ID, err)
	}
	return normalize(b.String()), nil
}

// fixEntry is a fix in an implementer skeleton. It shows root_cause, empty,
// for the findings whose fix requires one; core.Fix would omit it.
type fixEntry struct {
	core.Fix
	RootCause *string `json:"root_cause,omitempty"`
}

func skeleton(v *view) ([]byte, error) {
	var body any
	switch {
	case v.IsReview:
		b := core.ReviewerBody{
			Coverage:     []core.DomainAssessment{},
			Requirements: []core.RequirementAssessment{},
			Findings:     []core.FindingInput{},
		}
		for _, d := range core.Domains() {
			b.Coverage = append(b.Coverage, core.DomainAssessment{Domain: d})
		}
		for _, id := range v.ApplicableRequirements {
			b.Requirements = append(b.Requirements, core.RequirementAssessment{Requirement: id})
		}
		body = b
	case v.IsFix:
		fixes := []fixEntry{}
		for _, id := range v.Subjects {
			e := fixEntry{Fix: core.Fix{Finding: id, Commits: []string{}, Components: []string{}}}
			if slices.Contains(v.RootCause, id) {
				e.RootCause = new(string)
			}
			fixes = append(fixes, e)
		}
		body = struct {
			Fixes []fixEntry `json:"fixes"`
		}{fixes}
	case v.IsVerify:
		b := core.VerifierBody{Verifications: []core.Verification{}}
		for _, id := range v.Subjects {
			b.Verifications = append(b.Verifications, core.Verification{Finding: id})
		}
		body = b
	case v.IsAudit:
		b := core.AuditorBody{Resolutions: []core.EscalationResolution{}}
		for _, e := range v.Escalations {
			b.Resolutions = append(b.Resolutions, core.EscalationResolution{Escalation: e.ID})
		}
		for _, id := range v.Adjudicable {
			b.Adjudications = append(b.Adjudications, core.Adjudication{Finding: id})
		}
		body = b
	}
	raw, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("encode the report skeleton for packet %s: %w", v.ID, err)
	}
	out, err := json.MarshalIndent(core.Report{
		Schema:       core.SchemaReport,
		Packet:       v.ID,
		PacketDigest: v.Digest,
		Role:         v.Role,
		Body:         raw,
	}, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("encode the report skeleton for packet %s: %w", v.ID, err)
	}
	return append(out, '\n'), nil
}

// codes formats identifiers as inline code in an English list:
// "`a`", "`a` and `b`", "`a`, `b` and `c`".
func codes(items []string) string {
	quoted := make([]string, len(items))
	for i, s := range items {
		quoted[i] = "`" + s + "`"
	}
	if len(quoted) < 2 {
		return strings.Join(quoted, "")
	}
	return strings.Join(quoted[:len(quoted)-1], ", ") + " and " + quoted[len(quoted)-1]
}

// candidate names a candidate digest relative to the packet's own candidate,
// which is what a reader needs to know about a step in a finding's history:
// whether it was made on the candidate in front of them or on another one.
func (v *view) candidate(digest string) string {
	if digest == v.Candidate.Digest {
		return "this candidate"
	}
	return "candidate `" + digest + "`"
}

// contract names a contract digest relative to the packet's pinned contract:
// a finding established under another contract cites that contract's
// requirements and risks, which the pinned one may have changed.
func (v *view) contract(digest string) string {
	if digest == v.ContractDigest {
		return "the pinned contract"
	}
	return "contract `" + digest + "`, which is not the pinned one: check in the pinned contract that what it cites still says what the finding relies on"
}

// normalize removes trailing whitespace from every line, collapses runs of
// blank lines into one, and ends the text with exactly one newline. It lets
// the templates use whitespace for readability without it reaching the output.
func normalize(s string) string {
	var out []string
	blank := true
	for _, line := range strings.Split(s, "\n") {
		line = strings.TrimRight(line, " \t")
		if line == "" {
			if blank {
				continue
			}
			blank = true
		} else {
			blank = false
		}
		out = append(out, line)
	}
	for len(out) > 0 && out[len(out)-1] == "" {
		out = out[:len(out)-1]
	}
	return strings.Join(out, "\n") + "\n"
}
