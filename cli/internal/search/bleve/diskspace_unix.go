//go:build !windows

package bleve

import "syscall"

// availableDiskSpace returns the bytes that an unprivileged process can still
// write on the filesystem of path.
func availableDiskSpace(path string) (uint64, error) {
	var stat syscall.Statfs_t
	if err := syscall.Statfs(path, &stat); err != nil {
		return 0, err
	}
	// The field types differ between operating systems.
	return uint64(stat.Bavail) * uint64(stat.Bsize), nil
}
