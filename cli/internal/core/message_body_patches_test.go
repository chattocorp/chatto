package core

import (
	"bytes"
	"context"
	"errors"
	"testing"

	"github.com/nats-io/nats.go/jetstream"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/encoding/protowire"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/fieldmaskpb"
	"hmans.de/chatto/internal/encryption"
	"hmans.de/chatto/internal/evtstream"
	"hmans.de/chatto/internal/kms"
	evtv1 "hmans.de/chatto/internal/pb/chatto/core/evt/v1"
	"hmans.de/chatto/pkg/events"
)

// These tests use Chatto's real commands, JetStream atomic batches, encryption,
// projection, hydration, cleanup, and snapshots. No replacement test log is used.
func TestMessageBodyPatchesSurviveCleanupReplayAndSnapshots(t *testing.T) {
	for _, kind := range []RoomKind{KindChannel, KindDM} {
		t.Run(string(kind), func(t *testing.T) {
			c, _ := setupTestCore(t)
			ctx := testContext(t)
			author, err := c.CreateUser(ctx, SystemActorID, "patch-author", "Patch Author", "password123")
			require.NoError(t, err)
			var room *evtv1.Room
			if kind == KindDM {
				other, err := c.CreateUser(ctx, SystemActorID, "patch-other", "Patch Other", "password123")
				require.NoError(t, err)
				room, _, err = c.RoomCommands().StartDM(ctx, RoomStartDMInput{ActorID: author.Id, ParticipantIDs: []string{other.Id}})
				require.NoError(t, err)
			} else {
				room, err = c.CreateRoom(ctx, author.Id, kind, "", "patches", "")
				require.NoError(t, err)
				_, err = c.JoinRoom(ctx, author.Id, kind, author.Id, room.Id)
				require.NoError(t, err)
			}
			asset, err := c.UploadAttachment(ctx, author.Id, room.Id, "patch.png", "image/png", bytes.NewReader(createTestPNG(20, 20)))
			require.NoError(t, err)
			second, err := c.UploadAttachment(ctx, author.Id, room.Id, "second.png", "image/png", bytes.NewReader(createTestPNG(20, 20)))
			require.NoError(t, err)
			post, err := c.Messages().PostMessage(ctx, MessagePostInput{ActorID: author.Id, RoomID: room.Id, Body: "Original text", AttachmentAssetIDs: []string{asset.Id, second.Id}, AttachmentDescriptions: []MessageAttachmentDescriptionInput{{AssetID: asset.Id, Description: "Original description"}, {AssetID: second.Id, Description: "Unchanged description"}}})
			require.NoError(t, err)
			id := post.Event.Id
			original, err := c.currentMessageBody(ctx, id)
			require.NoError(t, err)
			_, originalSeq, ok := c.roomModel.bodyEventSeqs(id)
			require.True(t, ok)
			require.NoError(t, c.EditMessage(ctx, author.Id, kind, room.Id, id, "Edited text"))
			patches, _, err := c.EventPublisher.SubjectEvents(ctx, evtstream.RoomAggregate(room.Id).Subject(evtstream.EventMessageBody))
			require.NoError(t, err)
			require.Len(t, patches, 2)
			require.Nil(t, patches[0].GetMessageBody().GetUpdateMask(), "new posts remain complete bodies")
			patch := patches[1].GetMessageBody()
			require.Contains(t, patch.GetUpdateMask().GetPaths(), "encrypted_body")
			require.NotContains(t, patch.GetUpdateMask().GetPaths(), "asset_ids")
			require.Empty(t, patch.GetBody().GetAssetIds())
			require.Empty(t, patch.GetBody().GetAttachmentDescriptions())
			_, err = c.storage.serverEvtStream.GetMsg(ctx, originalSeq)
			require.NoError(t, err, "original payload still supplies attachments and description")
			afterText, err := c.currentMessageBody(ctx, id)
			require.NoError(t, err)
			require.Equal(t, original.GetAttachmentDescriptions()[0].GetEncryptedDescription(), afterText.GetAttachmentDescriptions()[0].GetEncryptedDescription())
			assertPatchReplay(t, ctx, c, id, "Edited text", map[string]string{asset.Id: "Original description", second.Id: "Unchanged description"})

			require.NoError(t, c.SetAttachmentDescription(ctx, author.Id, kind, room.Id, id, asset.Id, "New description"))
			afterDescription, err := c.currentMessageBody(ctx, id)
			require.NoError(t, err)
			require.Equal(t, afterText.GetEncryptedBody(), afterDescription.GetEncryptedBody(), "description updates preserve text ciphertext")
			require.Equal(t, afterText.GetBodyEventId(), afterDescription.GetBodyEventId(), "text keeps its original encryption context")
			for _, description := range afterDescription.GetAttachmentDescriptions() {
				if description.GetAssetId() == second.Id {
					for _, old := range original.GetAttachmentDescriptions() {
						if old.GetAssetId() == second.Id {
							require.Equal(t, old.GetEncryptedDescription(), description.GetEncryptedDescription())
							require.Equal(t, original.GetBodyEventId(), description.GetSourceBodyEventId())
						}
					}
				}
			}
			assertPatchReplay(t, ctx, c, id, "Edited text", map[string]string{asset.Id: "New description", second.Id: "Unchanged description"})

			require.NoError(t, c.SetAttachmentDescription(ctx, author.Id, kind, room.Id, id, asset.Id, ""))
			assertPatchReplay(t, ctx, c, id, "Edited text", map[string]string{second.Id: "Unchanged description"})
			require.NoError(t, c.DeleteAttachmentFromMessage(ctx, author.Id, kind, room.Id, id, asset.Id))
			_, err = c.storage.serverEvtStream.GetMsg(ctx, originalSeq)
			require.ErrorIs(t, err, jetstream.ErrMsgNotFound, "original payload is erased after its final live field is removed")
			assertPatchReplay(t, ctx, c, id, "Edited text", map[string]string{second.Id: "Unchanged description"})
			require.NoError(t, c.DeleteAttachmentFromMessage(ctx, author.Id, kind, room.Id, id, second.Id))
			assertPatchReplay(t, ctx, c, id, "Edited text", map[string]string{})
			history, _, _ := c.roomModel.bodyEventSeqs(id)
			require.NoError(t, c.DeleteMessage(ctx, author.Id, kind, room.Id, id))
			for _, seq := range history {
				_, err := c.storage.serverEvtStream.GetMsg(ctx, seq)
				require.ErrorIs(t, err, jetstream.ErrMsgNotFound)
			}
			cold := replayPatchTimeline(t, ctx, c)
			_, deleted, known := cold.LatestBodyReference(id)
			require.True(t, known)
			require.True(t, deleted)
		})
	}
}

func replayPatchTimeline(t *testing.T, ctx context.Context, c *ChattoCore) *RoomTimelineProjection {
	t.Helper()
	info, err := c.storage.serverEvtStream.Info(ctx)
	require.NoError(t, err)
	p := NewRoomTimelineProjection()
	for seq := info.State.FirstSeq; seq <= info.State.LastSeq; seq++ {
		record, err := c.eventReader.EventAt(ctx, seq)
		if errors.Is(err, jetstream.ErrMsgNotFound) {
			continue
		}
		require.NoError(t, err)
		require.NoError(t, p.Apply(record.Event, seq))
	}
	return p
}

func assertPatchReplay(t *testing.T, ctx context.Context, c *ChattoCore, id, text string, descriptions map[string]string) {
	t.Helper()
	live := c.roomModel.timeline.Projection()
	cold := replayPatchTimeline(t, ctx, c)
	data, err := live.Snapshot()
	require.NoError(t, err)
	require.NotContains(t, string(data), text)
	for _, description := range descriptions {
		require.NotContains(t, string(data), description)
	}
	restored := NewRoomTimelineProjection()
	require.NoError(t, restored.Restore(data))
	liveRef, _, _ := live.LatestBodyReference(id)
	for _, p := range []*RoomTimelineProjection{cold, restored} {
		ref, deleted, known := p.LatestBodyReference(id)
		require.True(t, known)
		require.False(t, deleted)
		require.Equal(t, liveRef, ref)
		body, err := c.timelineHydrator.body(ctx, ref)
		require.NoError(t, err)
		plaintext, err := c.decryptMessageBody(ctx, id, ref.RoomID, body)
		require.NoError(t, err)
		require.Equal(t, text, string(plaintext))
		actual, err := c.decryptAttachmentDescriptions(ctx, id, ref.RoomID, body)
		require.NoError(t, err)
		require.Equal(t, descriptions, actual)
	}
}

func TestMessageBodyMaskValidationIsAtomic(t *testing.T) {
	p := NewRoomTimelineProjection()
	valid := &evtv1.MessageBodyEvent{RoomId: "R", EventId: "M", Body: &evtv1.MessageBody{AuthorId: "U", BodyEventId: "P"}, UpdateMask: (evtstream.MessageBodyFields{Text: true}).Mask()}
	for _, mutate := range []func(*evtv1.MessageBodyEvent){
		func(s *evtv1.MessageBodyEvent) {
			s.UpdateMask = &fieldmaskpb.FieldMask{Paths: []string{"future_field"}}
		},
		func(s *evtv1.MessageBodyEvent) {
			s.UpdateMask = &fieldmaskpb.FieldMask{Paths: []string{"encrypted_body"}}
		},
		func(s *evtv1.MessageBodyEvent) {
			s.Body.ProtoReflect().SetUnknown(protowire.AppendVarint(protowire.AppendTag(nil, 100, protowire.VarintType), 1))
		},
	} {
		bad := proto.Clone(valid).(*evtv1.MessageBodyEvent)
		mutate(bad)
		event := &evtv1.Event{Id: "P", Event: &evtv1.Event_MessageBody{MessageBody: bad}}
		require.ErrorIs(t, p.Apply(event, 3), ErrMessageBodyCorrupt)
		require.Empty(t, p.bodyStates)
	}
	// A rejected event must not poison the replay guard for a corrected retry.
	require.NoError(t, p.Apply(&evtv1.Event{Id: "P", Event: &evtv1.Event_MessageBody{MessageBody: valid}}, 3))
	require.Equal(t, [4]uint64{3, 0, 0, 0}, p.bodyStates["M"].fieldSequences)
}

func TestMessageBodyPatchRejectsUnsupportedLegacyFields(t *testing.T) {
	body := &evtv1.MessageBody{}
	body.ProtoReflect().SetUnknown(protowire.AppendVarint(protowire.AppendTag(nil, 100, protowire.VarintType), 1))
	// A writer must stop before it omits content from a newer writer.
	_, err := (&ChattoCore{}).buildMessageBodyPatch(context.Background(), "R", "M", "P", body, body, "", nil, nil)
	require.ErrorIs(t, err, ErrMessageBodyCorrupt)
}

func TestMessageBodyClearsSurviveLaterUpdatesAndPartialCleanup(t *testing.T) {
	original := bodyEventWithAssets("B1", "M", "R", "U", "old ciphertext", []string{"A"}, 1)
	original.GetMessageBody().Body.LinkPreview = &evtv1.LinkPreview{Url: "https://example.test"}
	original.GetMessageBody().Body.AttachmentDescriptions = []*evtv1.EncryptedAttachmentDescription{{AssetId: "A", EncryptedDescription: []byte("old description")}}
	post := bodylessPostedEvent("M", "R", "U", 2)
	clear := bodyEventWithAssets("B2", "M", "R", "U", "", nil, 3)
	clear.GetMessageBody().UpdateMask = (evtstream.MessageBodyFields{Attachments: true, LinkPreview: true, Descriptions: true}).Mask()
	text := bodyEventWithAssets("B3", "M", "R", "U", "new ciphertext", []string{"ignored"}, 4)
	text.GetMessageBody().UpdateMask = (evtstream.MessageBodyFields{Text: true}).Mask()
	events := []*evtv1.Event{original, post, clear, text}
	live := NewRoomTimelineProjection()
	for i, event := range events[:3] {
		require.NoError(t, live.Apply(event, uint64(i+1)))
	}
	snapshot, err := live.Snapshot()
	require.NoError(t, err)
	require.NoError(t, live.Apply(text, 4))
	require.Equal(t, []uint64{1}, live.ObsoleteBodyEventSeqs("M"), "the clear remains live after a later text update")
	for _, eraseOld := range []bool{false, true} {
		// Both successful cleanup and failure to erase the obsolete old body
		// must produce the same values. Also cover snapshot plus tail replay.
		reader := testTimelineEventReader(events)
		cold := NewRoomTimelineProjection()
		for i, event := range events {
			if eraseOld && i == 0 {
				delete(reader.records, 1)
				continue
			}
			require.NoError(t, cold.Apply(event, uint64(i+1)))
		}
		restored := NewRoomTimelineProjection()
		require.NoError(t, restored.Restore(snapshot))
		require.NoError(t, restored.Apply(text, 4))
		for _, projection := range []*RoomTimelineProjection{cold, restored} {
			ref, retracted, known := projection.LatestBodyReference("M")
			require.True(t, known)
			require.False(t, retracted)
			body, err := newRoomTimelineHydrator(reader).body(context.Background(), ref)
			require.NoError(t, err)
			require.Equal(t, "new ciphertext", string(body.GetEncryptedBody()))
			require.Empty(t, body.GetAssetIds())
			require.Empty(t, body.GetAttachmentDescriptions())
			require.Nil(t, body.GetLinkPreview())
		}
	}
}

func TestMessageBodyPatchEditsLegacyEncryptedBodies(t *testing.T) {
	for _, version := range []string{"legacy-per-user-key", "v2-envelope"} {
		t.Run(version, func(t *testing.T) {
			c, _ := setupTestCore(t)
			ctx := testContext(t)
			author, err := c.CreateUser(ctx, SystemActorID, "legacy-editor", "Legacy Editor", "password123")
			require.NoError(t, err)
			room, err := c.CreateRoom(ctx, author.Id, KindChannel, "", "legacy-edits", "")
			require.NoError(t, err)
			_, err = c.JoinRoom(ctx, author.Id, KindChannel, author.Id, room.Id)
			require.NoError(t, err)
			asset, err := c.UploadAttachment(ctx, author.Id, room.Id, "legacy.png", "image/png", bytes.NewReader(createTestPNG(20, 20)))
			require.NoError(t, err)
			post, err := c.PostMessage(ctx, KindChannel, room.Id, author.Id, "old text", []string{asset.Id}, "", "", &evtv1.LinkPreview{Url: "https://example.test"}, false)
			require.NoError(t, err)
			body, err := c.currentMessageBody(ctx, post.Id)
			require.NoError(t, err)
			body.Attachments = c.mediaModel.MessageBodyAttachments(body)
			body.AssetIds = nil
			legacyID := NewEventID()
			if version == "legacy-per-user-key" {
				key, err := encryption.GenerateKey()
				require.NoError(t, err)
				_, err = c.storage.encryptionKV.Create(ctx, kms.LegacyUserKeyRef(author.Id), key)
				require.NoError(t, err)
				sealed, err := encryption.Encrypt(key, []byte("old text"))
				require.NoError(t, err)
				body.EncryptionVersion, body.ContentKeyEpoch, body.BodyEventId = 0, 0, ""
				body.EncryptedBody, body.EncryptionNonce = sealed.Ciphertext, sealed.Nonce
			} else {
				require.NoError(t, c.encryptMessageContent(ctx, body, room.Id, post.Id, post.Id, legacyID, "old text", nil))
			}
			// Both layouts are historical complete bodies: no mask, no description
			// source metadata, and embedded attachments instead of asset IDs.
			fact := newEvent(author.Id, &evtv1.Event{Id: legacyID, Event: &evtv1.Event_MessageBody{MessageBody: &evtv1.MessageBodyEvent{RoomId: room.Id, EventId: post.Id, Body: body}}})
			subject := evtstream.RoomAggregate(room.Id).Subject(evtstream.EventMessageBody)
			filter := evtstream.RoomAggregate(room.Id).AllEventsFilter()
			tail, err := c.EventPublisher.LastSubjectSeq(ctx, filter)
			require.NoError(t, err)
			seqs, err := c.EventPublisher.AppendBatch(ctx, []evtstream.BatchEntry{{Subject: subject, Event: fact, HasOCC: true, ExpectedSeq: tail, FilterSubject: filter}})
			require.NoError(t, err)
			require.NoError(t, c.roomModel.waitForTimeline(ctx, events.SubjectPosition(subject, seqs[0])))
			assertPatchReplay(t, ctx, c, post.Id, "old text", map[string]string{})
			require.NoError(t, c.DeleteLinkPreviewFromMessage(ctx, author.Id, KindChannel, room.Id, post.Id, "https://example.test"))
			updated, err := c.currentMessageBody(ctx, post.Id)
			require.NoError(t, err)
			require.Equal(t, body.GetEncryptedBody(), updated.GetEncryptedBody(), "metadata edit preserves historical ciphertext")
			require.Len(t, updated.GetAttachments(), 1)
			require.Nil(t, updated.GetLinkPreview())
			assertPatchReplay(t, ctx, c, post.Id, "old text", map[string]string{})
			require.NoError(t, c.EditMessage(ctx, author.Id, KindChannel, room.Id, post.Id, "new text"))
			assertPatchReplay(t, ctx, c, post.Id, "new text", map[string]string{})
			require.NoError(t, c.DeleteAttachmentFromMessage(ctx, author.Id, KindChannel, room.Id, post.Id, asset.Id))
			_, err = c.storage.serverEvtStream.GetMsg(ctx, seqs[0])
			require.ErrorIs(t, err, jetstream.ErrMsgNotFound)
			assertPatchReplay(t, ctx, c, post.Id, "new text", map[string]string{})
		})
	}
}
