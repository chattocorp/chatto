package cmd

import (
	"encoding/json"
	"fmt"
	"github.com/chattocorp/chatto/cli/internal/core"
	"maps"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestOperatorImportThroughSocket(t *testing.T) {
	env := newOperatorCLITestEnv(t)
	removed := httptest.NewRecorder()
	env.server.Handler.ServeHTTP(removed, httptest.NewRequest(http.MethodPost, "/api/connect/chatto.operator.v1.OperatorImportService/ImportArchive", strings.NewReader(`{}`)))
	require.Equal(t, http.StatusNotFound, removed.Code, "the server must not expose an archive endpoint")
	var uploads, posts atomic.Int32
	baseHandler := env.server.Handler
	env.server.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "ImportArchive") {
			t.Error("CLI sent an archive request")
		}
		if strings.HasSuffix(r.URL.Path, "/UploadChunk") {
			uploads.Add(1)
		}
		if strings.HasSuffix(r.URL.Path, "/ImportMessage") {
			posts.Add(1)
		}
		baseHandler.ServeHTTP(w, r)
	})
	path := "../../schema/interchange/v1/examples/conversation"
	progress := filepath.Join(t.TempDir(), "progress")
	before, err := env.core.EventPublisher.LastStreamSeq(env.ctx)
	require.NoError(t, err)
	output := env.run(t, "operator", "import", path, "--validate-only", "--channel-prefix", "source-", "--username-suffix", "-old", "--json", "--progress", progress)
	var result importResult
	require.NoError(t, json.Unmarshal([]byte(output), &result))
	require.EqualValues(t, 10, result.Events)
	require.False(t, result.Complete)
	after, err := env.core.EventPublisher.LastStreamSeq(env.ctx)
	require.NoError(t, err)
	require.Equal(t, before, after)
	output = env.run(t, "operator", "import", path, "--accept-omissions", "--channel-prefix", "source-", "--username-suffix", "-old", "--json", "--progress", progress)
	require.NoError(t, json.Unmarshal([]byte(output), &result))
	require.True(t, result.Complete)
	require.EqualValues(t, 1, uploads.Load())
	require.EqualValues(t, 3, posts.Load())
	before, err = env.core.EventPublisher.LastStreamSeq(env.ctx)
	require.NoError(t, err)
	_ = env.run(t, "operator", "import", path, "--accept-omissions", "--channel-prefix", "source-", "--username-suffix", "-old", "--progress", progress)
	after, err = env.core.EventPublisher.LastStreamSeq(env.ctx)
	require.NoError(t, err)
	require.Equal(t, before, after)
}

func TestOperatorImportLostResponse(t *testing.T) {
	for _, method := range []string{"CreateUser", "CreateRoom", "CompleteUpload", "ImportMessage", "ImportMessageChange", "ImportRoomEvent"} {
		t.Run(method, func(t *testing.T) {
			env := newOperatorCLITestEnv(t)
			original := env.server.Handler
			var dropped atomic.Bool
			env.server.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				recorded := httptest.NewRecorder()
				original.ServeHTTP(recorded, r)
				if strings.HasSuffix(r.URL.Path, "/"+method) && recorded.Code == http.StatusOK && !dropped.Swap(true) {
					// Drop a successful response after all server writes committed.
					connection, _, err := w.(http.Hijacker).Hijack()
					if err == nil {
						_ = connection.Close()
					}
					return
				}
				maps.Copy(w.Header(), recorded.Header())
				w.WriteHeader(recorded.Code)
				_, _ = w.Write(recorded.Body.Bytes())
			})
			args := []string{"operator", "import", "../../schema/interchange/v1/examples/conversation", "--accept-omissions", "--progress", filepath.Join(t.TempDir(), "progress.jsonl")}
			_, err := env.execute(t, args...)
			require.Error(t, err)
			before, err := env.core.EventPublisher.LastStreamSeq(env.ctx)
			require.NoError(t, err)
			_ = env.run(t, args...)
			after, err := env.core.EventPublisher.LastStreamSeq(env.ctx)
			require.NoError(t, err)
			require.Greater(t, after, before)
			facts, _, err := env.core.EventPublisher.SubjectEvents(env.ctx, "evt.room.*.message_posted")
			require.NoError(t, err)
			require.Len(t, facts, 3)
		})
	}
}

func TestOperatorImportEmailIdentity(t *testing.T) {
	for _, conflict := range []bool{false, true} {
		t.Run(fmt.Sprint(conflict), func(t *testing.T) {
			env := newOperatorCLITestEnv(t)
			existing, err := env.core.AdminCreateUserAs(env.ctx, core.SystemActorID, core.AdminCreateUserRequest{Login: "existing", DisplayName: "Keep this", VerifiedEmail: "alice@example.test"})
			require.NoError(t, err)
			if conflict {
				_, err = env.core.AdminCreateUserAs(env.ctx, core.SystemActorID, core.AdminCreateUserRequest{Login: "another", VerifiedEmail: "extra@example.test"})
				require.NoError(t, err)
			}
			dir := t.TempDir()
			require.NoError(t, os.CopyFS(dir, os.DirFS("../../schema/interchange/v1/examples/conversation")))
			raw, err := os.ReadFile(filepath.Join(dir, "users.jsonl"))
			require.NoError(t, err)
			var records []map[string]any
			for line := range strings.SplitSeq(strings.TrimSpace(string(raw)), "\n") {
				var u map[string]any
				require.NoError(t, json.Unmarshal([]byte(line), &u))
				records = append(records, u)
			}
			records[0]["emails"] = []string{"ALICE@example.test", "extra@example.test"}
			var out strings.Builder
			for _, u := range records {
				require.NoError(t, json.NewEncoder(&out).Encode(u))
			}
			require.NoError(t, os.WriteFile(filepath.Join(dir, "users.jsonl"), []byte(out.String()), 0600))
			before, err := env.core.EventPublisher.LastStreamSeq(env.ctx)
			require.NoError(t, err)
			_, err = env.execute(t, "operator", "import", dir, "--accept-omissions", "--username-prefix", "old-", "--progress", filepath.Join(t.TempDir(), "journal"))
			if conflict {
				require.ErrorContains(t, err, "different accounts")
				after, e := env.core.EventPublisher.LastStreamSeq(env.ctx)
				require.NoError(t, e)
				require.Equal(t, before, after)
				return
			}
			require.NoError(t, err)
			u, err := env.core.GetUser(env.ctx, existing.User.Id)
			require.NoError(t, err)
			require.Equal(t, "existing", u.Login)
			require.Equal(t, "Keep this", u.DisplayName)
			users, err := env.core.ListUsers(env.ctx)
			require.NoError(t, err)
			require.Len(t, users, 2)
		})
	}
}

func TestOperatorImportRejectsSourceBeforeConnecting(t *testing.T) {
	t.Setenv("CHATTO_OPERATOR_API_SOCKET_PATH", "/nonexistent/import-test.sock")
	command := newOperatorImportCommand()
	command.SetArgs([]string{"/nonexistent/source"})
	require.ErrorContains(t, command.Execute(), "source must be a directory")
}

func TestOperatorImportUnknownReactionNames(t *testing.T) {
	env := newOperatorCLITestEnv(t)
	dir := t.TempDir()
	require.NoError(t, os.CopyFS(dir, os.DirFS("../../schema/interchange/v1/examples/conversation")))
	path := filepath.Join(dir, "events.jsonl")
	raw, err := os.ReadFile(path)
	require.NoError(t, err)
	// Keep the Unicode fixture unchanged; this copy exercises named snapshots.
	require.Contains(t, string(raw), "❤️")
	require.NoError(t, os.WriteFile(path, []byte(strings.ReplaceAll(string(raw), "❤️", "Party_Parrot")), 0600))
	args := []string{"operator", "import", dir, "--accept-omissions", "--progress", filepath.Join(t.TempDir(), "progress")}
	env.run(t, args...)
	facts, _, err := env.core.EventPublisher.SubjectEvents(env.ctx, "evt.room.*.reaction_added")
	require.NoError(t, err)
	require.NotEmpty(t, facts)
	for _, event := range facts {
		require.Equal(t, "Party_Parrot", event.GetReactionAdded().GetEmoji())
		reactions, err := env.core.GetReactions(env.ctx, event.GetReactionAdded().GetMessageEventId())
		require.NoError(t, err)
		require.Equal(t, "Party_Parrot", reactions[0].Emoji)
	}
	before, err := env.core.EventPublisher.LastStreamSeq(env.ctx)
	require.NoError(t, err)
	env.run(t, args...)
	after, err := env.core.EventPublisher.LastStreamSeq(env.ctx)
	require.NoError(t, err)
	require.Equal(t, before, after)
}
