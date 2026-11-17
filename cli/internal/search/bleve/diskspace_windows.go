//go:build windows

package bleve

import "golang.org/x/sys/windows"

// availableDiskSpace returns the bytes that the calling user can still write
// on the volume of path.
func availableDiskSpace(path string) (uint64, error) {
	name, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return 0, err
	}
	var free uint64
	if err := windows.GetDiskFreeSpaceEx(name, &free, nil, nil); err != nil {
		return 0, err
	}
	return free, nil
}
