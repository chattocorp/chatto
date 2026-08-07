import { describe, expect, it, vi } from 'vitest';
import type { MessageActionParams, MessageActions } from '$lib/hooks';
import { buildMessageActionModel } from './messageActionModel';

function createActions(): MessageActions {
  return {
    addReaction: vi.fn(),
    removeReaction: vi.fn(),
    toggleReaction: vi.fn(),
    startEdit: vi.fn(),
    openDeleteConfirmation: vi.fn(),
    copyMessageText: vi.fn(),
    copyMessageLink: vi.fn()
  };
}

const params: MessageActionParams = {
  serverId: 'remote-server',
  roomId: 'room-1',
  messageEventId: 'message-1',
  eventId: 'original-thread-message',
  deleteEventId: 'channel-echo',
  messageBody: 'Hello',
  permalinkThreadRootEventId: 'thread-root',
  threadRootEventId: 'thread-root',
  channelEchoEventId: 'channel-echo',
  canAddChannelEcho: true
};

function buildModel(actions = createActions()) {
  return {
    actions,
    model: buildMessageActionModel({
      actions,
      params,
      reactions: [
        { emoji: 'thumbsup', hasReacted: true },
        { emoji: 'heart', hasReacted: false }
      ],
      canReact: true,
      canEdit: true,
      canDelete: true,
      replyInRoomLabel: 'Reply in thread',
      replyThreadLabel: 'Open thread'
    })
  };
}

describe('buildMessageActionModel', () => {
  it('binds all action surfaces to the same server-scoped message target', async () => {
    const { actions, model } = buildModel();

    model.edit();
    await model.copyText();
    await model.copyLink();
    model.delete();

    expect(actions.startEdit).toHaveBeenCalledWith(params);
    expect(actions.copyMessageText).toHaveBeenCalledWith(params);
    expect(actions.copyMessageLink).toHaveBeenCalledWith(params);
    expect(actions.openDeleteConfirmation).toHaveBeenCalledWith(params);
    expect(model.serverId).toBe('remote-server');
    expect(model.messageBody).toBe('Hello');
  });

  it('normalizes API reaction names before toggling picker and quick reactions', async () => {
    const { actions, model } = buildModel();

    expect(model.hasReacted('👍')).toBe(true);
    expect(model.hasReacted('thumbsup')).toBe(true);
    expect(model.hasReacted('❤️')).toBe(false);

    await model.toggleReaction('👍');
    await model.toggleReaction('❤️');

    expect(actions.toggleReaction).toHaveBeenNthCalledWith(1, params, '👍', true);
    expect(actions.toggleReaction).toHaveBeenNthCalledWith(2, params, '❤️', false);
  });
});
