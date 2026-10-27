package cmd

import (
	"maps"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	operatorv1 "github.com/chattocorp/chatto/cli/internal/pb/chatto/operator/v1"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/encoding/protojson"
)

func TestOperatorImportThroughSocket(t *testing.T) {
	env := newOperatorCLITestEnv(t)
	path := "../../schema/interchange/v1/examples/conversation"
	before, err := env.core.EventPublisher.LastStreamSeq(env.ctx)
	require.NoError(t, err)
	output := env.run(t, "operator", "import", path, "--validate-only", "--channel-prefix", "source-", "--username-suffix", "-old", "--json")
	var result operatorv1.ImportArchiveResponse
	require.NoError(t, protojson.Unmarshal([]byte(output), &result))
	require.EqualValues(t, 10, result.Events)
	require.False(t, result.Complete)
	after, err := env.core.EventPublisher.LastStreamSeq(env.ctx)
	require.NoError(t, err)
	require.Equal(t, before, after)
	output = env.run(t, "operator", "import", path, "--accept-omissions", "--channel-prefix", "source-", "--username-suffix", "-old", "--json")
	require.NoError(t, protojson.Unmarshal([]byte(output), &result))
	require.True(t, result.Complete)
	before, err = env.core.EventPublisher.LastStreamSeq(env.ctx)
	require.NoError(t, err)
	_ = env.run(t, "operator", "import", path, "--accept-omissions", "--channel-prefix", "source-", "--username-suffix", "-old")
	after, err = env.core.EventPublisher.LastStreamSeq(env.ctx)
	require.NoError(t, err)
	require.Equal(t, before, after)
}

func TestOperatorImportLostResponse(t *testing.T) {
	env := newOperatorCLITestEnv(t)
	original := env.server.Handler
	var dropped atomic.Bool
	env.server.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		recorded := httptest.NewRecorder()
		original.ServeHTTP(recorded, r)
		if !dropped.Swap(true) {
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
	args := []string{"operator", "import", "../../schema/interchange/v1/examples/conversation", "--accept-omissions"}
	_, err := env.execute(t, args...)
	require.Error(t, err)
	before, err := env.core.EventPublisher.LastStreamSeq(env.ctx)
	require.NoError(t, err)
	_ = env.run(t, args...)
	after, err := env.core.EventPublisher.LastStreamSeq(env.ctx)
	require.NoError(t, err)
	require.Equal(t, before, after)
}

func TestOperatorImportRejectsSourceBeforeConnecting(t *testing.T) {
	t.Setenv("CHATTO_OPERATOR_API_SOCKET_PATH", "/nonexistent/import-test.sock")
	command := newOperatorImportCommand()
	command.SetArgs([]string{"/nonexistent/source"})
	require.ErrorContains(t, command.Execute(), "source must be a directory")
}
