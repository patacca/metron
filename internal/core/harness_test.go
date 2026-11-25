package core

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"testing"
	"time"
)

// harness simulates a repository, a contract file and a store around the
// core: it builds facts, seals drafts into events and folds them.
type harness struct {
	tb       testing.TB
	state    State
	events   []Event
	contract Contract
	tree     int // content version of the files outside .metron/
	top      int // the highest content version used so far
	history  []Commit
	clean    bool
	clock    int
	packets  map[string]Packet // issued packets, as the store keeps them
	accepted map[string]Report // accepted reports by packet
	lost     string            // a packet whose stored file is unusable
	amend    func(f *Facts)    // changes the facts of the next ingestions
}

func digestOf(s string) string {
	sum := sha256.Sum256([]byte(s))
	return "sha256:" + hex.EncodeToString(sum[:])
}

func commitHash(n int) string {
	sum := sha256.Sum256(fmt.Appendf(nil, "commit %d", n))
	return hex.EncodeToString(sum[:20])
}

func mustJSON(tb testing.TB, v any) []byte {
	tb.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		tb.Fatalf("marshal: %v", err)
	}
	return b
}

func contractDigest(tb testing.TB, c Contract) string { return digestOf(string(mustJSON(tb, c))) }

// testContract is a valid prototype contract. REQ-1 has a check command;
// REQ-4 applies only from controlled-validation.
func testContract() Contract {
	return Contract{
		Schema:     SchemaContract,
		Project:    Project{Name: "demo", Objectives: []string{"parse configuration files"}},
		Stage:      StagePrototype,
		Authorizes: "internal demonstration",
		Scope:      []string{"the parser"},
		Envelope:   []string{"files under 1 MiB"},
		Requirements: []Requirement{
			{ID: "REQ-1", Domain: DomainFunctionalCorrectness, Statement: "valid input parses", Criterion: "the tests pass", Command: "go test ./..."},
			{ID: "REQ-2", Domain: DomainObjectiveAlignment, Statement: "it parses configuration", Criterion: "review"},
			{ID: "REQ-3", Domain: DomainArchitecture, Statement: "parsing is layered", Criterion: "review"},
			{ID: "REQ-4", Domain: DomainDataIntegrity, Statement: "no input is lost", Criterion: "review", From: StageControlledValidation},
		},
		Risks: Risks{Unacceptable: []Risk{{ID: "RISK-1", Statement: "silent data loss"}}},
	}
}

// stageTwoContract targets controlled-validation, waiving the domains it
// does not cover.
func stageTwoContract() Contract {
	c := testContract()
	c.Stage = StageControlledValidation
	c.NotApplicable = []Waiver{
		{Domain: DomainTestability, Rationale: "covered by REQ-1"},
		{Domain: DomainObservability, Rationale: "no service yet"},
		{Domain: DomainStability, Rationale: "single run"},
	}
	return c
}

func newHarness(tb testing.TB) *harness {
	tb.Helper()
	h := &harness{tb: tb, contract: testContract(), clean: true,
		history: []Commit{{Hash: commitHash(0), Substantive: true, Paths: []string{"src/v0.go"}}},
		packets: map[string]Packet{}, accepted: map[string]Report{}}
	h.must(Initialize(h.state))
	h.must(Pin(h.state, h.facts(), "initial contract", "gov"))
	return h
}

func (h *harness) candidate() Candidate {
	return Candidate{Commit: h.history[len(h.history)-1].Hash, Digest: digestOf(fmt.Sprintf("tree %d", h.tree))}
}

func (h *harness) facts() Facts {
	c := h.contract
	cd := contractDigest(h.tb, c)
	f := Facts{
		Time:              time.Unix(1_800_000_000+int64(h.clock), 0).UTC().Format(timeLayout),
		LedgerVerified:    true,
		Contract:          &c,
		ContractDigest:    cd,
		ContractRawDigest: digestOf("raw " + cd),
		Candidate:         h.candidate(),
		Clean:             h.clean,
	}
	if op, ok := h.state.OpenPacket(); ok {
		if op.Packet == h.lost {
			f.PacketProblem = "packets/" + op.Packet + ".json does not match its recorded digest"
		}
	}
	return f
}

// describe describes the commits a report lists, as the repository would:
// the history is linear, so every recorded commit is an ancestor of HEAD, and
// a commit is in the base when it is the base or comes before it.
func (h *harness) describe(base string, hashes []string) []Commit {
	b := slices.IndexFunc(h.history, func(c Commit) bool { return c.Hash == base })
	out := []Commit{}
	for _, hash := range hashes {
		c := Commit{Hash: hash}
		if i := slices.IndexFunc(h.history, func(c Commit) bool { return c.Hash == hash }); i >= 0 {
			c.Reachable, c.InBase, c.Substantive, c.Paths = true, b >= 0 && i <= b, h.history[i].Substantive, h.history[i].Paths
		}
		out = append(out, c)
	}
	return out
}

// interval lists the commits made after base, as the repository would: the
// history is linear, so they are the ones recorded after it. It is unknown
// when the repository no longer has base.
func (h *harness) interval(base string) ([]Commit, bool) {
	b := slices.IndexFunc(h.history, func(c Commit) bool { return c.Hash == base })
	if b < 0 {
		return nil, false
	}
	var out []Commit
	for _, c := range h.history[b+1:] {
		out = append(out, Commit{Hash: c.Hash, Reachable: true, Substantive: c.Substantive, Paths: c.Paths})
	}
	return out, true
}

// unlisted returns the commits since the reviewed one that change the
// candidate and that no accepted fix lists yet.
func (h *harness) unlisted() []string {
	base, ok := h.state.base()
	if !ok {
		return nil
	}
	commits, _ := h.interval(base.Candidate.Commit)
	var out []string
	for _, c := range commits {
		if c.Substantive && !slices.Contains(h.state.accounted, c.Hash) {
			out = append(out, c.Hash)
		}
	}
	return out
}

// repair commits a change and returns a complete fix for id.
func (h *harness) repair(id string, components ...string) Fix {
	h.commit(true)
	return h.restate(id, components...)
}

// restate returns a fix for id that lists every commit since the reviewed
// one that no fix accounts for yet and, when a fix for id was claimed before,
// the commits of that claim that are still part of the candidate.
func (h *harness) restate(id string, components ...string) Fix {
	fx := fix(id, "", components...)
	fx.Commits = h.unlisted()
	f, ok := lookup(h.state, id)
	base, reviewed := h.state.base()
	if !ok || !reviewed || f.Status != StatusClaimed || f.Fix == nil {
		return fx
	}
	interval, _ := h.interval(base.Candidate.Commit)
	for _, c := range f.Fix.Commits {
		if slices.ContainsFunc(interval, func(ic Commit) bool { return ic.Hash == c }) && !slices.Contains(fx.Commits, c) {
			fx.Commits = append(fx.Commits, c)
		}
	}
	return fx
}

// verifying records passing checks on the candidate the fixes were claimed
// on, which the loop requires before a verification, and returns the verify
// packet.
func (h *harness) verifying() Packet {
	h.tb.Helper()
	if h.next().Step == StepRunChecks {
		h.checks(true)
	}
	return h.packet()
}

// lookup returns the finding with the given identifier.
func lookup(s State, id string) (Finding, bool) {
	if i := s.findingIndex(id); i >= 0 {
		return s.findings[i], true
	}
	return Finding{}, false
}

// commit records a commit. A substantive one gives the files outside
// .metron/ content they never had, so it changes the candidate digest.
func (h *harness) commit(substantive bool) string {
	if substantive {
		return h.touch(fmt.Sprintf("src/v%d.go", h.top+1))
	}
	c := Commit{Hash: commitHash(len(h.history))}
	h.history = append(h.history, c)
	h.clean = true
	return c.Hash
}

// touch records a commit that changes the given paths, with content they
// never had.
func (h *harness) touch(paths ...string) string {
	h.top++
	h.tree = h.top
	c := Commit{Hash: commitHash(len(h.history)), Substantive: true, Paths: paths}
	h.history = append(h.history, c)
	h.clean = true
	return c.Hash
}

// noNull fails when a value the core produced would be written with a null
// member. Every nullable member is optional, so null can only be a required
// array left nil, which structural validation rejects.
func noNull(tb testing.TB, what string, b []byte) {
	tb.Helper()
	if bytes.Contains(b, []byte(":null")) {
		tb.Fatalf("%s has a null member: %s", what, b)
	}
}

// seal turns drafts into events and folds them, as the store would: one
// batch, with More counting down to 0.
func (h *harness) seal(drafts []Draft) {
	h.tb.Helper()
	for i, d := range drafts {
		e := Event{Seq: len(h.events) + 1, Type: d.Type, More: len(drafts) - 1 - i, Data: mustJSON(h.tb, d.Data)}
		noNull(h.tb, string(d.Type), e.Data)
		s, err := Apply(h.state, e)
		if err != nil {
			h.tb.Fatalf("apply %s %s: %v", d.Type, e.Data, err)
		}
		h.state = s
		h.events = append(h.events, e)
	}
	h.clock++
}

func (h *harness) must(drafts []Draft, err error) []Draft {
	h.tb.Helper()
	if err != nil {
		h.tb.Fatalf("command refused: %v", err)
	}
	h.seal(drafts)
	return drafts
}

func (h *harness) next() Action     { return Next(h.state, h.facts()) }
func (h *harness) gate() GateResult { return Gate(h.state, h.facts()) }

func (h *harness) expect(step Step) Action {
	h.tb.Helper()
	a := h.next()
	if a.Step != step {
		h.tb.Fatalf("next is %s (%s), want %s", a.Step, a.Reason, step)
	}
	return a
}

// checks records a result for every applicable requirement with a command.
func (h *harness) checks(pass bool) {
	h.tb.Helper()
	f := h.facts()
	var ev []Evidence
	for _, r := range f.Contract.Applicable() {
		if r.Command == "" {
			continue
		}
		code := 0
		if !pass {
			code = 1
		}
		ev = append(ev, Evidence{Requirement: r.ID, Command: r.Command, Candidate: f.Candidate.Digest,
			ExitCode: code, Passed: pass, OutputDigest: digestOf("output")})
	}
	h.must(RecordEvidence(h.state, f, ev))
}

// planned plans the packet for the next action and fills in its brief.
func (h *harness) planned() (Packet, string, error) {
	plan, err := PlanPacket(h.state, h.facts())
	if err != nil {
		return Packet{}, "", err
	}
	if plan.Open != "" {
		return h.packets[plan.Open], plan.Open, nil
	}
	p := plan.Packet
	noNull(h.tb, "packet "+p.ID, mustJSON(h.tb, p))
	p.Instructions = "brief for the " + string(p.Role)
	return p, "", nil
}

// packet issues the packet for the next action, or returns the open one.
func (h *harness) packet() Packet {
	h.tb.Helper()
	p, open, err := h.planned()
	if err != nil {
		h.tb.Fatalf("plan packet: %v", err)
	}
	if open != "" {
		return p
	}
	h.must(IssuePacket(h.state, h.facts(), p, digestOf(string(mustJSON(h.tb, p)))))
	h.packets[p.ID] = p
	return p
}

// report answers packet p, bound to the digest the ledger recorded for it.
func (h *harness) report(p Packet, author string, body any) Report {
	rec, _ := h.state.Packet(p.ID)
	return Report{Schema: SchemaReport, Packet: p.ID, PacketDigest: rec.Digest, Role: p.Role,
		Author: Author{ID: author, Kind: AuthorAgent}, Body: mustJSON(h.tb, body)}
}

// ingest submits a report with the facts the CLI gathers for it: for an
// implementer report, a description of every commit its fixes list.
func (h *harness) ingest(r Report) ([]Draft, bool, error) {
	raw := mustJSON(h.tb, r)
	f := h.facts()
	var b ImplementerBody
	if p, ok := h.state.Packet(r.Packet); ok && p.Base != nil && r.Role == RoleImplementer && json.Unmarshal(r.Body, &b) == nil {
		var hashes []string
		for _, fx := range b.Fixes {
			for _, c := range fx.Commits {
				if !slices.Contains(hashes, c) {
					hashes = append(hashes, c)
				}
			}
		}
		if len(hashes) > 0 {
			f.Commits = h.describe(p.Base.Commit, hashes)
		}
	}
	if p, ok := h.state.Packet(r.Packet); ok && p.Base != nil && r.Role == RoleImplementer {
		f.Interval, f.IntervalKnown = h.interval(p.Base.Commit)
	}
	if h.amend != nil {
		h.amend(&f)
	}
	return Ingest(h.state, f, r, digestOf(string(raw)), digestOf("raw "+string(raw)))
}

// accept ingests a report that must be accepted, and folds its events.
func (h *harness) accept(p Packet, author string, body any) []Draft {
	h.tb.Helper()
	r := h.report(p, author, body)
	drafts, dup, err := h.ingest(r)
	if err != nil || dup {
		h.tb.Fatalf("report for %s refused (duplicate %t): %v", p.ID, dup, err)
	}
	h.seal(drafts)
	h.accepted[p.ID] = r
	return drafts
}

// refused ingests a report that must be refused by layer, and returns the
// problems.
func (h *harness) refused(p Packet, author string, body any, layer Layer) []Problem {
	h.tb.Helper()
	drafts, dup, err := h.ingest(h.report(p, author, body))
	var r *Rejection
	if !errors.As(err, &r) || dup || len(drafts) != 0 {
		h.tb.Fatalf("report for %s: got %d drafts, duplicate %t, error %v; want a %s rejection", p.ID, len(drafts), dup, err, layer)
	}
	if r.Layer != layer {
		h.tb.Fatalf("report for %s rejected by %s layer, want %s: %v", p.ID, r.Layer, layer, err)
	}
	return r.Problems
}

func coverage() []DomainAssessment {
	var out []DomainAssessment
	for _, d := range Domains() {
		out = append(out, DomainAssessment{Domain: d, Status: CoverageAssessed, Summary: "inspected"})
	}
	return out
}

// review builds a reviewer body: every applicable requirement satisfied unless
// statuses says otherwise, and the given findings.
func review(c Contract, verdict Verdict, statuses map[string]string, findings ...FindingInput) ReviewerBody {
	b := ReviewerBody{Coverage: coverage(), Findings: findings, Verdict: verdict, Justification: "as found"}
	if b.Findings == nil {
		b.Findings = []FindingInput{}
	}
	for _, r := range c.Applicable() {
		st := RequirementSatisfied
		if s, ok := statuses[r.ID]; ok {
			st = s
		}
		b.Requirements = append(b.Requirements, RequirementAssessment{Requirement: r.ID, Status: st, Evidence: "checked"})
	}
	return b
}

func green(c Contract) ReviewerBody { return review(c, VerdictGreen, nil) }

// red opens one blocking finding against REQ-1, which it reports violated.
func red(c Contract, key string) ReviewerBody {
	return review(c, VerdictRed, map[string]string{"REQ-1": RequirementViolated}, blocker(key, "REQ-1"))
}

func blocker(key, req string) FindingInput {
	return FindingInput{Key: key, Tier: TierMaterial, Domain: DomainFunctionalCorrectness, Title: "empty input crashes",
		Components: []string{"parser"}, Description: "nil dereference", Evidence: "repro attached", Impact: "crash",
		Requirement: req, BlockingRationale: "the demo feeds empty files", Verification: "a regression test"}
}

func improvement(key string) FindingInput {
	return FindingInput{Key: key, Tier: TierImprovement, Domain: DomainMaintainability, Title: "long function",
		Components: []string{"parser"}, Description: "300 lines", Evidence: "read it", Impact: "slower changes",
		ExpectedBenefit: "easier review", DeferralRationale: "no change planned"}
}

// resolutions resolves every escalation of an audit packet.
func resolutions(p Packet, conclusion string) []EscalationResolution {
	out := []EscalationResolution{}
	for _, e := range p.Escalations {
		out = append(out, EscalationResolution{Escalation: e.ID, Analysis: "analysed " + e.ID, Conclusion: conclusion})
	}
	return out
}

func fix(id, commit string, components ...string) Fix {
	if len(components) == 0 {
		components = []string{"parser"}
	}
	return Fix{Finding: id, Commits: []string{commit}, Summary: "guard the nil case", Components: components}
}

// verify builds a verifier body that finds every fix proportionate.
func verify(results map[string]string, subjects ...string) VerifierBody {
	var b VerifierBody
	for _, id := range subjects {
		b.Verifications = append(b.Verifications, Verification{Finding: id, Result: results[id], Evidence: "ran the repro",
			Scope: ScopeProportionate, ScopeEvidence: "the commits change the nil guard and its test, nothing else"})
	}
	return b
}

func types(drafts []Draft) []EventType {
	var out []EventType
	for _, d := range drafts {
		out = append(out, d.Type)
	}
	return out
}

func eqTypes(tb testing.TB, drafts []Draft, want ...EventType) {
	tb.Helper()
	if got := types(drafts); !slices.Equal(got, want) {
		tb.Fatalf("event types %v, want %v", got, want)
	}
}

func hasProblem(ps []Problem, path string) bool {
	return slices.ContainsFunc(ps, func(p Problem) bool { return p.Path == path })
}

// raisedKinds lists the escalation kinds a set of drafts raises.
func raisedKinds(drafts []Draft) []EscalationKind {
	var out []EscalationKind
	for _, d := range drafts {
		if e, ok := d.Data.(EscalationRaised); ok {
			out = append(out, e.Kind)
		}
	}
	return out
}
