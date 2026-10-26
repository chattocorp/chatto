package cmd

import (
	"github.com/chattocorp/chatto/cli/internal/core"
	"github.com/chattocorp/chatto/cli/internal/interchange"
	"github.com/stretchr/testify/require"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestOperatorImportRejectsUnreferencedFileBeforeWrites(t *testing.T) {
	env := newOperatorCLITestEnv(t)
	dir := t.TempDir()
	require.NoError(t, os.CopyFS(dir, os.DirFS("../../schema/interchange/v1/examples/conversation")))
	raw, err := os.ReadFile(filepath.Join(dir, "events.jsonl"))
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "events.jsonl"), []byte(strings.ReplaceAll(string(raw), `["asset-1"]`, `[]`)), 0600))
	before, err := env.core.EventPublisher.LastStreamSeq(env.ctx)
	require.NoError(t, err)
	_, err = env.execute(t, "operator", "import", dir, "--accept-omissions", "--progress", filepath.Join(t.TempDir(), "journal"))
	require.ErrorContains(t, err, "no message reference")
	after, err := env.core.EventPublisher.LastStreamSeq(env.ctx)
	require.NoError(t, err)
	require.Equal(t, before, after)
}

func TestOperatorImportRejectsChangedEmailOwner(t *testing.T) {
	env := newOperatorCLITestEnv(t)
	_, err := env.core.AdminCreateUserAs(env.ctx, core.SystemActorID, core.AdminCreateUserRequest{Login: "existing-person", VerifiedEmail: "alice@example.test"})
	require.NoError(t, err)
	a, err := interchange.ReadDirectory("../../schema/interchange/v1/examples/conversation")
	require.NoError(t, err)
	defer a.Close()
	a.Users[0].Emails = []string{"alice@example.test"}
	m, err := newCLIArchiveImport(a, importNaming{}, filepath.Join(t.TempDir(), "journal"))
	require.NoError(t, err)
	defer m.close()
	require.NoError(t, m.progress.put("user/"+importRef(a.Users[0].ID), "different-account"))
	before, err := env.core.EventPublisher.LastStreamSeq(env.ctx)
	require.NoError(t, err)
	require.ErrorContains(t, m.run(env.ctx, false, true), "email ownership changed")
	after, err := env.core.EventPublisher.LastStreamSeq(env.ctx)
	require.NoError(t, err)
	require.Equal(t, before, after)
}
