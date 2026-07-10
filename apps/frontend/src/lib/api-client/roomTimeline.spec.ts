import { Timestamp } from '@bufbuild/protobuf';
import { Message } from '@chatto/api-types/api/v1/message_types_pb';
import { describe, expect, it } from 'vitest';
import { messagePostedPayload } from './roomTimeline';

describe('messagePostedPayload', () => {
  it('maps deleted_at to the exact ISO timestamp', () => {
    const deletedAt = Timestamp.fromDate(new Date('2026-07-10T10:11:12.345Z'));

    expect(messagePostedPayload(new Message({ deletedAt }), {}).deletedAt).toBe(
      '2026-07-10T10:11:12.345Z'
    );
  });

  it('keeps deletedAt null when the server omits the metadata', () => {
    expect(messagePostedPayload(new Message(), {}).deletedAt).toBeNull();
  });
});
