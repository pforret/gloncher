//go:build !windows

package proc

import (
	"os/exec"
	"syscall"
	"time"
)

// killGrace is how long a process group gets to handle SIGTERM before it is
// killed outright.
const killGrace = 3 * time.Second

// setProcessGroup puts the child in its own process group so that signals can
// reach the grandchildren it spawns.
func setProcessGroup(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}

// terminate signals the child's whole process group, escalating to SIGKILL if
// it does not exit in time.
func terminate(cmd *exec.Cmd) {
	if cmd.Process == nil {
		return
	}
	pid := cmd.Process.Pid
	signalGroup(pid, syscall.SIGTERM)

	deadline := time.Now().Add(killGrace)
	for time.Now().Before(deadline) {
		// Signal 0 probes for existence without delivering anything.
		if err := signalGroup(pid, 0); err != nil {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	signalGroup(pid, syscall.SIGKILL)
}

// signalGroup sends sig to the process group led by pid, falling back to the
// single process if the group is gone.
func signalGroup(pid int, sig syscall.Signal) error {
	if err := syscall.Kill(-pid, sig); err != nil {
		return syscall.Kill(pid, sig)
	}
	return nil
}

func shellCommand() (string, string) { return "/bin/sh", "-c" }
