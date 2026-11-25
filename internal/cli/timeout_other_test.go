//go:build !windows

package cli

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestCheckTimeoutKillsGroup times out a check whose shell left a process in
// the background, holding the output open, that would write a file 3 seconds
// in. Stopping the check kills its whole process group: the output closes at
// once rather than after waitDelay, and the file never appears.
func TestCheckTimeoutKillsGroup(t *testing.T) {
	marker := filepath.Join(t.TempDir(), "survived")
	r := pinnedWith(t, "(sleep 3; echo survived >'"+marker+"') & sleep 30")
	start := time.Now()
	r.expect(exitNegative, "check", "--timeout", "300ms")
	if took := time.Since(start); took >= waitDelay {
		t.Fatalf("metron check took %s: the output stayed open after the check was stopped", took)
	}
	// Give a survivor 3 more seconds than it needs to write the file.
	time.Sleep(time.Until(start.Add(6 * time.Second)))
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("a process the check started outlived it: %s exists (%v)", marker, err)
	}
	if ev := r.recorded(); len(ev) != 1 || ev[0].ExitCode != timeoutExit || ev[0].Passed {
		t.Fatalf("recorded %+v", ev)
	}
}
