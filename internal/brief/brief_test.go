package brief

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"maps"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/patacca/metron/internal/core"
)

var update = flag.Bool("update", false, "rewrite the golden files in testdata/ from the current output")

// escalationKindsInOrder lists every escalation kind, as section 10 of
// docs/architecture.md does.
var escalationKindsInOrder = []core.EscalationKind{
	core.EscalationDispute, core.EscalationContradiction, core.EscalationComponentChurn,
	core.EscalationRegressionSpiral, core.EscalationNonConvergence, core.EscalationProcessAudit,
	core.EscalationContractGap, core.EscalationEvidenceGap, core.EscalationSuspension,
}

func TestGolden(t *testing.T) {
	for _, action := range actions {
		p := full(action)
		instructions, err := Instructions(p)
		if err != nil {
			t.Fatalf("Instructions(%s): %v", action, err)
		}
		p.Instructions = instructions
		prompt, err := Prompt(p, packetDigest)
		if err != nil {
			t.Fatalf("Prompt(%s): %v", action, err)
		}
		golden(t, string(action)+".instructions.md", instructions)
		golden(t, string(action)+".prompt.md", prompt)
	}
}

func golden(t *testing.T, name, got string) {
	t.Helper()
	path := filepath.Join("testdata", name)
	if *update {
		if err := os.WriteFile(path, []byte(got), 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("%v; run go test ./internal/brief -update to create it", err)
	}
	if got != string(want) {
		t.Errorf("%s differs from the rendered output; inspect the change and run go test ./internal/brief -update to accept it", path)
	}
}

// render produces every output of the package for p.
func render(t *testing.T, p core.Packet) map[string]string {
	t.Helper()
	instructions, err := Instructions(p)
	if err != nil {
		t.Fatalf("Instructions: %v", err)
	}
	prompt, err := Prompt(p, packetDigest)
	if err != nil {
		t.Fatalf("Prompt: %v", err)
	}
	skeleton, err := Skeleton(p, packetDigest)
	if err != nil {
		t.Fatalf("Skeleton: %v", err)
	}
	return map[string]string{"instructions": instructions, "prompt": prompt, "skeleton": string(skeleton)}
}

var orderedListLine = regexp.MustCompile(`(?m)^[ \t]*[0-9]+[.)][ \t].*$`)

func TestRenderMinimalAndFull(t *testing.T) {
	for _, action := range actions {
		for name, p := range map[string]core.Packet{"minimal": minimal(action), "full": full(action)} {
			t.Run(string(action)+"/"+name, func(t *testing.T) {
				for output, text := range render(t, p) {
					for _, bad := range []string{"<no value>", "<nil>", "{{", "}}", "\n\n\n", "%!"} {
						if strings.Contains(text, bad) {
							t.Errorf("%s contains %q", output, bad)
						}
					}
					if !strings.HasSuffix(text, "\n") || strings.HasSuffix(text, "\n\n") {
						t.Errorf("%s does not end with exactly one newline", output)
					}
					// A value wrapped onto the start of a line as "2. " would
					// turn the rest of its paragraph into an ordered list.
					if m := orderedListLine.FindString(text); m != "" {
						t.Errorf("%s has a line that Markdown reads as an ordered list: %q", output, m)
					}
				}
			})
		}
	}
}

func TestDeterministic(t *testing.T) {
	for _, action := range actions {
		first := render(t, full(action))
		for range 3 {
			again := render(t, full(action))
			for output := range first {
				if again[output] != first[output] {
					t.Errorf("%s: %s differs between two renderings of the same packet", action, output)
				}
			}
		}
	}
}

func TestPromptKeepsIssuedInstructions(t *testing.T) {
	p := full(core.StepReview)
	p.Instructions = "The instructions this packet was issued with.\n"
	prompt, err := Prompt(p, packetDigest)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(prompt, p.Instructions) {
		t.Errorf("the prompt does not start with the packet's own instructions")
	}
	rendered, err := Instructions(p)
	if err != nil {
		t.Fatal(err)
	}
	if rendered == p.Instructions {
		t.Errorf("Instructions returned the packet's instructions instead of rendering them")
	}
}

func decodeStrict(data []byte, v any) error {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		return err
	}
	if err := dec.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return errors.New("trailing data after the JSON document")
	}
	return nil
}

func TestSkeleton(t *testing.T) {
	for _, action := range actions {
		for name, p := range map[string]core.Packet{"minimal": minimal(action), "full": full(action)} {
			t.Run(string(action)+"/"+name, func(t *testing.T) {
				data, err := Skeleton(p, packetDigest)
				if err != nil {
					t.Fatal(err)
				}
				if !json.Valid(data) {
					t.Fatalf("skeleton is not valid JSON:\n%s", data)
				}
				// No report member is nullable, and a nil Go slice in a
				// required array would marshal to null and fail the schema.
				var doc any
				if err := json.Unmarshal(data, &doc); err != nil {
					t.Fatal(err)
				}
				for _, path := range nulls(doc, "") {
					t.Errorf("skeleton has null at %s", path)
				}
				var r core.Report
				if err := decodeStrict(data, &r); err != nil {
					t.Fatalf("skeleton does not decode into core.Report: %v", err)
				}
				if r.Schema != core.SchemaReport || r.Packet != p.ID || r.PacketDigest != packetDigest || r.Role != p.Role {
					t.Errorf("envelope = %q %q %q %q, want %q %q %q %q", r.Schema, r.Packet, r.PacketDigest, r.Role,
						core.SchemaReport, p.ID, packetDigest, p.Role)
				}
				if r.Author != (core.Author{}) {
					t.Errorf("author = %+v, want it left empty", r.Author)
				}
				checkBody(t, p, r.Body)
			})
		}
	}
}

// nulls returns the JSON Pointer of every null in a decoded JSON value, in
// document order for arrays and sorted by member name for objects.
func nulls(v any, at string) []string {
	switch x := v.(type) {
	case nil:
		return []string{at}
	case []any:
		var out []string
		for i, e := range x {
			out = append(out, nulls(e, fmt.Sprintf("%s/%d", at, i))...)
		}
		return out
	case map[string]any:
		var out []string
		for _, k := range slices.Sorted(maps.Keys(x)) {
			out = append(out, nulls(x[k], at+"/"+k)...)
		}
		return out
	}
	return nil
}

func checkBody(t *testing.T, p core.Packet, raw json.RawMessage) {
	t.Helper()
	switch p.Action {
	case core.StepReview:
		var b core.ReviewerBody
		if err := decodeStrict(raw, &b); err != nil {
			t.Fatalf("body does not decode into core.ReviewerBody: %v", err)
		}
		var domains []core.Domain
		for _, c := range b.Coverage {
			domains = append(domains, c.Domain)
			if c.Status != "" || c.Summary != "" {
				t.Errorf("coverage for %s is not left empty", c.Domain)
			}
		}
		if !slices.Equal(domains, core.Domains()) {
			t.Errorf("coverage domains = %v, want exactly %v", domains, core.Domains())
		}
		var reqs []string
		for _, r := range b.Requirements {
			reqs = append(reqs, r.Requirement)
			if r.Status != "" || r.Evidence != "" {
				t.Errorf("requirement %s is not left empty", r.Requirement)
			}
		}
		if !slices.Equal(reqs, p.ApplicableRequirements) {
			t.Errorf("requirements = %v, want exactly %v", reqs, p.ApplicableRequirements)
		}
		// A nil slice here means the skeleton held null instead of an empty list.
		if b.Findings == nil || len(b.Findings) != 0 || b.Verdict != "" || b.Justification != "" {
			t.Errorf("findings, verdict or justification not left empty: %+v", b)
		}
	case core.StepFix:
		var b core.ImplementerBody
		if err := decodeStrict(raw, &b); err != nil {
			t.Fatalf("body does not decode into core.ImplementerBody: %v", err)
		}
		var ids []string
		for _, f := range b.Fixes {
			ids = append(ids, f.Finding)
			if f.Commits == nil || len(f.Commits) != 0 || f.Components == nil || len(f.Components) != 0 || f.Summary != "" {
				t.Errorf("fix for %s is not left empty", f.Finding)
			}
		}
		if !slices.Equal(ids, p.Subjects) {
			t.Errorf("fixes = %v, want one per subject %v", ids, p.Subjects)
		}
		var fixes []map[string]json.RawMessage
		var shape struct {
			Fixes *[]map[string]json.RawMessage `json:"fixes"`
		}
		shape.Fixes = &fixes
		if err := json.Unmarshal(raw, &shape); err != nil {
			t.Fatal(err)
		}
		for i, f := range fixes {
			_, has := f["root_cause"]
			want := slices.Contains(p.RootCauseRequired, p.Subjects[i])
			for _, x := range p.Findings {
				if x.ID == p.Subjects[i] && x.RootCauseRequired {
					want = true
				}
			}
			if has != want {
				t.Errorf("fix for %s: root_cause present = %v, want %v", p.Subjects[i], has, want)
			}
		}
	case core.StepVerify:
		var b core.VerifierBody
		if err := decodeStrict(raw, &b); err != nil {
			t.Fatalf("body does not decode into core.VerifierBody: %v", err)
		}
		var ids []string
		for _, v := range b.Verifications {
			ids = append(ids, v.Finding)
			if v.Result != "" || v.Evidence != "" {
				t.Errorf("verification of %s is not left empty", v.Finding)
			}
		}
		if !slices.Equal(ids, p.Subjects) {
			t.Errorf("verifications = %v, want one per claimed fix %v", ids, p.Subjects)
		}
	case core.StepAudit:
		var b core.AuditorBody
		if err := decodeStrict(raw, &b); err != nil {
			t.Fatalf("body does not decode into core.AuditorBody: %v", err)
		}
		var ids []string
		for _, a := range b.Adjudications {
			ids = append(ids, a.Finding)
			if a.Decision != "" || a.Tier != "" || a.Rationale != "" {
				t.Errorf("adjudication of %s is not left empty", a.Finding)
			}
		}
		if !slices.Equal(ids, p.Adjudicable) {
			t.Errorf("adjudications = %v, want one per adjudicable finding %v", ids, p.Adjudicable)
		}
		var escalations, resolved []string
		for _, e := range p.Escalations {
			escalations = append(escalations, e.ID)
		}
		for _, r := range b.Resolutions {
			resolved = append(resolved, r.Escalation)
			if r.Analysis != "" || r.Conclusion != "" {
				t.Errorf("resolution of %s is not left empty", r.Escalation)
			}
		}
		if !slices.Equal(resolved, escalations) {
			t.Errorf("resolutions = %v, want one per escalation %v", resolved, escalations)
		}
	}
}

func TestRefusesInconsistentPackets(t *testing.T) {
	tests := []struct {
		name   string
		action core.Step
		mutate func(*core.Packet)
	}{
		{"other protocol", core.StepReview, func(p *core.Packet) { p.Protocol = "metron.protocol/v2" }},
		{"other report schema", core.StepReview, func(p *core.Packet) { p.ReportSchema = "metron.report/v2" }},
		{"action without packet", core.StepReview, func(p *core.Packet) { p.Action = core.StepRunChecks }},
		{"role does not match action", core.StepReview, func(p *core.Packet) { p.Role = core.RoleVerifier }},
		{"review that names a round", core.StepReview, func(p *core.Packet) { p.Round = 3 }},
		{"review with earlier rounds", core.StepReview, func(p *core.Packet) { p.History = history(2) }},
		{"binding decision of another contract", core.StepReview, func(p *core.Packet) { p.Decisions[0].Contract = earlierContract }},
		{"earlier decision that binds", core.StepReview, func(p *core.Packet) { p.EarlierDecisions[0].SupersededBy = "" }},
		{"fix in round 0", core.StepFix, func(p *core.Packet) { p.Round = 0 }},
		{"verify in round 0", core.StepVerify, func(p *core.Packet) { p.Round = 0 }},
		{"audit in a negative round", core.StepAudit, func(p *core.Packet) { p.Round = -1 }},
		{"applicable list differs from contract", core.StepReview, func(p *core.Packet) { p.ApplicableRequirements = p.ApplicableRequirements[1:] }},
		{"evidence for a later requirement", core.StepReview, func(p *core.Packet) {
			p.Evidence = append(p.Evidence, passed("REQ-11", "true", p.Candidate.Digest))
		}},
		{"two results for one requirement", core.StepReview, func(p *core.Packet) { p.Evidence = append(p.Evidence, p.Evidence[0]) }},
		{"evidence for another candidate", core.StepReview, func(p *core.Packet) { p.Evidence[0].Candidate = digestRound1 }},
		{"duplicate finding", core.StepReview, func(p *core.Packet) { p.Findings = append(p.Findings, p.Findings[0]) }},
		{"review with subjects", core.StepReview, func(p *core.Packet) { p.Subjects = []string{"F-0001"} }},
		{"unknown subject", core.StepFix, func(p *core.Packet) { p.Subjects = append(p.Subjects, "F-0099") }},
		{"root cause outside the subjects", core.StepFix, func(p *core.Packet) {
			p.Findings = append(p.Findings, f1Resolved())
			p.RootCauseRequired = []string{"F-0001"}
		}},
		{"fix without base", core.StepFix, func(p *core.Packet) { p.Base = nil }},
		{"fix without subjects", core.StepFix, func(p *core.Packet) { p.Subjects, p.RootCauseRequired = nil, nil }},
		{"fix with escalations", core.StepFix, func(p *core.Packet) { p.Escalations = fullAudit().Escalations }},
		{"verify with a verified candidate", core.StepVerify, func(p *core.Packet) { p.Verified = &p.Candidate }},
		{"verify subject without fix", core.StepVerify, func(p *core.Packet) { p.Findings[0].Fix = nil }},
		{"audit without escalations", core.StepAudit, func(p *core.Packet) { p.Escalations = nil }},
		{"audit with discharged escalation", core.StepAudit, func(p *core.Packet) { p.Escalations[0].Open = false }},
		{"audit with governance escalation", core.StepAudit, func(p *core.Packet) { p.Escalations[0].Kind = core.EscalationContractGap }},
		{"audit with unknown adjudicable", core.StepAudit, func(p *core.Packet) { p.Adjudicable = []string{"F-0099"} }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := full(tt.action)
			tt.mutate(&p)
			if _, err := Instructions(p); err == nil {
				t.Errorf("Instructions accepted the packet")
			}
			if _, err := Prompt(p, packetDigest); err == nil {
				t.Errorf("Prompt accepted the packet")
			}
			if _, err := Skeleton(p, packetDigest); err == nil {
				t.Errorf("Skeleton accepted the packet")
			}
		})
	}
}

func TestRefusesMalformedPacketDigest(t *testing.T) {
	hex := strings.Repeat("ab", 32)
	for _, digest := range []string{
		"",
		hex,
		"sha256:",
		"sha256:" + hex[:63],
		"sha256:" + hex + "a",
		"sha256:" + strings.ToUpper(hex),
		"sha256:" + hex[:63] + "g",
		"SHA256:" + hex,
		"sha512:" + hex,
		" sha256:" + hex,
	} {
		p := full(core.StepReview)
		if _, err := Skeleton(p, digest); err == nil {
			t.Errorf("Skeleton accepted packet digest %q", digest)
		}
		if _, err := Prompt(p, digest); err == nil {
			t.Errorf("Prompt accepted packet digest %q", digest)
		}
	}
}

func TestEveryAuditorEscalationKindIsExplained(t *testing.T) {
	headings := map[core.EscalationKind]string{}
	for _, kind := range escalationKindsInOrder {
		if kind.Owner() == core.RoleAuditor {
			headings[kind] = "### `" + string(kind) + "`:"
		}
	}
	if len(headings) != 6 {
		t.Fatalf("found %d auditor-owned escalation kinds, want 6", len(headings))
	}
	for kind := range headings {
		p := minimal(core.StepAudit)
		p.Escalations[0].Kind = kind
		text, err := Instructions(p)
		if err != nil {
			t.Fatalf("%s: %v", kind, err)
		}
		for other, h := range headings {
			if got := strings.Contains(text, h); got != (other == kind) {
				t.Errorf("brief for a %s escalation: contains %q = %v", kind, h, got)
			}
		}
	}
}

func TestReviewerBriefNamesEveryDomain(t *testing.T) {
	text, err := Instructions(minimal(core.StepReview))
	if err != nil {
		t.Fatal(err)
	}
	for _, d := range core.Domains() {
		if !strings.Contains(text, "| `"+string(d)+"` |") {
			t.Errorf("the reviewer brief's domain table lacks %s", d)
		}
	}
	for _, r := range []core.ReopenReason{core.ReopenNewEvidence, core.ReopenRegression, core.ReopenInvalidVerification,
		core.ReopenRequirementChanged, core.ReopenContractConflict} {
		if !strings.Contains(text, "| `"+string(r)+"` |") {
			t.Errorf("the reviewer brief's reopen table lacks %s", r)
		}
	}
	for _, d := range core.Domains() {
		listed := strings.Contains(proportionalityList(text), "`"+string(d)+"`")
		if listed != d.NeedsProportionality() {
			t.Errorf("the reviewer brief lists %s among the improvement-prone domains = %v, want %v", d, listed, d.NeedsProportionality())
		}
	}
}

// proportionalityList returns the paragraph of the reviewer brief that names
// the improvement-prone domains.
func proportionalityList(text string) string {
	_, after, _ := strings.Cut(text, "### Improvement-prone domains")
	paragraph, _, _ := strings.Cut(strings.TrimLeft(after, "\n"), "\n\n")
	_, list, _ := strings.Cut(paragraph, "In ")
	list, _, _ = strings.Cut(list, `"it`)
	return list
}

// TestBriefVocabulary checks that every hyphenated identifier a brief shows as
// code is spelled as one of the protocol's constants.
func TestBriefVocabulary(t *testing.T) {
	known := map[string]bool{}
	for _, d := range core.Domains() {
		known[string(d)] = true
	}
	for _, s := range core.Stages() {
		known[string(s)] = true
	}
	for _, s := range []string{string(core.ReopenNewEvidence), string(core.ReopenInvalidVerification),
		string(core.ReopenRequirementChanged), string(core.ReopenContractConflict), core.CoverageNotApplicable} {
		known[s] = true
	}
	for _, k := range escalationKindsInOrder {
		known[string(k)] = true
	}
	token := regexp.MustCompile("`([a-z]+(?:-[a-z]+)+)`")
	for _, action := range actions {
		text, err := Instructions(full(action))
		if err != nil {
			t.Fatal(err)
		}
		for _, m := range token.FindAllStringSubmatch(text, -1) {
			if !known[m[1]] {
				t.Errorf("%s brief shows `%s`, which is not a protocol identifier", action, m[1])
			}
		}
	}
}

func TestProtocol(t *testing.T) {
	text, err := Protocol()
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.HasPrefix(text, []byte("# The Metron Engineering Review Protocol\n")) {
		t.Errorf("protocol text does not start with its title")
	}
	if !bytes.Contains(text, []byte("`"+core.ProtocolVersion+"`")) {
		t.Errorf("protocol text does not name its version %s", core.ProtocolVersion)
	}
}

func TestCodes(t *testing.T) {
	tests := []struct {
		in   []string
		want string
	}{
		{nil, ""},
		{[]string{"F-0001"}, "`F-0001`"},
		{[]string{"F-0001", "F-0002"}, "`F-0001` and `F-0002`"},
		{[]string{"a", "b", "c"}, "`a`, `b` and `c`"},
	}
	for _, tt := range tests {
		if got := codes(tt.in); got != tt.want {
			t.Errorf("codes(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestNormalize(t *testing.T) {
	tests := []struct {
		in, want string
	}{
		{"", "\n"},
		{"a", "a\n"},
		{"\n\na\n", "a\n"},
		{"a  \n\t\n\n\nb\n\n", "a\n\nb\n"},
		{"a\nb\n\nc", "a\nb\n\nc\n"},
	}
	for _, tt := range tests {
		if got := normalize(tt.in); got != tt.want {
			t.Errorf("normalize(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

// An audit can be due before any round has a verdict, when the checks keep
// failing; its packet names round 0 and is briefed like any other.
func TestAuditBeforeAnyRound(t *testing.T) {
	p := minimal(core.StepAudit)
	p.Round = 0
	text, err := Instructions(p)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(text, "before any review round has a verdict") || strings.Contains(text, "round 0") {
		t.Fatalf("the brief does not describe an audit before any round:\n%s", text[:400])
	}
	if _, err := Prompt(p, packetDigest); err != nil {
		t.Fatal(err)
	}
}

// A review packet says which candidate the fixes were verified on, and the
// brief says whether that is the candidate under review.
func TestReviewBriefNamesTheVerifiedCandidate(t *testing.T) {
	same := fullReview()
	text, err := Instructions(same)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(text, "That is the candidate in front of you.") || strings.Contains(text, "nobody has verified them") {
		t.Fatal("the brief does not say that the verified candidate is the reviewed one")
	}
	other := fullReview()
	other.Verified = &core.Candidate{Commit: commitBatch, Digest: digestBatch}
	if text, err = Instructions(other); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(text, "git log "+commitBatch+".."+commitFixed) || !strings.Contains(text, "nobody has verified them") {
		t.Fatal("the brief does not warn about the commits made after the verification")
	}
	none := fullReview()
	none.Verified = nil
	if text, err = Instructions(none); err != nil || strings.Contains(text, "verified by a") {
		t.Fatalf("a review without a verification before it mentions one: %v", err)
	}
}

// A fix packet whose subjects already have a claimed fix asks for the claim
// again, and does not offer a dispute.
func TestFixBriefForLapsedClaims(t *testing.T) {
	p := fullFix()
	p.Findings[1].Status = core.StatusClaimed
	p.Findings[1].Fix = fixF5()
	text, err := Instructions(p)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"disputing it: `F-0004`.", "A fix for `F-0005` is already claimed.", "it again in this report"} {
		if !strings.Contains(text, want) {
			t.Errorf("the brief lacks %q", want)
		}
	}
}

// What a reviewer is handed names no round: not the packet's, not an earlier
// one, not the round a finding was opened in.
func TestReviewPromptNamesNoRound(t *testing.T) {
	p := full(core.StepReview)
	text, err := Prompt(p, packetDigest)
	if err != nil {
		t.Fatal(err)
	}
	for _, line := range strings.Split(text, "\n") {
		lower := strings.ToLower(line)
		// The brief speaks of rounds in general; no line may number one.
		for _, bad := range []string{"round 1", "round 2", "round 3", "in round", "reviewer |", "review-agent"} {
			if strings.Contains(lower, bad) {
				t.Fatalf("the review prompt names a round or a reviewer: %q", line)
			}
		}
	}
}
