package search

import (
	"context"
	"errors"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/nats-io/nats.go"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/durationpb"

	searchv1 "hmans.de/chatto/internal/pb/chatto/search/v1"
	"hmans.de/chatto/internal/testutil"
)

type testProvider struct {
	mu          sync.Mutex
	query       *searchv1.QueryRequest
	queryCalls  int
	queryResult *searchv1.QueryResponse
	queryErr    error
	status      *searchv1.GetStatusResponse
	statusErr   error
}

func (p *testProvider) Query(_ context.Context, request *searchv1.QueryRequest) (*searchv1.QueryResponse, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.queryCalls++
	p.query = proto.Clone(request).(*searchv1.QueryRequest)
	return p.queryResult, p.queryErr
}

func (p *testProvider) GetStatus(context.Context, *searchv1.GetStatusRequest) (*searchv1.GetStatusResponse, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.status, p.statusErr
}

func (p *testProvider) calls() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.queryCalls
}

func testContext(t *testing.T) context.Context {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	t.Cleanup(cancel)
	return ctx
}

func validQuery() *searchv1.QueryRequest {
	return &searchv1.QueryRequest{
		RequiredTerms: []string{"full", "text"},
		RoomIds:       []string{"rm_one"},
		Order:         searchv1.SearchOrder_SEARCH_ORDER_RELEVANCE,
		PageSize:      20,
	}
}

func TestClientAndServiceRoundTrip(t *testing.T) {
	_, nc := testutil.StartNATS(t)
	provider := &testProvider{
		queryResult: &searchv1.QueryResponse{
			Hits:       []*searchv1.QueryHit{{MessageId: "msg_one", RoomId: "rm_one", BodyEventId: "body_one"}},
			NextCursor: []byte("provider-page-2"),
		},
		status: &searchv1.GetStatusResponse{
			State:             searchv1.ProviderState_PROVIDER_STATE_READY,
			IndexedEventCount: proto.Uint64(40),
			TargetEventCount:  proto.Uint64(40),
			RetryAfter:        durationpb.New(time.Second),
		},
	}
	service, err := AddService(testContext(t), nc, provider, ServiceOptions{ImplementationVersion: "bleve-test"})
	if err != nil {
		t.Fatalf("AddService: %v", err)
	}
	t.Cleanup(func() { _ = service.Stop() })

	client := NewClient(nc)
	query := validQuery()
	response, err := client.Query(testContext(t), query)
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	if len(response.GetHits()) != 1 || response.GetHits()[0].GetMessageId() != "msg_one" || string(response.GetNextCursor()) != "provider-page-2" {
		t.Fatalf("query response = %+v", response)
	}
	if !proto.Equal(provider.query, query) {
		t.Fatalf("provider query = %+v, want %+v", provider.query, query)
	}

	status, err := client.GetStatus(testContext(t))
	if err != nil {
		t.Fatalf("GetStatus: %v", err)
	}
	if status.GetState() != searchv1.ProviderState_PROVIDER_STATE_READY || status.GetIndexedEventCount() != 40 {
		t.Fatalf("status = %+v", status)
	}

	info := service.Info()
	if info.Name != ServiceName || info.Version != ServiceVersion || info.Metadata["implementation_version"] != "bleve-test" {
		t.Fatalf("service info = %+v", info)
	}
	var subjects []string
	for _, endpoint := range info.Endpoints {
		if endpoint.QueueGroup != QueueGroup {
			t.Fatalf("endpoint queue group = %q, want %q", endpoint.QueueGroup, QueueGroup)
		}
		subjects = append(subjects, endpoint.Subject)
	}
	slices.Sort(subjects)
	if !slices.Equal(subjects, []string{QuerySubject, StatusSubject}) {
		t.Fatalf("endpoint subjects = %v", subjects)
	}
}

func TestStartupStatusServiceJoinsReadyQueuesOnlyWhenReady(t *testing.T) {
	_, nc := testutil.StartNATS(t)
	provider := &testProvider{
		queryResult: &searchv1.QueryResponse{},
		status:      &searchv1.GetStatusResponse{State: searchv1.ProviderState_PROVIDER_STATE_INDEXING},
	}
	service, err := AddStartupStatusService(testContext(t), nc, provider, ServiceOptions{})
	if err != nil {
		t.Fatalf("AddStartupStatusService: %v", err)
	}
	t.Cleanup(func() { _ = service.Stop() })

	status, err := NewClient(nc).GetStatus(testContext(t))
	if err != nil || status.GetState() != searchv1.ProviderState_PROVIDER_STATE_INDEXING {
		t.Fatalf("GetStatus = %+v, %v", status, err)
	}
	_, err = NewClient(nc).Query(testContext(t), validQuery())
	if !errors.Is(err, ErrUnavailable) {
		t.Fatalf("Query before readiness = %v, want unavailable", err)
	}

	provider.mu.Lock()
	provider.status = &searchv1.GetStatusResponse{State: searchv1.ProviderState_PROVIDER_STATE_READY}
	provider.mu.Unlock()
	if err := AddQueryEndpoint(testContext(t), service, provider); err != nil {
		t.Fatalf("AddQueryEndpoint: %v", err)
	}
	if err := AddStatusEndpoint(testContext(t), service, provider); err != nil {
		t.Fatalf("AddStatusEndpoint: %v", err)
	}
	if _, err := NewClient(nc).Query(testContext(t), validQuery()); err != nil {
		t.Fatalf("Query after readiness: %v", err)
	}
	status, err = NewClient(nc).GetStatus(testContext(t))
	if err != nil || status.GetState() != searchv1.ProviderState_PROVIDER_STATE_READY {
		t.Fatalf("GetStatus after readiness = %+v, %v", status, err)
	}
}

func TestReadyProviderStatusWinsDuringReplicaStartup(t *testing.T) {
	_, nc := testutil.StartNATS(t)
	ready := &testProvider{
		queryResult: &searchv1.QueryResponse{},
		status:      &searchv1.GetStatusResponse{State: searchv1.ProviderState_PROVIDER_STATE_READY},
	}
	readyService, err := AddService(testContext(t), nc, ready, ServiceOptions{})
	if err != nil {
		t.Fatalf("AddService: %v", err)
	}
	t.Cleanup(func() { _ = readyService.Stop() })

	starting := &testProvider{status: &searchv1.GetStatusResponse{State: searchv1.ProviderState_PROVIDER_STATE_INDEXING}}
	startingService, err := AddStartupStatusService(testContext(t), nc, starting, ServiceOptions{})
	if err != nil {
		t.Fatalf("AddStartupStatusService: %v", err)
	}
	t.Cleanup(func() { _ = startingService.Stop() })

	client := NewClient(nc)
	for range 100 {
		status, err := client.GetStatus(testContext(t))
		if err != nil || status.GetState() != searchv1.ProviderState_PROVIDER_STATE_READY {
			t.Fatalf("GetStatus = %+v, %v; want ready", status, err)
		}
		if _, err := client.Query(testContext(t), validQuery()); err != nil {
			t.Fatalf("Query while another replica starts: %v", err)
		}
	}
}

func TestServiceRejectsInvalidQueryBeforeProvider(t *testing.T) {
	_, nc := testutil.StartNATS(t)
	provider := &testProvider{status: &searchv1.GetStatusResponse{State: searchv1.ProviderState_PROVIDER_STATE_READY}}
	service, err := AddService(testContext(t), nc, provider, ServiceOptions{})
	if err != nil {
		t.Fatalf("AddService: %v", err)
	}
	t.Cleanup(func() { _ = service.Stop() })

	payload, err := proto.Marshal(&searchv1.QueryRequest{PageSize: 20, Order: searchv1.SearchOrder_SEARCH_ORDER_RELEVANCE})
	if err != nil {
		t.Fatal(err)
	}
	response, err := nc.RequestWithContext(testContext(t), QuerySubject, payload)
	if err != nil {
		t.Fatalf("request invalid query: %v", err)
	}
	if response.Header.Get("Nats-Service-Error-Code") != ErrorCodeInvalidArgument {
		t.Fatalf("service error headers = %v", response.Header)
	}
	if provider.calls() != 0 {
		t.Fatalf("provider query calls = %d, want 0", provider.calls())
	}
}

func TestProviderNotReadyIsRetryableServiceError(t *testing.T) {
	_, nc := testutil.StartNATS(t)
	provider := &testProvider{queryErr: ErrProviderNotReady}
	service, err := AddService(testContext(t), nc, provider, ServiceOptions{})
	if err != nil {
		t.Fatalf("AddService: %v", err)
	}
	t.Cleanup(func() { _ = service.Stop() })

	_, err = NewClient(nc).Query(testContext(t), validQuery())
	var serviceError *ServiceError
	if !errors.As(err, &serviceError) || serviceError.Code != ErrorCodeUnavailable || !errors.Is(err, ErrUnavailable) {
		t.Fatalf("Query error = %v", err)
	}
}

func TestClientReportsNoRespondersAsUnavailable(t *testing.T) {
	_, nc := testutil.StartNATS(t)
	_, err := NewClient(nc).Query(testContext(t), validQuery())
	if !errors.Is(err, ErrUnavailable) {
		t.Fatalf("Query error = %v, want unavailable", err)
	}
}

func TestClientBoundsUnresponsiveProviderRequests(t *testing.T) {
	_, nc := testutil.StartNATS(t)
	subscription, err := nc.Subscribe(QuerySubject, func(*nats.Msg) {})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = subscription.Unsubscribe() })
	if err := nc.Flush(); err != nil {
		t.Fatal(err)
	}

	client := NewClient(nc)
	client.requestTimeout = 20 * time.Millisecond
	started := time.Now()
	_, err = client.Query(context.Background(), validQuery())
	if !errors.Is(err, ErrUnavailable) {
		t.Fatalf("Query error = %v, want unavailable", err)
	}
	if elapsed := time.Since(started); elapsed > time.Second {
		t.Fatalf("Query took %s, want a bounded request", elapsed)
	}
}

func TestClientRejectsMalformedProviderResponse(t *testing.T) {
	_, nc := testutil.StartNATS(t)
	subscription, err := nc.Subscribe(QuerySubject, func(message *nats.Msg) {
		_ = message.Respond([]byte("not protobuf"))
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = subscription.Unsubscribe() })
	if err := nc.Flush(); err != nil {
		t.Fatal(err)
	}

	_, err = NewClient(nc).Query(testContext(t), validQuery())
	if !errors.Is(err, ErrInvalidResponse) {
		t.Fatalf("Query error = %v, want invalid response", err)
	}
}

func TestClientValidatesBeforeRequest(t *testing.T) {
	_, nc := testutil.StartNATS(t)
	_, err := NewClient(nc).Query(testContext(t), &searchv1.QueryRequest{})
	if err == nil || errors.Is(err, ErrUnavailable) {
		t.Fatalf("Query error = %v, want local validation error", err)
	}
}
