package cli

import (
	"encoding/json"
	"os"

	"strings"
	"testing"

	"github.com/patacca/metron/internal/core"
)

func TestShellQuote(t *testing.T) {
	for _, s := range []string{
		"plain/dir-1.2", "with space", "it's", `say "hi"`, "$(touch PWNED)", "`id > PWNED2`", "a;rm -rf b",
		"line\nbreak", "naïve/状态", "'", "", `back\slash`, "$HOME",
	} {
		q := shellQuote(s)
		cmd, err := shellCommand("printf '%s' " + q)
		if err != nil {
			t.Fatal(err)
		}
		cmd.Dir = t.TempDir()
		out, err := cmd.CombinedOutput()
		if err != nil || string(out) != s {
			t.Errorf("shellQuote(%q) = %s: the shell read %q (%v)", s, q, out, err)
		}
		if entries, _ := os.ReadDir(cmd.Dir); len(entries) > 0 {
			t.Errorf("shellQuote(%q) = %s: the shell created %s", s, q, entries[0].Name())
		}
	}
	if got := shellQuote("plain/dir-1.2"); got != "plain/dir-1.2" {
		t.Errorf("a plain path is quoted: %s", got)
	}
}

// hostile are committed file names that a shell would execute.
var hostile = []string{"docs/$(touch PWNED).md", "docs/`id > PWNED2`.md"}

// TestHostileFileNames commits files whose names hold command substitutions
// and leaves them out, by a sparse checkout or by skip-worktree flags: no
// command metron prints may hold them, and running the printed command
// executes nothing.
func TestHostileFileNames(t *testing.T) {
	for _, sparse := range []bool{true, false} {
		t.Run(map[bool]string{true: "sparse checkout", false: "skip-worktree"}[sparse], func(t *testing.T) {
			hostileFileNames(t, sparse)
		})
	}
}

func hostileFileNames(t *testing.T, sparse bool) {
	r := pinned(t)
	blob := r.git("hash-object", "-w", "greeting.txt")
	for _, name := range hostile {
		r.git("update-index", "--add", "--cacheinfo", "100644,"+blob+","+name)
	}
	r.git("commit", "-q", "-m", "add documents")
	if sparse {
		r.git("sparse-checkout", "set", ".metron")
	} else {
		r.git(append([]string{"update-index", "--skip-worktree", "--"}, hostile...)...)
	}
	r.expectNext(core.StepCommitChanges)

	var outputs []string
	for _, args := range [][]string{
		{"next"}, {"next", "--json"}, {"status"}, {"status", "--json"}, {"check"}, {"check", "--json"},
		{"gate"}, {"gate", "--json"}, {"packet"}, {"fsck"}, {"findings", "--all"}, {"log"}, {"contract"},
	} {
		res := r.run(args...)
		outputs = append(outputs, res.out, res.err)
		if strings.Contains(strings.Join(args, " "), "--json") {
			var doc any
			if err := json.Unmarshal([]byte(res.out), &doc); err != nil {
				t.Fatalf("metron %s: %v", strings.Join(args, " "), err)
			}
			checkCommands(t, strings.Join(args, " "), doc)
		}
	}
	for _, out := range outputs {
		for _, line := range strings.Split(out, "\n") {
			if strings.Contains(line, "PWNED") && (strings.Contains(line, "git ") || strings.Contains(line, "metron ")) {
				t.Errorf("a command line holds a file name: %q", line)
			}
		}
	}
	// What next offers to run, run as it stands, executes nothing hostile.
	if a := r.next(); a.Command != "" {
		cmd, err := shellCommand(a.Command)
		if err != nil {
			t.Fatal(err)
		}
		cmd.Dir = r.dir
		cmd.Run()
	}
	for _, name := range []string{"PWNED", "PWNED2", "docs/PWNED", "docs/PWNED2"} {
		if _, err := os.Stat(r.path(name)); !os.IsNotExist(err) {
			t.Fatalf("%s exists: a file name was executed", name)
		}
	}
}

// checkCommands fails when a command or usage member of a JSON document holds
// a hostile file name.
func checkCommands(t *testing.T, what string, doc any) {
	t.Helper()
	switch v := doc.(type) {
	case map[string]any:
		for k, x := range v {
			if s, ok := x.(string); ok && (k == "command" || k == "usage") && strings.Contains(s, "PWNED") {
				t.Errorf("metron %s: %s %q holds a file name", what, k, s)
			}
			checkCommands(t, what, x)
		}
	case []any:
		for _, x := range v {
			checkCommands(t, what, x)
		}
	}
}
