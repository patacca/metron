package cli

import (
	"bytes"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/patacca/metron/internal/core"
)

// End-to-end campaigns that test what the ledger is: the same commands give
// the same bytes, the state directory and the repository are all a campaign
// needs to continue after a crash, and an accepted report can be submitted
// again at any time without changing anything.

// move is one step of a scripted campaign: a metron command, or a change made
// with git between two. A move that ingests a report returns its bytes.
type move struct {
	metron bool
	run    func(r *repo) []byte
}

// scriptContract is the contract of the scripted campaign: a failing check is
// its second failed step and calls for a convergence analysis.
var scriptContract = campaignContract(2, 2, 3, 20)

// script is a campaign through a RED round, a fix that breaks a check, the
// convergence analysis that follows, a claim that lapses and is made again,
// a confirmed fix and a GREEN round: every command and every commit it
// needs, in order. Each move takes what it needs from the repository and
// the state directory, so that the script can resume after any of them.
func script() []move {
	command := func(f func(r *repo)) move { return move{metron: true, run: func(r *repo) []byte { f(r); return nil }} }
	change := func(f func(r *repo)) move { return move{run: func(r *repo) []byte { f(r); return nil }} }
	check := func(code int) move { return command(func(r *repo) { r.expect(code, "check") }) }
	issue := command(func(r *repo) { r.must("packet") })
	answer := func(build func(r *repo, sk core.Report) []byte) move {
		return move{metron: true, run: func(r *repo) []byte {
			var sk core.Report
			unmarshal(r.t, r.must("packet", "--format", "skeleton").out, &sk)
			report := build(r, sk)
			r.ingest(exitOK, report)
			return report
		}}
	}
	head := func(r *repo, rev string) string { return r.git("rev-parse", rev) }
	return []move{
		check(exitOK),
		issue,
		answer(func(r *repo, sk core.Report) []byte { return red(r.t, sk, "rev-1", misspelled()) }),
		issue,
		change(func(r *repo) {
			r.write("greeting.txt", "hello\n")
			r.write("broken", "yes\n")
			r.commitOnly("spell the greeting, and break the check", "greeting.txt", "broken")
		}),
		answer(func(r *repo, sk core.Report) []byte { return claim(r.t, sk, "impl-1", "", head(r, "HEAD")) }),
		check(exitNegative),
		issue,
		answer(func(r *repo, sk core.Report) []byte { return upheld(r.t, sk, "aud-1") }),
		change(func(r *repo) { r.remove("broken"); r.commitOnly("repair the check", "broken") }),
		issue,
		answer(func(r *repo, sk core.Report) []byte {
			return claim(r.t, sk, "impl-1", "", head(r, "HEAD~1"), head(r, "HEAD"))
		}),
		check(exitOK),
		issue,
		answer(func(r *repo, sk core.Report) []byte { return verify(r.t, sk, "ver-1", core.VerificationConfirmed) }),
		issue,
		answer(func(r *repo, sk core.Report) []byte { return review(r.t, sk, "rev-1", nil) }),
		command(func(r *repo) { r.must("gate") }),
	}
}

// stateFiles returns every file of the state directory but the lock, by path.
func stateFiles(r *repo) map[string][]byte {
	r.t.Helper()
	out := map[string][]byte{}
	root := r.path(".metron/state")
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || d.Name() == "lock" {
			return err
		}
		rel, err := filepath.Rel(root, path)
		if err == nil {
			out[filepath.ToSlash(rel)], err = os.ReadFile(path)
		}
		return err
	})
	if err != nil {
		r.t.Fatal(err)
	}
	return out
}

// sameState fails unless two state directories hold the same files with the
// same bytes.
func sameState(t *testing.T, got, want map[string][]byte) {
	t.Helper()
	var names []string
	for name := range want {
		names = append(names, name)
	}
	for name := range got {
		if _, ok := want[name]; !ok {
			names = append(names, name)
		}
	}
	slices.Sort(names)
	for _, name := range names {
		if !bytes.Equal(got[name], want[name]) {
			t.Errorf("state file %s differs:\n got %q\nwant %q", name, got[name], want[name])
		}
	}
}

// TestCampaignDeterministic runs the scripted campaign in two fresh
// repositories, with the same clock, identities and commit dates: the two
// state directories, ledger, packets and reports, are byte for byte the same.
func TestCampaignDeterministic(t *testing.T) {
	var states [2]map[string][]byte
	for i := range states {
		r := campaign(t, scriptContract)
		for _, m := range script() {
			m.run(r)
		}
		states[i] = stateFiles(r)
	}
	if len(states[0]["ledger.jsonl"]) == 0 {
		t.Fatal("the campaign wrote no ledger")
	}
	sameState(t, states[1], states[0])
}

// TestCampaignResumesAfterAnyCommand stops the scripted campaign after each of
// its commands, as a crash would, and continues it from a copy of the state
// directory in a clone of the repository at that point: the final state
// directory is the one the uninterrupted campaign wrote.
func TestCampaignResumesAfterAnyCommand(t *testing.T) {
	moves := script()
	r := campaign(t, scriptContract)
	type point struct {
		next  int
		head  string
		state string
	}
	save := func(next int) point {
		p := point{next: next, head: r.git("rev-parse", "HEAD"), state: filepath.Join(t.TempDir(), "state")}
		if err := os.CopyFS(p.state, os.DirFS(r.path(".metron/state"))); err != nil {
			t.Fatal(err)
		}
		return p
	}
	points := []point{save(0)}
	for i, m := range moves {
		m.run(r)
		if m.metron {
			points = append(points, save(i+1))
		}
	}
	want := stateFiles(r)
	for _, p := range points {
		t.Run(fmt.Sprintf("from move %d", p.next), func(t *testing.T) {
			c := &repo{t: t, dir: t.TempDir(), env: map[string]string{}}
			r.git("clone", "-q", r.dir, c.dir)
			c.git("config", "user.name", "Grace Governance")
			c.git("config", "user.email", "grace@example.com")
			c.git("reset", "-q", "--hard", p.head)
			if err := os.RemoveAll(c.path(".metron/state")); err != nil {
				t.Fatal(err)
			}
			if err := os.CopyFS(c.path(".metron/state"), os.DirFS(p.state)); err != nil {
				t.Fatal(err)
			}
			for _, m := range moves[p.next:] {
				m.run(c)
			}
			sameState(t, stateFiles(c), want)
		})
	}
}

// TestCampaignIngestAgain submits every report accepted so far again after
// each move of the scripted campaign, later packets issued or not: each is
// accepted as a duplicate and nothing is recorded.
func TestCampaignIngestAgain(t *testing.T) {
	r := campaign(t, scriptContract)
	var accepted [][]byte
	for i, m := range script() {
		if report := m.run(r); report != nil {
			accepted = append(accepted, report)
		}
		ledger := r.read(".metron/state/ledger.jsonl")
		for j, report := range accepted {
			if d := r.ingestDoc(exitOK, report); !d.Accepted || !d.Duplicate || len(d.Events) != 0 || d.Restored {
				t.Fatalf("after move %d, report %d submitted again: %+v", i, j, d)
			}
		}
		if !bytes.Equal(ledger, r.read(".metron/state/ledger.jsonl")) {
			t.Fatalf("after move %d, submitting accepted reports again changed the ledger", i)
		}
	}
	if len(accepted) != 6 {
		t.Fatalf("the script had %d reports accepted, want 6", len(accepted))
	}
}
