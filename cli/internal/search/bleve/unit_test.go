package bleve

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/charmbracelet/log"
	"github.com/nats-io/nats.go/jetstream"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	"hmans.de/chatto/internal/config"
	"hmans.de/chatto/internal/dekstore"
	"hmans.de/chatto/internal/encryption"
	"hmans.de/chatto/internal/events"
	"hmans.de/chatto/internal/kms"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
	searchv1 "hmans.de/chatto/internal/pb/chatto/search/v1"
	"hmans.de/chatto/internal/runtimeunit"
	"hmans.de/chatto/internal/search"
	"hmans.de/chatto/internal/testutil"
)

type synchronizedBuffer struct {
	mu     sync.Mutex
	buffer bytes.Buffer
}

func (b *synchronizedBuffer) Write(data []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buffer.Write(data)
}

func (b *synchronizedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buffer.String()
}

func TestUnitName(t *testing.T) {
	require.Equal(t, "search.BleveProvider", (Unit{}).Name())
}

func TestUnitReplaysEVTAndServesNATSContract(t *testing.T) {
	_, nc := testutil.StartNATS(t)
	js, err := jetstream.New(nc)
	require.NoError(t, err)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	t.Cleanup(cancel)
	stream, err := js.CreateStream(ctx, jetstream.StreamConfig{
		Name: "EVT", Subjects: []string{"evt.>"}, Storage: jetstream.MemoryStorage,
		Metadata: map[string]string{events.EVTStreamIdentityMetadataKey: "evt-incarnation-v1:cccccccccccccccccccccccccccccccc"},
	})
	require.NoError(t, err)
	encryptionKeys, err := js.CreateKeyValue(ctx, jetstream.KeyValueConfig{Bucket: "ENCRYPTION_KEYS", Storage: jetstream.MemoryStorage})
	require.NoError(t, err)
	runtimeState, err := js.CreateKeyValue(ctx, jetstream.KeyValueConfig{Bucket: "RUNTIME_STATE", Storage: jetstream.MemoryStorage})
	require.NoError(t, err)
	keyStore := kms.NewBuiltin(encryptionKeys, log.New(io.Discard))
	wrappingKeyRef, err := keyStore.CreateKey(ctx, "U1")
	require.NoError(t, err)
	contentKey, err := encryption.GenerateKey()
	require.NoError(t, err)
	wrapped, err := keyStore.WrapContentKey(ctx, wrappingKeyRef, contentKey, encryption.UserDEKAAD("U1", corev1.UserDEKPurpose_USER_DEK_PURPOSE_MESSAGE_BODY, 1))
	require.NoError(t, err)
	contentKeyRef := "dek.integration"
	storedDEK, err := proto.Marshal(&corev1.UserDataEncryptionKey{
		EncryptedContentKey: wrapped.EncryptedContentKey,
		ContentKeyNonce:     wrapped.Nonce,
		WrappingAlgorithm:   wrapped.Algorithm,
		WrappingKeyRef:      wrappingKeyRef,
		WrappingMetadata:    wrapped.Metadata,
	})
	require.NoError(t, err)
	_, err = runtimeState.Create(ctx, contentKeyRef, storedDEK)
	require.NoError(t, err)

	publisher := events.NewPublisher(js, stream, log.New(io.Discard))
	_, err = publisher.AppendEventually(ctx, events.UserAggregate("U1").Subject(events.EventUserDEKGenerated), &corev1.Event{
		Id: "D1", ActorId: "U1", CreatedAt: timestamppb.Now(),
		Event: &corev1.Event_UserDekGenerated{UserDekGenerated: &corev1.UserDEKGeneratedEvent{
			UserId: "U1", Purpose: corev1.UserDEKPurpose_USER_DEK_PURPOSE_MESSAGE_BODY,
			Epoch: 1, ContentKeyRef: contentKeyRef, WrappingKeyRef: wrappingKeyRef,
		}},
	})
	require.NoError(t, err)
	createdAt := timestamppb.Now()
	encrypted, err := encryption.EncryptWithContentKey(contentKey, []byte("search contract integration"), encryption.MessageBodyAAD("M1", "B1", "R1", "U1", 1))
	require.NoError(t, err)
	_, err = publisher.AppendEventually(ctx, events.RoomAggregate("R1").Subject(events.EventMessageBody), &corev1.Event{
		Id: "B1", ActorId: "U1", CreatedAt: createdAt,
		Event: &corev1.Event_MessageBody{MessageBody: &corev1.MessageBodyEvent{
			RoomId: "R1", EventId: "M1", Body: &corev1.MessageBody{
				AuthorId: "U1", CreatedAt: createdAt, BodyEventId: "B1",
				EncryptionVersion: encryption.EnvelopeVersionV2, ContentKeyEpoch: 1,
				EncryptedBody: encrypted.Ciphertext, EncryptionNonce: encrypted.Nonce,
			},
		}},
	})
	require.NoError(t, err)
	_, err = publisher.AppendEventually(ctx, events.RoomAggregate("R1").Subject(events.EventMessagePosted), &corev1.Event{
		Id: "M1", ActorId: "U1", CreatedAt: createdAt,
		Event: &corev1.Event_MessagePosted{MessagePosted: &corev1.MessagePostedEvent{RoomId: "R1"}},
	})
	require.NoError(t, err)

	var unitLogs synchronizedBuffer
	unitLogger := log.New(&unitLogs)
	unitLogger.SetFormatter(log.JSONFormatter)
	indexDirectory := t.TempDir() + "/index"
	unitConfig := config.ChattoConfig{SearchProvider: config.SearchProviderConfig{Directory: indexDirectory, Languages: []string{}}}
	var stopUnit context.CancelFunc
	var done chan error
	startUnit := func() {
		unitContext, cancelUnit := context.WithCancel(context.Background())
		stopUnit = cancelUnit
		done = make(chan error, 1)
		go func() {
			done <- (Unit{}).Run(unitContext, runtimeunit.Env{
				Config: unitConfig,
				NC:     nc, JS: js, Logger: unitLogger, Version: "test",
			})
		}()
	}
	stopActiveUnit := func() {
		if stopUnit == nil {
			return
		}
		stopUnit()
		require.NoError(t, <-done)
		stopUnit = nil
		done = nil
	}
	t.Cleanup(stopActiveUnit)
	startUnit()

	client := search.NewClient(nc)
	var response *searchv1.QueryResponse
	for ctx.Err() == nil {
		response, err = client.Query(ctx, &searchv1.QueryRequest{
			RequiredTerms: []string{"integration"}, Order: searchv1.SearchOrder_SEARCH_ORDER_RELEVANCE, PageSize: 10,
		})
		if err == nil {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	require.NoError(t, err)
	require.Equal(t, []string{"M1"}, hitIDs(response))
	require.Eventually(t, func() bool {
		return strings.Contains(unitLogs.String(), "Projection startup complete")
	}, time.Second, 10*time.Millisecond)
	logged := unitLogs.String()
	require.Contains(t, logged, "Starting bundled search provider")
	require.Contains(t, logged, "Search index opened")
	require.Contains(t, logged, "Search provider service registered")
	require.Contains(t, logged, "Projection startup complete")
	require.Contains(t, logged, `"projection":"message_search"`)

	stopActiveUnit()
	unrelatedAck, err := js.Publish(ctx, "evt.unrelated.integration", []byte{0xff})
	require.NoError(t, err)
	streamInfo, err := stream.Info(ctx)
	require.NoError(t, err)
	legacyProjection, err := NewProjection(
		indexDirectory,
		unitConfig.SearchProvider.LanguagesOrDefault(),
		keyStore,
		keyStore,
		dekstore.New(runtimeState, unitLogger),
		unitLogger,
	)
	require.NoError(t, err)
	checkpoint, err := legacyProjection.RestoreCheckpoint(ctx, events.ProjectionCheckpointRequest{
		ProjectionKey:  "message_search",
		ContractID:     legacyProjection.CheckpointContractID(),
		StreamName:     streamInfo.Config.Name,
		StreamIdentity: streamInfo.Config.Metadata[events.EVTStreamIdentityMetadataKey],
		FirstSequence:  streamInfo.State.FirstSeq,
		LastSequence:   streamInfo.State.LastSeq,
	})
	require.NoError(t, err)
	require.Less(t, checkpoint.CutoffSequence, unrelatedAck.Sequence)
	// Simulate the previous broad projection filters, which atomically recorded
	// irrelevant EVT positions even though they did not change the search index.
	require.NoError(t, legacyProjection.Apply(&corev1.Event{Id: "ignored-legacy-event"}, unrelatedAck.Sequence))
	require.NoError(t, legacyProjection.Close())

	startUnit()
	response = nil
	for ctx.Err() == nil {
		response, err = client.Query(ctx, &searchv1.QueryRequest{
			RequiredTerms: []string{"integration"}, Order: searchv1.SearchOrder_SEARCH_ORDER_RELEVANCE, PageSize: 10,
		})
		if err == nil {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	require.NoError(t, err)
	require.Equal(t, []string{"M1"}, hitIDs(response))
	status, err := client.GetStatus(ctx)
	require.NoError(t, err)
	require.Equal(t, searchv1.ProviderState_PROVIDER_STATE_READY, status.GetState())
	require.Eventually(t, func() bool {
		return strings.Contains(unitLogs.String(), "Projection checkpoint restored")
	}, time.Second, 10*time.Millisecond)
}

func TestUnitFailsClosedWhenCheckpointPrecedesRetainedEVT(t *testing.T) {
	_, nc := testutil.StartNATS(t)
	js, err := jetstream.New(nc)
	require.NoError(t, err)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	t.Cleanup(cancel)
	stream, err := js.CreateStream(ctx, jetstream.StreamConfig{
		Name: "EVT", Subjects: []string{"evt.>"}, Storage: jetstream.MemoryStorage,
		Metadata: map[string]string{events.EVTStreamIdentityMetadataKey: "evt-incarnation-v1:dddddddddddddddddddddddddddddddd"},
	})
	require.NoError(t, err)
	encryptionKeys, err := js.CreateKeyValue(ctx, jetstream.KeyValueConfig{Bucket: "ENCRYPTION_KEYS", Storage: jetstream.MemoryStorage})
	require.NoError(t, err)
	runtimeState, err := js.CreateKeyValue(ctx, jetstream.KeyValueConfig{Bucket: "RUNTIME_STATE", Storage: jetstream.MemoryStorage})
	require.NoError(t, err)
	logger := log.New(io.Discard)
	keyStore := kms.NewBuiltin(encryptionKeys, logger)

	retraction := &corev1.Event{
		Id: "E1", ActorId: "U1", CreatedAt: timestamppb.Now(),
		Event: &corev1.Event_MessageRetracted{MessageRetracted: &corev1.MessageRetractedEvent{RoomId: "R1", EventId: "M1"}},
	}
	payload, err := proto.Marshal(retraction)
	require.NoError(t, err)
	var sequences []uint64
	for range 3 {
		ack, publishErr := js.Publish(ctx, events.RoomAggregate("R1").Subject(events.EventMessageRetracted), payload)
		require.NoError(t, publishErr)
		sequences = append(sequences, ack.Sequence)
	}

	unitConfig := config.ChattoConfig{SearchProvider: config.SearchProviderConfig{
		Directory: t.TempDir() + "/index",
		Languages: []string{},
	}}
	projection, err := NewProjection(
		unitConfig.SearchProvider.Directory,
		unitConfig.SearchProvider.LanguagesOrDefault(),
		keyStore,
		keyStore,
		dekstore.New(runtimeState, logger),
		logger,
	)
	require.NoError(t, err)
	streamInfo, err := stream.Info(ctx)
	require.NoError(t, err)
	_, err = projection.RestoreCheckpoint(ctx, events.ProjectionCheckpointRequest{
		ProjectionKey:  "message_search",
		ContractID:     projection.CheckpointContractID(),
		StreamName:     streamInfo.Config.Name,
		StreamIdentity: streamInfo.Config.Metadata[events.EVTStreamIdentityMetadataKey],
		FirstSequence:  streamInfo.State.FirstSeq,
		LastSequence:   streamInfo.State.LastSeq,
	})
	require.NoError(t, err)
	require.NoError(t, projection.Apply(retraction, sequences[0]))
	require.NoError(t, projection.Close())
	require.NoError(t, stream.Purge(ctx, jetstream.WithPurgeSequence(sequences[2])))

	startupResult := make(chan error, 1)
	releaseStartupFailure := make(chan struct{})
	unit := Unit{startupResultHook: func(err error) {
		startupResult <- err
		<-releaseStartupFailure
	}}
	unitContext, stopUnit := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- unit.Run(unitContext, runtimeunit.Env{
			Config: unitConfig,
			NC:     nc, JS: js, Logger: logger, Version: "test",
		})
	}()
	var releaseOnce sync.Once
	release := func() { releaseOnce.Do(func() { close(releaseStartupFailure) }) }
	finished := false
	t.Cleanup(func() {
		release()
		stopUnit()
		if !finished {
			<-done
		}
	})

	startupErr := <-startupResult
	require.ErrorContains(t, startupErr, "behind retained EVT start")
	client := search.NewClient(nc)
	status, err := client.GetStatus(ctx)
	require.NoError(t, err)
	require.Equal(t, searchv1.ProviderState_PROVIDER_STATE_UNAVAILABLE, status.GetState())
	_, err = client.Query(ctx, &searchv1.QueryRequest{
		RequiredTerms: []string{"anything"}, Order: searchv1.SearchOrder_SEARCH_ORDER_RELEVANCE, PageSize: 10,
	})
	require.ErrorIs(t, err, search.ErrUnavailable)

	release()
	unitErr := <-done
	finished = true
	require.ErrorContains(t, unitErr, "behind retained EVT start")
	require.ErrorContains(t, unitErr, "move or delete")
	afterContext, cancelAfter := context.WithTimeout(context.Background(), time.Second)
	defer cancelAfter()
	_, err = client.GetStatus(afterContext)
	require.True(t, errors.Is(err, search.ErrUnavailable), "GetStatus after unit exit = %v", err)
}

func TestLogSearchIndexingStatusReportsSafeProgressFields(t *testing.T) {
	var output bytes.Buffer
	logger := log.New(&output)
	logger.SetFormatter(log.JSONFormatter)

	logSearchIndexingStatus(logger, events.ProjectorStatus{
		Started:          true,
		LastSeq:          600,
		StartupTargetSeq: 1_000,
		StartupDuration:  2 * time.Second,
		StartupMessages:  400,
	}, 100, 500*time.Millisecond)

	logged := output.String()
	for _, expected := range []string{
		"Search provider indexing progress",
		`"stage":"startup_replay"`,
		`"indexed_events":400`,
		`"events_since_last_report":300`,
		`"events_per_second":200`,
		`"average_events_per_second":200`,
		`"stalled":false`,
		`"current_seq":600`,
		`"target_seq":1000`,
	} {
		if !strings.Contains(logged, expected) {
			t.Fatalf("progress log %q does not contain %q", logged, expected)
		}
	}
	for _, forbidden := range []string{"query", "message_id", "room_id", "author_id"} {
		if strings.Contains(logged, forbidden) {
			t.Fatalf("progress log %q contains forbidden field %q", logged, forbidden)
		}
	}
}

func TestLogSearchIndexingStatusMakesStallsExplicit(t *testing.T) {
	var output bytes.Buffer
	logger := log.New(&output)
	logger.SetFormatter(log.JSONFormatter)

	logSearchIndexingStatus(logger, events.ProjectorStatus{
		Started:          true,
		LastSeq:          31_876,
		StartupTargetSeq: 54_207,
		StartupDuration:  50 * time.Second,
		StartupMessages:  27_904,
	}, 27_904, 40*time.Second)

	logged := output.String()
	require.Contains(t, logged, `"events_since_last_report":0`)
	require.Contains(t, logged, `"events_per_second":0`)
	require.Contains(t, logged, `"stalled":true`)
}
