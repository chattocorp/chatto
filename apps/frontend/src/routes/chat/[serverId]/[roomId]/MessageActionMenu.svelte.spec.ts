import { beforeEach, describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { q } from '$lib/test-utils';
import MessageActionMenu from './MessageActionMenu.svelte';
import MessageEventActionOverlays from './MessageEventActionOverlays.svelte';
import { MessageEventInteractionState } from './messageEventInteractions.svelte';

const mocks = vi.hoisted(() => ({
  actions: {
    toggleReaction: vi.fn(),
    startEdit: vi.fn(),
    openDeleteConfirmation: vi.fn(),
    copyMessageText: vi.fn(),
    copyMessageLink: vi.fn()
  }
}));

vi.mock('$lib/hooks', () => ({
  useMessageActions: () => mocks.actions
}));

vi.mock('$lib/state/recentEmojis.svelte', () => ({
  MAX_RECENT_EMOJIS: 16,
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

function renderMenu(props: Record<string, unknown> = {}) {
  return render(MessageActionMenu, {
    props: {
      ...baseProps,
      ...props
    }
  });
}

function navActionLabels(container: HTMLElement): string[] {
  return Array.from(container.querySelectorAll<HTMLButtonElement>('nav button'))
    .map((button) => button.textContent?.trim())
    .filter((label): label is string => !!label);
}

beforeEach(() => {
  vi.clearAllMocks();
  baseProps.onClose.mockClear();
});

describe('MessageActionMenu', () => {
  it('renders reaction buttons when reactions are allowed', async () => {
    const { container } = renderMenu({ canReact: true });

    await expect.element(q(container, '[aria-label="React with 👍"]')).toBeInTheDocument();
    await expect.element(q(container, '[aria-label="React with ❤️"]')).toBeInTheDocument();
  });

  it('renders author actions when allowed', async () => {
    const { container } = renderMenu({
      canEdit: true,
      canDelete: true,
      onReply: vi.fn(),
      onReplyInRoom: vi.fn()
    });

    await expect.element(q(container, '[role="menuitem"]')).toBeInTheDocument();
    expect(container.textContent).toContain('Reply');
    expect(container.textContent).toContain('Reply in thread');
    expect(container.textContent).toContain('Edit');
    expect(container.textContent).toContain('Copy text');
    expect(container.textContent).toContain('Copy link');
    expect(container.textContent).toContain('Delete');
    expect(
      Array.from(container.querySelectorAll('.menu-section')).map((section) =>
        Array.from(section.querySelectorAll('[role="menuitem"]')).map((button) =>
          button.textContent?.trim()
        )
      )
    ).toEqual([
      ['Reply', 'Reply in thread', 'Edit'],
      ['Copy text', 'Copy link'],
      ['Delete']
    ]);
  });

  it('uses custom reply action labels when provided', () => {
    const { container } = renderMenu({
      onReply: vi.fn(),
      onReplyInRoom: vi.fn(),
      replyInRoomLabel: 'Reply in thread',
      replyThreadLabel: 'Open thread'
    });

    const actionLabels = Array.from(
      container.querySelectorAll<HTMLButtonElement>('[role="menuitem"]')
    )
      .map((button) => button.textContent?.trim())
      .filter(Boolean);

    expect(actionLabels).toEqual(['Reply in thread', 'Open thread', 'Copy text', 'Copy link']);
  });

  it('orders clipboard actions between edit and delete', () => {
    const { container } = renderMenu({
      canEdit: true,
      canDelete: true
    });

    const actionLabels = Array.from(
      container.querySelectorAll<HTMLButtonElement>('[role="menuitem"]')
    )
      .map((button) => button.textContent?.trim())
      .filter(Boolean);

    expect(actionLabels).toEqual(['Edit', 'Copy text', 'Copy link', 'Delete']);
  });

  it('renders no empty actions section for a non-author thread reply', () => {
    const { container } = renderMenu({
      canReact: true,
      onReplyInRoom: vi.fn()
    });

    expect(container.textContent).toContain('Reply');
    expect(container.textContent).not.toContain('Reply in thread');
    expect(container.textContent).not.toContain('Edit');
    expect(container.textContent).not.toContain('Delete');
    expect(container.querySelectorAll('.menu-section')).toHaveLength(3);
  });

  it('renders clipboard actions when no permissions are granted', () => {
    const { container } = renderMenu();

    const actionLabels = Array.from(
      container.querySelectorAll<HTMLButtonElement>('[role="menuitem"]')
    )
      .map((button) => button.textContent?.trim())
      .filter(Boolean);

    expect(actionLabels).toEqual(['Copy text', 'Copy link']);
  });

  it('omits copy text when the message has no text body', () => {
    const { container } = renderMenu({ messageBody: '' });

    const actionLabels = Array.from(
      container.querySelectorAll<HTMLButtonElement>('[role="menuitem"]')
    )
      .map((button) => button.textContent?.trim())
      .filter(Boolean);

    expect(actionLabels).toEqual(['Copy link']);
  });

  it('closes after invoking menu actions', async () => {
    const onReply = vi.fn();
    const { container } = renderMenu({
      canReact: true,
      canEdit: true,
      canDelete: true,
      permalinkThreadRootEventId: 'thread-root-1',
      onReply
    });

    (q(container, '[aria-label="React with 👍"]') as HTMLButtonElement).click();
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
    Array.from(container.querySelectorAll<HTMLButtonElement>('[role="menuitem"]'))
      .find((button) => button.textContent?.trim() === 'Reply in thread')!
      .click();
    expect(onReply).toHaveBeenCalledOnce();
    expect(baseProps.onClose).toHaveBeenCalledOnce();

    baseProps.onClose.mockClear();
    Array.from(container.querySelectorAll<HTMLButtonElement>('[role="menuitem"]'))
      .find((button) => button.textContent?.includes('Edit'))!
      .click();
    expect(mocks.actions.startEdit).toHaveBeenCalledWith(
      expect.objectContaining({ eventId: 'event-1', messageBody: 'Hello' })
    );
    expect(baseProps.onClose).toHaveBeenCalledOnce();

    baseProps.onClose.mockClear();
    Array.from(container.querySelectorAll<HTMLButtonElement>('[role="menuitem"]'))
      .find((button) => button.textContent?.includes('Copy text'))!
      .click();
    expect(mocks.actions.copyMessageText).toHaveBeenCalledWith(
      expect.objectContaining({ messageBody: 'Hello' })
    );
    await vi.waitFor(() => {
      expect(baseProps.onClose).toHaveBeenCalledOnce();
    });

    baseProps.onClose.mockClear();
    Array.from(container.querySelectorAll<HTMLButtonElement>('[role="menuitem"]'))
      .find((button) => button.textContent?.includes('Copy link'))!
      .click();
    expect(mocks.actions.copyMessageLink).toHaveBeenCalledWith(
      expect.objectContaining({
        serverId: 'server-1',
        roomId: 'room-1',
        messageEventId: 'message-event-1',
        permalinkThreadRootEventId: 'thread-root-1'
      })
    );
    await vi.waitFor(() => {
      expect(baseProps.onClose).toHaveBeenCalledOnce();
    });

    baseProps.onClose.mockClear();
    Array.from(container.querySelectorAll<HTMLButtonElement>('[role="menuitem"]'))
      .find((button) => button.textContent?.includes('Delete'))!
      .click();
    expect(mocks.actions.openDeleteConfirmation).toHaveBeenCalledWith(
      expect.objectContaining({ eventId: 'event-1' })
    );
    expect(baseProps.onClose).toHaveBeenCalledOnce();
  });

  describe('sheet presentation', () => {
    it('preserves action order, grouping, sizing, and non-menu semantics', () => {
      const { container } = renderMenu({
        presentation: 'sheet',
        canReact: true,
        canEdit: true,
        canDelete: true,
        onReply: vi.fn(),
        onReplyInRoom: vi.fn()
      });

      expect(navActionLabels(container)).toEqual([
        'Reply',
        'Reply in thread',
        'Edit',
        'Copy text',
        'Copy link',
        'Delete'
      ]);
      expect(
        Array.from(container.querySelectorAll('nav')).map((section) =>
          Array.from(section.querySelectorAll('button')).map((button) =>
            button.textContent?.trim()
          )
        )
      ).toEqual([
        ['Reply', 'Reply in thread', 'Edit'],
        ['Copy text', 'Copy link'],
        ['Delete']
      ]);
      expect(container.querySelector('[role="menuitem"]')).toBeNull();
      expect(container.querySelector('nav button')).toHaveClass('min-h-11');
      expect(q(container, '[aria-label="React with 👍"]')).toHaveClass('rounded-full', 'text-xl');
      expect(
        Array.from(container.querySelectorAll<HTMLButtonElement>('nav button')).find((button) =>
          button.textContent?.includes('Delete')
        )
      ).toHaveClass('text-danger');
    });

    it('uses the shared handlers and closes after a sheet action', () => {
      const onReplyInRoom = vi.fn();
      const { container } = renderMenu({
        presentation: 'sheet',
        onReplyInRoom
      });

      Array.from(container.querySelectorAll<HTMLButtonElement>('nav button'))
        .find((button) => button.textContent?.trim() === 'Reply')!
        .click();

      expect(onReplyInRoom).toHaveBeenCalledOnce();
      expect(baseProps.onClose).toHaveBeenCalledOnce();
    });

    it('notifies the message owner when the sheet is dismissed natively', async () => {
      const interactions = new MessageEventInteractionState();
      interactions.showActionSheet = true;
      const onClose = vi.fn();
      const { container } = render(MessageEventActionOverlays, {
        props: {
          interactions,
          serverId: 'server-1',
          roomId: 'room-1',
          messageEventId: 'message-event-1',
          eventId: 'event-1',
          deleteEventId: 'event-1',
          messageBody: 'Hello',
          onEmojiSelect: vi.fn(),
          onClose
        }
      });
      const dialog = q(container, 'dialog') as HTMLDialogElement;

      await vi.waitFor(() => {
        expect(dialog.open).toBe(true);
      });
      dialog.close();

      await vi.waitFor(() => {
        expect(interactions.showActionSheet).toBe(false);
        expect(onClose).toHaveBeenCalledOnce();
      });
    });
  });
});
