//go:build !windows

package cli

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"syscall"
)

// shellCommand returns the process that runs a check command through the
// POSIX shell. The shell leads a process group of its own, and when ctx is
// done the whole group is killed, so that nothing the command started
// outlives it.
func shellCommand(ctx context.Context, command string) (*exec.Cmd, error) {
	cmd := exec.CommandContext(ctx, "/bin/sh", "-c", command)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		err := syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		if errors.Is(err, syscall.ESRCH) {
			return os.ErrProcessDone
		}
		return err
	}
	return cmd, nil
}
