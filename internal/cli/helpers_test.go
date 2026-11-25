package cli

import (
	"bytes"
	"encoding/json"
	jsonv2 "encoding/json/v2"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/patacca/metron/internal/core"
	"github.com/patacca/metron/internal/store"
)

var update = flag.Bool("update", false, "rewrite the golden files under testdata/")

// TestMain gives every git command a hermetic environment: no system or
// global configuration, a fixed identity and fixed dates, so that commit
// hashes, and with them the golden files, are the same on every machine.
func TestMain(m *testing.M) {
	flag.Parse()
	home, err := os.MkdirTemp("", "metron-cli-test-")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	global := filepath.Join(home, "gitconfig")
	if err := os.WriteFile(global, nil, 0o666); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	for _, kv := range [][2]string{
		{"HOME", home},
		{"XDG_CONFIG_HOME", home},
		{"GIT_CONFIG_NOSYSTEM", "1"},
		{"GIT_CONFIG_GLOBAL", global},
		{"GIT_AUTHOR_NAME", "Ada Author"},
		{"GIT_AUTHOR_EMAIL", "ada@example.com"},
		{"GIT_COMMITTER_NAME", "Ada Author"},
		{"GIT_COMMITTER_EMAIL", "ada@example.com"},
		{"GIT_AUTHOR_DATE", "1767225600 +0000"},
		{"GIT_COMMITTER_DATE", "1767225600 +0000"},
		{"GIT_TERMINAL_PROMPT", "0"},
	} {
		os.Setenv(kv[0], kv[1])
	}
	for _, k := range []string{"GIT_DIR", "GIT_WORK_TREE", "GIT_INDEX_FILE", "GIT_OBJECT_DIRECTORY",
		"GIT_ALTERNATE_OBJECT_DIRECTORIES", "GIT_CEILING_DIRECTORIES", envStateDir} {
		os.Unsetenv(k)
	}
	code := m.Run()
	os.RemoveAll(home)
	os.Exit(code)
}

// testClock is the time every test command observes.
var testClock = time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)

// repo is a temporary git repository that metron commands run in.
type repo struct {
	t   *testing.T
	dir string
	env map[string]string
	// stdin is what the next command reads from standard input.
	stdin string
}

const testContract = `schema: metron.contract/v1
project:
  name: greeter
  objectives:
    - "Greet whoever reads greeting.txt"
stage: prototype
authorizes: "An internal demonstration"
scope:
  - "The greeting in greeting.txt"
operating_envelope:
  - "One reader at a time"
requirements:
  - id: REQ-1
    domain: functional-correctness
    statement: "The greeting is committed"
    criterion: "git tracks greeting.txt"
    command: "git ls-files --error-unmatch greeting.txt"
  - id: REQ-2
    domain: objective-alignment
    statement: "The greeting is spelled correctly"
    criterion: "greeting.txt reads hello"
  - id: REQ-3
    domain: architecture
    statement: "One file holds the greeting"
    criterion: "The tree has one greeting file"
risks:
  unacceptable:
    - id: RISK-1
      statement: "Nobody is greeted"
`

// newRepo creates a repository with one commit holding a misspelled
// greeting.
func newRepo(t *testing.T) *repo {
	t.Helper()
	r := &repo{t: t, dir: t.TempDir(), env: map[string]string{}}
	r.git("init", "-q")
	r.git("config", "user.name", "Grace Governance")
	r.git("config", "user.email", "grace@example.com")
	r.write("greeting.txt", "helo\n")
	r.commit("add the greeting")
	return r
}

// pinned is newRepo with metron initialized, the test contract written and
// pinned, and everything committed.
func pinned(t *testing.T) *repo {
	t.Helper()
	r := newRepo(t)
	r.must("init")
	r.write(".metron/contract.yaml", testContract)
	r.must("contract", "pin", "-m", "first contract")
	r.commit("pin the contract")
	return r
}

func (r *repo) git(args ...string) string {
	r.t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = r.dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		r.t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
	}
	return strings.TrimSpace(string(out))
}

func (r *repo) path(name string) string { return filepath.Join(r.dir, filepath.FromSlash(name)) }

func (r *repo) write(name, content string) {
	r.t.Helper()
	if err := os.MkdirAll(filepath.Dir(r.path(name)), 0o777); err != nil {
		r.t.Fatal(err)
	}
	if err := os.WriteFile(r.path(name), []byte(content), 0o666); err != nil {
		r.t.Fatal(err)
	}
}

func (r *repo) read(name string) []byte {
	r.t.Helper()
	data, err := os.ReadFile(r.path(name))
	if err != nil {
		r.t.Fatal(err)
	}
	return data
}

// commit commits every change, .metron/ included, and returns the hash.
func (r *repo) commit(msg string) string {
	r.t.Helper()
	r.git("add", "-A")
	r.git("commit", "-q", "--allow-empty", "-m", msg)
	return r.git("rev-parse", "HEAD")
}

type result struct {
	out, err string
	code     int
}

func (r *repo) run(args ...string) result {
	r.t.Helper()
	var stdout, stderr bytes.Buffer
	code := Run(args, Env{
		Stdin:  strings.NewReader(r.stdin),
		Stdout: &stdout,
		Stderr: &stderr,
		Dir:    r.dir,
		Getenv: func(k string) string { return r.env[k] },
		Now:    func() time.Time { return testClock },
	})
	r.stdin = ""
	return result{out: stdout.String(), err: stderr.String(), code: code}
}

// expect runs a command that must exit with the given status.
func (r *repo) expect(code int, args ...string) result {
	r.t.Helper()
	res := r.run(args...)
	if res.code != code {
		r.t.Fatalf("metron %s: exit %d, want %d\nstdout:\n%s\nstderr:\n%s", strings.Join(args, " "), res.code, code, res.out, res.err)
	}
	return res
}

func (r *repo) must(args ...string) result {
	r.t.Helper()
	return r.expect(0, args...)
}

// state replays the repository's ledger.
func (r *repo) state(t *testing.T) core.State {
	t.Helper()
	events, err := store.New(r.path(".metron/state")).Load()
	if err != nil {
		t.Fatal(err)
	}
	st, err := core.Replay(events)
	if err != nil {
		t.Fatal(err)
	}
	return st
}

// next returns the next action, as metron next --json prints it.
func (r *repo) next() actionDoc {
	r.t.Helper()
	var a actionDoc
	unmarshal(r.t, r.must("next", "--json").out, &a)
	return a
}

// expectNext checks the next step, and that next and gate agree: the step
// is none exactly when the gate is open.
func (r *repo) expectNext(step core.Step) actionDoc {
	r.t.Helper()
	a := r.next()
	if a.Step != step {
		r.t.Fatalf("next is %s (%s), want %s", a.Step, a.Reason, step)
	}
	if g := r.run("gate"); (g.code == exitOK) != (a.Step == core.StepNone) {
		r.t.Fatalf("next is %s but gate exits %d:\n%s%s", a.Step, g.code, g.out, g.err)
	}
	return a
}

// packet issues, or returns again, the packet for the next action, and the
// report skeleton for it.
func (r *repo) packet() (core.Packet, core.Report) {
	r.t.Helper()
	var p core.Packet
	if err := jsonv2.Unmarshal([]byte(r.must("packet").out), &p, jsonv2.RejectUnknownMembers(true)); err != nil {
		r.t.Fatalf("decode the packet: %v", err)
	}
	var sk core.Report
	unmarshal(r.t, r.must("packet", "--format", "skeleton").out, &sk)
	return p, sk
}

// ingest submits a report from a file outside the repository.
func (r *repo) ingest(code int, report []byte, args ...string) result {
	r.t.Helper()
	path := filepath.Join(r.t.TempDir(), "report.json")
	if err := os.WriteFile(path, report, 0o666); err != nil {
		r.t.Fatal(err)
	}
	return r.expect(code, append([]string{"ingest", path}, args...)...)
}

func unmarshal(t *testing.T, data string, v any) {
	t.Helper()
	if err := json.Unmarshal([]byte(data), v); err != nil {
		t.Fatalf("decode %T: %v\n%s", v, err, data)
	}
}

func marshal(t *testing.T, v any) []byte {
	t.Helper()
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// complete fills a skeleton's envelope and replaces its body.
func complete(t *testing.T, sk core.Report, author string, body any) []byte {
	t.Helper()
	sk.Author = core.Author{ID: author, Kind: core.AuthorAgent}
	sk.Body = marshal(t, body)
	return marshal(t, sk)
}

// review completes a reviewer skeleton: every domain assessed, every
// requirement satisfied unless statuses says otherwise, the given findings,
// and the verdict they imply.
func review(t *testing.T, sk core.Report, author string, statuses map[string]string, findings ...core.FindingInput) []byte {
	t.Helper()
	return complete(t, sk, author, reviewBody(t, sk, statuses, findings...))
}

func reviewBody(t *testing.T, sk core.Report, statuses map[string]string, findings ...core.FindingInput) core.ReviewerBody {
	t.Helper()
	var b core.ReviewerBody
	unmarshal(t, string(sk.Body), &b)
	for i := range b.Coverage {
		b.Coverage[i].Status, b.Coverage[i].Summary = core.CoverageAssessed, "inspected the tree"
	}
	b.Verdict = core.VerdictGreen
	for i := range b.Requirements {
		st, ok := statuses[b.Requirements[i].Requirement]
		if !ok {
			st = core.RequirementSatisfied
		}
		b.Requirements[i].Status, b.Requirements[i].Evidence = st, "read greeting.txt"
		if st == core.RequirementUnverified {
			b.Verdict = core.VerdictIndeterminate
		}
	}
	for _, f := range findings {
		if f.Tier.Blocking() {
			b.Verdict = core.VerdictRed
		}
	}
	b.Findings = append(b.Findings, findings...)
	b.Justification = "as the requirements and findings show"
	return b
}

// misspelled is a blocking finding against REQ-2.
func misspelled() core.FindingInput {
	return core.FindingInput{Key: "typo", Tier: core.TierMaterial, Domain: core.DomainObjectiveAlignment,
		Title: "The greeting is misspelled", Components: []string{"greeting.txt"},
		Description: "greeting.txt reads helo", Evidence: "cat greeting.txt prints helo", Impact: "Readers are not greeted",
		Requirement: "REQ-2", BlockingRationale: "The demonstration shows the greeting", Verification: "cat greeting.txt prints hello"}
}

// fix completes an implementer skeleton with one commit for every subject.
func fix(t *testing.T, sk core.Report, author, commit string) []byte {
	t.Helper()
	var b core.ImplementerBody
	unmarshal(t, string(sk.Body), &b)
	for i := range b.Fixes {
		b.Fixes[i].Commits = []string{commit}
		b.Fixes[i].Summary = "spell the greeting correctly"
		b.Fixes[i].Components = []string{"greeting.txt"}
	}
	return complete(t, sk, author, b)
}

// verify completes a verifier skeleton with the same result for every fix.
func verify(t *testing.T, sk core.Report, author, result string) []byte {
	t.Helper()
	var b core.VerifierBody
	unmarshal(t, string(sk.Body), &b)
	for i := range b.Verifications {
		b.Verifications[i].Result, b.Verifications[i].Evidence = result, "cat greeting.txt prints hello"
		b.Verifications[i].Scope, b.Verifications[i].ScopeEvidence = core.ScopeProportionate, "the commit changes one word in greeting.txt"
	}
	return complete(t, sk, author, b)
}

// golden compares output with testdata/name, or rewrites it with -update.
func golden(t *testing.T, name, got string) {
	t.Helper()
	path := filepath.Join("testdata", name)
	if *update {
		if err := os.WriteFile(path, []byte(got), 0o666); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("%v; run go test -run %s -update to create it", err, t.Name())
	}
	if string(want) != got {
		t.Errorf("%s differs from the output; if the change is intended, run go test -run %s -update\ngot:\n%s\nwant:\n%s",
			path, t.Name(), got, want)
	}
}

// contains fails unless s contains every part.
func contains(t *testing.T, what, s string, parts ...string) {
	t.Helper()
	for _, p := range parts {
		if !strings.Contains(s, p) {
			t.Errorf("%s does not contain %q:\n%s", what, p, s)
		}
	}
}
