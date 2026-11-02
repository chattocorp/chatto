package cmd

import (
	"fmt"
	"github.com/stretchr/testify/require"
	"os"
	"path/filepath"
	"testing"
)

func TestImportProgressGroupedCheckpoints(t *testing.T) {
	t.Parallel()
	p, err := openImportProgress(filepath.Join(t.TempDir(), "progress"), "binding")
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, p.file.Close()) })
	for i := range importProgressSyncEvery - 1 {
		require.NoError(t, p.putReplayable(fmt.Sprint(i), "result"))
	}
	require.Equal(t, importProgressSyncEvery-1, p.unsynced)
	require.NoError(t, p.putReplayable("last", "result"))
	require.Zero(t, p.unsynced)
	require.NoError(t, p.putReplayable("next", "result"))
	require.NoError(t, p.put("complete", "true"))
	require.Zero(t, p.unsynced, "completion must sync all preceding results")
}

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
