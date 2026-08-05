package system

import "runtime"

// platformSupported reports whether the system.* tools can run on this host.
// They read Linux /proc interfaces, so only linux is supported.
func platformSupported() (bool, string) {
	if runtime.GOOS != "linux" {
		return false, "unsupported platform"
	}
	return true, ""
}
