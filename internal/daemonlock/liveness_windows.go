//go:build windows

package daemonlock

import "golang.org/x/sys/windows"

// processAlive reports whether pid names a running process. Windows'
// os.Process.Signal only supports os.Kill (any other signal, including the
// POSIX "signal 0" liveness probe, is rejected outright), so liveness is
// checked directly via OpenProcess instead: it fails when no process with
// that PID exists.
func processAlive(pid int) bool {
	h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, uint32(pid))
	if err != nil {
		return false
	}
	defer func() { _ = windows.CloseHandle(h) }()

	var exitCode uint32
	if err := windows.GetExitCodeProcess(h, &exitCode); err != nil {
		return false
	}
	return exitCode == 259 // STILL_ACTIVE
}
