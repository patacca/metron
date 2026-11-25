package cli

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"runtime/debug"
	"strings"

	"github.com/patacca/metron/internal/brief"
	"github.com/patacca/metron/internal/core"
	"github.com/patacca/metron/internal/schema"
	"github.com/patacca/metron/internal/store"
)

var cmdInit = &command{
	name:     "init",
	synopsis: "init",
	summary:  "Create .metron/ with a contract template and an initialized ledger",
	about: `Init prepares the repository for Metron. It creates .metron/contract.yaml from
a commented template, a .metron/.gitignore that keeps the state directory's
lock and temporary file out of git, the state directory, and an initialized
ledger. It never overwrites anything: it creates what is missing and says
what it found.`,
	run: runInit,
}

// gitignore keeps the runtime files of the default state directory out of
// git. Everything else under .metron/ is meant to be committed.
const gitignore = `# Written by metron init. The lock and the temporary file are runtime files of
# the state directory. Commit everything else under .metron/, state/ included,
# together with the work.
/state/lock
/state/*.tmp
`

func runInit(c *ctx, args []string) error {
	fs := c.flags()
	dir := stateFlag(fs)
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
	meta := filepath.Join(s.repo.Root(), ".metron")
	var b strings.Builder
	created := false
	for _, f := range []struct {
		path string
		data []byte
	}{
		{filepath.Join(meta, "contract.yaml"), contractTemplate},
		{filepath.Join(meta, ".gitignore"), []byte(gitignore)},
	} {
		made, err := createFile(f.path, f.data)
		if err != nil {
			return err
		}
		if made {
			fmt.Fprintf(&b, "Created %s.\n", s.show(f.path))
		} else {
			fmt.Fprintf(&b, "Found %s; left it as it is.\n", s.show(f.path))
		}
		created = created || made && f.path == filepath.Join(meta, "contract.yaml")
	}
	if err := os.MkdirAll(s.store.Dir(), 0o777); err != nil {
		return err
	}
	ledger := s.show(filepath.Join(s.store.Dir(), store.LedgerName))
	if err := initLedger(c, s, ledger, &b); err != nil {
		io.WriteString(c.Stdout, b.String())
		return err
	}
	w, err := c.gather(s, false)
	if err != nil {
		io.WriteString(c.Stdout, b.String())
		return err
	}
	if created {
		fmt.Fprintf(&b, "\nFill in %s: every empty value is a decision for\ngovernance, and the comments explain each member. %s lists what\nis still missing.\n",
			s.show(w.contract.path), s.metron("contract"))
	}
	if !filepath.IsAbs(s.show(s.store.Dir())) {
		b.WriteString("Commit .metron/, state included, together with the work: git history is the\nonly anchor the ledger has.\n")
	}
	b.WriteString("\n")
	writeNext(&b, w, w.next())
	_, err = io.WriteString(c.Stdout, b.String())
	return err
}

// initLedger initializes the ledger under the lock unless it exists.
func initLedger(c *ctx, s *site, ledger string, b *strings.Builder) error {
	wr, err := s.store.Lock()
	if err != nil {
		return err
	}
	defer wr.Close()
	events, err := s.store.Load()
	switch {
	case errors.Is(err, fs.ErrNotExist):
		drafts, err := core.Initialize(core.State{})
		if err != nil {
			return err
		}
		sealed, err := store.Seal(nil, c.Now().UTC().Format(store.TimeFormat), drafts)
		if err != nil {
			return err
		}
		if err := wr.Append(sealed); err != nil {
			return err
		}
		fmt.Fprintf(b, "Created %s, protocol %s.\n", ledger, core.ProtocolVersion)
		return nil
	case err != nil:
		return fail(exitEnv, "found %s, but it fails verification: %v\nRun %s and restore the ledger from version control.", ledger, err, s.metron("fsck"))
	}
	if _, err := core.Replay(events); err != nil {
		return fail(exitEnv, "found %s, but it does not replay: %v\nRun %s and restore the ledger from version control.", ledger, err, s.metron("fsck"))
	}
	fmt.Fprintf(b, "Found %s with %s; left it as it is.\n", ledger, plural(len(events), "event"))
	return nil
}

// createFile writes data to a new file and reports whether it did; an
// existing file is left untouched.
func createFile(path string, data []byte) (bool, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o777); err != nil {
		return false, err
	}
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o666)
	if errors.Is(err, fs.ErrExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	_, err = f.Write(data)
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		os.Remove(path)
		return false, err
	}
	return true, nil
}

var cmdSchema = &command{
	name:     "schema",
	synopsis: "schema [name]",
	summary:  "List the embedded JSON Schemas, or print one",
	about: `Schema prints the embedded JSON Schema of the given name, the canonical public
protocol, or lists the names when none is given.`,
	run: func(c *ctx, args []string) error {
		fs := c.flags()
		args, err := c.parse(fs, args)
		if err != nil {
			return err
		}
		if err := positional(args, 0, 1); err != nil {
			return err
		}
		names := schema.Names()
		if len(args) == 0 {
			_, err := fmt.Fprintf(c.Stdout, "%s\n", strings.Join(names, "\n"))
			return err
		}
		data, err := schema.Bytes(args[0])
		if err != nil {
			return usageError("unknown schema %q: choose one of %s", args[0], strings.Join(names, ", "))
		}
		_, err = c.Stdout.Write(data)
		return err
	},
}

var cmdProtocol = &command{
	name:     "protocol",
	synopsis: "protocol",
	summary:  "Print the Metron Engineering Review Protocol",
	about:    "Protocol prints the text of the review protocol, " + core.ProtocolVersion + ",\nthat every role works under.",
	run: func(c *ctx, args []string) error {
		fs := c.flags()
		args, err := c.parse(fs, args)
		if err != nil {
			return err
		}
		if err := positional(args, 0, 0); err != nil {
			return err
		}
		text, err := brief.Protocol()
		if err != nil {
			return err
		}
		_, err = c.Stdout.Write(text)
		return err
	},
}

var cmdVersion = &command{
	name:     "version",
	synopsis: "version [--json]",
	summary:  "Print version information",
	about: `Version prints the module version and the VCS revision recorded in the build,
and the protocol version this binary implements. With --json it prints a
metron.version/v1 document.`,
	run: runVersion,
}

type versionDoc struct {
	Schema   string `json:"schema"`
	Version  string `json:"version"`
	Revision string `json:"revision,omitempty"`
	Time     string `json:"time,omitempty"`
	Modified bool   `json:"modified"`
	Protocol string `json:"protocol"`
	Go       string `json:"go"`
}

func runVersion(c *ctx, args []string) error {
	fs := c.flags()
	asJSON := jsonFlag(fs)
	args, err := c.parse(fs, args)
	if err != nil {
		return err
	}
	if err := positional(args, 0, 0); err != nil {
		return err
	}
	d := versionDoc{Schema: "metron.version/v1", Version: "unknown", Protocol: core.ProtocolVersion}
	if info, ok := debug.ReadBuildInfo(); ok {
		d.Version, d.Go = info.Main.Version, info.GoVersion
		for _, s := range info.Settings {
			switch s.Key {
			case "vcs.revision":
				d.Revision = s.Value
			case "vcs.time":
				d.Time = s.Value
			case "vcs.modified":
				d.Modified = s.Value == "true"
			}
		}
	}
	if *asJSON {
		return writeJSON(c.Stdout, d)
	}
	var b strings.Builder
	fmt.Fprintf(&b, "metron %s\n", d.Version)
	if d.Revision != "" {
		modified := ""
		if d.Modified {
			modified = ", with uncommitted changes"
		}
		fmt.Fprintf(&b, "revision %s (%s%s)\n", d.Revision, d.Time, modified)
	}
	fmt.Fprintf(&b, "protocol %s\n", d.Protocol)
	if d.Go != "" {
		fmt.Fprintf(&b, "built with %s\n", d.Go)
	}
	_, err = io.WriteString(c.Stdout, b.String())
	return err
}
