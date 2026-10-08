import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { getToasts, toast } from '$lib/ui/toast';
import { ComposerSubmissionState, type PreparedPost, uploadPercentage } from './submission.svelte';
import type { MentionRolesStatus } from '@chatto/client/server/mentionRoles';
import { StaleResponseError } from '@chatto/client/api/connect';
import {
  createMessageSend,
  MESSAGE_SEND_RETRY_WINDOW_MS
} from '@chatto/client/messaging/messageSend';
import type { CreateMessageInput, CreateMessageResult } from '@chatto/client/api/messages';

function preparedPost(overrides: Partial<PreparedPost> = {}): PreparedPost {
  return {
    draftKey: 'chatto:draft:room_1',
    roomId: 'room_1',
    bodyToSend: 'Hello',
    filesToSend: null,
    threadRootEventId: null,
    inReplyTo: null,
    linkPreviewToken: null,
    alsoSendToChannel: false,
    createThread: false,
    ...overrides
  };
}

describe('ComposerSubmissionState', () => {
  const createMessage = vi.fn<(input: CreateMessageInput) => Promise<CreateMessageResult>>();
  const prepareMessage = vi.fn((input: CreateMessageInput) =>
    createMessageSend({
      prepare: async (idempotencyKey) => ({ ...input, idempotencyKey }),
      post: (request) => createMessage(request)
    })
  );
  const updateMessage = vi.fn();
  const loadMentionRoles = vi.fn();
  const onPostSuccess = vi.fn();
  const onEditSuccess = vi.fn();
  let mentionRoleStatus: MentionRolesStatus;
  let mentionRoleNames: string[];
  let state: ComposerSubmissionState;

  beforeEach(() => {
    createMessage.mockReset();
    createMessage.mockResolvedValue({ event: null });
    prepareMessage.mockClear();
    updateMessage.mockReset();
    updateMessage.mockResolvedValue({ updated: true, event: null });
    loadMentionRoles.mockReset();
    loadMentionRoles.mockResolvedValue(true);
    onPostSuccess.mockReset();
    onEditSuccess.mockReset();
    mentionRoleStatus = 'ready';
    mentionRoleNames = [];
    toast.clear();

    state = new ComposerSubmissionState({
      getAPI: () => ({
        prepareMessage,
        updateMessage
      }),
      getMentionRoleStatus: () => mentionRoleStatus,
      loadMentionRoles,
      getMentionRoleNames: () => mentionRoleNames,
      onPostSuccess,
      onEditSuccess
    });
  });

  afterEach(() => {
    vi.useRealTimers();
    vi.restoreAllMocks();
  });

  it('submits ordinary messages and reports success', async () => {
    const post = preparedPost();

    await state.requestPost(post);

    expect(createMessage).toHaveBeenCalledWith(
      expect.objectContaining({ roomId: 'room_1', body: 'Hello' })
    );
    expect(onPostSuccess).toHaveBeenCalledWith(post, null);
    expect(state.loading).toBe(false);
  });

  it('reuses an unchanged failed send and releases it after success', async () => {
    createMessage.mockRejectedValueOnce(new Error('response lost'));
    const errorLog = vi.spyOn(console, 'error').mockImplementation(() => {});
    try {
      await state.requestPost(preparedPost());
      await state.requestPost(preparedPost());
      expect(createMessage.mock.calls[0]![0]).toBe(createMessage.mock.calls[1]![0]);
      await state.requestPost(preparedPost());
      expect(createMessage.mock.calls[2]![0]).not.toBe(createMessage.mock.calls[1]![0]);
    } finally {
      errorLog.mockRestore();
    }
  });

  it('releases an expired send and waits for another click before starting a new send', async () => {
    vi.useFakeTimers({ toFake: ['Date'] });
    vi.spyOn(console, 'error').mockImplementation(() => {});
    createMessage.mockRejectedValueOnce(new Error('response lost'));
    await state.requestPost(preparedPost());

    vi.setSystemTime(Date.now() + MESSAGE_SEND_RETRY_WINDOW_MS);
    await state.requestPost(preparedPost());

    expect(createMessage).toHaveBeenCalledOnce();
    expect(onPostSuccess).not.toHaveBeenCalled();
    expect(state.loading).toBe(false);
    const messages = getToasts().map(({ message }) => message);

    await state.requestPost(preparedPost());
    expect(prepareMessage).toHaveBeenCalledTimes(2);
    expect(createMessage).toHaveBeenCalledTimes(2);
    expect(createMessage.mock.calls[1]![0].idempotencyKey).not.toBe(
      createMessage.mock.calls[0]![0].idempotencyKey
    );
    expect(onPostSuccess).toHaveBeenCalledOnce();
    expect(messages).toContain(
      'The send retry period has ended. Check whether your message arrived before sending again.'
    );
  });

  it.each([false, true])(
    'releases a stale send without automatically reposting (mutationSucceeded: %s)',
    async (mutationSucceeded) => {
      vi.spyOn(console, 'error').mockImplementation(() => {});
      prepareMessage.mockImplementationOnce(() => ({
        idempotencyKey: crypto.randomUUID(),
        send: async () => {
          throw new StaleResponseError(mutationSucceeded);
        }
      }));

      await state.requestPost(preparedPost());

      expect(createMessage).not.toHaveBeenCalled();
      expect(onPostSuccess).not.toHaveBeenCalled();
      expect(state.loading).toBe(false);
      const messages = getToasts().map(({ message }) => message);

      await state.requestPost(preparedPost());
      expect(prepareMessage).toHaveBeenCalledTimes(2);
      expect(createMessage).toHaveBeenCalledOnce();
      expect(onPostSuccess).toHaveBeenCalledOnce();
      expect(messages).toContain(
        'The connection was reset. Check whether your message arrived before sending again.'
      );
    }
  );

  it('keeps a newer pending send when an older operation becomes stale', async () => {
    vi.spyOn(console, 'error').mockImplementation(() => {});
    let rejectOlder!: (error: unknown) => void;
    createMessage
      .mockReturnValueOnce(
        new Promise((_, reject) => {
          rejectOlder = reject;
        })
      )
      .mockRejectedValueOnce(new Error('response lost'));
    const older = state.requestPost(preparedPost());
    await vi.waitFor(() => expect(createMessage).toHaveBeenCalledOnce());
    const newer = preparedPost({ bodyToSend: 'Changed message' });
    await state.requestPost(newer);

    rejectOlder(new StaleResponseError(true));
    await older;
    await state.requestPost(newer);

    expect(prepareMessage).toHaveBeenCalledTimes(2);
    expect(createMessage.mock.calls[2]![0]).toBe(createMessage.mock.calls[1]![0]);
    expect(onPostSuccess).toHaveBeenCalledExactlyOnceWith(newer, null);
  });

  it('loads role metadata before deciding whether a mention needs confirmation', async () => {
    mentionRoleStatus = 'idle';
    mentionRoleNames = ['moderators'];
    const post = preparedPost({ bodyToSend: 'Hello @moderators' });

    await state.requestPost(post);

    expect(loadMentionRoles).toHaveBeenCalledOnce();
    expect(state.pendingRoleMentionConfirmation).toEqual(post);
    expect(createMessage).not.toHaveBeenCalled();
  });

  it('submits a confirmed role mention and clears the confirmation', async () => {
    mentionRoleNames = ['moderators'];
    const post = preparedPost({ bodyToSend: 'Hello @moderators' });
    await state.requestPost(post);

    await state.confirmRoleMentionSend();

    expect(createMessage).toHaveBeenCalledOnce();
    expect(state.pendingRoleMentionConfirmation).toBeNull();
    expect(state.roleMentionConfirmationLoading).toBe(false);
  });

  it('keeps failed attachment status visible after a send failure', async () => {
    const file = new File(['content'], 'photo.png', { type: 'image/png' });
    createMessage.mockImplementation(async (input) => {
      input.onAttachmentUploadUpdate?.({ file, phase: 'failed' });
      throw new Error('upload failed');
    });
    vi.spyOn(console, 'error').mockImplementation(() => {});

    await state.requestPost(preparedPost({ filesToSend: [file] }));

    expect(state.attachmentStatus(file)).toEqual({ phase: 'failed' });
    expect(getToasts().map(({ message }) => message)).toContain('Failed to send message');
    expect(onPostSuccess).not.toHaveBeenCalled();
  });

  it('updates messages and reports failures without leaking loading state', async () => {
    await state.editMessage({ roomId: 'room_1', eventId: 'event_1', body: 'Updated' });
    expect(onEditSuccess).toHaveBeenCalledExactlyOnceWith({
      roomId: 'room_1',
      eventId: 'event_1',
      body: 'Updated'
    });

    updateMessage.mockRejectedValueOnce(new Error('edit failed'));
    await state.editMessage({ roomId: 'room_1', eventId: 'event_1', body: 'Again' });

    expect(getToasts().map(({ message }) => message)).toContain('edit failed');
    expect(state.loading).toBe(false);
  });
});

describe('uploadPercentage', () => {
  it('clamps upload progress and handles terminal states', () => {
    expect(uploadPercentage({ phase: 'uploading', committedBytes: 120, totalBytes: 100 })).toBe(
      100
    );
    expect(uploadPercentage({ phase: 'uploading', committedBytes: 1, totalBytes: 0 })).toBeNull();
    expect(uploadPercentage({ phase: 'uploaded' })).toBe(100);
    expect(uploadPercentage({ phase: 'failed' })).toBeNull();
  });
});
