import { Code, ConnectError } from '@connectrpc/connect';
import { beforeEach, describe, expect, it, vi } from 'vitest';

const mocks = vi.hoisted(() => ({
  addReaction: vi.fn(),
  applyServerReaction: vi.fn(),
  beginOptimisticReaction: vi.fn(),
  rollback: vi.fn(),
  toastError: vi.fn()
}));

vi.mock('$lib/state/server/scope.svelte', () => ({
  useServerScope: () => ({
    connection: { getAPI: () => ({ addReaction: mocks.addReaction }) },
    isCurrent: () => true
  })
}));

vi.mock('$lib/ui/toast', () => ({
  toast: { error: mocks.toastError, success: vi.fn() }
}));

vi.mock('$lib/i18n/messages', () => ({
  m: (key: string) =>
    ({
      'common.copied_to_clipboard': 'Copied',
      'room.message.actions.copy_text_failed': 'Copy failed',
      'room.message.reaction_failed': 'Failed to update reaction',
      'room.message.reaction_limit_reached': 'You can add up to 20 reactions to a message.'
    })[key] ?? key
}));

vi.mock('$lib/state/room/composerContext.svelte', () => ({
  ComposerContext: class {},
  EditState: class {},
  JumpToMessageState: class {},
  LastEditableMessageContext: class {},
  QuoteInsertionState: class {},
  ReplyState: class {},
  ScrollState: class {},
  createComposerContext: vi.fn(),
  getComposerContext: vi.fn(),
  setComposerContext: vi.fn()
}));

import { useReactionActions, type MessageActionParams } from './useMessageActions.svelte';

const params: MessageActionParams = {
  serverId: 'server-1',
  roomId: 'room-1',
  messageEventId: 'message-1',
  eventId: 'message-1',
  messageBody: 'Hello',
  messageStore: {
    beginOptimisticReaction: mocks.beginOptimisticReaction
  } as unknown as MessageActionParams['messageStore']
};

describe('useReactionActions', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mocks.beginOptimisticReaction.mockReturnValue({
      applyServerReaction: mocks.applyServerReaction,
      rollback: mocks.rollback
    });
  });

  it('rolls back and shows the reaction-limit message for resource exhaustion', async () => {
    mocks.addReaction.mockRejectedValue(
      new ConnectError('reaction limit reached', Code.ResourceExhausted)
    );

    await useReactionActions().addReaction(params, 'heart');

    expect(mocks.rollback).toHaveBeenCalledOnce();
    expect(mocks.toastError).toHaveBeenCalledWith(
      'You can add up to 20 reactions to a message.'
    );
  });

  it('rolls back and keeps the generic message for other failures', async () => {
    mocks.addReaction.mockRejectedValue(new ConnectError('unavailable', Code.Unavailable));

    await useReactionActions().addReaction(params, 'heart');

    expect(mocks.rollback).toHaveBeenCalledOnce();
    expect(mocks.toastError).toHaveBeenCalledWith('Failed to update reaction');
  });
});
