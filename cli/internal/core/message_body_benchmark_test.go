package core

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

var benchmarkHydratedBodyBytes int

// messageBodyBenchmarkPage is the projected hot-read fixture for one room
// timeline page. It benchmarks decryption from projection state rather than
// message publishing, authorization, or transport mapping.
type messageBodyBenchmarkPage struct {
	core     *ChattoCore
	eventIDs []string
}

// BenchmarkGetFullMessageBodyByEventID_DEKCache measures timeline body
// hydration with a request-scoped DEK cache compared to no request cache, which
// approximates the old per-message unwrap behavior.
func BenchmarkGetFullMessageBodyByEventID_DEKCache(b *testing.B) {
	runMessageBodyPageBenchmark(b, "same_author_page_50", 1, 50)
	runMessageBodyPageBenchmark(b, "five_authors_page_50", 5, 50)
}

func runMessageBodyPageBenchmark(b *testing.B, name string, authorCount, messageCount int) {
	b.Helper()
	page := setupMessageBodyBenchmarkPage(b, authorCount, messageCount)
	wrapper := &countingKeyWrapper{KeyWrapper: page.core.encryption.keyWrapper}
	page.core.dekResolver.keyWrapper = wrapper

	b.Run(name+"/request_cache_page", func(b *testing.B) {
		wrapper.unwraps.Store(0)

		b.ReportAllocs()
		b.ResetTimer()
		total := 0
		for i := 0; i < b.N; i++ {
			total += readBenchmarkPage(b, WithDEKRequestCache(context.Background()), page)
		}
		b.StopTimer()

		benchmarkHydratedBodyBytes = total
		b.ReportMetric(float64(wrapper.unwraps.Load())/float64(b.N), "unwraps/op")
	})

	b.Run(name+"/no_request_cache", func(b *testing.B) {
		wrapper.unwraps.Store(0)

		b.ReportAllocs()
		b.ResetTimer()
		total := 0
		for i := 0; i < b.N; i++ {
			total += readBenchmarkPage(b, context.Background(), page)
		}
		b.StopTimer()

		benchmarkHydratedBodyBytes = total
		b.ReportMetric(float64(wrapper.unwraps.Load())/float64(b.N), "unwraps/op")
	})
}

func setupMessageBodyBenchmarkPage(b *testing.B, authorCount, messageCount int) messageBodyBenchmarkPage {
	b.Helper()
	core := setupTestCoreWithEncryption(b)
	ctx := context.Background()

	authors := make([]string, 0, authorCount)
	for i := 0; i < authorCount; i++ {
		user, err := core.CreateUser(ctx, "system", fmt.Sprintf("benchuser%d", i), fmt.Sprintf("Bench User %d", i), "password123")
		require.NoError(b, err)
		authors = append(authors, user.Id)
	}

	room, err := core.CreateRoom(ctx, authors[0], KindChannel, "", "Bench", "Benchmark room")
	require.NoError(b, err)
	for _, authorID := range authors {
		_, err := core.JoinRoom(ctx, authorID, KindChannel, authorID, room.Id)
		require.NoError(b, err)
	}

	body := strings.Repeat("message body content ", 8)
	eventIDs := make([]string, 0, messageCount)
	for i := 0; i < messageCount; i++ {
		authorID := authors[i%len(authors)]
		event, err := core.PostMessage(ctx, KindChannel, room.Id, authorID, body, nil, "", "", nil, false)
		require.NoError(b, err)
		eventIDs = append(eventIDs, event.Id)
	}

	return messageBodyBenchmarkPage{
		core:     core,
		eventIDs: eventIDs,
	}
}

func readBenchmarkPage(b *testing.B, ctx context.Context, page messageBodyBenchmarkPage) int {
	b.Helper()
	total := 0
	for _, eventID := range page.eventIDs {
		total += readBenchmarkBody(b, ctx, page.core, eventID)
	}
	return total
}

func readBenchmarkBody(b *testing.B, ctx context.Context, core *ChattoCore, eventID string) int {
	b.Helper()
	body, err := core.GetFullMessageBodyByEventID(ctx, eventID)
	require.NoError(b, err)
	require.NotNil(b, body)
	return len(body.Body)
}
