//go:build windows

package cli

import (
	"os"
	"os/exec"
	"syscall"
)

// shell returns the process that runs a check command through the platform
// shell. The command line is passed to cmd.exe verbatim: /s makes it strip
// only the outer quotes, so the command runs exactly as written in the
// contract.
func shell(command string) *exec.Cmd {
	comspec := os.Getenv("ComSpec")
	if comspec == "" {
		comspec = "cmd.exe"
	}
	cmd := exec.Command(comspec)
	cmd.SysProcAttr = &syscall.SysProcAttr{CmdLine: `"` + comspec + `" /d /s /c "` + command + `"`}
	return cmd
}
