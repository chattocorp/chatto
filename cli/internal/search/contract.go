// Package search implements Chatto's trusted NATS boundary for replaceable
// message-search providers.
package search

import (
	"errors"
	"fmt"
)

const (
	// QuerySubject accepts normalized message-search requests.
	QuerySubject = "svc.chatto_ext.search.v1.query"
	// StatusSubject reports health from providers that can answer queries.
	StatusSubject = "svc.chatto_ext.search.v1.status"
	// StartupStatusSubject reports startup progress only when no ready provider
	// is subscribed to StatusSubject.
	StartupStatusSubject = "svc.chatto_ext.search.v1.status.startup"

	ServiceName    = "chatto-ext-search-v1"
	ServiceVersion = "1.0.0"
	QueueGroup     = "svc.chatto_ext.search.v1"

	ErrorCodeInvalidArgument = "400"
	ErrorCodeUnavailable     = "503"
	ErrorCodeInternal        = "500"
)

var (
	// ErrUnavailable means no provider can currently satisfy the request.
	ErrUnavailable = errors.New("search provider unavailable")
	// ErrProviderNotReady asks the service adapter to return a retryable error.
	ErrProviderNotReady = errors.New("search provider not ready")
	// ErrInvalidResponse means a provider violated the search wire contract.
	ErrInvalidResponse = errors.New("invalid search provider response")
)

// ServiceError is a standard NATS micro service error returned by a provider.
// Descriptions and details must not contain message text or other PII.
type ServiceError struct {
	Code        string
	Description string
	Details     []byte
}

func (e *ServiceError) Error() string {
	return fmt.Sprintf("search provider error %s: %s", e.Code, e.Description)
}

func (e *ServiceError) Is(target error) bool {
	return target == ErrUnavailable && e.Code == ErrorCodeUnavailable
}
