//go:build linux

package system

import (
	"fmt"
	"syscall"
)

// statfsRoot reads statfs(2) for the root filesystem. available is the space
// usable by an unprivileged user (bavail); used is total minus free (bfree).
func statfsRoot(path string) (diskStat, error) {
	var s syscall.Statfs_t
	if err := syscall.Statfs(path, &s); err != nil {
		return diskStat{}, fmt.Errorf("statfs %s: %w", path, err)
	}
	return diskStat{
		total:     int64(s.Blocks) * int64(s.Bsize),
		used:      (int64(s.Blocks) - int64(s.Bfree)) * int64(s.Bsize),
		available: int64(s.Bavail) * int64(s.Bsize),
	}, nil
}
