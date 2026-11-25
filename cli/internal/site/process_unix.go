//go:build !windows

package site

import (
	"errors"
	"fmt"
	"os/exec"
	"strconv"
	"syscall"
	"time"
)

// processAlive reports whether pid names a live process. EPERM means it
// exists but belongs to another user.
func processAlive(pid int) bool {
	err := syscall.Kill(pid, 0)
	return err == nil || errors.Is(err, syscall.EPERM)
}

// processCommandLine returns pid's command line, as ps prints it.
func processCommandLine(pid int) (string, error) {
	out, err := exec.Command("ps", "-o", "command=", "-p", strconv.Itoa(pid)).Output()
	return string(out), err
}

// stopProcess sends SIGTERM, then SIGKILL after a grace period, as Astro's own
// stop does.
func stopProcess(pid int) error {
	if err := syscall.Kill(pid, syscall.SIGTERM); err != nil && !errors.Is(err, syscall.ESRCH) {
		return fmt.Errorf("stopping pid %d: %w", pid, err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if syscall.Kill(pid, 0) != nil {
			return nil
		}
		time.Sleep(100 * time.Millisecond)
	}
	if err := syscall.Kill(pid, syscall.SIGKILL); err != nil && !errors.Is(err, syscall.ESRCH) {
		return fmt.Errorf("killing pid %d: %w", pid, err)
	}
	return nil
}
