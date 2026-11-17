package search

import (
	"context"
	"errors"
	"math"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/nats-io/nats.go"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/timestamppb"

	searchv1 "github.com/chattocorp/chatto/cli/internal/pb/chatto/search/v1"
	"github.com/chattocorp/chatto/cli/internal/testutil"
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
			Hits:       []*searchv1.QueryHit{{MessageId: "msg_one", RoomId: "rm_one", BodyEventId: "body_one", RelevanceScore: 7.5}},
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
	if len(response.GetHits()) != 1 || response.GetHits()[0].GetMessageId() != "msg_one" || response.GetHits()[0].GetRelevanceScore() != 7.5 || string(response.GetNextCursor()) != "provider-page-2" {
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

func TestValidateQueryRequestAcceptsFilterOnlyQueries(t *testing.T) {
	tests := map[string]*searchv1.QueryRequest{
		"room":           {RoomIds: []string{"rm_one"}},
		"author":         {AuthorIds: []string{"usr_one"}},
		"thread":         {ThreadRootIds: []string{"thread_one"}},
		"created after":  {CreatedAfter: timestamppb.New(time.Unix(100, 0))},
		"created before": {CreatedBefore: timestamppb.New(time.Unix(200, 0))},
		"attachments":    {HasAttachments: true},
	}
	for name, query := range tests {
		t.Run(name, func(t *testing.T) {
			query.Order = searchv1.SearchOrder_SEARCH_ORDER_NEWEST
			query.PageSize = 20
			if err := ValidateQueryRequest(query); err != nil {
				t.Fatalf("ValidateQueryRequest() = %v", err)
			}
		})
	}
}

func TestValidateQueryResponseRejectsInvalidRelevanceScores(t *testing.T) {
	for _, score := range []float64{-1, math.NaN(), math.Inf(1)} {
		response := &searchv1.QueryResponse{Hits: []*searchv1.QueryHit{{
			MessageId: "msg_one", RoomId: "rm_one", BodyEventId: "body_one", RelevanceScore: score,
		}}}
		if err := validateQueryResponse(response, 1); !errors.Is(err, ErrInvalidResponse) {
			t.Fatalf("validateQueryResponse(score %v) = %v, want invalid response", score, err)
		}
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
	if err := AddQueryEndpoint(testContext(t), nc, service, provider); err != nil {
		t.Fatalf("AddQueryEndpoint: %v", err)
	}
	if err := addStatusEndpoint(testContext(t), service, provider, ServiceOptions{}); err != nil {
		t.Fatalf("addStatusEndpoint: %v", err)
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

// blockingProvider ignores query cancellation, like a query that waits for
// an index lock that a blocked commit holds.
type blockingProvider struct {
	testProvider
	entered chan struct{}
	release chan struct{}
}

func (p *blockingProvider) Query(ctx context.Context, request *searchv1.QueryRequest) (*searchv1.QueryResponse, error) {
	p.entered <- struct{}{}
	<-p.release
	return p.testProvider.Query(ctx, request)
}

func TestQueryEndpointBoundsBlockedQueries(t *testing.T) {
	_, nc := testutil.StartNATS(t)
	provider := &blockingProvider{
		testProvider: testProvider{queryResult: &searchv1.QueryResponse{}},
		entered:      make(chan struct{}, maxConcurrentQueries+1),
		release:      make(chan struct{}),
	}
	var releaseOnce sync.Once
	release := func() { releaseOnce.Do(func() { close(provider.release) }) }
	t.Cleanup(release)
	service, err := AddService(testContext(t), nc, provider, ServiceOptions{})
	if err != nil {
		t.Fatalf("AddService: %v", err)
	}
	t.Cleanup(func() { _ = service.Stop() })
	payload, err := proto.Marshal(validQuery())
	if err != nil {
		t.Fatal(err)
	}

	// Fill every handler slot with a blocked query. Handlers run
	// concurrently, so each request reaches the provider.
	for range maxConcurrentQueries {
		if err := nc.PublishRequest(QuerySubject, nats.NewInbox(), payload); err != nil {
			t.Fatal(err)
		}
	}
	for range maxConcurrentQueries {
		select {
		case <-provider.entered:
		case <-testContext(t).Done():
			t.Fatal("blocked queries did not run concurrently")
		}
	}

	// The next query is rejected at once with a retryable error.
	started := time.Now()
	response, err := nc.RequestWithContext(testContext(t), QuerySubject, payload)
	if err != nil {
		t.Fatalf("request while busy: %v", err)
	}
	if code := response.Header.Get("Nats-Service-Error-Code"); code != ErrorCodeUnavailable {
		t.Fatalf("busy error code = %q, headers = %v", code, response.Header)
	}
	if elapsed := time.Since(started); elapsed > time.Second {
		t.Fatalf("busy rejection took %s", elapsed)
	}

	// After the blocked queries return, the endpoint accepts queries again.
	// Handlers free their slots asynchronously, so retry only busy errors.
	release()
	ctx := testContext(t)
	for {
		_, err := NewClient(nc).Query(ctx, validQuery())
		if err == nil {
			break
		}
		var serviceError *ServiceError
		if !errors.As(err, &serviceError) || serviceError.Description != "search provider is busy" || ctx.Err() != nil {
			t.Fatalf("Query after release: %v", err)
		}
		time.Sleep(time.Millisecond)
	}
}

func TestProviderReportsIncludeEveryProvider(t *testing.T) {
	_, nc := testutil.StartNATS(t)
	for _, provider := range []struct {
		options ServiceOptions
		status  *searchv1.GetStatusResponse
	}{
		{ServiceOptions{InstanceID: "b", Host: "search-1", ImplementationVersion: "1.0"}, &searchv1.GetStatusResponse{State: searchv1.ProviderState_PROVIDER_STATE_UNAVAILABLE}},
		{ServiceOptions{InstanceID: "a", Host: "search-0", ImplementationVersion: "1.0"}, &searchv1.GetStatusResponse{
			State:     searchv1.ProviderState_PROVIDER_STATE_READY,
			Freshness: searchv1.IndexFreshness_INDEX_FRESHNESS_STALLED,
		}},
	} {
		service, err := AddStartupStatusService(testContext(t), nc, &testProvider{status: provider.status}, provider.options)
		if err != nil {
			t.Fatalf("AddStartupStatusService: %v", err)
		}
		t.Cleanup(func() { _ = service.Stop() })
	}

	reports, err := NewClient(nc).ProviderReports(testContext(t))
	if err != nil {
		t.Fatalf("ProviderReports: %v", err)
	}
	if len(reports) != 2 {
		t.Fatalf("ProviderReports len = %d, want 2", len(reports))
	}
	if got := reports[0]; got.Status.GetInstanceId() != "a" || got.Status.GetHost() != "search-0" || got.Health != ProviderHealthReady || got.Index != IndexHealthStalled {
		t.Fatalf("first report = %+v", got)
	}
	if got := reports[1]; got.Status.GetInstanceId() != "b" || got.Status.GetImplementationVersion() != "1.0" || got.Health != ProviderHealthUnavailable || got.Index != IndexHealthUnknown {
		t.Fatalf("second report = %+v", got)
	}
}

// A provider that predates AllStatusSubject answers only the queue-grouped
// status subjects.
func TestProviderReportsFallBackToQueueGroupStatus(t *testing.T) {
	_, nc := testutil.StartNATS(t)
	provider := &testProvider{status: &searchv1.GetStatusResponse{State: searchv1.ProviderState_PROVIDER_STATE_READY}}
	service, err := AddService(testContext(t), nc, provider, ServiceOptions{})
	if err != nil {
		t.Fatalf("AddService: %v", err)
	}
	t.Cleanup(func() { _ = service.Stop() })

	reports, err := NewClient(nc).ProviderReports(testContext(t))
	if err != nil || len(reports) != 1 || reports[0].Health != ProviderHealthReady {
		t.Fatalf("ProviderReports = %+v, %v; want one ready provider", reports, err)
	}

	_ = service.Stop()
	if _, err := NewClient(nc).ProviderReports(testContext(t)); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("ProviderReports without providers = %v, want unavailable", err)
	}
}

func TestClassifyStatus(t *testing.T) {
	now := time.Now()
	for name, tc := range map[string]struct {
		status     *searchv1.GetStatusResponse
		wantHealth ProviderHealth
		wantIndex  IndexHealth
	}{
		"ready and current": {
			status:     &searchv1.GetStatusResponse{State: searchv1.ProviderState_PROVIDER_STATE_READY, Freshness: searchv1.IndexFreshness_INDEX_FRESHNESS_CURRENT, ObservedAt: timestamppb.New(now)},
			wantHealth: ProviderHealthReady, wantIndex: IndexHealthCurrent,
		},
		"degraded and stalled": {
			status:     &searchv1.GetStatusResponse{State: searchv1.ProviderState_PROVIDER_STATE_DEGRADED, Freshness: searchv1.IndexFreshness_INDEX_FRESHNESS_STALLED},
			wantHealth: ProviderHealthDegraded, wantIndex: IndexHealthStalled,
		},
		"indexing without freshness": {
			status:     &searchv1.GetStatusResponse{State: searchv1.ProviderState_PROVIDER_STATE_INDEXING},
			wantHealth: ProviderHealthIndexing, wantIndex: IndexHealthUnknown,
		},
		"unspecified state": {
			status:     &searchv1.GetStatusResponse{Freshness: searchv1.IndexFreshness_INDEX_FRESHNESS_BEHIND},
			wantHealth: ProviderHealthUnknown, wantIndex: IndexHealthBehind,
		},
		"outdated status": {
			status:     &searchv1.GetStatusResponse{State: searchv1.ProviderState_PROVIDER_STATE_READY, Freshness: searchv1.IndexFreshness_INDEX_FRESHNESS_CURRENT, ObservedAt: timestamppb.New(now.Add(-MaxStatusAge - time.Second))},
			wantHealth: ProviderHealthUnknown, wantIndex: IndexHealthUnknown,
		},
	} {
		t.Run(name, func(t *testing.T) {
			report := ClassifyStatus(tc.status, now)
			if report.Health != tc.wantHealth || report.Index != tc.wantIndex {
				t.Fatalf("ClassifyStatus = %s/%s, want %s/%s", report.Health, report.Index, tc.wantHealth, tc.wantIndex)
			}
		})
	}
}

// A provider with broken diagnostics stays visible as unknown next to a
// healthy one.
func TestProviderReportsKeepInvalidRepliesAsUnknown(t *testing.T) {
	_, nc := testutil.StartNATS(t)
	for _, provider := range []struct {
		options ServiceOptions
		status  *searchv1.GetStatusResponse
		err     error
	}{
		{ServiceOptions{InstanceID: "a", Host: "search-0"}, &searchv1.GetStatusResponse{
			State:     searchv1.ProviderState_PROVIDER_STATE_READY,
			Freshness: searchv1.IndexFreshness_INDEX_FRESHNESS_CURRENT,
		}, nil},
		{ServiceOptions{InstanceID: "b", Host: "search-1"}, nil, errors.New("status failed")},
	} {
		service, err := AddStartupStatusService(testContext(t), nc, &testProvider{status: provider.status, statusErr: provider.err}, provider.options)
		if err != nil {
			t.Fatalf("AddStartupStatusService: %v", err)
		}
		t.Cleanup(func() { _ = service.Stop() })
	}

	reports, err := NewClient(nc).ProviderReports(testContext(t))
	if err != nil {
		t.Fatalf("ProviderReports: %v", err)
	}
	healths := map[ProviderHealth]int{}
	for _, report := range reports {
		healths[report.Health]++
	}
	if len(reports) != 2 || healths[ProviderHealthReady] != 1 || healths[ProviderHealthUnknown] != 1 {
		t.Fatalf("ProviderReports = %+v, want one ready and one unknown provider", reports)
	}
}

func TestStatusFromReplyKeepsIdentityOfInvalidStatus(t *testing.T) {
	data, err := proto.Marshal(&searchv1.GetStatusResponse{InstanceId: "a", Host: "search-0", CurrentOperation: strings.Repeat("x", maxStatusOperationBytes+1), State: searchv1.ProviderState_PROVIDER_STATE_READY})
	if err != nil {
		t.Fatal(err)
	}
	status := statusFromReply(&nats.Msg{Data: data})
	if status.GetState() != searchv1.ProviderState_PROVIDER_STATE_UNSPECIFIED || status.GetInstanceId() != "a" || status.GetHost() != "search-0" {
		t.Fatalf("statusFromReply = %+v, want identity without state", status)
	}
}
