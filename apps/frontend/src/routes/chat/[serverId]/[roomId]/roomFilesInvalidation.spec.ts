import { describe, expect, it } from 'vitest';
import { Message, MessageAttachment } from '@chatto/api-types/api/v1/message_types_pb';
import {
  RoomMessagePosted,
  RoomTimelineEvent
} from '@chatto/api-types/api/v1/room_timeline_pb';
import {
  RealtimeProjectionEvent,
  RealtimeProjectionOperation,
  RealtimeProjectionReactionChange,
  RealtimeProjectionRoomTimelineEventUpsert,
  RealtimeProjectionThreadViewerStatesReplace
} from '@chatto/api-types/realtime/v1/realtime_pb';
import { projectionEventInvalidatesRoomFiles } from './roomFilesInvalidation';

function messageUpsert(
  eventId: string,
  roomId = 'room-1',
  attachments: MessageAttachment[] = [],
  reaction = false
): RealtimeProjectionOperation {
  return new RealtimeProjectionOperation({
    operation: {
      case: 'roomTimelineEventUpsert',
      value: new RealtimeProjectionRoomTimelineEventUpsert({
        roomId,
        event: new RoomTimelineEvent({
          id: eventId,
          event: {
            case: 'messagePosted',
            value: new RoomMessagePosted({ message: new Message({ attachments }) })
          }
        }),
        reactionChange: reaction ? new RealtimeProjectionReactionChange() : undefined
      })
    }
  });
}

function invalidates(event: RealtimeProjectionEvent): boolean {
  return projectionEventInvalidatesRoomFiles(event, 'room-1');
}

describe('projectionEventInvalidatesRoomFiles', () => {
  it('invalidates for a newly posted attachment', () => {
    expect(
      invalidates(
        new RealtimeProjectionEvent({
          id: 'message-1',
          operations: [messageUpsert('message-1', 'room-1', [new MessageAttachment({ id: 'a' })])]
        })
      )
    ).toBe(true);
  });

  it('invalidates message edits that can change attachments', () => {
    expect(
      invalidates(
        new RealtimeProjectionEvent({ id: 'edit-1', operations: [messageUpsert('message-1')] })
      )
    ).toBe(true);
  });

  it('ignores attachment-free posts and their secondary thread-root summary', () => {
    expect(
      invalidates(
        new RealtimeProjectionEvent({
          id: 'reply-1',
          operations: [messageUpsert('reply-1'), messageUpsert('root-1')]
        })
      )
    ).toBe(false);
  });

  it('ignores reactions, including secondary channel-echo upserts', () => {
    expect(
      invalidates(
        new RealtimeProjectionEvent({
          id: 'reaction-1',
          operations: [
            messageUpsert('message-1', 'room-1', [new MessageAttachment({ id: 'a' })], true),
            messageUpsert('echo-1', 'room-1', [new MessageAttachment({ id: 'a' })])
          ]
        })
      )
    ).toBe(false);
  });

  it('ignores thread-viewer-state replacements', () => {
    expect(
      invalidates(
        new RealtimeProjectionEvent({
          id: 'viewer-state-1',
          operations: [
            messageUpsert('message-1'),
            new RealtimeProjectionOperation({
              operation: {
                case: 'threadViewerStatesReplace',
                value: new RealtimeProjectionThreadViewerStatesReplace()
              }
            })
          ]
        })
      )
    ).toBe(false);
  });

  it('ignores other rooms', () => {
    expect(
      invalidates(
        new RealtimeProjectionEvent({
          id: 'message-1',
          operations: [
            messageUpsert('message-1', 'room-2', [new MessageAttachment({ id: 'a' })])
          ]
        })
      )
    ).toBe(false);
  });
});
