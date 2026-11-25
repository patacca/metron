package cli

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"runtime/debug"
	"slices"
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
a commented template; a .metron/.gitignore that keeps the state directory's
lock and temporary file, and the scratch space .metron/work/, out of git; a
.metron/.gitattributes that stops git from converting line endings under
state/; the state directory; and an initialized ledger. It never overwrites
anything: it creates what is missing, adds the lines an existing .gitignore
or .gitattributes lacks, and says what it found.`,
	run: runInit,
}

// metronFile is a file init writes under .metron/: a header for a new file,
// and the lines it must hold, which init adds to an existing file that lacks
// them.
type metronFile struct {
	name   string
	header string
	lines  []string
}

var metronFiles = []metronFile{
	// The lock and the temporary file are runtime files of the state
	// directory, and work/ is scratch space. Everything else under .metron/
	// is meant to be committed.
	{".gitignore", `# Written by metron init. The lock and the temporary file are runtime files of
# the state directory, and work/ is scratch space for skeletons and reports
# being written. Commit everything else under .metron/, state/ included,
# together with the work.
`, []string{"/state/lock", "/state/*.tmp", "/work/"}},
	// Stored reports are compared byte for byte with what was ingested.
	{".gitattributes", `# Written by metron init. Stored reports are compared byte for byte with what
# was ingested, so git must never convert line endings under state/.
`, []string{"state/** -text"}},
}

// ensure creates a .metron/ file, or adds the lines an existing one lacks,
// and says what it did.
func (f metronFile) ensure(s *site, meta string, b *strings.Builder) error {
	path := filepath.Join(meta, f.name)
	made, err := createFile(path, []byte(f.header+strings.Join(f.lines, "\n")+"\n"))
	switch {
	case err != nil:
		return err
	case made:
		fmt.Fprintf(b, "Created %s.\n", s.show(path))
		return nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	have := strings.Split(strings.ReplaceAll(string(data), "\r\n", "\n"), "\n")
	var missing []string
	for _, l := range f.lines {
		if !slices.ContainsFunc(have, func(h string) bool { return strings.TrimSpace(h) == l }) {
			missing = append(missing, l)
		}
	}
	if len(missing) == 0 {
		fmt.Fprintf(b, "Found %s; left it as it is.\n", s.show(path))
		return nil
	}
	add := strings.Join(missing, "\n") + "\n"
	if len(data) > 0 && data[len(data)-1] != '\n' {
		add = "\n" + add
	}
	out, err := os.OpenFile(path, os.O_WRONLY|os.O_APPEND, 0)
	if err != nil {
		return err
	}
	_, err = out.WriteString(add)
	if cerr := out.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		return err
	}
	fmt.Fprintf(b, "Added %s to %s.\n", strings.Join(missing, ", "), s.show(path))
	return nil
}

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
	contract := filepath.Join(meta, "contract.yaml")
	created, err := createFile(contract, contractTemplate)
	if err != nil {
		return err
	}
	if created {
		fmt.Fprintf(&b, "Created %s.\n", s.show(contract))
	} else {
		fmt.Fprintf(&b, "Found %s; left it as it is.\n", s.show(contract))
	}
	for _, f := range metronFiles {
		if err := f.ensure(s, meta, &b); err != nil {
			return err
		}
	}
	if err := os.MkdirAll(s.store.Dir(), 0o777); err != nil {
		return err
	}
	ledger := s.show(filepath.Join(s.store.Dir(), store.LedgerName))
	if err := initLedger(c, s, ledger, &b); err != nil {
		io.WriteString(c.Stdout, b.String())
		return err
	}
	w, err := c.gather(s)
	if err != nil {
		io.WriteString(c.Stdout, b.String())
		return err
	}
	if created {
		fmt.Fprintf(&b, "\nFill in %s: every empty value is a decision for\ngovernance, and the comments explain each member. %s lists what\nis still missing.\n",
			s.show(w.contract.path), s.metron("contract"))
	}
	if s.inRepo {
		b.WriteString("Commit .metron/, state included, together with the work: git history is the\nonly anchor the ledger has.\n")
	}
	b.WriteString("\n")
	writeNext(&b, w, w.actionDoc(w.next()))
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
	case errors.Is(err, fs.ErrNotExist), err == nil && len(events) == 0:
		verb := "Created"
		if err == nil {
			verb = "Initialized the empty"
		}
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
		fmt.Fprintf(b, "%s %s, protocol %s.\n", verb, ledger, core.ProtocolVersion)
		return nil
	case err != nil:
		return fail(exitEnv, "found %s, but it fails verification: %s\nRun %s for the details.", ledger, restoreLedger(err.Error()), s.metron("fsck"))
	}
	if _, err := core.Replay(events); err != nil {
		return fail(exitEnv, "found %s, but it does not replay: %s\nRun %s for the details.", ledger, restoreLedger(err.Error()), s.metron("fsck"))
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
	asJSON := c.jsonFlag(fs)
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
		return c.emit(d)
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
