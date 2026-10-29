package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/charmbracelet/log"
	"github.com/chattocorp/chatto/cli/internal/interchange"
	"github.com/stretchr/testify/require"
)

// Change the version when the fixture mix changes. The fixture contains no
// source archive data. Both cases use the same history, apart from reactions.
const importBenchmarkVersion = "v1"

// BenchmarkOperatorImportJournal isolates the existing durable checkpoint cost.
// It is a separate measurement, not a subtraction from the full import timer.
func BenchmarkOperatorImportJournal(b *testing.B) {
	for _, grouped := range []bool{false, true} {
		b.Run(fmt.Sprintf("grouped_%t", grouped), func(b *testing.B) {
			p, err := openImportProgress(filepath.Join(b.TempDir(), "progress.jsonl"), "benchmark")
			require.NoError(b, err)
			b.Cleanup(func() { require.NoError(b, p.file.Close()) })
			put := p.put
			if grouped {
				put = p.putReplayable
			}
			b.ResetTimer()
			for i := range b.N {
				require.NoError(b, put(fmt.Sprintf("event/%d/post", i), "synthetic-event-id"))
			}
			require.NoError(b, p.put("complete", "true"))
		})
	}
}

// BenchmarkOperatorImport measures a complete, fresh import over the Unix
// socket, including validation and durable progress writes. Use -benchtime=1x:
// each sample must start with an empty destination, not a completed journal.
func BenchmarkOperatorImport(b *testing.B) {
	// The command package runs these benchmarks serially. Suppress informational
	// logs so benchmark rows remain readable by benchstat.
	level := log.GetLevel()
	log.SetLevel(log.ErrorLevel)
	b.Cleanup(func() { log.SetLevel(level) })
	for _, reactions := range []int{0, 2} {
		b.Run(fmt.Sprintf("%s/reactions_%d", importBenchmarkVersion, reactions), func(b *testing.B) {
			if b.N != 1 {
				b.Fatal("use -benchtime=1x and -count=N for independent fresh imports")
			}
			b.StopTimer()
			env := newOperatorCLITestEnv(b)
			path := writeImportBenchmarkArchive(b, 1000, reactions)
			progress := filepath.Join(b.TempDir(), "progress.jsonl")
			var mu sync.Mutex
			calls := map[string][]time.Duration{}
			handler := env.server.Handler
			env.server.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				start := time.Now()
				mu.Lock()
				handler.ServeHTTP(w, r)
				elapsed := time.Since(start)
				method := filepath.Base(r.URL.Path)
				calls[method] = append(calls[method], elapsed)
				mu.Unlock()
			})
			b.StartTimer()
			started := time.Now()
			output := env.run(b, "operator", "import", path, "--accept-omissions", "--json", "--progress", progress)
			elapsed := time.Since(started)
			b.StopTimer()
			var result importResult
			require.NoError(b, json.Unmarshal([]byte(output), &result))
			require.True(b, result.Complete)
			require.Equal(b, 1030, result.Events)
			// Verify committed facts, not only the command's declared input counts.
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			facts, _, err := env.core.EventPublisher.SubjectEvents(ctx, "evt.room.*.message_posted")
			require.NoError(b, err)
			require.Len(b, facts, 1000)
			roots := map[string]string{}
			for _, fact := range facts {
				if fact.GetMessagePosted().GetInThread() == "" {
					roots[fact.Id] = fact.GetMessagePosted().GetRoomId()
				}
			}
			replies := 0
			for _, fact := range facts {
				post := fact.GetMessagePosted()
				if post.GetInThread() != "" {
					replies++
					require.Equal(b, post.RoomId, roots[post.InThread])
					require.Equal(b, post.InThread, post.InReplyTo)
				}
			}
			require.Equal(b, 100, replies)
			facts, _, err = env.core.EventPublisher.SubjectEvents(ctx, "evt.room.*.reaction_added")
			require.NoError(b, err)
			require.Len(b, facts, 1000*reactions)
			mu.Lock()
			defer mu.Unlock()
			require.Empty(b, calls["ImportMessage"])
			require.Len(b, calls["ImportMessages"], 40, "four batches per 100-message channel")
			require.Empty(b, calls["ImportMessageChange"])
			require.Empty(b, calls["ImportReactionChanges"], "snapshot reactions commit with messages")
			var serverTotal time.Duration
			for method, samples := range calls {
				slices.Sort(samples)
				var total time.Duration
				for _, d := range samples {
					total += d
				}
				serverTotal += total
				b.ReportMetric(float64(len(samples)), method+"/calls")
				b.ReportMetric(float64(total.Microseconds())/float64(len(samples)), method+"/mean-us")
				b.ReportMetric(float64(samples[(len(samples)*95+99)/100-1].Microseconds()), method+"/p95-us")
			}
			b.ReportMetric(1000/elapsed.Seconds(), "messages/s")
			b.ReportMetric(serverTotal.Seconds(), "server-s")
			b.ReportMetric((elapsed - serverTotal).Seconds(), "outside-server-s")
		})
	}
}

// writeImportBenchmarkArchive creates ten channels with two members each and
// 100 snapshots per channel in the standard benchmark. Every tenth snapshot is
// a reply to the previous snapshot. Each selected reaction has one reactor.
func writeImportBenchmarkArchive(t testing.TB, messages, reactions int) string {
	t.Helper()
	dir := t.TempDir()
	write := func(name string, records ...any) {
		f, err := os.Create(filepath.Join(dir, name))
		require.NoError(t, err)
		encoder := json.NewEncoder(f)
		for _, record := range records {
			require.NoError(t, encoder.Encode(record))
		}
		require.NoError(t, f.Close())
	}
	write("manifest.json", map[string]any{
		"format": "chatto-interchange", "version": 1,
		"export_id": "6e38caa1-c411-4a41-b340-54e1d157bbf5", "captured_at": "2020-03-15T00:00:00Z",
		"consistency": "consistent", "history": "partial", "omissions": []any{map[string]any{"category": "history", "reason": "Synthetic snapshots have no edit or reaction transition history.", "count": nil}},
		"source": map[string]any{"platform": "synthetic", "workspace_id": "benchmark-" + importBenchmarkVersion},
		"scope":  map[string]any{"kind": "channels", "coverage": "all"},
	})
	write("assets.jsonl")
	users := []any{}
	for i := range 2 {
		users = append(users, map[string]any{"id": fmt.Sprintf("u%d", i), "username": fmt.Sprintf("bench-user-%d", i), "display_name": "Benchmark user", "kind": "human", "status": "active", "emails": []any{}})
	}
	write("users.jsonl", users...)
	rooms, events := []any{}, []any{}
	for room := range 10 {
		id := fmt.Sprintf("r%d", room)
		rooms = append(rooms, map[string]any{"id": id, "kind": "channel", "name": fmt.Sprintf("bench-room-%d", room), "topic": "", "purpose": "", "visibility": "public", "archived": false})
		for user := range 2 {
			u := fmt.Sprintf("u%d", user)
			events = append(events, map[string]any{"id": fmt.Sprintf("join-%d-%d", room, user), "type": "room.member_joined", "room_id": id, "actor_id": u, "occurred_at": "2020-03-14T00:00:00Z", "data": map[string]any{"user_id": u}})
		}
		for i := range messages / 10 {
			message := fmt.Sprintf("m%d-%d", room, i)
			var thread any
			if i%10 == 9 {
				thread = fmt.Sprintf("m%d-%d", room, i-1)
			}
			r := []any{}
			for n := range reactions {
				r = append(r, map[string]any{"emoji": []string{"heart", "thumbsup"}[n], "user_ids": []string{fmt.Sprintf("u%d", n)}})
			}
			events = append(events, map[string]any{"id": message, "type": "message.snapshot", "room_id": id, "actor_id": nil, "occurred_at": nil, "data": map[string]any{
				"message_id": message, "author_id": fmt.Sprintf("u%d", i%2), "thread_id": thread, "reply_to": thread,
				"created_at": time.Date(2020, 3, 14, 1, 0, i, 0, time.UTC).Format(time.RFC3339), "edited_at": nil, "deleted_at": nil, "reactions": r,
				"content": map[string]any{"status": "available", "body": map[string]any{"format": "chatto-markdown", "text": strings.Repeat("Synthetic **history** for import measurement. ", 6)}, "asset_ids": []any{}},
			}})
		}
		events = append(events, map[string]any{"id": "members-" + id, "type": "room.membership_snapshot", "room_id": id, "actor_id": nil, "occurred_at": nil, "data": map[string]any{"user_ids": []string{"u0", "u1"}}})
	}
	write("rooms.jsonl", rooms...)
	write("events.jsonl", events...)
	return dir
}

func TestOperatorImportBenchmarkFixture(t *testing.T) {
	for _, reactions := range []int{0, 2} {
		a, err := interchange.ReadDirectory(writeImportBenchmarkArchive(t, 100, reactions))
		require.NoError(t, err)
		require.Equal(t, 130, a.EventCount)
		require.NoError(t, a.Close())
	}
}
