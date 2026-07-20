package bleve

import (
	"context"
	"io"
	"testing"
	"time"

	"github.com/charmbracelet/log"
	"github.com/nats-io/nats.go/jetstream"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	"hmans.de/chatto/internal/config"
	"hmans.de/chatto/internal/encryption"
	"hmans.de/chatto/internal/events"
	"hmans.de/chatto/internal/kms"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
	searchv1 "hmans.de/chatto/internal/pb/chatto/search/v1"
	"hmans.de/chatto/internal/runtimeunit"
	"hmans.de/chatto/internal/search"
	"hmans.de/chatto/internal/testutil"
)

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

	unitContext, stopUnit := context.WithCancel(context.Background())
	done := make(chan error, 1)
	indexDirectory := t.TempDir() + "/index"
	go func() {
		done <- (Unit{}).Run(unitContext, runtimeunit.Env{
			Config: config.ChattoConfig{SearchProvider: config.SearchProviderConfig{Directory: indexDirectory}},
			NC:     nc, JS: js, Logger: log.New(io.Discard), Version: "test",
		})
	}()
	t.Cleanup(func() {
		stopUnit()
		require.NoError(t, <-done)
	})

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
}
