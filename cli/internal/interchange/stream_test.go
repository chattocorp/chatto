package interchange

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"github.com/stretchr/testify/require"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestArchiveStreamsBeyondFormerLimits(t *testing.T) {
	t.Parallel()
	dir := copyExample(t)
	f, err := os.OpenFile(filepath.Join(dir, "files/asset-1.txt"), os.O_RDWR, 0600)
	require.NoError(t, err)
	require.NoError(t, f.Truncate(65<<20))
	_, err = f.Seek(0, 0)
	require.NoError(t, err)
	h := sha256.New()
	_, err = io.Copy(h, f)
	require.NoError(t, err)
	require.NoError(t, f.Close())
	raw, err := os.ReadFile(filepath.Join(dir, "assets.jsonl"))
	require.NoError(t, err)
	var asset map[string]any
	require.NoError(t, json.Unmarshal(raw, &asset))
	storage := asset["storage"].(map[string]any)
	storage["size"] = 65 << 20
	storage["sha256"] = hex.EncodeToString(h.Sum(nil))
	raw, err = json.Marshal(asset)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "assets.jsonl"), append(raw, '\n'), 0600))
	f, err = os.OpenFile(filepath.Join(dir, "events.jsonl"), os.O_APPEND|os.O_WRONLY, 0600)
	require.NoError(t, err)
	w := bufio.NewWriter(f)
	for i := range 20001 {
		_, err = fmt.Fprintf(w, "{\"id\":\"extra-%d\",\"type\":\"room.topic_changed\",\"room_id\":\"room-1\",\"actor_id\":null,\"occurred_at\":\"2020-03-14T13:00:00Z\",\"data\":{\"value\":\"topic\"}}\n", i)
		require.NoError(t, err)
	}
	require.NoError(t, w.Flush())
	require.NoError(t, f.Close())
	a, err := ReadDirectory(dir)
	require.NoError(t, err)
	defer a.Close()
	require.Equal(t, 20011, a.EventCount)
	count := 0
	require.NoError(t, a.VisitEvents(func(Event) error { count++; return nil }))
	require.Equal(t, a.EventCount, count)
}

func TestArchiveRejectsChangedRecordBeforeCallback(t *testing.T) {
	t.Parallel()
	dir := copyExample(t)
	a, err := ReadDirectory(dir)
	require.NoError(t, err)
	defer a.Close()
	raw, err := os.ReadFile(filepath.Join(dir, "events.jsonl"))
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "events.jsonl"), []byte(strings.Replace(string(raw), "e1", "changed", 1)), 0600))
	calls := 0
	err = a.VisitEvents(func(Event) error { calls++; return nil })
	require.ErrorContains(t, err, "changed since validation")
	require.Zero(t, calls)
}
