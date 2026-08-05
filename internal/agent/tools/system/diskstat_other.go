//go:build !linux

package system

import "errors"

// statfsRoot is unsupported outside Linux; the tool is a Linux-only capability.
func statfsRoot(string) (diskStat, error) {
	return diskStat{}, errors.New("system.disk: unsupported platform")
}
