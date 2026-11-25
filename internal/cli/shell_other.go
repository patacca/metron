//go:build !windows

package cli

import "os/exec"

// shellCommand returns the process that runs a check command through the
// POSIX shell.
func shellCommand(command string) (*exec.Cmd, error) {
	return exec.Command("/bin/sh", "-c", command), nil
}
