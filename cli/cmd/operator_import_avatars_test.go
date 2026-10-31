package cmd

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestOperatorImportAvatarThroughSocket(t *testing.T) {
	env := newOperatorCLITestEnv(t)
	dir := t.TempDir()
	require.NoError(t, os.CopyFS(dir, os.DirFS("../../schema/interchange/v1/examples/conversation")))
	path := filepath.Join(dir, "users.jsonl")
	raw, err := os.ReadFile(path)
	require.NoError(t, err)
	lines := strings.Split(strings.TrimSpace(string(raw)), "\n")
	var user map[string]any
	require.NoError(t, json.Unmarshal([]byte(lines[0]), &user))
	user["avatar_asset_id"] = "avatar-1"
	var imageBytes bytes.Buffer
	require.NoError(t, png.Encode(&imageBytes, image.NewRGBA(image.Rect(0, 0, 8, 8))))
	digest := sha256.Sum256(imageBytes.Bytes())
	require.NoError(t, os.WriteFile(filepath.Join(dir, "files/avatar.png"), imageBytes.Bytes(), 0600))
	asset := map[string]any{"id": "avatar-1", "filename": "avatar.png", "media_type": "image/png", "storage": map[string]any{"status": "available", "path": "files/avatar.png", "size": imageBytes.Len(), "sha256": hex.EncodeToString(digest[:])}}
	assetBytes, err := json.Marshal(asset)
	require.NoError(t, err)
	f, err := os.OpenFile(filepath.Join(dir, "assets.jsonl"), os.O_APPEND|os.O_WRONLY, 0600)
	require.NoError(t, err)
	_, err = f.Write(append(assetBytes, '\n'))
	require.NoError(t, err)
	require.NoError(t, f.Close())
	data, err := json.Marshal(user)
	require.NoError(t, err)
	lines[0] = string(data)
	require.NoError(t, os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0600))
	journal := filepath.Join(t.TempDir(), "journal")
	args := []string{"operator", "import", dir, "--accept-omissions", "--progress", journal}
	env.run(t, args...)
	u, err := env.core.GetUserByLogin(env.ctx, "alice")
	require.NoError(t, err)
	avatar, err := env.core.GetUserAvatar(env.ctx, u.Id)
	require.NoError(t, err)
	require.NotNil(t, avatar)
	before, err := env.core.EventPublisher.LastStreamSeq(env.ctx)
	require.NoError(t, err)
	// Lose the successful avatar result, as after a dropped response or crash.
	progress, err := os.ReadFile(journal)
	require.NoError(t, err)
	var retained bytes.Buffer
	for line := range bytes.SplitSeq(bytes.TrimSpace(progress), []byte{'\n'}) {
		var entry progressEntry
		require.NoError(t, json.Unmarshal(line, &entry))
		if entry.Key == "complete" || strings.HasPrefix(entry.Key, "avatar/") {
			continue
		}
		retained.Write(line)
		retained.WriteByte('\n')
	}
	require.NoError(t, os.WriteFile(journal, retained.Bytes(), 0600))
	env.run(t, args...)
	after, err := env.core.EventPublisher.LastStreamSeq(env.ctx)
	require.NoError(t, err)
	require.Equal(t, before, after)
}
