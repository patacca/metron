//go:build windows

package cli

import "os/exec"

// shellCommand returns the process that runs a check command through a POSIX
// shell, the sh that ships with Git for Windows, so that a command means the
// same on every platform.
func shellCommand(command string) (*exec.Cmd, error) {
	sh, err := findShell(exec.LookPath, func() (string, error) {
		out, err := exec.Command("git", "--exec-path").Output()
		return string(out), err
	}, "sh.exe")
	if err != nil {
		return nil, err
	}
	return exec.Command(sh, "-c", command), nil
}
