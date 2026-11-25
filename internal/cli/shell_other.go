//go:build !windows

package cli

import "os/exec"

// shell returns the process that runs a check command through the platform
// shell.
func shell(command string) *exec.Cmd {
	return exec.Command("/bin/sh", "-c", command)
}
