//go:build !windows

package daemonlock

import (
	"os"
	"syscall"
)

// processAlive reports whether pid names a running process, using the
// standard POSIX "signal 0" probe (sends no actual signal, only checks
// deliverability/permission).
func processAlive(pid int) bool {
	proc, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	return proc.Signal(syscall.Signal(0)) == nil
}
