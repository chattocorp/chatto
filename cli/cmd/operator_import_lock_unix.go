//go:build !windows

package cmd

import (
	"errors"
	"os"
	"syscall"
)

func lockImportProgress(f *os.File) error {
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		return errors.New("import: progress is in use by another process")
	}
	return nil
}
