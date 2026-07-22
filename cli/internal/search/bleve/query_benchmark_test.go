package bleve

import (
	"context"
	"fmt"
	"testing"
	"time"

	blevesearch "github.com/blevesearch/bleve/v2"

	searchv1 "hmans.de/chatto/internal/pb/chatto/search/v1"
)

// BenchmarkQueryAuthorizedRoomScope tracks whether sending Chatto's complete
// authorization scope remains practical before the provider contract grows a
// prepared-scope/cache lifecycle.
func BenchmarkQueryAuthorizedRoomScope(b *testing.B) {
	const roomCount = 10_000
	index, err := blevesearch.NewMemOnly(newIndexMapping(nil))
	if err != nil {
		b.Fatal(err)
	}
	b.Cleanup(func() { _ = index.Close() })

	roomIDs := make([]string, roomCount)
	batch := index.NewBatch()
	for i := range roomIDs {
		roomIDs[i] = fmt.Sprintf("room-%05d", i)
		document := messageDocument{
			MessageID: fmt.Sprintf("message-%05d", i), RoomID: roomIDs[i],
			AuthorID: "author", Body: "scopebenchmark", BodyEventID: fmt.Sprintf("body-%05d", i),
			CreatedAt: time.Unix(int64(i), 0), UpdatedAt: time.Unix(int64(i), 0), Visible: true,
		}
		if err := batch.Index(messageDocumentID(document.MessageID), document); err != nil {
			b.Fatal(err)
		}
	}
	if err := index.Batch(batch); err != nil {
		b.Fatal(err)
	}
	projection := &Projection{index: index}

	for _, scopeSize := range []int{100, 1_000, 10_000} {
		b.Run(fmt.Sprintf("rooms-%d", scopeSize), func(b *testing.B) {
			request := &searchv1.QueryRequest{
				RequiredTerms: []string{"scopebenchmark"},
				RoomIds:       roomIDs[:scopeSize],
				Order:         searchv1.SearchOrder_SEARCH_ORDER_RELEVANCE,
				PageSize:      50,
			}
			b.ResetTimer()
			for range b.N {
				if _, err := projection.query(context.Background(), request); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
