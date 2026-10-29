package cmd

import (
	"github.com/stretchr/testify/require"
	"os"
	"path/filepath"
	"testing"
)

func TestImportProgressCrashRecoveryAndLock(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "progress")
	p, err := openImportProgress(path, "binding-a")
	require.NoError(t, err)
	require.NoError(t, p.put("operation", "result"))
	key := p.operation("next")
	_, err = openImportProgress(path, "binding-a")
	require.ErrorContains(t, err, "in use")
	_, err = p.file.WriteString(`{"key":"unfinished`)
	require.NoError(t, err)
	require.NoError(t, p.file.Close())
	p, err = openImportProgress(path, "binding-a")
	require.NoError(t, err)
	require.Equal(t, "result", p.values["operation"])
	require.Equal(t, key, p.operation("next"))
	require.NoError(t, p.file.Close())
	_, err = openImportProgress(path, "binding-b")
	require.ErrorContains(t, err, "another archive")
	require.NoError(t, os.Chmod(path, 0644))
	_, err = openImportProgress(path, "binding-a")
	require.ErrorContains(t, err, "private regular")
}
