package bleve

import (
	"fmt"
	"testing"
	"time"

	"github.com/charmbracelet/log"
	"github.com/chattocorp/chatto/cli/internal/encryption"
	"github.com/chattocorp/chatto/cli/internal/evtstream"
)

// BenchmarkIndexCommits measures durable Bleve commits for the same ordered
// 1,000-message fixture. Index creation and close are outside the timer.
func BenchmarkIndexCommits(b *testing.B) {
	key, err := encryption.GenerateKey()
	if err != nil {
		b.Fatal(err)
	}
	items := make([]evtstream.SequencedEvent, 0, 2000)
	for i := range 1000 {
		id := fmt.Sprintf("M%d", i)
		items = append(items,
			evtstream.SequencedEvent{Event: legacyBodyEvent(b, key, id, "B"+id, "R1", "U1", "search benchmark searchable message with some ordinary words", time.Unix(int64(i), 0), nil), Sequence: uint64(2*i + 1)},
			evtstream.SequencedEvent{Event: messagePostedEvent(id, "R1", "U1", time.Unix(int64(i), 0)), Sequence: uint64(2*i + 2)})
	}
	for _, size := range []int{1, 256} {
		b.Run(fmt.Sprintf("events_%d", size), func(b *testing.B) {
			for range b.N {
				b.StopTimer()
				p, err := NewProjection(b.TempDir()+"/index", []string{"de", "en"}, nil, staticLegacyKeys{key: key}, nil, log.New(nil))
				if err != nil {
					b.Fatal(err)
				}
				b.StartTimer()
				for offset := 0; offset < len(items); offset += size {
					if err := p.ApplyLiveBatch(items[offset:min(offset+size, len(items))]); err != nil {
						b.Fatal(err)
					}
				}
				b.StopTimer()
				count, err := p.index.DocCount()
				if err != nil {
					b.Fatal(err)
				}
				if count != 1000 || p.checkpoint.CutoffSequence != 2000 {
					b.Fatalf("count=%d cutoff=%d", count, p.checkpoint.CutoffSequence)
				}
				if err := p.Close(); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
