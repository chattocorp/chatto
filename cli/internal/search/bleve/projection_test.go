package bleve

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/charmbracelet/log"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/timestamppb"

	"hmans.de/chatto/internal/encryption"
	"hmans.de/chatto/internal/events"
	"hmans.de/chatto/internal/kms"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
	searchv1 "hmans.de/chatto/internal/pb/chatto/search/v1"
)

type staticLegacyKeys struct{ key []byte }

func (s staticLegacyKeys) LegacyUserKey(context.Context, string) ([]byte, error) {
	return append([]byte(nil), s.key...), nil
}

type staticKeyWrapper struct {
	key         []byte
	expectedAAD []byte
}

func (s staticKeyWrapper) CreateKey(context.Context, string) (string, error) { return "", nil }
func (s staticKeyWrapper) KeyExists(context.Context, string) (bool, error)   { return true, nil }
func (s staticKeyWrapper) WrapContentKey(context.Context, string, []byte, []byte) (*kms.WrappedContentKey, error) {
	return nil, nil
}
func (s staticKeyWrapper) UnwrapContentKey(_ context.Context, _ string, _ kms.WrappedContentKey, aad []byte) ([]byte, error) {
	if !bytes.Equal(aad, s.expectedAAD) {
		return nil, errors.New("unexpected DEK AAD")
	}
	return append([]byte(nil), s.key...), nil
}
func (s staticKeyWrapper) ShredKey(context.Context, string) error { return nil }

type staticDEKStore struct{ value *corev1.UserDataEncryptionKey }

func (s staticDEKStore) Get(context.Context, string) (*corev1.UserDataEncryptionKey, error) {
	return s.value, nil
}

func TestProjectionIndexesRestoresAndRemovesMessages(t *testing.T) {
	key, err := encryption.GenerateKey()
	require.NoError(t, err)
	directory := t.TempDir() + "/index"
	projection, err := NewProjection(directory, nil, staticLegacyKeys{key: key}, nil, log.New(nil))
	require.NoError(t, err)

	request := events.ProjectionCheckpointRequest{
		ProjectionKey: "message_search", ContractID: checkpointContractID,
		StreamName: "EVT", StreamIdentity: "evt-incarnation-v1:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		FirstSequence: 1, LastSequence: 10,
	}
	checkpoint, err := projection.RestoreCheckpoint(context.Background(), request)
	require.NoError(t, err)
	require.Zero(t, checkpoint.CutoffSequence)

	applyLegacyMessage(t, projection, key, "M1", "B1", "R1", "U1", "motherfucking search works", time.Unix(100, 0), 1)
	applyLegacyMessage(t, projection, key, "M2", "B2", "R2", "U2", "search works elsewhere", time.Unix(200, 0), 3)

	response, err := projection.query(context.Background(), &searchv1.QueryRequest{
		RequiredTerms: []string{"search", "works"}, RoomIds: []string{"R1"},
		Order: searchv1.SearchOrder_SEARCH_ORDER_RELEVANCE, PageSize: 10,
	})
	require.NoError(t, err)
	require.Equal(t, []string{"M1"}, hitIDs(response))
	firstPage, err := projection.query(context.Background(), &searchv1.QueryRequest{
		RequiredTerms: []string{"search"}, Order: searchv1.SearchOrder_SEARCH_ORDER_NEWEST, PageSize: 1,
	})
	require.NoError(t, err)
	require.Equal(t, []string{"M2"}, hitIDs(firstPage))
	require.NotEmpty(t, firstPage.GetNextCursor())
	secondPage, err := projection.query(context.Background(), &searchv1.QueryRequest{
		RequiredTerms: []string{"search"}, Order: searchv1.SearchOrder_SEARCH_ORDER_NEWEST, PageSize: 1,
		Cursor: firstPage.GetNextCursor(),
	})
	require.NoError(t, err)
	require.Equal(t, []string{"M1"}, hitIDs(secondPage))
	require.Empty(t, secondPage.GetNextCursor())

	require.NoError(t, projection.Close())
	projection, err = NewProjection(directory, nil, staticLegacyKeys{key: key}, nil, log.New(nil))
	require.NoError(t, err)
	t.Cleanup(func() { _ = projection.Close() })
	checkpoint, err = projection.RestoreCheckpoint(context.Background(), request)
	require.NoError(t, err)
	require.Equal(t, uint64(4), checkpoint.CutoffSequence)

	require.NoError(t, projection.Apply(&corev1.Event{Event: &corev1.Event_MessageRetracted{MessageRetracted: &corev1.MessageRetractedEvent{EventId: "M1"}}}, 5))
	response, err = projection.query(context.Background(), &searchv1.QueryRequest{
		RequiredTerms: []string{"search"}, Order: searchv1.SearchOrder_SEARCH_ORDER_NEWEST, PageSize: 10,
	})
	require.NoError(t, err)
	require.Equal(t, []string{"M2"}, hitIDs(response))

	require.NoError(t, projection.Apply(&corev1.Event{Event: &corev1.Event_RoomDeleted{RoomDeleted: &corev1.RoomDeletedEvent{RoomId: "R2"}}}, 6))
	response, err = projection.query(context.Background(), &searchv1.QueryRequest{
		RequiredTerms: []string{"search"}, Order: searchv1.SearchOrder_SEARCH_ORDER_NEWEST, PageSize: 10,
	})
	require.NoError(t, err)
	require.Empty(t, response.GetHits())
	require.NoError(t, projection.Apply(&corev1.Event{
		Event: &corev1.Event_UserKeyShredded{UserKeyShredded: &corev1.UserKeyShreddedEvent{UserId: "U1"}},
	}, 7))
	pending, err := projection.index.GetInternal([]byte(privacyCompactionKey))
	require.NoError(t, err)
	require.Empty(t, pending)
	require.NoError(t, projection.index.SetInternal([]byte(privacyCompactionKey), []byte{1}))
	require.NoError(t, projection.Close())
	projection, err = NewProjection(directory, nil, staticLegacyKeys{key: key}, nil, log.New(nil))
	require.NoError(t, err)
	_, err = projection.RestoreCheckpoint(context.Background(), request)
	require.NoError(t, err)
	pending, err = projection.index.GetInternal([]byte(privacyCompactionKey))
	require.NoError(t, err)
	require.Empty(t, pending)
}

func TestProjectionRestoresDEKMetadataForTailEdits(t *testing.T) {
	key, err := encryption.GenerateKey()
	require.NoError(t, err)
	request := events.ProjectionCheckpointRequest{
		ProjectionKey: "message_search", ContractID: checkpointContractID,
		StreamName: "EVT", StreamIdentity: "evt-incarnation-v1:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
		FirstSequence: 1, LastSequence: 10,
	}
	dekEvent := &corev1.UserDEKGeneratedEvent{
		UserId: "U1", Purpose: corev1.UserDEKPurpose_USER_DEK_PURPOSE_MESSAGE_BODY,
		Epoch: 1, ContentKeyRef: "dek.test", WrappingKeyRef: "kek.test",
	}
	wrapper := staticKeyWrapper{key: key, expectedAAD: encryption.UserDEKAAD("U1", dekEvent.GetPurpose(), 1)}
	store := staticDEKStore{value: &corev1.UserDataEncryptionKey{WrappingKeyRef: "kek.test"}}
	directory := t.TempDir() + "/index"
	projection, err := NewProjection(directory, wrapper, nil, store, log.New(nil))
	require.NoError(t, err)
	_, err = projection.RestoreCheckpoint(context.Background(), request)
	require.NoError(t, err)
	require.NoError(t, projection.Apply(&corev1.Event{Event: &corev1.Event_UserDekGenerated{UserDekGenerated: dekEvent}}, 1))
	applyV2MessageBody(t, projection, key, "M1", "B1", "R1", "U1", "original searchable body", time.Unix(100, 0), 2)
	require.NoError(t, projection.Apply(&corev1.Event{
		Id: "M1", CreatedAt: timestamppb.New(time.Unix(100, 0)), ActorId: "U1",
		Event: &corev1.Event_MessagePosted{MessagePosted: &corev1.MessagePostedEvent{RoomId: "R1"}},
	}, 3))
	require.NoError(t, projection.Close())

	projection, err = NewProjection(directory, wrapper, nil, store, log.New(nil))
	require.NoError(t, err)
	t.Cleanup(func() { _ = projection.Close() })
	checkpoint, err := projection.RestoreCheckpoint(context.Background(), request)
	require.NoError(t, err)
	require.Equal(t, uint64(3), checkpoint.CutoffSequence)
	applyV2MessageBody(t, projection, key, "M1", "B2", "R1", "U1", "edited searchable body", time.Unix(200, 0), 4)

	response, err := projection.query(context.Background(), &searchv1.QueryRequest{
		RequiredTerms: []string{"edited"}, Order: searchv1.SearchOrder_SEARCH_ORDER_RELEVANCE, PageSize: 10,
	})
	require.NoError(t, err)
	require.Equal(t, []string{"M1"}, hitIDs(response))
	response, err = projection.query(context.Background(), &searchv1.QueryRequest{
		RequiredTerms: []string{"original"}, Order: searchv1.SearchOrder_SEARCH_ORDER_RELEVANCE, PageSize: 10,
	})
	require.NoError(t, err)
	require.Empty(t, response.GetHits())
}

func TestProjectionIndexesMessagesInEitherEventOrder(t *testing.T) {
	key, err := encryption.GenerateKey()
	require.NoError(t, err)
	projection, err := NewProjection(t.TempDir()+"/index", nil, staticLegacyKeys{key: key}, nil, log.New(nil))
	require.NoError(t, err)
	t.Cleanup(func() { _ = projection.Close() })

	applyLegacyBody(t, projection, key, "M1", "B1", "R1", "U1", "sequenceword body first", time.Unix(100, 0), nil, 1)
	applyMessagePosted(t, projection, "M1", "R1", "U1", time.Unix(100, 0), 2)
	applyMessagePosted(t, projection, "M2", "R1", "U2", time.Unix(200, 0), 3)
	applyLegacyBody(t, projection, key, "M2", "B2", "R1", "U2", "sequenceword post first", time.Unix(200, 0), nil, 4)

	response, err := projection.query(context.Background(), &searchv1.QueryRequest{
		RequiredTerms: []string{"sequenceword"}, Order: searchv1.SearchOrder_SEARCH_ORDER_NEWEST, PageSize: 10,
	})
	require.NoError(t, err)
	require.Equal(t, []string{"M2", "M1"}, hitIDs(response))
}

func TestProjectionFiltersByAuthorDateAndAttachments(t *testing.T) {
	key, err := encryption.GenerateKey()
	require.NoError(t, err)
	projection, err := NewProjection(t.TempDir()+"/index", nil, staticLegacyKeys{key: key}, nil, log.New(nil))
	require.NoError(t, err)
	t.Cleanup(func() { _ = projection.Close() })

	applyLegacyMessageWithAssets(t, projection, key, "M1", "B1", "R1", "U1", "shared filter term", time.Unix(100, 0), []string{"A1"}, 1)
	applyLegacyMessage(t, projection, key, "M2", "B2", "R1", "U2", "shared filter term", time.Unix(200, 0), 3)
	applyLegacyMessageWithAssets(t, projection, key, "M3", "B3", "R2", "U1", "shared filter term", time.Unix(300, 0), []string{"A2"}, 5)

	tests := []struct {
		name    string
		request *searchv1.QueryRequest
		want    []string
	}{
		{
			name: "author",
			request: &searchv1.QueryRequest{
				RequiredTerms: []string{"filter"}, AuthorIds: []string{"U2"},
				Order: searchv1.SearchOrder_SEARCH_ORDER_NEWEST, PageSize: 10,
			},
			want: []string{"M2"},
		},
		{
			name: "creation window",
			request: &searchv1.QueryRequest{
				RequiredTerms: []string{"filter"}, CreatedAfter: timestamppb.New(time.Unix(150, 0)),
				CreatedBefore: timestamppb.New(time.Unix(250, 0)), Order: searchv1.SearchOrder_SEARCH_ORDER_NEWEST, PageSize: 10,
			},
			want: []string{"M2"},
		},
		{
			name: "attachments",
			request: &searchv1.QueryRequest{
				RequiredTerms: []string{"filter"}, HasAttachments: true,
				Order: searchv1.SearchOrder_SEARCH_ORDER_NEWEST, PageSize: 10,
			},
			want: []string{"M3", "M1"},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			response, err := projection.query(context.Background(), test.request)
			require.NoError(t, err)
			require.Equal(t, test.want, hitIDs(response))
		})
	}
}

func TestProjectionRejectsMalformedOrForeignCursors(t *testing.T) {
	key, err := encryption.GenerateKey()
	require.NoError(t, err)
	projection, err := NewProjection(t.TempDir()+"/index", nil, staticLegacyKeys{key: key}, nil, log.New(nil))
	require.NoError(t, err)
	t.Cleanup(func() { _ = projection.Close() })

	applyLegacyMessage(t, projection, key, "M1", "B1", "R1", "U1", "cursor search", time.Unix(100, 0), 1)
	applyLegacyMessage(t, projection, key, "M2", "B2", "R1", "U1", "cursor search", time.Unix(200, 0), 3)
	firstPage, err := projection.query(context.Background(), &searchv1.QueryRequest{
		RequiredTerms: []string{"cursor"}, Order: searchv1.SearchOrder_SEARCH_ORDER_NEWEST, PageSize: 1,
	})
	require.NoError(t, err)
	require.NotEmpty(t, firstPage.GetNextCursor())
	request := &searchv1.QueryRequest{
		RequiredTerms: []string{"cursor"}, Order: searchv1.SearchOrder_SEARCH_ORDER_NEWEST, PageSize: 1,
	}
	hash, err := queryHash(request)
	require.NoError(t, err)
	wrongSortCursor, err := json.Marshal(cursor{QueryHash: hash, Sort: []string{"too-short"}})
	require.NoError(t, err)

	tests := []struct {
		name   string
		cursor []byte
		terms  []string
	}{
		{name: "malformed", cursor: []byte("not-json"), terms: []string{"cursor"}},
		{name: "different query", cursor: firstPage.GetNextCursor(), terms: []string{"search"}},
		{name: "wrong sort shape", cursor: wrongSortCursor, terms: []string{"cursor"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := projection.query(context.Background(), &searchv1.QueryRequest{
				RequiredTerms: test.terms, Order: searchv1.SearchOrder_SEARCH_ORDER_NEWEST,
				PageSize: 1, Cursor: test.cursor,
			})
			require.ErrorIs(t, err, errInvalidCursor)
		})
	}
}

func TestProjectionKeyShreddingRemovesIndexedMessages(t *testing.T) {
	key, err := encryption.GenerateKey()
	require.NoError(t, err)
	projection, err := NewProjection(t.TempDir()+"/index", nil, staticLegacyKeys{key: key}, nil, log.New(nil))
	require.NoError(t, err)
	t.Cleanup(func() { _ = projection.Close() })

	applyLegacyMessage(t, projection, key, "M1", "B1", "R1", "U1", "privacy boundary", time.Unix(100, 0), 1)
	applyLegacyMessage(t, projection, key, "M2", "B2", "R1", "U2", "privacy boundary", time.Unix(200, 0), 3)
	require.NoError(t, projection.Apply(&corev1.Event{
		Event: &corev1.Event_UserKeyShredded{UserKeyShredded: &corev1.UserKeyShreddedEvent{UserId: "U1"}},
	}, 5))

	response, err := projection.query(context.Background(), &searchv1.QueryRequest{
		RequiredTerms: []string{"privacy"}, Order: searchv1.SearchOrder_SEARCH_ORDER_NEWEST, PageSize: 10,
	})
	require.NoError(t, err)
	require.Equal(t, []string{"M2"}, hitIDs(response))
	state, err := projection.index.GetInternal(messageStateKey("M1"))
	require.NoError(t, err)
	require.Empty(t, state)
	pending, err := projection.index.GetInternal([]byte(privacyCompactionKey))
	require.NoError(t, err)
	require.Empty(t, pending)
}

func applyLegacyMessage(t *testing.T, projection *Projection, key []byte, messageID, bodyEventID, roomID, authorID, text string, createdAt time.Time, startSeq uint64) {
	t.Helper()
	applyLegacyMessageWithAssets(t, projection, key, messageID, bodyEventID, roomID, authorID, text, createdAt, nil, startSeq)
}

func applyLegacyMessageWithAssets(t *testing.T, projection *Projection, key []byte, messageID, bodyEventID, roomID, authorID, text string, createdAt time.Time, assetIDs []string, startSeq uint64) {
	t.Helper()
	applyLegacyBody(t, projection, key, messageID, bodyEventID, roomID, authorID, text, createdAt, assetIDs, startSeq)
	applyMessagePosted(t, projection, messageID, roomID, authorID, createdAt, startSeq+1)
}

func applyLegacyBody(t *testing.T, projection *Projection, key []byte, messageID, bodyEventID, roomID, authorID, text string, createdAt time.Time, assetIDs []string, seq uint64) {
	t.Helper()
	encrypted, err := encryption.Encrypt(key, []byte(text))
	require.NoError(t, err)
	body := &corev1.MessageBody{
		AuthorId: authorID, CreatedAt: timestamppb.New(createdAt), BodyEventId: bodyEventID,
		EncryptedBody: encrypted.Ciphertext, EncryptionNonce: encrypted.Nonce, AssetIds: assetIDs,
	}
	require.NoError(t, projection.Apply(&corev1.Event{
		Id: bodyEventID, CreatedAt: timestamppb.New(createdAt), ActorId: authorID,
		Event: &corev1.Event_MessageBody{MessageBody: &corev1.MessageBodyEvent{RoomId: roomID, EventId: messageID, Body: body}},
	}, seq))
}

func applyMessagePosted(t *testing.T, projection *Projection, messageID, roomID, authorID string, createdAt time.Time, seq uint64) {
	t.Helper()
	require.NoError(t, projection.Apply(&corev1.Event{
		Id: messageID, CreatedAt: timestamppb.New(createdAt), ActorId: authorID,
		Event: &corev1.Event_MessagePosted{MessagePosted: &corev1.MessagePostedEvent{RoomId: roomID}},
	}, seq))
}

func applyV2MessageBody(t *testing.T, projection *Projection, key []byte, messageID, bodyEventID, roomID, authorID, text string, timestamp time.Time, seq uint64) {
	t.Helper()
	encrypted, err := encryption.EncryptWithContentKey(key, []byte(text), encryption.MessageBodyAAD(messageID, bodyEventID, roomID, authorID, 1))
	require.NoError(t, err)
	body := &corev1.MessageBody{
		AuthorId: authorID, CreatedAt: timestamppb.New(timestamp), UpdatedAt: timestamppb.New(timestamp),
		EncryptionVersion: encryption.EnvelopeVersionV2, ContentKeyEpoch: 1, BodyEventId: bodyEventID,
		EncryptedBody: encrypted.Ciphertext, EncryptionNonce: encrypted.Nonce,
	}
	require.NoError(t, projection.Apply(&corev1.Event{
		Id: bodyEventID, CreatedAt: timestamppb.New(timestamp), ActorId: authorID,
		Event: &corev1.Event_MessageBody{MessageBody: &corev1.MessageBodyEvent{RoomId: roomID, EventId: messageID, Body: body}},
	}, seq))
}

func hitIDs(response *searchv1.QueryResponse) []string {
	ids := make([]string, 0, len(response.GetHits()))
	for _, hit := range response.GetHits() {
		ids = append(ids, hit.GetMessageId())
	}
	return ids
}
