//go:build windows

package site

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"time"
)

// processAlive reports whether pid names a live process. On Windows
// os.FindProcess opens a handle to the process, so it fails when there is
// none.
func processAlive(pid int) bool {
	p, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	_ = p.Release()
	return true
}

// processCommandLine returns pid's command line from CIM, through PowerShell.
// Windows has no ps, and the command line is what tells Astro's preview
// server from another process that reuses its PID. An error (no PowerShell,
// a timeout) leaves the PID unattributed, so okfw never stops it.
func processCommandLine(pid int) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	query := fmt.Sprintf("(Get-CimInstance Win32_Process -Filter 'ProcessId=%s').CommandLine", strconv.Itoa(pid))
	out, err := exec.CommandContext(ctx, "powershell", "-NoProfile", "-NonInteractive", "-Command", query).Output()
	return string(out), err
}

// stopProcess terminates pid. Windows has no SIGTERM to ask first.
func stopProcess(pid int) error {
	p, err := os.FindProcess(pid)
	if err != nil {
		return nil // already gone
	}
	defer p.Release()
	if err := p.Kill(); err != nil && processAlive(pid) {
		return fmt.Errorf("stopping pid %d: %w", pid, err)
	}
	return nil
}
