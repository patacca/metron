//go:build windows

package cli

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestGitShellFallback takes the Git for Windows fallback on purpose, not
// PATH, and runs a check that pipes through grep with nothing of Git on PATH:
// the sh it finds must bring Git's tools along.
func TestGitShellFallback(t *testing.T) {
	notOnPath := func(string) (string, error) { return "", errors.New("not on PATH") }
	sh, err := findShell(notOnPath, gitExecPath, "sh.exe")
	if err != nil {
		execPath, gerr := gitExecPath()
		t.Fatalf("no sh next to git (git --exec-path printed %q, error %v): %v", execPath, gerr, err)
	}
	if !strings.EqualFold(filepath.Base(filepath.Dir(sh)), "bin") || strings.EqualFold(filepath.Base(filepath.Dir(filepath.Dir(sh))), "usr") {
		t.Fatalf("found %s, want the sh in <git>/bin", sh)
	}
	cmd := exec.Command(sh, "-c", "printf 'one\\ntwo\\n' | grep -c o | sed 's/2/two lines/'")
	cmd.Env = []string{"PATH=" + filepath.Join(os.Getenv("SystemRoot"), "System32"), "SystemRoot=" + os.Getenv("SystemRoot")}
	out, err := cmd.CombinedOutput()
	if err != nil || strings.TrimSpace(string(out)) != "two lines" {
		t.Fatalf("%s -c ... | grep | sed: %v\n%s", sh, err, out)
	}
}
