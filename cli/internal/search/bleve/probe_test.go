package bleve

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	searchv1 "github.com/chattocorp/chatto/cli/internal/pb/chatto/search/v1"
)

func TestQueryProbeRecordsResults(t *testing.T) {
	tests := []struct {
		name   string
		err    error
		result searchv1.QueryProbeResult
	}{
		{"success", nil, searchv1.QueryProbeResult_QUERY_PROBE_RESULT_SUCCEEDED},
		{"failure", errors.New("index failed"), searchv1.QueryProbeResult_QUERY_PROBE_RESULT_FAILED},
		{"timeout", context.DeadlineExceeded, searchv1.QueryProbeResult_QUERY_PROBE_RESULT_TIMED_OUT},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			probe := newQueryProbe(func(context.Context) error { return test.err })
			_, started := probe.Snapshot()
			require.False(t, started)
			done := probe.Start(context.Background())
			require.NotNil(t, done)
			<-done
			snapshot, started := probe.Snapshot()
			require.True(t, started)
			require.Equal(t, test.result, snapshot.LastResult)
			require.True(t, snapshot.InFlightSince.IsZero())
			succeeded := test.err == nil
			require.Equal(t, succeeded, snapshot.Healthy(time.Now()))
			require.Equal(t, !succeeded, snapshot.Failing(time.Now()))
			if succeeded {
				require.Zero(t, snapshot.ConsecutiveFailures)
				require.False(t, snapshot.LastSucceededAt.IsZero())
			} else {
				require.EqualValues(t, 1, snapshot.ConsecutiveFailures)
				require.True(t, snapshot.LastSucceededAt.IsZero())
			}
			status := snapshot.proto()
			require.Equal(t, test.result, status.GetLastResult())
			require.NotNil(t, status.GetLastCompletedAt())
			require.Nil(t, status.GetInFlightSince())
		})
	}
}

// A query that ignores cancellation must not make the probe start more
// goroutines. The probe reports the blocked probe as failing after its timeout.
func TestQueryProbeRunsAtMostOneBlockedProbe(t *testing.T) {
	release := make(chan struct{})
	var calls atomic.Int32
	var blocking atomic.Bool
	blocking.Store(true)
	probe := newQueryProbe(func(context.Context) error {
		calls.Add(1)
		if blocking.Load() {
			<-release // Ignores the context, like a blocked lock acquisition.
		}
		return nil
	})
	probe.timeout = 20 * time.Millisecond
	var clock atomic.Int64
	clock.Store(time.Now().UnixNano())
	probe.now = func() time.Time { return time.Unix(0, clock.Load()) }

	done := probe.Start(context.Background())
	require.NotNil(t, done)
	require.Eventually(t, func() bool { return calls.Load() == 1 }, time.Second, time.Millisecond)
	for range 10 {
		require.Nil(t, probe.Start(context.Background()))
	}
	require.EqualValues(t, 1, calls.Load())

	snapshot, _ := probe.Snapshot()
	require.False(t, snapshot.Healthy(time.Now()))
	require.False(t, snapshot.InFlightSince.IsZero())
	require.False(t, snapshot.Failing(snapshot.InFlightSince.Add(probe.timeout)))
	require.True(t, snapshot.Failing(snapshot.InFlightSince.Add(probe.timeout+time.Millisecond)))
	require.NotNil(t, snapshot.proto().GetInFlightSince())

	// A probe that returns after its timeout counts as timed out.
	clock.Add(int64(probe.timeout + time.Millisecond))
	blocking.Store(false)
	close(release)
	<-done
	snapshot, _ = probe.Snapshot()
	require.Equal(t, searchv1.QueryProbeResult_QUERY_PROBE_RESULT_TIMED_OUT, snapshot.LastResult)
	require.EqualValues(t, 1, snapshot.Timeouts)

	// The next probe starts only after the blocked one returned.
	done = probe.Start(context.Background())
	require.NotNil(t, done)
	<-done
	snapshot, _ = probe.Snapshot()
	require.True(t, snapshot.Healthy(probe.now()))
	require.EqualValues(t, 2, calls.Load())
}

func TestQueryProbeRequestContainsNoUserData(t *testing.T) {
	request := queryProbeRequest()
	require.Empty(t, request.GetRequiredTerms())
	require.Empty(t, request.GetRequiredPhrases())
	require.Empty(t, request.GetAuthorIds())
	require.Equal(t, []string{queryProbeRoomID}, request.GetRoomIds())
	require.EqualValues(t, 1, request.GetPageSize())
}
