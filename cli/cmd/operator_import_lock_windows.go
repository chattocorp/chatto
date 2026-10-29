//go:build windows

package cmd

import (
	"errors"
	"os"
)

func lockImportProgress(_ *os.File) error {
	return errors.New("import: local operator import requires Unix")
}
