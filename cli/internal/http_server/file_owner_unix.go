//go:build aix || darwin || dragonfly || freebsd || linux || netbsd || openbsd || solaris

package http_server

import (
	"os"
	"syscall"
)

func fileOwnerIDs(info os.FileInfo) (uint32, uint32, bool) {
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return 0, 0, false
	}
	return stat.Uid, stat.Gid, true
}
