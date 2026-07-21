package search

import (
	"fmt"
	"time"
	"unicode/utf8"

	searchv1 "hmans.de/chatto/internal/pb/chatto/search/v1"
)

const (
	maxQueryParts     = 32
	maxQueryPartBytes = 256
	maxFilterIDs      = 100
	maxIDBytes        = 128
	maxCursorBytes    = 4096
	maxPageSize       = 100
)

// ValidateQueryRequest verifies the trusted provider contract before a query
// crosses the NATS boundary.
func ValidateQueryRequest(request *searchv1.QueryRequest) error {
	return validateQueryRequest(request)
}

func validateQueryRequest(request *searchv1.QueryRequest) error {
	if request == nil {
		return fmt.Errorf("request is required")
	}
	if len(request.GetRequiredTerms()) == 0 && len(request.GetRequiredPhrases()) == 0 {
		return fmt.Errorf("at least one term or phrase is required")
	}
	if err := validateStrings("required terms", request.GetRequiredTerms(), maxQueryParts, maxQueryPartBytes); err != nil {
		return err
	}
	if err := validateStrings("required phrases", request.GetRequiredPhrases(), maxQueryParts, maxQueryPartBytes); err != nil {
		return err
	}
	// The public request accepts at most maxFilterIDs explicit room filters,
	// but Chatto expands them into the caller's complete authorized room scope
	// before crossing the provider boundary. Do not truncate or reject that
	// security boundary merely because a user belongs to many rooms.
	if err := validateStrings("room IDs", request.GetRoomIds(), 0, maxIDBytes); err != nil {
		return err
	}
	if err := validateStrings("author IDs", request.GetAuthorIds(), maxFilterIDs, maxIDBytes); err != nil {
		return err
	}
	if request.GetOrder() != searchv1.SearchOrder_SEARCH_ORDER_RELEVANCE && request.GetOrder() != searchv1.SearchOrder_SEARCH_ORDER_NEWEST {
		return fmt.Errorf("order is required")
	}
	if request.GetPageSize() == 0 || request.GetPageSize() > maxPageSize {
		return fmt.Errorf("page size must be between 1 and %d", maxPageSize)
	}
	if len(request.GetCursor()) > maxCursorBytes {
		return fmt.Errorf("cursor exceeds %d bytes", maxCursorBytes)
	}
	if request.GetCreatedAfter() != nil {
		if err := request.GetCreatedAfter().CheckValid(); err != nil {
			return fmt.Errorf("created after is invalid: %w", err)
		}
	}
	if request.GetCreatedBefore() != nil {
		if err := request.GetCreatedBefore().CheckValid(); err != nil {
			return fmt.Errorf("created before is invalid: %w", err)
		}
	}
	if request.GetCreatedAfter() != nil && request.GetCreatedBefore() != nil && !request.GetCreatedAfter().AsTime().Before(request.GetCreatedBefore().AsTime()) {
		return fmt.Errorf("created after must precede created before")
	}
	return nil
}

func validateStrings(name string, values []string, maxItems, maxBytes int) error {
	if maxItems > 0 && len(values) > maxItems {
		return fmt.Errorf("%s exceed %d items", name, maxItems)
	}
	for _, value := range values {
		if value == "" {
			return fmt.Errorf("%s contain an empty value", name)
		}
		if !utf8.ValidString(value) || len(value) > maxBytes {
			return fmt.Errorf("%s contain an invalid value", name)
		}
	}
	return nil
}

func validateQueryResponse(response *searchv1.QueryResponse, pageSize uint32) error {
	if response == nil {
		return fmt.Errorf("%w: query response is required", ErrInvalidResponse)
	}
	if len(response.GetHits()) > int(pageSize) {
		return fmt.Errorf("%w: provider returned too many hits", ErrInvalidResponse)
	}
	if len(response.GetNextCursor()) > maxCursorBytes {
		return fmt.Errorf("%w: provider cursor exceeds %d bytes", ErrInvalidResponse, maxCursorBytes)
	}
	for _, hit := range response.GetHits() {
		if hit == nil || hit.GetMessageId() == "" || hit.GetRoomId() == "" || hit.GetBodyEventId() == "" || len(hit.GetMessageId()) > maxIDBytes || len(hit.GetRoomId()) > maxIDBytes || len(hit.GetBodyEventId()) > maxIDBytes {
			return fmt.Errorf("%w: provider returned an invalid hit", ErrInvalidResponse)
		}
	}
	return nil
}

func validateStatusResponse(response *searchv1.GetStatusResponse) error {
	if response == nil || response.GetState() == searchv1.ProviderState_PROVIDER_STATE_UNSPECIFIED {
		return fmt.Errorf("%w: provider status is unspecified", ErrInvalidResponse)
	}
	if retryAfter := response.GetRetryAfter(); retryAfter != nil {
		if err := retryAfter.CheckValid(); err != nil || retryAfter.AsDuration() < 0 || retryAfter.AsDuration() > 24*time.Hour {
			return fmt.Errorf("%w: provider retry delay is invalid", ErrInvalidResponse)
		}
	}
	if response.IndexedEventCount != nil && response.TargetEventCount != nil && response.GetIndexedEventCount() > response.GetTargetEventCount() {
		return fmt.Errorf("%w: indexed event count exceeds target", ErrInvalidResponse)
	}
	return nil
}
