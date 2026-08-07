import { beforeEach, describe, expect, it, vi } from 'vitest';
import { getToasts, toast } from '$lib/ui/toast';
import { ComposerSubmissionState, type PreparedPost, uploadPercentage } from './submission.svelte';
import type { MentionRolesStatus } from '$lib/state/server/mentionRoles.svelte';

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
    ...overrides
  };
}

describe('ComposerSubmissionState', () => {
  const createMessage = vi.fn();
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
      getAPI: () => ({ createMessage, updateMessage }),
      getMentionRoleStatus: () => mentionRoleStatus,
      loadMentionRoles,
      getMentionRoleNames: () => mentionRoleNames,
      onPostSuccess,
      onEditSuccess
    });
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
      input.onAttachmentUploadUpdate({ file, phase: 'failed' });
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
    expect(onEditSuccess).toHaveBeenCalledOnce();

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
