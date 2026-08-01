//go:build windows

package proc

import (
	"os/exec"
	"strconv"
)

// setProcessGroup is a no-op on Windows: the job-object equivalent is set up
// per-kill by taskkill /T below.
func setProcessGroup(cmd *exec.Cmd) {}

// terminate kills the child and its descendants. Windows has no signal to
// ask politely with, so this is always a hard kill of the tree.
func terminate(cmd *exec.Cmd) {
	if cmd.Process == nil {
		return
	}
	pid := strconv.Itoa(cmd.Process.Pid)
	if err := exec.Command("taskkill", "/T", "/F", "/PID", pid).Run(); err != nil {
		_ = cmd.Process.Kill()
	}
}

func shellCommand() (string, string) { return "cmd", "/c" }
