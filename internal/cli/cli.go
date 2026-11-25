// Package cli is the metron command line. It parses arguments, gathers the
// facts the core decides on, commits the events the core returns, writes the
// files the ledger refers to, and prints the outcome for people (plain text)
// and orchestrators (--json).
//
// docs/architecture.md, section 12, is the command surface and the meaning of
// the exit statuses.
package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	"github.com/patacca/metron/internal/core"
)

// Exit statuses, docs/architecture.md section 12.
const (
	exitOK       = 0
	exitNegative = 1
	exitUsage    = 2
	exitEnv      = 3
)

// Env is what a command observes of its process. Tests run commands
// in-process with their own streams, directory, environment and clock.
type Env struct {
	Stdin  io.Reader
	Stdout io.Writer
	Stderr io.Writer
	// Dir is the working directory; relative paths are resolved against it.
	Dir string
	// Getenv looks up an environment variable.
	Getenv func(string) string
	// Now returns the current time.
	Now func() time.Time
}

// command is one metron subcommand.
type command struct {
	name string
	// synopsis follows "metron" in the usage line.
	synopsis string
	summary  string
	about    string
	run      func(c *ctx, args []string) error
}

var commands []*command

func init() {
	commands = []*command{
		cmdInit, cmdContract, cmdNext, cmdCheck, cmdPacket, cmdIngest, cmdResolve,
		cmdGate, cmdStatus, cmdFindings, cmdLog, cmdFsck, cmdSchema, cmdProtocol, cmdVersion, cmdHelp,
	}
}

func lookup(name string) *command {
	for _, c := range commands {
		if c.name == name {
			return c
		}
	}
	return nil
}

// ctx is one invocation of one command.
type ctx struct {
	Env
	cmd *command
	// asJSON is the command's --json flag, when it has one. jsonArg is set
	// when the arguments ask for JSON, even if they fail to parse. printed is
	// set once a JSON document was printed.
	asJSON  *bool
	jsonArg bool
	printed bool
}

// Run executes metron with args, the command line without the program name,
// and returns the exit status.
func Run(args []string, env Env) int {
	c := &ctx{Env: env, cmd: cmdHelp}
	if len(args) == 0 {
		fmt.Fprint(c.Stderr, overview())
		return exitUsage
	}
	name := args[0]
	switch name {
	case "-h", "-help", "--help":
		fmt.Fprint(c.Stdout, overview())
		return exitOK
	}
	if c.cmd = lookup(name); c.cmd == nil {
		hint := "run 'metron help' for the list of commands"
		if strings.HasPrefix(name, "-") {
			hint = "flags go after the command name, as in 'metron next --state-dir <dir>'"
		}
		fmt.Fprintf(c.Stderr, "metron: unknown command %q: %s\n", name, hint)
		return exitUsage
	}
	return c.finish(c.cmd.run(c, args[1:]))
}

// exitError ends a command with a status. Its message, if any, goes to
// standard error.
type exitError struct {
	status int
	msg    string
}

func (e *exitError) Error() string { return e.msg }

// errShown ends a command that printed its help.
var errShown = &exitError{status: exitOK}

// negative ends a command whose output already reports a negative outcome.
var negative = &exitError{status: exitNegative}

func usageError(format string, args ...any) error {
	return &exitError{status: exitUsage, msg: fmt.Sprintf(format, args...)}
}

func fail(status int, format string, args ...any) error {
	return &exitError{status: status, msg: fmt.Sprintf(format, args...)}
}

// finish prints an error and maps it to an exit status: a refusal by the core
// is a negative outcome, anything unexpected an environment or internal error.
func (c *ctx) finish(err error) int {
	status, msg := outcome(err)
	if msg != "" {
		fmt.Fprintf(c.Stderr, "metron %s: %s\n", c.cmd.name, msg)
	}
	if status == exitUsage {
		fmt.Fprintf(c.Stderr, "Run 'metron help %s' for usage.\n", strings.Fields(c.cmd.name)[0])
	}
	// A command asked for JSON prints exactly one document, failure included.
	if status != exitOK && c.asJSON != nil && (*c.asJSON || c.jsonArg) && !c.printed {
		if err := c.emit(errorDoc{Schema: "metron.error/v1", Status: status, Message: msg}); err != nil {
			fmt.Fprintf(c.Stderr, "metron %s: %v\n", c.cmd.name, err)
		}
	}
	return status
}

// errorDoc is what a command asked for JSON prints when it fails.
type errorDoc struct {
	Schema  string `json:"schema"`
	Status  int    `json:"status"`
	Message string `json:"message"`
}

// outcome maps an error to an exit status and the message that explains it.
func outcome(err error) (int, string) {
	var ee *exitError
	var r *core.Rejection
	switch {
	case err == nil:
		return exitOK, ""
	case errors.As(err, &ee):
		return ee.status, ee.msg
	case errors.As(err, &r):
		return exitNegative, strings.TrimSuffix(rejectionText(r), "\n")
	}
	return exitEnv, err.Error()
}

// rejectionText renders a refusal. A single problem about the command as a
// whole reads as a sentence; anything else lists the problems by location.
func rejectionText(r *core.Rejection) string {
	if len(r.Problems) == 1 && r.Problems[0].Path == "" {
		return r.Problems[0].Message + "\n"
	}
	var b strings.Builder
	fmt.Fprintf(&b, "rejected by %s validation:\n", r.Layer)
	for _, p := range r.Problems {
		b.WriteString("  ")
		if p.Path != "" {
			b.WriteString(p.Path + ": ")
		}
		b.WriteString(p.Message + "\n")
	}
	return b.String()
}

// flags returns an empty flag set for the command.
func (c *ctx) flags() *flag.FlagSet {
	fs := flag.NewFlagSet("metron "+c.cmd.name, flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	return fs
}

func stateFlag(fs *flag.FlagSet) *string {
	return fs.String("state-dir", "", "the state `directory` (default $METRON_STATE_DIR, else .metron/state)")
}

// jsonFlag defines --json, which makes the command print one JSON document,
// an error document if it fails.
func (c *ctx) jsonFlag(fs *flag.FlagSet) *bool {
	c.asJSON = fs.Bool("json", false, "print one JSON document instead of text")
	return c.asJSON
}

// jsonRequested reports whether arguments that may not parse ask for --json.
func jsonRequested(args []string) bool {
	on := false
	for _, a := range args {
		if a == "--" {
			break
		}
		name, value, set := strings.Cut(strings.TrimLeft(a, "-"), "=")
		if !strings.HasPrefix(a, "-") || name != "json" {
			continue
		}
		on = true
		if set {
			on, _ = strconv.ParseBool(value)
		}
	}
	return on
}

// parse parses flags that may appear before, between or after the positional
// arguments, which it returns. "--" ends the flags.
func (c *ctx) parse(fs *flag.FlagSet, args []string) ([]string, error) {
	c.jsonArg = c.asJSON != nil && jsonRequested(args)
	var pos []string
	for {
		err := fs.Parse(args)
		if errors.Is(err, flag.ErrHelp) {
			c.help(fs)
			return nil, errShown
		}
		if err != nil {
			return nil, usageError("%v", err)
		}
		rest := fs.Args()
		if len(rest) == 0 {
			return pos, nil
		}
		if used := len(args) - len(rest); used > 0 && args[used-1] == "--" {
			return append(pos, rest...), nil
		}
		pos = append(pos, rest[0])
		args = rest[1:]
	}
}

// positional checks the number of positional arguments.
func positional(args []string, min, max int) error {
	switch {
	case len(args) < min:
		return usageError("missing argument")
	case len(args) > max:
		return usageError("unexpected argument %q", args[max])
	}
	return nil
}

func (c *ctx) help(fs *flag.FlagSet) {
	var b strings.Builder
	fmt.Fprintf(&b, "Usage: metron %s\n\n%s\n", c.cmd.synopsis, strings.TrimSpace(c.cmd.about))
	var defaults bytes.Buffer
	fs.SetOutput(&defaults)
	fs.PrintDefaults()
	fs.SetOutput(io.Discard)
	if defaults.Len() > 0 {
		fmt.Fprintf(&b, "\nFlags:\n%s", defaults.String())
	}
	fmt.Fprint(c.Stdout, b.String())
}

func overview() string {
	var b strings.Builder
	b.WriteString(`Metron governs engineering review rounds. It names the next permitted action,
issues packets to the roles, validates their reports, and decides whether one
commit is approved for one lifecycle stage.

Usage: metron <command> [flags] [arguments]

Commands:
`)
	for _, c := range commands {
		fmt.Fprintf(&b, "  %-10s %s\n", c.name, c.summary)
	}
	b.WriteString(`
Every command accepts -h. Commands that use the state directory accept
--state-dir <dir>; the default is $METRON_STATE_DIR, else .metron/state in the
repository.

Exit status: 0 success (for gate: the gate is open); 1 a negative outcome
(gate closed, report rejected, contract invalid or unpinned, a check failed);
2 a usage error; 3 an environment or internal error.
`)
	return b.String()
}

var cmdHelp = &command{
	name:     "help",
	synopsis: "help [command]",
	summary:  "Print help for a command",
	about:    "Help prints the list of commands, or the help of one command.",
	run: func(c *ctx, args []string) error {
		fs := c.flags()
		args, err := c.parse(fs, args)
		if err != nil {
			return err
		}
		if err := positional(args, 0, 1); err != nil {
			return err
		}
		if len(args) == 0 {
			fmt.Fprint(c.Stdout, overview())
			return nil
		}
		target := lookup(args[0])
		if target == nil {
			return usageError("unknown command %q", args[0])
		}
		c.cmd = target
		return target.run(c, []string{"-h"})
	},
}

// emit prints one indented JSON document. HTML characters are not escaped,
// so identities such as "Name <email>" read as written.
func (c *ctx) emit(v any) error {
	var b bytes.Buffer
	enc := json.NewEncoder(&b)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(v); err != nil {
		return err
	}
	c.printed = true
	_, err := c.Stdout.Write(b.Bytes())
	return err
}
