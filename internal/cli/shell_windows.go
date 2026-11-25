//go:build windows

package cli

import (
	"context"
	"os/exec"
	"strconv"
)

// shellCommand returns the process that runs a check command through a POSIX
// shell, the sh that ships with Git for Windows, so that a command means the
// same on every platform. When ctx is done, taskkill ends the shell and every
// process it started; should taskkill fail, the shell alone is killed.
func shellCommand(ctx context.Context, command string) (*exec.Cmd, error) {
	sh, err := findShell(exec.LookPath, gitExecPath, "sh.exe")
	if err != nil {
		return nil, err
	}
	cmd := exec.CommandContext(ctx, sh, "-c", command)
	cmd.Cancel = func() error {
		if exec.Command("taskkill", "/T", "/F", "/PID", strconv.Itoa(cmd.Process.Pid)).Run() == nil {
			return nil
		}
		return cmd.Process.Kill()
	}
	return cmd, nil
}

// gitExecPath returns what git --exec-path prints.
func gitExecPath() (string, error) {
	out, err := exec.Command("git", "--exec-path").Output()
	return string(out), err
}
