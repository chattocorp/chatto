import { beforeEach, describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { q } from '$lib/test-utils';
import MessageActionSheet from './MessageActionSheet.svelte';

const mocks = vi.hoisted(() => ({
  actions: {
    toggleReaction: vi.fn(),
    startEdit: vi.fn(),
    openDeleteConfirmation: vi.fn(),
    copyMessageLink: vi.fn()
  }
}));

vi.mock('$lib/hooks', () => ({
  useMessageActions: () => mocks.actions
}));

vi.mock('$lib/state/recentEmojis.svelte', () => ({
  getRecentEmojis: () => ({
    quickReactions: ['👍', '❤️']
  })
}));

const baseProps = {
  serverId: 'server-1',
  roomId: 'room-1',
  messageEventId: 'message-event-1',
  eventId: 'event-1',
  messageBody: 'Hello',
  onClose: vi.fn()
};

function renderSheet(props: Record<string, unknown> = {}) {
  return render(MessageActionSheet, {
    props: {
      ...baseProps,
      ...props
    }
  });
}

function actionLabels(container: HTMLElement): string[] {
  return Array.from(container.querySelectorAll<HTMLButtonElement>('nav button'))
    .map((button) => button.textContent?.trim())
    .filter((label): label is string => !!label);
}

beforeEach(() => {
  vi.clearAllMocks();
  baseProps.onClose.mockClear();
});

describe('MessageActionSheet', () => {
  it('renders quick reactions when reactions are allowed', async () => {
    const { container } = renderSheet({ canReact: true });

    await expect.element(q(container, '[aria-label="React with 👍"]')).toBeInTheDocument();
    await expect.element(q(container, '[aria-label="React with ❤️"]')).toBeInTheDocument();
  });

  it('keeps the action order unchanged', () => {
    const { container } = renderSheet({
      canEdit: true,
      canDelete: true,
      onReply: vi.fn(),
      onReplyInRoom: vi.fn()
    });

    expect(actionLabels(container)).toEqual([
      'Reply',
      'Reply in thread',
      'Edit',
      'Copy link',
      'Delete'
    ]);
  });

  it('closes after invoking sheet actions', async () => {
    const onReplyInRoom = vi.fn();
    const onReply = vi.fn();
    const { container } = renderSheet({
      canReact: true,
      canEdit: true,
      canDelete: true,
      onReplyInRoom,
      onReply
    });

    container.querySelector<HTMLButtonElement>('[aria-label="React with 👍"]')!.click();
    await vi.waitFor(() => {
      expect(mocks.actions.toggleReaction).toHaveBeenCalledWith(
        expect.objectContaining({
          roomId: 'room-1',
          messageEventId: 'message-event-1'
        }),
        '👍',
        false
      );
    });
    expect(baseProps.onClose).toHaveBeenCalledOnce();

    baseProps.onClose.mockClear();
    Array.from(container.querySelectorAll<HTMLButtonElement>('nav button'))
      .find((button) => button.textContent?.trim() === 'Reply')!
      .click();
    expect(onReplyInRoom).toHaveBeenCalledOnce();
    expect(baseProps.onClose).toHaveBeenCalledOnce();

    baseProps.onClose.mockClear();
    Array.from(container.querySelectorAll<HTMLButtonElement>('nav button'))
      .find((button) => button.textContent?.trim() === 'Reply in thread')!
      .click();
    expect(onReply).toHaveBeenCalledOnce();
    expect(baseProps.onClose).toHaveBeenCalledOnce();

    baseProps.onClose.mockClear();
    Array.from(container.querySelectorAll<HTMLButtonElement>('nav button'))
      .find((button) => button.textContent?.includes('Edit'))!
      .click();
    expect(mocks.actions.startEdit).toHaveBeenCalledWith(
      expect.objectContaining({ eventId: 'event-1', messageBody: 'Hello' })
    );
    expect(baseProps.onClose).toHaveBeenCalledOnce();

    baseProps.onClose.mockClear();
    Array.from(container.querySelectorAll<HTMLButtonElement>('nav button'))
      .find((button) => button.textContent?.includes('Copy link'))!
      .click();
    expect(mocks.actions.copyMessageLink).toHaveBeenCalledWith(
      expect.objectContaining({
        serverId: 'server-1',
        roomId: 'room-1',
        messageEventId: 'message-event-1'
      })
    );
    await vi.waitFor(() => {
      expect(baseProps.onClose).toHaveBeenCalledOnce();
    });

    baseProps.onClose.mockClear();
    Array.from(container.querySelectorAll<HTMLButtonElement>('nav button'))
      .find((button) => button.textContent?.includes('Delete'))!
      .click();
    expect(mocks.actions.openDeleteConfirmation).toHaveBeenCalledWith(
      expect.objectContaining({ eventId: 'event-1' })
    );
    expect(baseProps.onClose).toHaveBeenCalledOnce();
  });

  it('keeps Delete styled as destructive', () => {
    const { container } = renderSheet({ canDelete: true });

    const deleteButton = Array.from(
      container.querySelectorAll<HTMLButtonElement>('nav button')
    ).find((button) => button.textContent?.includes('Delete'));

    expect(deleteButton).toBeDefined();
    expect(deleteButton).toHaveClass('text-danger');
  });
});
