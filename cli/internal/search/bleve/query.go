package bleve

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	blevesearch "github.com/blevesearch/bleve/v2"
	"github.com/blevesearch/bleve/v2/analysis/token/lowercase"
	unicodetokenizer "github.com/blevesearch/bleve/v2/analysis/tokenizer/unicode"
	blevequery "github.com/blevesearch/bleve/v2/search/query"
	"google.golang.org/protobuf/proto"

	searchv1 "hmans.de/chatto/internal/pb/chatto/search/v1"
)

var errInvalidCursor = fmt.Errorf("invalid search cursor")

const (
	exactMatchBoost = 4
	stemMatchBoost  = 2
	fuzzyMatchBoost = 0.35
)

type cursor struct {
	QueryHash string   `json:"query_hash"`
	Sort      []string `json:"sort"`
}

func (p *Projection) query(_ context.Context, request *searchv1.QueryRequest) (*searchv1.QueryResponse, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	query, err := buildQuery(request, p.languages)
	if err != nil {
		return nil, err
	}
	hash, err := queryHash(request)
	if err != nil {
		return nil, err
	}

	pageSize := int(request.GetPageSize())
	searchRequest := blevesearch.NewSearchRequestOptions(query, pageSize+1, 0, false)
	searchRequest.Fields = []string{"room_id", "body_event_id"}
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
		bodyEventID, _ := hit.Fields["body_event_id"].(string)
		response.Hits = append(response.Hits, &searchv1.QueryHit{
			MessageId: strings.TrimPrefix(hit.ID, "message:"), RoomId: roomID, BodyEventId: bodyEventID,
		})
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

func buildQuery(request *searchv1.QueryRequest, languages []languageAnalyzer) (blevequery.Query, error) {
	conjuncts := []blevequery.Query{}
	visible := blevesearch.NewBoolFieldQuery(true)
	visible.SetField("visible")
	conjuncts = append(conjuncts, visible)
	for _, term := range request.GetRequiredTerms() {
		conjuncts = append(conjuncts, bodyTermQuery(term, languages))
	}
	for _, phrase := range request.GetRequiredPhrases() {
		conjuncts = append(conjuncts, bodyPhraseQuery(phrase, languages))
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

func bodyTermQuery(term string, languages []languageAnalyzer) blevequery.Query {
	queries := []blevequery.Query{
		exactBodyTermQuery(term),
	}
	for _, language := range languages {
		queries = append(queries, boostedMatchQuery(
			term,
			language.field,
			language.analyzer,
			stemMatchBoost,
		))
	}
	// One edit is useful for ordinary words but creates surprising matches for
	// short chat tokens, initials, and identifiers. Requiring a shared prefix
	// also keeps the fuzzy term dictionary scan bounded.
	exactTokens := exactBodyTokens(term)
	if len(exactTokens) == 1 && len([]rune(exactTokens[0])) >= 5 {
		fuzzy := blevesearch.NewFuzzyQuery(exactTokens[0])
		fuzzy.SetField(bodyExactField)
		fuzzy.SetBoost(fuzzyMatchBoost)
		fuzzy.SetFuzziness(1)
		fuzzy.SetPrefix(2)
		queries = append(queries, fuzzy)
	}
	return blevesearch.NewDisjunctionQuery(queries...)
}

func exactBodyTermQuery(term string) blevequery.Query {
	tokens := exactBodyTokens(term)
	queries := make([]blevequery.Query, 0, len(tokens))
	for _, token := range tokens {
		query := blevesearch.NewTermQuery(token)
		query.SetField(bodyExactField)
		query.SetBoost(exactMatchBoost)
		queries = append(queries, query)
	}
	return blevesearch.NewConjunctionQuery(queries...)
}

func bodyPhraseQuery(phrase string, languages []languageAnalyzer) blevequery.Query {
	exact := blevesearch.NewPhraseQuery(exactBodyTokens(phrase), bodyExactField)
	exact.SetBoost(exactMatchBoost)
	if !hasLanguageAnalyzer(languages, "cjk") {
		return exact
	}
	cjkPhrase := blevesearch.NewMatchPhraseQuery(phrase)
	cjkPhrase.SetField(bodyCJKField)
	cjkPhrase.Analyzer = bodyCJKAnalyzer
	cjkPhrase.SetBoost(stemMatchBoost)
	return blevesearch.NewDisjunctionQuery(exact, cjkPhrase)
}

func hasLanguageAnalyzer(languages []languageAnalyzer, code string) bool {
	for _, language := range languages {
		if language.code == code {
			return true
		}
	}
	return false
}

func exactBodyTokens(value string) []string {
	stream := unicodetokenizer.NewUnicodeTokenizer().Tokenize([]byte(value))
	stream = lowercase.NewLowerCaseFilter().Filter(stream)
	terms := make([]string, 0, len(stream))
	for _, token := range stream {
		terms = append(terms, string(token.Term))
	}
	return terms
}

func boostedMatchQuery(term, field, analyzer string, boost float64) *blevequery.MatchQuery {
	query := blevesearch.NewMatchQuery(term)
	query.SetField(field)
	query.Analyzer = analyzer
	query.SetBoost(boost)
	return query
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

func (b *projectionBatch) deleteMatching(field, value string) error {
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
		result, err := b.projection.index.Search(request)
		if err != nil {
			return fmt.Errorf("find messages by %s: %w", field, err)
		}
		for _, hit := range result.Hits {
			id := strings.TrimPrefix(hit.ID, "message:")
			if state, pending := b.messages[id]; pending && !messageFieldMatches(state, field, value) {
				continue
			}
			b.deleteMessage(id)
		}
		if len(result.Hits) < 1000 {
			break
		}
		after = result.Hits[len(result.Hits)-1].Sort
	}
	for id, state := range b.messages {
		if messageFieldMatches(state, field, value) {
			b.deleteMessage(id)
		}
	}
	return nil
}

func messageFieldMatches(state messageDocument, field, value string) bool {
	switch field {
	case "room_id":
		return state.RoomID == value
	case "author_id":
		return state.AuthorID == value
	default:
		return false
	}
}
