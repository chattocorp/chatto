package cmd

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/require"
)

func TestOperatorImportReactionBatchBound(t *testing.T) {
	env := newOperatorCLITestEnv(t)
	path := writeImportBenchmarkArchive(t, 10, 2)
	file := filepath.Join(path, "events.jsonl")
	raw, err := os.ReadFile(file)
	require.NoError(t, err)
	var output bytes.Buffer
	changed := false
	for line := range bytes.SplitSeq(bytes.TrimSpace(raw), []byte{'\n'}) {
		var record map[string]any
		require.NoError(t, json.Unmarshal(line, &record))
		if !changed && record["type"] == "message.snapshot" {
			reactions := []any{}
			for i := range 130 {
				reactions = append(reactions, map[string]any{"emoji": fmt.Sprintf("bench-%d", i), "user_ids": []string{"u0"}})
			}
			record["data"].(map[string]any)["reactions"] = reactions
			changed = true
		}
		require.NoError(t, json.NewEncoder(&output).Encode(record))
	}
	require.NoError(t, os.WriteFile(file, output.Bytes(), 0600))
	base := env.server.Handler
	var batches atomic.Int32
	env.server.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/ImportReactionChanges") {
			batches.Add(1)
		}
		base.ServeHTTP(w, r)
	})
	env.run(t, "operator", "import", path, "--accept-omissions", "--progress", filepath.Join(t.TempDir(), "progress"))
	require.EqualValues(t, 2, batches.Load(), "130 changes split into 128 and 2; smaller snapshots use message batches")
	facts, _, err := env.core.EventPublisher.SubjectEvents(env.ctx, "evt.room.*.reaction_added")
	require.NoError(t, err)
	require.Len(t, facts, 148)
}

func TestOperatorImportReactionBatchRecovery(t *testing.T) {
	testOperatorImportBatchRecovery(t, "ImportReactionChanges")
}

func TestOperatorImportMessageBatchRecovery(t *testing.T) {
	testOperatorImportBatchRecovery(t, "ImportMessages")
}

func testOperatorImportBatchRecovery(t *testing.T, method string) {
	// Serial: the harness owns CLI globals and package-shared NATS resources.
	for _, mode := range []string{"lost response", "older server", "invalid batch"} {
		t.Run(mode, func(t *testing.T) {
			env := newOperatorCLITestEnv(t)
			base := env.server.Handler
			var batchCalls, singleCalls atomic.Int32
			env.server.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if strings.HasSuffix(r.URL.Path, "/ImportMessageChange") {
					singleCalls.Add(1)
				}
				if !strings.HasSuffix(r.URL.Path, "/"+method) {
					base.ServeHTTP(w, r)
					return
				}
				call := batchCalls.Add(1)
				if mode == "older server" || mode == "invalid batch" {
					code := connect.CodeUnimplemented
					if mode == "invalid batch" {
						code = connect.CodeInvalidArgument
					}
					if err := connect.NewErrorWriter().Write(w, r, connect.NewError(code, errors.New("test response"))); err != nil {
						t.Error(err)
					}
					return
				}
				recorded := httptest.NewRecorder()
				base.ServeHTTP(recorded, r)
				if call == 1 && recorded.Code == http.StatusOK {
					connection, _, err := w.(http.Hijacker).Hijack()
					if err != nil {
						t.Error(err)
						return
					}
					_ = connection.Close()
					return
				}
				maps.Copy(w.Header(), recorded.Header())
				w.WriteHeader(recorded.Code)
				_, _ = w.Write(recorded.Body.Bytes())
			})
			path := writeImportBenchmarkArchive(t, 100, 2)
			if method == "ImportReactionChanges" {
				useReactionTransitions(t, path)
			}
			args := []string{"operator", "import", path, "--accept-omissions", "--progress", filepath.Join(t.TempDir(), "progress")}
			_, err := env.execute(t, args...)
			switch mode {
			case "invalid batch":
				require.Error(t, err)
				require.Zero(t, singleCalls.Load(), "do not downgrade validation failures")
				require.EqualValues(t, 1, batchCalls.Load())
				return
			case "older server":
				require.NoError(t, err)
				require.EqualValues(t, 1, batchCalls.Load(), "probe once per import")
				require.EqualValues(t, 200, singleCalls.Load())
			case "lost response":
				require.Error(t, err)
				env.run(t, args...)
				require.Zero(t, singleCalls.Load())
			}
			facts, _, err := env.core.EventPublisher.SubjectEvents(env.ctx, "evt.room.*.reaction_added")
			require.NoError(t, err)
			require.Len(t, facts, 200, "lost acknowledgement must not duplicate reactions")
			facts, _, err = env.core.EventPublisher.SubjectEvents(env.ctx, "evt.room.*.message_posted")
			require.NoError(t, err)
			require.Len(t, facts, 100)
		})
	}
}

// useReactionTransitions exercises the standalone reaction batch path rather
// than co-importing final reactions with each new message snapshot.
func useReactionTransitions(t *testing.T, path string) {
	t.Helper()
	file := filepath.Join(path, "events.jsonl")
	raw, err := os.ReadFile(file)
	require.NoError(t, err)
	var output bytes.Buffer
	for line := range bytes.SplitSeq(bytes.TrimSpace(raw), []byte{'\n'}) {
		var record map[string]any
		require.NoError(t, json.Unmarshal(line, &record))
		var reactions []any
		if record["type"] == "message.snapshot" {
			data := record["data"].(map[string]any)
			reactions = data["reactions"].([]any)
			record["type"], record["occurred_at"], record["actor_id"] = "message.posted", data["created_at"], data["author_id"]
			for _, field := range []string{"reactions", "created_at", "edited_at", "deleted_at"} {
				delete(data, field)
			}
		}
		require.NoError(t, json.NewEncoder(&output).Encode(record))
		for i, raw := range reactions {
			r := raw.(map[string]any)
			change := map[string]any{"id": fmt.Sprintf("%s-reaction-%d", record["id"], i), "type": "reaction.added", "room_id": record["room_id"], "actor_id": nil, "occurred_at": "2020-03-14T23:00:00Z", "data": map[string]any{"message_id": record["data"].(map[string]any)["message_id"], "user_id": r["user_ids"].([]any)[0], "emoji": r["emoji"]}}
			require.NoError(t, json.NewEncoder(&output).Encode(change))
		}
	}
	require.NoError(t, os.WriteFile(file, output.Bytes(), 0600))
}
