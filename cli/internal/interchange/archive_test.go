package interchange

import (
	"bytes"
	"encoding/json"
	"maps"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

const exampleDirectory = "../../../schema/interchange/v1/examples/conversation"

func TestSyntheticArchive(t *testing.T) {
	_, schemaErr := resolved()
	require.NoError(t, schemaErr)
	a, err := ReadDirectory(exampleDirectory)
	require.NoError(t, err)
	defer a.Close()
	require.Equal(t, 10, a.EventCount)
	require.Len(t, a.Users, 2)
	require.Len(t, a.Assets, 1)
	b, err := ReadDirectory(exampleDirectory)
	require.NoError(t, err)
	defer b.Close()
	require.Equal(t, a.Digest(), b.Digest())
}

func TestSchemasMatchSource(t *testing.T) {
	for name, embedded := range schemaJSON {
		raw, err := os.ReadFile(filepath.Join("../../../schema/interchange/v1", name+".schema.json"))
		require.NoError(t, err)
		require.JSONEq(t, string(raw), embedded, "regenerate embedded schemas after contract changes")
	}
}

func TestStrictJSON(t *testing.T) {
	for _, input := range []string{`{"x":1,"x":2}`, `{"x":{"a":1,"a":2}}`, `{"x":"\ud800"}`, `{"x":"\udc00"}`, `{"x":"\ud800\u0041"}`, "{\"x\":\"\xff\"}"} {
		_, err := strictJSON([]byte(input))
		require.Error(t, err)
	}
	for _, input := range []string{`{"x":"\ud83d\ude00"}`, `{"x":"\\ud800"}`, `{"x": [null, true, 1]}`} {
		_, err := strictJSON([]byte(input))
		require.NoError(t, err)
	}
}

func TestArchiveRejectsBrokenReferencesAndIntegrityWithoutContentInErrors(t *testing.T) {
	for _, tc := range []struct {
		name   string
		change func([]byte) []byte
	}{
		{"files/asset-1.txt", func([]byte) []byte { return []byte("private content") }},
		{"events.jsonl", func(b []byte) []byte { return []byte(strings.ReplaceAll(string(b), `"user-2"`, `"private-user"`)) }},
		{"users.jsonl", func(b []byte) []byte {
			return []byte(strings.Replace(string(b), `"emails":[]`, `"emails":[],"avatar_asset_id":"private-avatar"`, 1))
		}},
		{"manifest.json", func(b []byte) []byte { return []byte(strings.ReplaceAll(string(b), `"version": 1`, `"version": 2`)) }},
	} {
		dir := copyExample(t)
		raw, err := os.ReadFile(filepath.Join(dir, tc.name))
		require.NoError(t, err)
		require.NoError(t, os.WriteFile(filepath.Join(dir, tc.name), tc.change(raw), 0600))
		_, err = ReadDirectory(dir)
		require.Error(t, err)
		require.NotContains(t, err.Error(), "private")
	}
}

func TestDirectoryRejectsLinks(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.Symlink(exampleDirectory, filepath.Join(root, "link")))
	_, err := ReadDirectory(filepath.Join(root, "link"))
	require.Error(t, err)
	require.NoError(t, os.Remove(filepath.Join(root, "link")))
	require.NoError(t, os.WriteFile(filepath.Join(root, "manifest.json"), []byte("{}"), 0600))
	require.NoError(t, os.Link(filepath.Join(root, "manifest.json"), filepath.Join(root, "users.jsonl")))
	_, err = ReadDirectory(root)
	require.ErrorContains(t, err, "hard links")
}

// These cases exercise semantic rules that JSON Schema alone cannot establish.
func TestArchiveHistoryRules(t *testing.T) {
	for _, tc := range []struct {
		name   string
		mutate func([]map[string]any) []map[string]any
		want   string
	}{
		{"missing membership", func(events []map[string]any) []map[string]any { return events[:len(events)-1] }, "missing final membership"},
		{"membership after snapshot", func(events []map[string]any) []map[string]any {
			leave := events[8]
			return append(append(events[:8:8], events[9]), leave)
		}, "membership follows final snapshot"},
		{"forward reply", func(events []map[string]any) []map[string]any {
			events[2]["data"].(map[string]any)["reply_to"] = "message-2"
			return events
		}, "invalid message reference"},
		{"duplicate thread", func(events []map[string]any) []map[string]any {
			event := map[string]any{}
			maps.Copy(event, events[3])
			event["id"] = "another-thread-event"
			return append(events, event)
		}, "invalid thread declaration"},
		{"tombstone plaintext", func(events []map[string]any) []map[string]any {
			return append(events, map[string]any{"id": "delete-root", "type": "message.deleted", "room_id": "room-1", "actor_id": nil, "occurred_at": "2020-03-14T13:00:00Z", "data": map[string]any{"message_id": "message-1"}})
		}, "deleted content exposed"},
		{"mutation after snapshot", func(events []map[string]any) []map[string]any {
			return append(events, map[string]any{"id": "react-final", "type": "reaction.added", "room_id": "room-1", "actor_id": nil, "occurred_at": "2020-03-14T13:00:00Z", "data": map[string]any{"message_id": "message-3", "user_id": "user-2", "emoji": "❤️"}})
		}, "mutation follows final"},
		{"unknown snapshot reactor", func(events []map[string]any) []map[string]any {
			events[7]["data"].(map[string]any)["reactions"] = []any{map[string]any{"emoji": "❤️", "user_ids": []string{"missing"}}}
			return events
		}, "unknown reactor reference"},
		{"unknown membership reactor", func(events []map[string]any) []map[string]any {
			events[0]["data"].(map[string]any)["reactions"] = []any{map[string]any{"emoji": "wave", "user_ids": []string{"missing"}}}
			return events
		}, "unknown reactor reference"},
		{"duplicate membership reaction", func(events []map[string]any) []map[string]any {
			r := map[string]any{"emoji": "wave", "user_ids": []string{"user-2"}}
			events[0]["data"].(map[string]any)["reactions"] = []any{r, r}
			return events
		}, "duplicate reaction group"},
		{"future event", func(events []map[string]any) []map[string]any {
			events[0]["occurred_at"] = "2099-01-01T00:00:00Z"
			return events
		}, "invalid historical time"},
		{"edit before creation", func(events []map[string]any) []map[string]any {
			events[5]["occurred_at"] = "2020-03-13T00:00:00Z"
			return events
		}, "mutation precedes creation"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := copyExample(t)
			raw, err := os.ReadFile(filepath.Join(dir, "events.jsonl"))
			require.NoError(t, err)
			var events []map[string]any
			for line := range bytes.SplitSeq(bytes.TrimSpace(raw), []byte{'\n'}) {
				var event map[string]any
				require.NoError(t, json.Unmarshal(line, &event))
				events = append(events, event)
			}
			var output bytes.Buffer
			for _, event := range tc.mutate(events) {
				require.NoError(t, json.NewEncoder(&output).Encode(event))
			}
			require.NoError(t, os.WriteFile(filepath.Join(dir, "events.jsonl"), output.Bytes(), 0600))
			_, err = ReadDirectory(dir)
			require.ErrorContains(t, err, tc.want)
		})
	}
}

func copyExample(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	require.NoError(t, os.CopyFS(dir, os.DirFS(exampleDirectory)))
	return dir
}

func TestArchiveRejectsBlankRecord(t *testing.T) {
	dir := copyExample(t)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "events.jsonl"), []byte("\n"), 0600))
	_, err := ReadDirectory(dir)
	require.ErrorContains(t, err, "record 1 violates")
}

func FuzzStrictJSON(f *testing.F) {
	for _, seed := range []string{`{"x":1}`, `{"x":"\ud800"}`, `{"x":"\ud83d\ude00"}`, `{"a":{"a":1,"a":2}}`, `[]`} {
		f.Add([]byte(seed))
	}
	f.Fuzz(func(t *testing.T, raw []byte) {
		if len(raw) > MaxRecordBytes {
			t.Skip()
		}
		_, _ = strictJSON(raw)
	})
}
