package core

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"sync"
	"testing"
	"time"

	notificationv1 "github.com/chattocorp/chatto/cli/internal/pb/chatto/core/notification/v1"
	projectionv1 "github.com/chattocorp/chatto/cli/internal/pb/chatto/core/projection/v1"
	"github.com/chattocorp/chatto/cli/internal/testutil"
	"github.com/nats-io/nats.go/jetstream"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protodesc"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/descriptorpb"
	"google.golang.org/protobuf/types/dynamicpb"
)

func TestNotificationReadRetentionUsesSeparateSnapshotContract(t *testing.T) {
	t.Parallel()
	// Rebuild the pre-change schema by removing only the derived read_at field.
	// This verifies that old and new replicas cannot exchange snapshots that
	// would silently lose the first-read time. Lifecycle event schemas stay equal.
	root := (&projectionv1.NotificationProjectionSnapshot{}).ProtoReflect().Descriptor()
	files := &descriptorpb.FileDescriptorSet{}
	seen := make(map[string]bool)
	var collect func(protoreflect.FileDescriptor)
	collect = func(file protoreflect.FileDescriptor) {
		if seen[file.Path()] {
			return
		}
		seen[file.Path()] = true
		definition := protodesc.ToFileDescriptorProto(file)
		if file.Package() == "chatto.core.notification.v1" {
			for _, message := range definition.MessageType {
				if message.GetName() == "NotificationOccurrence" {
					message.Field = slices.DeleteFunc(message.Field, func(field *descriptorpb.FieldDescriptorProto) bool {
						return field.GetName() == "read_at"
					})
				}
			}
		}
		files.File = append(files.File, definition)
		for i := 0; i < file.Imports().Len(); i++ {
			collect(file.Imports().Get(i).FileDescriptor)
		}
	}
	collect(root.ParentFile())
	legacy, err := protodesc.NewFiles(files)
	require.NoError(t, err)
	legacyRoot, err := legacy.FindDescriptorByName(root.FullName())
	require.NoError(t, err)
	require.NotEqual(t, notificationSnapshotContractID,
		snapshotContractID("v2", dynamicpb.NewMessage(legacyRoot.(protoreflect.MessageDescriptor))))
	event := (&notificationv1.NotificationEvent{}).ProtoReflect().Descriptor()
	legacyEvent, err := legacy.FindDescriptorByName(event.FullName())
	require.NoError(t, err)
	require.Equal(t, snapshotSchemaFingerprint(event), snapshotSchemaFingerprint(legacyEvent.(protoreflect.MessageDescriptor)),
		"all reachable persisted lifecycle fields must remain compatible with beta servers")
}

func TestNotificationReadRetentionReplayAndSnapshot(t *testing.T) {
	t.Parallel()
	now := time.Now().UTC().Truncate(time.Second)
	for _, initiallyRead := range []bool{false, true} {
		t.Run(fmt.Sprintf("initially_read_%t", initiallyRead), func(t *testing.T) {
			p := NewNotificationProjection()
			p.now = func() time.Time { return now }
			occurrence := &notificationv1.NotificationOccurrence{
				Id: "N1", RecipientId: "U1", SourceEventId: "E1",
				SourceCreatedAt: timestamp(now.Add(-30 * 24 * time.Hour)),
				Signal:          testNotificationSignal(notificationTestSignalReply, "R1", "E1"),
				ExpiresAt:       timestamp(now.Add(60 * 24 * time.Hour)), Read: initiallyRead,
			}
			// Both facts use the existing beta wire shape. No new event field is
			// needed to recover read time when upgrading or cold-replaying.
			signal := newNotificationSignalledLifecycleEvent(now, occurrence)
			require.NoError(t, p.Apply(signal, 1))
			read := &notificationv1.NotificationEvent{
				Id: "read", RecipientId: "U1", NotificationId: "N1",
				OccurredAt: timestamp(now), ExpiresAt: occurrence.ExpiresAt,
				Event: &notificationv1.NotificationEvent_Read{Read: &notificationv1.NotificationRead{}},
			}
			if !initiallyRead {
				require.Empty(t, p.readOccurrencesDueForDeletion(now.Add(notificationReadRetention)))
				require.NoError(t, p.Apply(read, 2))
			}
			// A repeated read and duplicate signal cannot postpone cleanup.
			read.OccurredAt = timestamp(now.Add(time.Hour))
			require.NoError(t, p.Apply(read, 3))
			signal.OccurredAt = timestamp(now.Add(2 * time.Hour))
			require.NoError(t, p.Apply(signal, 4))
			got, ok := p.occurrence("U1", "N1", now)
			require.True(t, ok)
			require.Equal(t, now, got.GetReadAt().AsTime())
			require.True(t, proto.Equal(occurrence.ExpiresAt, got.ExpiresAt), "read must not change source expiry")

			data, err := p.Snapshot()
			require.NoError(t, err)
			restored := NewNotificationProjection()
			restored.now = p.now
			require.NoError(t, restored.Restore(data))
			for _, projection := range []*NotificationProjection{p, restored} {
				deadline := now.Add(notificationReadRetention)
				require.Empty(t, projection.readOccurrencesDueForDeletion(deadline.Add(-time.Nanosecond)))
				due := projection.readOccurrencesDueForDeletion(deadline)
				require.Len(t, due, 1)
				require.Equal(t, "N1", due[0].GetId())
				// Candidate selection does not remove the row before the durable
				// removal succeeds, and returns a detached copy.
				due[0].Id = "changed"
				_, visible := projection.occurrence("U1", "N1", deadline)
				require.True(t, visible)
				require.Empty(t, projection.readOccurrencesDueForDeletion(occurrence.ExpiresAt.AsTime()))
			}
		})
	}
}

func TestNotificationReadRetentionDoesNotExtendSourceExpiry(t *testing.T) {
	t.Parallel()
	now := time.Now().UTC().Truncate(time.Second)
	p := NewNotificationProjection()
	p.now = func() time.Time { return now }
	occurrence := &notificationv1.NotificationOccurrence{
		Id: "N1", RecipientId: "U1", SourceEventId: "E1", Read: true,
		SourceCreatedAt: timestamp(now.Add(-89 * 24 * time.Hour)),
		Signal:          testNotificationSignal(notificationTestSignalReply, "R1", "E1"),
		ExpiresAt:       timestamp(now.Add(24 * time.Hour)),
	}
	require.NoError(t, p.Apply(newNotificationSignalledLifecycleEvent(now, occurrence), 1))
	require.Empty(t, p.readOccurrencesDueForDeletion(now.Add(24*time.Hour)))
	_, visible := p.occurrence("U1", "N1", now.Add(24*time.Hour))
	require.False(t, visible)
}

// failNotificationSignalDelete exercises recovery without changing the stream
// handle used by any running worker.
type failNotificationSignalDelete struct{ jetstream.Stream }

func (s failNotificationSignalDelete) SecureDeleteMsg(context.Context, uint64) error {
	return errors.New("injected signal delete failure")
}

func TestNotificationReadRetentionReplicasAndPhysicalCleanupRetry(t *testing.T) {
	t.Parallel()
	ns, _ := testutil.StartNATS(t)
	first := startReplica(t, ns, testCoreConfig())
	second := startReplica(t, ns, testCoreConfig())
	ctx := testContext(t)
	now := time.Now().UTC().Truncate(time.Second)
	expires := now.Add(notificationTTL)
	sequences := make(map[string]uint64)
	for _, id := range []string{"due", "recent", "unread"} {
		occurrence := &notificationv1.NotificationOccurrence{
			Id: id, RecipientId: "U1", SourceEventId: "E-" + id,
			SourceCreatedAt: timestamp(now), ExpiresAt: timestamp(expires),
			Signal: testNotificationSignal(notificationTestSignalReply, "R1", "E-"+id),
		}
		pos, err := first.notificationOccurrences.publisher.AppendEventually(ctx, newNotificationSignalledLifecycleEvent(now, occurrence))
		require.NoError(t, err)
		sequences[id] = pos.Seq
		if id != "unread" {
			readAt := now
			if id == "recent" {
				readAt = now.Add(time.Hour)
			}
			_, err = first.notificationOccurrences.publisher.AppendEventually(ctx, &notificationv1.NotificationEvent{
				Id: "read-" + id, RecipientId: "U1", NotificationId: id,
				OccurredAt: timestamp(readAt), ExpiresAt: timestamp(expires),
				Event: &notificationv1.NotificationEvent_Read{Read: &notificationv1.NotificationRead{}},
			})
			require.NoError(t, err)
		}
	}
	for _, core := range []*ChattoCore{first, second} {
		require.NoError(t, core.NotificationOccurrences().WaitCurrent(ctx))
	}
	// Failure before the removal commits must leave the row available for retry.
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	require.Error(t, first.NotificationOccurrences().maintain(cancelled, now.Add(notificationReadRetention)))
	_, err := first.NotificationOccurrences().Get(ctx, "U1", "due")
	require.NoError(t, err)
	// Separate models share their replica's projection but own their cleanup
	// cache and fault seam. Running workers never see a test-mutated clock or
	// handle. Simulated time makes only "due" eligible in these passes.
	models := make([]*NotificationOccurrenceModel, 0, 2)
	for _, core := range []*ChattoCore{first, second} {
		original := core.NotificationOccurrences()
		models = append(models, NewNotificationOccurrenceModel(core, original.projection, original.publisher,
			failNotificationSignalDelete{original.stream}, original.kv, original.logger))
	}
	var wg sync.WaitGroup
	errs := make(chan error, len(models))
	for _, model := range models {
		wg.Go(func() { errs <- model.maintain(ctx, now.Add(notificationReadRetention)) })
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		require.NoError(t, err, "physical cleanup failure must not undo logical deletion")
	}
	for _, core := range []*ChattoCore{first, second} {
		require.NoError(t, core.NotificationOccurrences().WaitCurrent(ctx))
		_, err := core.NotificationOccurrences().Get(ctx, "U1", "due")
		require.ErrorIs(t, err, ErrNotFound)
		for _, id := range []string{"recent", "unread"} {
			_, err := core.NotificationOccurrences().Get(ctx, "U1", id)
			require.NoError(t, err)
			_, err = core.storage.notificationStream.GetMsg(ctx, sequences[id])
			require.NoError(t, err)
		}
	}
	// A fresh replica cold-replays the existing removal variant and retries
	// physical cleanup during startup, even though the signal is now absent
	// from application-visible state.
	restarted := startReplica(t, ns, testCoreConfig())
	require.EventuallyWithT(t, func(c *assert.CollectT) {
		_, err := restarted.storage.notificationStream.GetMsg(ctx, sequences["due"])
		assert.True(c, notificationSignalAlreadyAbsent(err), "signal cleanup error: %v", err)
	}, 5*time.Second, 10*time.Millisecond)
	_, err = restarted.NotificationOccurrences().Get(ctx, "U1", "due")
	require.ErrorIs(t, err, ErrNotFound)
}
