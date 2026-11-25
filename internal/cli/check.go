package cli

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/patacca/metron/internal/canon"
	"github.com/patacca/metron/internal/core"
)

var cmdCheck = &command{
	name:     "check",
	synopsis: "check [--timeout <duration>] [--json]",
	summary:  "Run the requirements' check commands and record the results",
	about: `Check runs the command of every applicable requirement that has one,
through a POSIX shell (sh -c) from the repository root, and captures its
combined output; on Windows the shell is the sh that ships with Git. A check
passes when it exits with status 0.

Each command may run for at most --timeout, a duration such as 90s or 10m;
the default is 30m. A command still running at the limit is stopped and
recorded as a failed result with exit code 124, as timeout(1) reports it,
and the other checks still run.

The commands run without the lock. Then check takes the lock and records the
results against the candidate, provided the candidate, the working tree and
the contract are the ones it started with. A check that leaves the working
tree dirty is reported as such and nothing is recorded: checks must not
change the candidate they check.

It prints one line per requirement and the end of the output of each failing
check, and exits 0 only if every check passed and the results were recorded.
With --json it prints a metron.check/v1 document.`,
	run: runCheck,
}

// At most this much of a failing check's output is shown: the last lines,
// and no more than this many bytes of them.
const (
	tailLines = 40
	tailBytes = 4096
)

const (
	// timeoutExit is the exit code recorded for a command stopped at the
	// limit, the one timeout(1) reports.
	timeoutExit = 124
	// waitDelay is how long the output of a command may stay open once its
	// shell has exited or was stopped, held by a process it left running.
	waitDelay = 5 * time.Second
)

type checkResult struct {
	Requirement  string `json:"requirement"`
	Command      string `json:"command"`
	ExitCode     int    `json:"exit_code"`
	Passed       bool   `json:"passed"`
	TimedOut     bool   `json:"timed_out,omitempty"`
	OutputDigest string `json:"output_digest"`
	// OutputTail is the end of a failing check's combined output: at most
	// its last 40 lines and 4 KiB. OutputTruncated says that it is not all.
	OutputTail      string `json:"output_tail,omitempty"`
	OutputTruncated bool   `json:"output_truncated,omitempty"`
}

type checkDoc struct {
	Schema    string         `json:"schema"`
	Candidate core.Candidate `json:"candidate"`
	Results   []checkResult  `json:"results"`
	// Passed is true when checks ran and every one passed.
	Passed bool `json:"passed"`
	// Recorded is true when the results were recorded in the ledger.
	Recorded bool `json:"recorded"`
	// Problem says why nothing was recorded.
	Problem string `json:"problem,omitempty"`
	// EscalationsRaised lists the escalations a recording with a failure
	// raised: it is a failed step, and enough of them call for an audit.
	EscalationsRaised []string   `json:"escalations_raised"`
	Next              *actionDoc `json:"next,omitempty"`
	// limit is how long each command could run.
	limit time.Duration
}

func runCheck(c *ctx, args []string) error {
	fs := c.flags()
	dir, asJSON := stateFlag(fs), c.jsonFlag(fs)
	timeout := fs.String("timeout", "30m", "stop any check still running after this `duration`")
	args, err := c.parse(fs, args)
	if err != nil {
		return err
	}
	if err := positional(args, 0, 0); err != nil {
		return err
	}
	limit, err := time.ParseDuration(*timeout)
	if err != nil || limit <= 0 {
		return usageError("invalid --timeout %q: give a positive duration, such as 90s or 10m", *timeout)
	}
	s, err := c.open(*dir)
	if err != nil {
		return err
	}
	before, err := c.gather(s)
	if err != nil {
		return err
	}
	if err := before.usable(true); err != nil {
		return err
	}
	if err := before.candidate(); err != nil {
		return err
	}
	d := checkDoc{Schema: "metron.check/v1", Candidate: before.facts.Candidate, Results: []checkResult{}, EscalationsRaised: []string{}, limit: limit}
	// Refuse before running anything when the results could not be recorded.
	if a := core.Next(before.state, before.facts); a.Step == core.StepRepairLedger || a.Step == core.StepPinContract {
		d.Problem = a.Reason
	} else if !before.facts.Clean {
		d.Problem = "the working tree is not clean, so the candidate is not a commit: make it clean as listed below, then run the checks"
	}
	if d.Problem != "" {
		d.Next = before.actionDoc(before.next())
		return c.checked(d, *asJSON, before, nil, false)
	}
	d.Passed = true
	var evidence []core.Evidence
	for _, r := range before.facts.Contract.Applicable() {
		if r.Command == "" {
			continue
		}
		code, output, timedOut, err := runShell(s.repo.Root(), r.Command, limit)
		if err != nil {
			return fail(exitEnv, "cannot run the check of %s: %v", r.ID, err)
		}
		res := checkResult{Requirement: r.ID, Command: r.Command, ExitCode: code, Passed: code == 0, TimedOut: timedOut,
			OutputDigest: canon.Digest(output)}
		if !res.Passed {
			res.OutputTail, res.OutputTruncated = tail(output)
			d.Passed = false
		}
		d.Results = append(d.Results, res)
		evidence = append(evidence, core.Evidence{Requirement: r.ID, Command: r.Command, Candidate: before.facts.Candidate.Digest,
			ExitCode: code, Passed: res.Passed, OutputDigest: res.OutputDigest})
	}
	if len(evidence) == 0 {
		d.Problem = "no applicable requirement has a check command: there is nothing to run"
		d.Passed = false
		d.Next = before.actionDoc(before.next())
		return c.checked(d, *asJSON, before, nil, true)
	}

	wr, after, err := c.lock(s)
	if err != nil {
		return err
	}
	defer wr.Close()
	d.Problem = changedDuringChecks(before, after)
	var events []core.Event
	if d.Problem == "" {
		drafts, err := core.RecordEvidence(after.state, after.facts, evidence)
		var r *core.Rejection
		switch {
		case errors.As(err, &r):
			d.Problem = strings.TrimSpace(rejectionText(r))
		case err != nil:
			return err
		default:
			if events, err = after.commit(wr, drafts); err != nil {
				return err
			}
			d.Recorded = true
			for _, e := range events {
				if e.Type == core.EventEscalationRaised {
					d.EscalationsRaised = append(d.EscalationsRaised, decode[core.EscalationRaised](e).ID)
				}
			}
		}
	}
	if d.Problem != "" {
		d.Problem += "; nothing was recorded"
	}
	d.Next = after.actionDoc(after.next())
	if !after.facts.Clean && before.facts.Clean {
		// What the checks left behind is removed, never committed.
		d.Next.Command, d.Next.Usage, d.Next.leftovers = "", "", true
		d.Next.hint = "remove what the checks left behind, then run " + after.metron("check") + " again"
	}
	return c.checked(d, *asJSON, after, events, d.Passed && d.Recorded)
}

// changedDuringChecks says what changed between the start of the checks and
// the moment their results would be recorded, or returns "".
func changedDuringChecks(before, after *world) string {
	bp, _ := before.state.Pin()
	ap, _ := after.state.Pin()
	switch {
	case after.facts.Candidate != before.facts.Candidate:
		return fmt.Sprintf("HEAD moved from commit %s to %s while the checks ran: run them again on the new candidate",
			short(before.facts.Candidate.Commit), short(after.facts.Candidate.Commit))
	case !after.facts.Clean:
		return "the checks left the working tree unclean, and a check must not change the candidate it checks: remove what they left, as listed below, make the checks write their output to an ignored or temporary path, then run them again"
	case after.facts.ContractDigest != before.facts.ContractDigest || ap.Digest != bp.Digest:
		return "the contract or its pin changed while the checks ran: run them again"
	}
	return ""
}

// checked prints the outcome of check, which is negative unless ok.
func (c *ctx) checked(d checkDoc, asJSON bool, w *world, events []core.Event, ok bool) error {
	var err error
	if asJSON {
		err = c.emit(d)
	} else {
		var b strings.Builder
		for _, r := range d.Results {
			status := "passed"
			if !r.Passed {
				status = "FAILED"
			}
			stopped := ""
			if r.TimedOut {
				stopped = fmt.Sprintf("[timed out: stopped after the %s limit] ", d.limit)
			}
			fmt.Fprintf(&b, "%-8s %s  exit %-3d %s%s\n", r.Requirement, status, r.ExitCode, stopped, printable(r.Command))
			if r.OutputTruncated {
				fmt.Fprintf(&b, "         | [earlier output omitted: only the last %d lines and %d bytes are shown]\n", tailLines, tailBytes)
			}
			if r.OutputTail != "" {
				for _, line := range strings.Split(printable(r.OutputTail), "\n") {
					fmt.Fprintf(&b, "         | %s\n", line)
				}
			}
		}
		switch {
		case d.Recorded:
			fmt.Fprintf(&b, "Recorded %s for commit %s (candidate %s).\n", plural(len(d.Results), "result"), short(d.Candidate.Commit), d.Candidate.Digest)
			for _, e := range events {
				if e.Type == core.EventEscalationRaised {
					fmt.Fprintf(&b, "  escalation raised: %s\n", describe(e))
				}
			}
		case d.Problem != "":
			fmt.Fprintf(&b, "%s.\n", upper(d.Problem))
		}
		b.WriteString("\n")
		writeNext(&b, w, d.Next)
		_, err = io.WriteString(c.Stdout, b.String())
	}
	if err != nil {
		return err
	}
	if !ok {
		return negative
	}
	return nil
}

// runShell runs a check command through sh -c in dir and returns its exit
// status and combined output. A command not over within limit is stopped and
// reported as timed out, with the exit status timeoutExit and the output it
// wrote until then. An error means the shell could not be run at all.
func runShell(dir, command string, limit time.Duration) (int, []byte, bool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), limit)
	defer cancel()
	cmd, err := shellCommand(ctx, command)
	if err != nil {
		return 0, nil, false, err
	}
	cmd.Dir = dir
	cmd.WaitDelay = waitDelay
	var out bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &out
	err = cmd.Run()
	var ee *exec.ExitError
	switch {
	case ctx.Err() != nil:
		// The limit passed before the command was over, whatever it ended with.
		return timeoutExit, out.Bytes(), true, nil
	case err == nil, errors.Is(err, exec.ErrWaitDelay):
		// ErrWaitDelay: the shell exited 0, and a process it left running
		// held the output open for longer than waitDelay.
		return 0, out.Bytes(), false, nil
	case errors.As(err, &ee):
		return ee.ExitCode(), out.Bytes(), false, nil
	}
	return 0, nil, false, err
}

// tail returns the end of a check's output, at most its last tailLines lines
// and tailBytes bytes, without trailing blank space and carriage returns,
// and whether anything was left out.
func tail(output []byte) (string, bool) {
	lines := strings.Split(strings.TrimRight(string(output), "\r\n\t "), "\n")
	truncated := len(lines) > tailLines
	if truncated {
		lines = lines[len(lines)-tailLines:]
	}
	for i := range lines {
		lines[i] = strings.TrimRight(lines[i], "\r")
	}
	s := strings.Join(lines, "\n")
	if len(s) > tailBytes {
		s, truncated = s[len(s)-tailBytes:], true
		// Never start in the middle of a character.
		for s != "" && !utf8.RuneStart(s[0]) {
			s = s[1:]
		}
	}
	return s, truncated
}

// printable escapes the control characters other than newline and tab, and
// the Unicode format characters (category Cf, such as the bidirectional
// overrides), so that printed text cannot drive the terminal or disguise
// itself.
func printable(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch {
		case r == '\n' || r == '\t':
			b.WriteRune(r)
		case r < 0x20 || r == 0x7f:
			fmt.Fprintf(&b, "\\x%02x", r)
		case r >= 0x80 && r < 0xa0, unicode.Is(unicode.Cf, r) && r <= 0xffff:
			fmt.Fprintf(&b, "\\u%04x", r)
		case unicode.Is(unicode.Cf, r):
			fmt.Fprintf(&b, "\\U%08x", r)
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

func upper(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}
