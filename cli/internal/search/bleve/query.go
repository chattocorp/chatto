package bleve

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	blevesearch "github.com/blevesearch/bleve/v2"
	blevequery "github.com/blevesearch/bleve/v2/search/query"
	"google.golang.org/protobuf/proto"

	searchv1 "hmans.de/chatto/internal/pb/chatto/search/v1"
)

var errInvalidCursor = fmt.Errorf("invalid search cursor")

type cursor struct {
	QueryHash string   `json:"query_hash"`
	Sort      []string `json:"sort"`
}

func (p *Projection) query(_ context.Context, request *searchv1.QueryRequest) (*searchv1.QueryResponse, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	query, err := buildQuery(request)
	if err != nil {
		return nil, err
	}
	hash, err := queryHash(request)
	if err != nil {
		return nil, err
	}

	pageSize := int(request.GetPageSize())
	searchRequest := blevesearch.NewSearchRequestOptions(query, pageSize+1, 0, false)
	searchRequest.Fields = []string{"room_id"}
	switch request.GetOrder() {
	case searchv1.SearchOrder_SEARCH_ORDER_RELEVANCE:
		searchRequest.SortBy([]string{"-_score", "-created_at", "_id"})
	case searchv1.SearchOrder_SEARCH_ORDER_NEWEST:
		searchRequest.SortBy([]string{"-created_at", "_id"})
	default:
		return nil, fmt.Errorf("unsupported search order")
	}
	if len(request.GetCursor()) > 0 {
		var decoded cursor
		if err := json.Unmarshal(request.GetCursor(), &decoded); err != nil || decoded.QueryHash != hash || len(decoded.Sort) != len(searchRequest.Sort) {
			return nil, errInvalidCursor
		}
		searchRequest.SetSearchAfter(decoded.Sort)
	}
	result, err := p.index.Search(searchRequest)
	if err != nil {
		return nil, fmt.Errorf("search Bleve index: %w", err)
	}
	hits := result.Hits
	hasMore := len(hits) > pageSize
	if hasMore {
		hits = hits[:pageSize]
	}
	response := &searchv1.QueryResponse{Hits: make([]*searchv1.QueryHit, 0, len(hits))}
	for _, hit := range hits {
		roomID, _ := hit.Fields["room_id"].(string)
		response.Hits = append(response.Hits, &searchv1.QueryHit{MessageId: strings.TrimPrefix(hit.ID, "message:"), RoomId: roomID})
	}
	if hasMore {
		last := hits[len(hits)-1]
		encoded, err := json.Marshal(cursor{QueryHash: hash, Sort: last.Sort})
		if err != nil {
			return nil, err
		}
		response.NextCursor = encoded
	}
	return response, nil
}

func buildQuery(request *searchv1.QueryRequest) (blevequery.Query, error) {
	conjuncts := []blevequery.Query{}
	visible := blevesearch.NewBoolFieldQuery(true)
	visible.SetField("visible")
	conjuncts = append(conjuncts, visible)
	for _, term := range request.GetRequiredTerms() {
		q := blevesearch.NewMatchQuery(term)
		q.SetField("body")
		conjuncts = append(conjuncts, q)
	}
	for _, phrase := range request.GetRequiredPhrases() {
		q := blevesearch.NewMatchPhraseQuery(phrase)
		q.SetField("body")
		conjuncts = append(conjuncts, q)
	}
	if len(request.GetRoomIds()) > 0 {
		conjuncts = append(conjuncts, termsQuery("room_id", request.GetRoomIds()))
	}
	if len(request.GetAuthorIds()) > 0 {
		conjuncts = append(conjuncts, termsQuery("author_id", request.GetAuthorIds()))
	}
	if request.GetCreatedAfter() != nil || request.GetCreatedBefore() != nil {
		start, end := time.Time{}, time.Time{}
		if request.GetCreatedAfter() != nil {
			start = request.GetCreatedAfter().AsTime()
		}
		if request.GetCreatedBefore() != nil {
			end = request.GetCreatedBefore().AsTime()
		}
		no := false
		q := blevesearch.NewDateRangeInclusiveQuery(start, end, &no, &no)
		q.SetField("created_at")
		conjuncts = append(conjuncts, q)
	}
	if request.GetHasAttachments() {
		q := blevesearch.NewBoolFieldQuery(true)
		q.SetField("has_attachments")
		conjuncts = append(conjuncts, q)
	}
	return blevesearch.NewConjunctionQuery(conjuncts...), nil
}

func termsQuery(field string, values []string) blevequery.Query {
	disjuncts := make([]blevequery.Query, 0, len(values))
	for _, value := range values {
		q := blevesearch.NewTermQuery(value)
		q.SetField(field)
		disjuncts = append(disjuncts, q)
	}
	return blevesearch.NewDisjunctionQuery(disjuncts...)
}

func queryHash(request *searchv1.QueryRequest) (string, error) {
	clone := proto.Clone(request).(*searchv1.QueryRequest)
	clone.Cursor = nil
	data, err := proto.MarshalOptions{Deterministic: true}.Marshal(clone)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(data)
	return fmt.Sprintf("%x", sum[:]), nil
}

func (p *Projection) deleteMatching(batch *blevesearch.Batch, field, value string) error {
	if value == "" {
		return nil
	}
	q := blevesearch.NewTermQuery(value)
	q.SetField(field)
	var after []string
	for {
		request := blevesearch.NewSearchRequestOptions(q, 1000, 0, false)
		request.SortBy([]string{"_id"})
		if len(after) > 0 {
			request.SetSearchAfter(after)
		}
		result, err := p.index.Search(request)
		if err != nil {
			return fmt.Errorf("find messages by %s: %w", field, err)
		}
		for _, hit := range result.Hits {
			id := strings.TrimPrefix(hit.ID, "message:")
			batch.Delete(hit.ID)
			batch.DeleteInternal(messageStateKey(id))
		}
		if len(result.Hits) < 1000 {
			return nil
		}
		after = result.Hits[len(result.Hits)-1].Sort
	}
}
