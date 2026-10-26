package cmd

import (
	"encoding/json"
	"github.com/stretchr/testify/require"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestOperatorImportSharedFiles(t *testing.T) {
	env := newOperatorCLITestEnv(t)
	dir := t.TempDir()
	require.NoError(t, os.CopyFS(dir, os.DirFS("../../schema/interchange/v1/examples/conversation")))
	raw, err := os.ReadFile(filepath.Join(dir, "events.jsonl"))
	require.NoError(t, err)
	var out strings.Builder
	// An archive event can precede later partial message snapshots.
	out.WriteString("{\"id\":\"archive-first\",\"type\":\"room.archived\",\"room_id\":\"room-1\",\"actor_id\":null,\"occurred_at\":\"2020-03-14T11:00:00Z\",\"data\":{}}\n")
	for line := range strings.SplitSeq(strings.TrimSpace(string(raw)), "\n") {
		var e map[string]any
		require.NoError(t, json.Unmarshal([]byte(line), &e))
		data := e["data"].(map[string]any)
		if data["message_id"] == "message-2" {
			if content, ok := data["content"].(map[string]any); ok {
				content["asset_ids"] = []string{"asset-1"}
			}
		}
		require.NoError(t, json.NewEncoder(&out).Encode(e))
	}
	require.NoError(t, os.WriteFile(filepath.Join(dir, "events.jsonl"), []byte(out.String()), 0600))
	env.run(t, "operator", "import", dir, "--accept-omissions", "--progress", filepath.Join(t.TempDir(), "progress"))
	created, _, err := env.core.EventPublisher.SubjectEvents(env.ctx, "evt.asset.*.asset_created")
	require.NoError(t, err)
	require.Len(t, created, 2)
	attached, _, err := env.core.EventPublisher.SubjectEvents(env.ctx, "evt.asset.*.asset_attached")
	require.NoError(t, err)
	owners := map[string]string{}
	for _, e := range attached {
		a := e.GetAssetAttached()
		if old := owners[a.AssetId]; old != "" {
			require.Equal(t, old, a.MessageEventId)
		}
		owners[a.AssetId] = a.MessageEventId
	}
	require.Len(t, owners, 2)
}
