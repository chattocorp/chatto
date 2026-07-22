package bleve

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	blevesearch "github.com/blevesearch/bleve/v2"
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

func TestProjectionSubjectsOnlyConsumeSearchFacts(t *testing.T) {
	projection := &Projection{}
	require.Equal(t, []string{
		events.RoomEventTypeFilter(events.EventMessageBody),
		events.RoomEventTypeFilter(events.EventMessagePosted),
		events.RoomEventTypeFilter(events.EventMessageRetracted),
		events.RoomEventTypeFilter(events.EventRoomDeleted),
		events.UserEventTypeFilter(events.EventUserDEKGenerated),
		events.UserEventTypeFilter(events.EventUserKeyShredded),
	}, projection.Subjects())
}

func TestProjectionIndexesRestoresAndRemovesMessages(t *testing.T) {
	key, err := encryption.GenerateKey()
	require.NoError(t, err)
	directory := t.TempDir() + "/index"
	projection, err := NewProjection(directory, nil, nil, staticLegacyKeys{key: key}, nil, log.New(nil))
	require.NoError(t, err)

	request := events.ProjectionCheckpointRequest{
		ProjectionKey: "message_search", ContractID: projection.CheckpointContractID(),
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
	projection, err = NewProjection(directory, nil, nil, staticLegacyKeys{key: key}, nil, log.New(nil))
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
	require.NoError(t, projection.Close())
	projection, err = NewProjection(directory, nil, nil, staticLegacyKeys{key: key}, nil, log.New(nil))
	require.NoError(t, err)
	checkpoint, err = projection.RestoreCheckpoint(context.Background(), request)
	require.NoError(t, err)
	require.Equal(t, uint64(7), checkpoint.CutoffSequence)
}

func TestProjectionStartupBatchCommitsMessageAndCheckpointOnce(t *testing.T) {
	key, err := encryption.GenerateKey()
	require.NoError(t, err)
	directory := t.TempDir() + "/index"
	projection, err := NewProjection(directory, []string{"de", "en"}, nil, staticLegacyKeys{key: key}, nil, log.New(nil))
	require.NoError(t, err)
	request := events.ProjectionCheckpointRequest{
		ProjectionKey: "message_search", ContractID: projection.CheckpointContractID(),
		StreamName: "EVT", StreamIdentity: "evt-incarnation-v1:dddddddddddddddddddddddddddddddd",
		FirstSequence: 1, LastSequence: 2,
	}
	_, err = projection.RestoreCheckpoint(context.Background(), request)
	require.NoError(t, err)
	batches := 0
	projection.commitBatch = func(batch *blevesearch.Batch) error {
		batches++
		return projection.index.Batch(batch)
	}

	createdAt := time.Unix(100, 0)
	bodyEvent := legacyBodyEvent(t, key, "M1", "B1", "R1", "U1", "batched searchable body", createdAt, nil)
	postedEvent := messagePostedEvent("M1", "R1", "U1", createdAt)
	require.NoError(t, projection.ApplyStartupBatch([]events.SequencedEvent{
		{Event: bodyEvent, Sequence: 1},
		{Event: postedEvent, Sequence: 2},
	}))
	require.Equal(t, 1, batches)

	response, err := projection.query(context.Background(), &searchv1.QueryRequest{
		RequiredTerms: []string{"batched"}, Order: searchv1.SearchOrder_SEARCH_ORDER_RELEVANCE, PageSize: 10,
	})
	require.NoError(t, err)
	require.Equal(t, []string{"M1"}, hitIDs(response))
	require.NoError(t, projection.Close())

	projection, err = NewProjection(directory, []string{"de", "en"}, nil, staticLegacyKeys{key: key}, nil, log.New(nil))
	require.NoError(t, err)
	t.Cleanup(func() { _ = projection.Close() })
	checkpoint, err := projection.RestoreCheckpoint(context.Background(), request)
	require.NoError(t, err)
	require.Equal(t, uint64(2), checkpoint.CutoffSequence)
}

func TestProjectionStartupBatchAppliesDeletesAgainstPendingMessages(t *testing.T) {
	key, err := encryption.GenerateKey()
	require.NoError(t, err)
	projection, err := NewProjection(t.TempDir()+"/index", []string{"en"}, nil, staticLegacyKeys{key: key}, nil, log.New(nil))
	require.NoError(t, err)
	t.Cleanup(func() { _ = projection.Close() })
	createdAt := time.Unix(100, 0)
	items := []events.SequencedEvent{
		{Event: legacyBodyEvent(t, key, "M1", "B1", "R1", "U1", "pending retract", createdAt, nil), Sequence: 1},
		{Event: messagePostedEvent("M1", "R1", "U1", createdAt), Sequence: 2},
		{Event: &corev1.Event{Event: &corev1.Event_MessageRetracted{MessageRetracted: &corev1.MessageRetractedEvent{EventId: "M1"}}}, Sequence: 3},
		{Event: legacyBodyEvent(t, key, "M2", "B2", "R2", "U2", "pending room deletion", createdAt, nil), Sequence: 4},
		{Event: messagePostedEvent("M2", "R2", "U2", createdAt), Sequence: 5},
		{Event: &corev1.Event{Event: &corev1.Event_RoomDeleted{RoomDeleted: &corev1.RoomDeletedEvent{RoomId: "R2"}}}, Sequence: 6},
		{Event: legacyBodyEvent(t, key, "M3", "B3", "R3", "U3", "pending key shredding", createdAt, nil), Sequence: 7},
		{Event: messagePostedEvent("M3", "R3", "U3", createdAt), Sequence: 8},
		{Event: &corev1.Event{Event: &corev1.Event_UserKeyShredded{UserKeyShredded: &corev1.UserKeyShreddedEvent{UserId: "U3"}}}, Sequence: 9},
	}
	require.NoError(t, projection.ApplyStartupBatch(items))

	response, err := projection.query(context.Background(), &searchv1.QueryRequest{
		RequiredTerms: []string{"pending"}, Order: searchv1.SearchOrder_SEARCH_ORDER_RELEVANCE, PageSize: 10,
	})
	require.NoError(t, err)
	require.Empty(t, response.GetHits())
	for _, id := range []string{"M1", "M2", "M3"} {
		document, err := projection.index.Document(messageDocumentID(id))
		require.NoError(t, err)
		require.Nil(t, document)
	}
}

func TestProjectionStartupReplayKeepsBoltMetadataBounded(t *testing.T) {
	key, err := encryption.GenerateKey()
	require.NoError(t, err)
	directory := t.TempDir() + "/index"
	projection, err := NewProjection(directory, []string{}, nil, staticLegacyKeys{key: key}, nil, log.New(nil))
	require.NoError(t, err)
	createdAt := time.Unix(100, 0)
	sequence := uint64(0)
	batch := make([]events.SequencedEvent, 0, startupReplayBatchSize)
	for i := uint32(0); i < 4096; i++ {
		// Multiplicative hashing prevents this fixture from accidentally testing
		// only bbolt's cheapest monotonically increasing key pattern.
		messageID := fmt.Sprintf("M%08x", i*2654435761)
		bodyEventID := "B" + messageID[1:]
		sequence++
		batch = append(batch, events.SequencedEvent{
			Event:    legacyBodyEvent(t, key, messageID, bodyEventID, fmt.Sprintf("R%d", i%32), fmt.Sprintf("U%d", i%64), "bounded bolt metadata", createdAt, nil),
			Sequence: sequence,
		})
		sequence++
		batch = append(batch, events.SequencedEvent{
			Event:    messagePostedEvent(messageID, fmt.Sprintf("R%d", i%32), fmt.Sprintf("U%d", i%64), createdAt),
			Sequence: sequence,
		})
		if len(batch) == startupReplayBatchSize {
			require.NoError(t, projection.ApplyStartupBatch(batch))
			batch = batch[:0]
		}
	}
	require.Empty(t, batch)
	require.NoError(t, projection.Close())

	info, err := os.Stat(filepath.Join(directory, "store", "root.bolt"))
	require.NoError(t, err)
	require.Less(t, info.Size(), int64(32<<20), "per-message internal state must not inflate root.bolt")
}

func TestProjectionStartupBatchUsesPendingDEKMetadata(t *testing.T) {
	key, err := encryption.GenerateKey()
	require.NoError(t, err)
	dekEvent := &corev1.UserDEKGeneratedEvent{
		UserId: "U1", Purpose: corev1.UserDEKPurpose_USER_DEK_PURPOSE_MESSAGE_BODY,
		Epoch: 1, ContentKeyRef: "dek.test", WrappingKeyRef: "kek.test",
	}
	wrapper := staticKeyWrapper{key: key, expectedAAD: encryption.UserDEKAAD("U1", dekEvent.GetPurpose(), 1)}
	store := staticDEKStore{value: &corev1.UserDataEncryptionKey{WrappingKeyRef: "kek.test"}}
	projection, err := NewProjection(t.TempDir()+"/index", []string{"en"}, wrapper, nil, store, log.New(nil))
	require.NoError(t, err)
	t.Cleanup(func() { _ = projection.Close() })
	createdAt := time.Unix(100, 0)

	require.NoError(t, projection.ApplyStartupBatch([]events.SequencedEvent{
		{Event: &corev1.Event{Event: &corev1.Event_UserDekGenerated{UserDekGenerated: dekEvent}}, Sequence: 1},
		{Event: v2MessageBodyEvent(t, key, "M1", "B1", "R1", "U1", "pending encrypted body", createdAt), Sequence: 2},
		{Event: messagePostedEvent("M1", "R1", "U1", createdAt), Sequence: 3},
	}))

	response, err := projection.query(context.Background(), relevanceRequest([]string{"encrypted"}, nil))
	require.NoError(t, err)
	require.Equal(t, []string{"M1"}, hitIDs(response))
}

func TestProjectionFailedStartupBatchDoesNotAdvanceDurableCheckpoint(t *testing.T) {
	projection, err := NewProjection(t.TempDir()+"/index", []string{}, nil, nil, nil, log.New(nil))
	require.NoError(t, err)
	t.Cleanup(func() { _ = projection.Close() })
	request := events.ProjectionCheckpointRequest{
		ProjectionKey: "message_search", ContractID: projection.CheckpointContractID(),
		StreamName: "EVT", StreamIdentity: "evt-incarnation-v1:eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee",
		FirstSequence: 1, LastSequence: 2,
	}
	_, err = projection.RestoreCheckpoint(context.Background(), request)
	require.NoError(t, err)
	commitErr := errors.New("injected batch failure")
	projection.commitBatch = func(*blevesearch.Batch) error { return commitErr }

	err = projection.ApplyStartupBatch([]events.SequencedEvent{
		{Event: &corev1.Event{}, Sequence: 1},
		{Event: &corev1.Event{}, Sequence: 2},
	})
	require.ErrorIs(t, err, commitErr)
	checkpoint, err := projection.RestoreCheckpoint(context.Background(), request)
	require.NoError(t, err)
	require.Zero(t, checkpoint.CutoffSequence)
}

func TestProjectionNonDEKBatchRetainsDEKMap(t *testing.T) {
	projection, err := NewProjection(t.TempDir()+"/index", nil, nil, nil, nil, log.New(nil))
	require.NoError(t, err)
	t.Cleanup(func() { _ = projection.Close() })
	dek := &corev1.UserDEKGeneratedEvent{
		UserId: "U1", Purpose: corev1.UserDEKPurpose_USER_DEK_PURPOSE_MESSAGE_BODY,
		Epoch: 1, ContentKeyRef: "dek.test", WrappingKeyRef: "kek.test",
	}
	require.NoError(t, projection.Apply(&corev1.Event{
		Event: &corev1.Event_UserDekGenerated{UserDekGenerated: dek},
	}, 1))

	retained := projection.deks
	require.NoError(t, projection.Apply(&corev1.Event{}, 2))
	retained["map-identity-sentinel"] = dek
	require.Contains(t, projection.deks, "map-identity-sentinel",
		"ordinary events must not copy the retained DEK map")
}

func TestProjectionFailedDEKBatchDoesNotMutateRetainedMetadata(t *testing.T) {
	projection, err := NewProjection(t.TempDir()+"/index", nil, nil, nil, nil, log.New(nil))
	require.NoError(t, err)
	t.Cleanup(func() { _ = projection.Close() })
	first := &corev1.UserDEKGeneratedEvent{
		UserId: "U1", Purpose: corev1.UserDEKPurpose_USER_DEK_PURPOSE_MESSAGE_BODY,
		Epoch: 1, ContentKeyRef: "dek.1", WrappingKeyRef: "kek.test",
	}
	require.NoError(t, projection.Apply(&corev1.Event{
		Event: &corev1.Event_UserDekGenerated{UserDekGenerated: first},
	}, 1))

	retained := projection.deks
	commitErr := errors.New("injected batch failure")
	projection.commitBatch = func(*blevesearch.Batch) error { return commitErr }
	second := &corev1.UserDEKGeneratedEvent{
		UserId: "U1", Purpose: corev1.UserDEKPurpose_USER_DEK_PURPOSE_MESSAGE_BODY,
		Epoch: 2, ContentKeyRef: "dek.2", WrappingKeyRef: "kek.test",
	}
	err = projection.Apply(&corev1.Event{
		Event: &corev1.Event_UserDekGenerated{UserDekGenerated: second},
	}, 2)
	require.ErrorIs(t, err, commitErr)
	require.Len(t, retained, 1)
	require.Len(t, projection.deks, 1)
	require.NotContains(t, projection.deks, dekKey("U1", second.GetPurpose(), 2))
	retained["map-identity-sentinel"] = first
	require.Contains(t, projection.deks, "map-identity-sentinel",
		"a failed copy-on-write batch must leave the retained map installed")
}

func TestProjectionRestoresDEKMetadataForTailEdits(t *testing.T) {
	key, err := encryption.GenerateKey()
	require.NoError(t, err)
	dekEvent := &corev1.UserDEKGeneratedEvent{
		UserId: "U1", Purpose: corev1.UserDEKPurpose_USER_DEK_PURPOSE_MESSAGE_BODY,
		Epoch: 1, ContentKeyRef: "dek.test", WrappingKeyRef: "kek.test",
	}
	wrapper := staticKeyWrapper{key: key, expectedAAD: encryption.UserDEKAAD("U1", dekEvent.GetPurpose(), 1)}
	store := staticDEKStore{value: &corev1.UserDataEncryptionKey{WrappingKeyRef: "kek.test"}}
	directory := t.TempDir() + "/index"
	projection, err := NewProjection(directory, nil, wrapper, nil, store, log.New(nil))
	require.NoError(t, err)
	request := events.ProjectionCheckpointRequest{
		ProjectionKey: "message_search", ContractID: projection.CheckpointContractID(),
		StreamName: "EVT", StreamIdentity: "evt-incarnation-v1:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
		FirstSequence: 1, LastSequence: 10,
	}
	_, err = projection.RestoreCheckpoint(context.Background(), request)
	require.NoError(t, err)
	require.NoError(t, projection.Apply(&corev1.Event{Event: &corev1.Event_UserDekGenerated{UserDekGenerated: dekEvent}}, 1))
	applyV2MessageBody(t, projection, key, "M1", "B1", "R1", "U1", "original searchable body", time.Unix(100, 0), 2)
	require.NoError(t, projection.Apply(&corev1.Event{
		Id: "M1", CreatedAt: timestamppb.New(time.Unix(100, 0)), ActorId: "U1",
		Event: &corev1.Event_MessagePosted{MessagePosted: &corev1.MessagePostedEvent{RoomId: "R1"}},
	}, 3))
	require.NoError(t, projection.Close())

	projection, err = NewProjection(directory, nil, wrapper, nil, store, log.New(nil))
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
	projection, err := NewProjection(t.TempDir()+"/index", nil, nil, staticLegacyKeys{key: key}, nil, log.New(nil))
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
	projection, err := NewProjection(t.TempDir()+"/index", nil, nil, staticLegacyKeys{key: key}, nil, log.New(nil))
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
		{
			name: "multiple rooms are alternatives",
			request: &searchv1.QueryRequest{
				RequiredTerms: []string{"filter"}, RoomIds: []string{"R1", "R2"},
				Order: searchv1.SearchOrder_SEARCH_ORDER_NEWEST, PageSize: 10,
			},
			want: []string{"M3", "M2", "M1"},
		},
		{
			name: "multiple authors are alternatives",
			request: &searchv1.QueryRequest{
				RequiredTerms: []string{"filter"}, AuthorIds: []string{"U1", "U2"},
				Order: searchv1.SearchOrder_SEARCH_ORDER_NEWEST, PageSize: 10,
			},
			want: []string{"M3", "M2", "M1"},
		},
		{
			name: "after excludes exact boundary",
			request: &searchv1.QueryRequest{
				RequiredTerms: []string{"filter"}, CreatedAfter: timestamppb.New(time.Unix(200, 0)),
				Order: searchv1.SearchOrder_SEARCH_ORDER_NEWEST, PageSize: 10,
			},
			want: []string{"M3"},
		},
		{
			name: "before excludes exact boundary",
			request: &searchv1.QueryRequest{
				RequiredTerms: []string{"filter"}, CreatedBefore: timestamppb.New(time.Unix(200, 0)),
				Order: searchv1.SearchOrder_SEARCH_ORDER_NEWEST, PageSize: 10,
			},
			want: []string{"M1"},
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

func TestProjectionImprovesRecallWithoutWeakeningExactPhrases(t *testing.T) {
	key, err := encryption.GenerateKey()
	require.NoError(t, err)
	projection, err := NewProjection(t.TempDir()+"/index", nil, nil, staticLegacyKeys{key: key}, nil, log.New(nil))
	require.NoError(t, err)
	t.Cleanup(func() { _ = projection.Close() })

	applyLegacyMessage(t, projection, key, "english", "B1", "R1", "U1", "the runner was running quickly", time.Unix(100, 0), 1)
	applyLegacyMessage(t, projection, key, "german", "B2", "R1", "U1", "die Häuser stehen am Fluss", time.Unix(200, 0), 3)
	applyLegacyMessage(t, projection, key, "typo", "B3", "R1", "U1", "deployment status", time.Unix(300, 0), 5)
	applyLegacyMessage(t, projection, key, "short", "B4", "R1", "U1", "cat", time.Unix(400, 0), 7)
	applyLegacyMessage(t, projection, key, "phrase", "B5", "R1", "U1", "the quick brown fox", time.Unix(500, 0), 9)
	applyLegacyMessage(t, projection, key, "version-two", "B6", "R1", "U1", "the v2 rollout", time.Unix(600, 0), 11)
	applyLegacyMessage(t, projection, key, "version-three", "B7", "R1", "U1", "the v3 rollout", time.Unix(700, 0), 13)
	applyLegacyMessage(t, projection, key, "stop-phrase", "B8", "R1", "U1", "to be or not to be", time.Unix(800, 0), 15)
	applyLegacyMessage(t, projection, key, "stop-distractor", "B9", "R1", "U1", "be ready", time.Unix(900, 0), 17)
	applyLegacyMessage(t, projection, key, "cjk", "B6", "R1", "U1", "検索機能は便利です", time.Unix(600, 0), 11)
	applyLegacyMessage(t, projection, key, "finnish", "B10", "R1", "U1", "edeltäjiinsä", time.Unix(1000, 0), 19)
	applyLegacyMessage(t, projection, key, "hungarian", "B11", "R1", "U1", "babakocsijáért", time.Unix(1100, 0), 21)
	applyLegacyMessage(t, projection, key, "norwegian", "B12", "R1", "U1", "havnedistriktene", time.Unix(1200, 0), 23)
	applyLegacyMessage(t, projection, key, "polish", "B13", "R1", "U1", "przypadku", time.Unix(1300, 0), 25)
	applyLegacyMessage(t, projection, key, "russian", "B14", "R1", "U1", "километрах", time.Unix(1400, 0), 27)

	tests := []struct {
		name    string
		request *searchv1.QueryRequest
		want    []string
	}{
		{name: "English stemming", request: relevanceRequest([]string{"run"}, nil), want: []string{"english"}},
		{name: "German stemming", request: relevanceRequest([]string{"Haus"}, nil), want: []string{"german"}},
		{name: "Finnish stemming", request: relevanceRequest([]string{"edeltäjistään"}, nil), want: []string{"finnish"}},
		{name: "Hungarian stemming", request: relevanceRequest([]string{"babakocsi"}, nil), want: []string{"hungarian"}},
		{name: "Norwegian stemming", request: relevanceRequest([]string{"havnedistrikter"}, nil), want: []string{"norwegian"}},
		{name: "Polish stemming", request: relevanceRequest([]string{"przypadek"}, nil), want: []string{"polish"}},
		{name: "Russian stemming", request: relevanceRequest([]string{"километр"}, nil), want: []string{"russian"}},
		{name: "single edit typo", request: relevanceRequest([]string{"deploymant"}, nil), want: []string{"typo"}},
		{name: "short terms stay exact", request: relevanceRequest([]string{"bat"}, nil), want: []string{}},
		{name: "CJK terms", request: relevanceRequest([]string{"検索"}, nil), want: []string{"cjk"}},
		{name: "exact phrase", request: relevanceRequest(nil, []string{"quick brown"}), want: []string{"phrase"}},
		{name: "non-contiguous phrase", request: relevanceRequest(nil, []string{"quick fox"}), want: []string{}},
		{name: "numeric exact phrase", request: relevanceRequest(nil, []string{"v2 rollout"}), want: []string{"version-two"}},
		{name: "stop words remain exact", request: relevanceRequest(nil, []string{"to be"}), want: []string{"stop-phrase"}},
		{name: "stop-word term", request: relevanceRequest([]string{"to"}, nil), want: []string{"stop-phrase"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			response, err := projection.query(context.Background(), test.request)
			require.NoError(t, err)
			require.Equal(t, test.want, hitIDs(response))
		})
	}
}

func TestProjectionUsesOnlyConfiguredLanguageAnalyzers(t *testing.T) {
	key, err := encryption.GenerateKey()
	require.NoError(t, err)

	newProjection := func(t *testing.T, languages []string) *Projection {
		t.Helper()
		projection, err := NewProjection(
			t.TempDir()+"/index",
			languages,
			nil,
			staticLegacyKeys{key: key},
			nil,
			log.New(nil),
		)
		require.NoError(t, err)
		t.Cleanup(func() { _ = projection.Close() })
		applyLegacyMessage(t, projection, key, "english", "B1", "R1", "U1", "running", time.Unix(100, 0), 1)
		return projection
	}

	request := relevanceRequest([]string{"run"}, nil)
	withEnglish, err := newProjection(t, []string{"en"}).query(context.Background(), request)
	require.NoError(t, err)
	require.Equal(t, []string{"english"}, hitIDs(withEnglish))

	literalOnly, err := newProjection(t, []string{}).query(context.Background(), request)
	require.NoError(t, err)
	require.Empty(t, hitIDs(literalOnly))
}

func TestProjectionCheckpointContractTracksConfiguredLanguages(t *testing.T) {
	english, err := NewProjection(t.TempDir()+"/en", []string{"en"}, nil, nil, nil, log.New(nil))
	require.NoError(t, err)
	t.Cleanup(func() { _ = english.Close() })
	englishReordered, err := NewProjection(t.TempDir()+"/en-reordered", []string{"fr", "en"}, nil, nil, nil, log.New(nil))
	require.NoError(t, err)
	t.Cleanup(func() { _ = englishReordered.Close() })
	englishReorderedAgain, err := NewProjection(t.TempDir()+"/en-reordered-again", []string{"en", "fr"}, nil, nil, nil, log.New(nil))
	require.NoError(t, err)
	t.Cleanup(func() { _ = englishReorderedAgain.Close() })

	require.NotEqual(t, english.CheckpointContractID(), englishReordered.CheckpointContractID())
	require.Equal(t, englishReordered.CheckpointContractID(), englishReorderedAgain.CheckpointContractID())
}

func TestProjectionMatchesCaseInsensitivelyAndRequiresEveryTerm(t *testing.T) {
	key, err := encryption.GenerateKey()
	require.NoError(t, err)
	projection, err := NewProjection(t.TempDir()+"/index", nil, nil, staticLegacyKeys{key: key}, nil, log.New(nil))
	require.NoError(t, err)
	t.Cleanup(func() { _ = projection.Close() })

	applyLegacyMessage(t, projection, key, "both", "B1", "R1", "U1", "The Quick Brown Fox", time.Unix(100, 0), 1)
	applyLegacyMessage(t, projection, key, "quick-only", "B2", "R1", "U1", "quick rabbit", time.Unix(200, 0), 3)
	applyLegacyMessage(t, projection, key, "fox-only", "B3", "R1", "U1", "sleepy fox", time.Unix(300, 0), 5)

	tests := []struct {
		name  string
		terms []string
		want  []string
	}{
		{name: "case insensitive", terms: []string{"QUICK", "FOX"}, want: []string{"both"}},
		{name: "all terms required", terms: []string{"quick", "rabbit"}, want: []string{"quick-only"}},
		{name: "no result", terms: []string{"xyzzyplugh"}, want: []string{}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			response, err := projection.query(context.Background(), relevanceRequest(test.terms, nil))
			require.NoError(t, err)
			require.Equal(t, test.want, hitIDs(response))
		})
	}
}

func TestProjectionUpdatesAttachmentFilterWhenBodyIsEdited(t *testing.T) {
	key, err := encryption.GenerateKey()
	require.NoError(t, err)
	projection, err := NewProjection(t.TempDir()+"/index", nil, nil, staticLegacyKeys{key: key}, nil, log.New(nil))
	require.NoError(t, err)
	t.Cleanup(func() { _ = projection.Close() })

	applyLegacyMessageWithAssets(t, projection, key, "M1", "B1", "R1", "U1", "attached release notes", time.Unix(100, 0), []string{"A1"}, 1)
	withAttachments := &searchv1.QueryRequest{
		RequiredTerms: []string{"release"}, HasAttachments: true,
		Order: searchv1.SearchOrder_SEARCH_ORDER_RELEVANCE, PageSize: 10,
	}
	response, err := projection.query(context.Background(), withAttachments)
	require.NoError(t, err)
	require.Equal(t, []string{"M1"}, hitIDs(response))

	applyLegacyBody(t, projection, key, "M1", "B2", "R1", "U1", "updated release notes", time.Unix(200, 0), nil, 3)
	response, err = projection.query(context.Background(), withAttachments)
	require.NoError(t, err)
	require.Empty(t, response.GetHits())

	response, err = projection.query(context.Background(), relevanceRequest([]string{"updated"}, nil))
	require.NoError(t, err)
	require.Equal(t, []string{"M1"}, hitIDs(response))
}

func TestProjectionDoesNotDeleteUnreadableIndex(t *testing.T) {
	directory := t.TempDir() + "/index"
	require.NoError(t, os.MkdirAll(directory, 0o755))
	metadata := []byte("not json")
	metadataPath := filepath.Join(directory, "index_meta.json")
	sentinel := []byte("operator data")
	sentinelPath := filepath.Join(directory, "do-not-delete")
	require.NoError(t, os.WriteFile(metadataPath, metadata, 0o600))
	require.NoError(t, os.WriteFile(sentinelPath, sentinel, 0o600))

	_, err := NewProjection(directory, nil, nil, nil, nil, log.New(nil))
	require.ErrorContains(t, err, "Chatto will not modify an unreadable index")
	retainedMetadata, readErr := os.ReadFile(metadataPath)
	require.NoError(t, readErr)
	require.Equal(t, metadata, retainedMetadata)
	retainedSentinel, readErr := os.ReadFile(sentinelPath)
	require.NoError(t, readErr)
	require.Equal(t, sentinel, retainedSentinel)
}

func TestProjectionCreatesIndexInExistingEmptyDirectory(t *testing.T) {
	directory := t.TempDir()
	projection, err := NewProjection(directory, []string{"de", "en"}, nil, nil, nil, log.New(nil))
	require.NoError(t, err)
	t.Cleanup(func() { _ = projection.Close() })

	_, err = os.Stat(filepath.Join(directory, "index_meta.json"))
	require.NoError(t, err)
}

func TestProjectionDoesNotResetIncompatibleCheckpoint(t *testing.T) {
	directory := t.TempDir() + "/index"
	projection, err := NewProjection(directory, nil, nil, nil, nil, log.New(nil))
	require.NoError(t, err)
	t.Cleanup(func() { _ = projection.Close() })
	sentinelPath := filepath.Join(directory, "do-not-delete")
	require.NoError(t, os.WriteFile(sentinelPath, []byte("operator data"), 0o600))

	err = projection.ResetCheckpoint(context.Background(), events.ProjectionCheckpointRequest{ProjectionKey: "message_search"})
	require.ErrorContains(t, err, "move or delete that directory")
	retained, readErr := os.ReadFile(sentinelPath)
	require.NoError(t, readErr)
	require.Equal(t, []byte("operator data"), retained)
}

func TestProjectionIgnoresMismatchedBodyRevisionID(t *testing.T) {
	key, err := encryption.GenerateKey()
	require.NoError(t, err)
	projection, err := NewProjection(t.TempDir()+"/index", nil, nil, staticLegacyKeys{key: key}, nil, log.New(nil))
	require.NoError(t, err)
	t.Cleanup(func() { _ = projection.Close() })
	applyLegacyMessage(t, projection, key, "M1", "B1", "R1", "U1", "original body", time.Unix(100, 0), 1)

	encrypted, err := encryption.Encrypt(key, []byte("poisoned revision"))
	require.NoError(t, err)
	require.NoError(t, projection.Apply(&corev1.Event{
		Id: "B2",
		Event: &corev1.Event_MessageBody{MessageBody: &corev1.MessageBodyEvent{
			RoomId: "R1", EventId: "M1", Body: &corev1.MessageBody{
				AuthorId: "U1", BodyEventId: "B1",
				EncryptedBody: encrypted.Ciphertext, EncryptionNonce: encrypted.Nonce,
			},
		}},
	}, 3))

	poisoned, err := projection.query(context.Background(), relevanceRequest([]string{"poisoned"}, nil))
	require.NoError(t, err)
	require.Empty(t, poisoned.GetHits())
	original, err := projection.query(context.Background(), relevanceRequest([]string{"original"}, nil))
	require.NoError(t, err)
	require.Equal(t, []string{"M1"}, hitIDs(original))
}

func TestProjectionDoesNotDeleteIndexForUnclassifiedOpenFailure(t *testing.T) {
	directory := t.TempDir() + "/index"
	require.NoError(t, os.MkdirAll(directory, 0o755))
	metadata := []byte(`{"storage":"scorch","index_type":"unknown"}`)
	metadataPath := filepath.Join(directory, "index_meta.json")
	require.NoError(t, os.WriteFile(metadataPath, metadata, 0o600))

	_, err := NewProjection(directory, nil, nil, nil, nil, log.New(nil))
	require.Error(t, err)
	retained, readErr := os.ReadFile(metadataPath)
	require.NoError(t, readErr)
	require.Equal(t, metadata, retained)
}

func TestProjectionRanksLiteralMatchesAboveStemmedMatches(t *testing.T) {
	key, err := encryption.GenerateKey()
	require.NoError(t, err)
	projection, err := NewProjection(t.TempDir()+"/index", nil, nil, staticLegacyKeys{key: key}, nil, log.New(nil))
	require.NoError(t, err)
	t.Cleanup(func() { _ = projection.Close() })

	applyLegacyMessage(t, projection, key, "literal", "B1", "R1", "U1", "run", time.Unix(100, 0), 1)
	applyLegacyMessage(t, projection, key, "stemmed", "B2", "R1", "U1", "running", time.Unix(200, 0), 3)

	response, err := projection.query(context.Background(), relevanceRequest([]string{"run"}, nil))
	require.NoError(t, err)
	require.Equal(t, []string{"literal", "stemmed"}, hitIDs(response))
}

func relevanceRequest(terms, phrases []string) *searchv1.QueryRequest {
	return &searchv1.QueryRequest{
		RequiredTerms: terms, RequiredPhrases: phrases,
		Order: searchv1.SearchOrder_SEARCH_ORDER_RELEVANCE, PageSize: 10,
	}
}

func TestProjectionRejectsMalformedOrForeignCursors(t *testing.T) {
	key, err := encryption.GenerateKey()
	require.NoError(t, err)
	projection, err := NewProjection(t.TempDir()+"/index", nil, nil, staticLegacyKeys{key: key}, nil, log.New(nil))
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
	projection, err := NewProjection(t.TempDir()+"/index", nil, nil, staticLegacyKeys{key: key}, nil, log.New(nil))
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
	document, err := projection.index.Document(messageDocumentID("M1"))
	require.NoError(t, err)
	require.Nil(t, document)
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
	require.NoError(t, projection.Apply(legacyBodyEvent(t, key, messageID, bodyEventID, roomID, authorID, text, createdAt, assetIDs), seq))
}

func legacyBodyEvent(t *testing.T, key []byte, messageID, bodyEventID, roomID, authorID, text string, createdAt time.Time, assetIDs []string) *corev1.Event {
	t.Helper()
	encrypted, err := encryption.Encrypt(key, []byte(text))
	require.NoError(t, err)
	body := &corev1.MessageBody{
		AuthorId: authorID, CreatedAt: timestamppb.New(createdAt), BodyEventId: bodyEventID,
		EncryptedBody: encrypted.Ciphertext, EncryptionNonce: encrypted.Nonce, AssetIds: assetIDs,
	}
	return &corev1.Event{
		Id: bodyEventID, CreatedAt: timestamppb.New(createdAt), ActorId: authorID,
		Event: &corev1.Event_MessageBody{MessageBody: &corev1.MessageBodyEvent{RoomId: roomID, EventId: messageID, Body: body}},
	}
}

func applyMessagePosted(t *testing.T, projection *Projection, messageID, roomID, authorID string, createdAt time.Time, seq uint64) {
	t.Helper()
	require.NoError(t, projection.Apply(messagePostedEvent(messageID, roomID, authorID, createdAt), seq))
}

func messagePostedEvent(messageID, roomID, authorID string, createdAt time.Time) *corev1.Event {
	return &corev1.Event{
		Id: messageID, CreatedAt: timestamppb.New(createdAt), ActorId: authorID,
		Event: &corev1.Event_MessagePosted{MessagePosted: &corev1.MessagePostedEvent{RoomId: roomID}},
	}
}

func applyV2MessageBody(t *testing.T, projection *Projection, key []byte, messageID, bodyEventID, roomID, authorID, text string, timestamp time.Time, seq uint64) {
	t.Helper()
	require.NoError(t, projection.Apply(v2MessageBodyEvent(t, key, messageID, bodyEventID, roomID, authorID, text, timestamp), seq))
}

func v2MessageBodyEvent(t *testing.T, key []byte, messageID, bodyEventID, roomID, authorID, text string, timestamp time.Time) *corev1.Event {
	t.Helper()
	encrypted, err := encryption.EncryptWithContentKey(key, []byte(text), encryption.MessageBodyAAD(messageID, bodyEventID, roomID, authorID, 1))
	require.NoError(t, err)
	body := &corev1.MessageBody{
		AuthorId: authorID, CreatedAt: timestamppb.New(timestamp), UpdatedAt: timestamppb.New(timestamp),
		EncryptionVersion: encryption.EnvelopeVersionV2, ContentKeyEpoch: 1, BodyEventId: bodyEventID,
		EncryptedBody: encrypted.Ciphertext, EncryptionNonce: encrypted.Nonce,
	}
	return &corev1.Event{
		Id: bodyEventID, CreatedAt: timestamppb.New(timestamp), ActorId: authorID,
		Event: &corev1.Event_MessageBody{MessageBody: &corev1.MessageBodyEvent{RoomId: roomID, EventId: messageID, Body: body}},
	}
}

func hitIDs(response *searchv1.QueryResponse) []string {
	ids := make([]string, 0, len(response.GetHits()))
	for _, hit := range response.GetHits() {
		ids = append(ids, hit.GetMessageId())
	}
	return ids
}
