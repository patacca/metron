package cli

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
)

// findShell locates the POSIX sh that runs check commands where it has no
// fixed path: on PATH first, then in the Git installation that git's exec
// path lies in. Git for Windows keeps git-core under <git>/mingw64/libexec,
// and sh in <git>/bin and <git>/usr/bin. The one in <git>/bin comes first: it
// puts Git's own tools, grep and sed among them, on PATH, which the one in
// <git>/usr/bin, started directly, does not. exe is the file name of sh on
// the platform.
func findShell(lookPath func(string) (string, error), gitExecPath func() (string, error), exe string) (string, error) {
	if p, err := lookPath("sh"); err == nil {
		return p, nil
	}
	if out, err := gitExecPath(); err == nil && strings.TrimSpace(out) != "" {
		dir := filepath.Clean(filepath.FromSlash(strings.TrimSpace(out)))
		for range 4 {
			for _, sub := range []string{"bin", "usr/bin"} {
				p := filepath.Join(dir, filepath.FromSlash(sub), exe)
				if fi, err := os.Stat(p); err == nil && !fi.IsDir() {
					return p, nil
				}
			}
			parent := filepath.Dir(dir)
			if parent == dir {
				break
			}
			dir = parent
		}
	}
	return "", errors.New("check commands run under a POSIX sh, and none was found on PATH or next to git: they need the sh that ships with Git for Windows; install Git for Windows, or put its usr/bin directory on PATH")
}
