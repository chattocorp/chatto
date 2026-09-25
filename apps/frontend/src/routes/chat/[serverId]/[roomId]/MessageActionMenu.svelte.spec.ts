import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { q } from '$lib/test-utils';
import { toast } from '$lib/ui/toast';
import MessageActionMenuTestHarness from './MessageActionMenuTestHarness.svelte';
import MessageEventActionOverlays from './MessageEventActionOverlays.svelte';
import { MessageEventInteractionState } from './messageEventInteractions.svelte';
import { buildMessageActionModel } from './messageActionModel';

const mocks = vi.hoisted(() => ({
  copyImageToClipboard: vi.fn(),
  actions: {
    toggleReaction: vi.fn(),
    addReaction: vi.fn(),
    removeReaction: vi.fn(),
    startEdit: vi.fn(),
    openDeleteConfirmation: vi.fn(),
    copyMessageText: vi.fn(),
    copyMessageLink: vi.fn()
  }
}));

vi.mock('$lib/attachments/copyImage', () => ({
  copyImageToClipboard: mocks.copyImageToClipboard
}));

vi.mock('$lib/state/recentEmojis.svelte', () => ({
  MAX_RECENT_EMOJIS: 16,
  getRecentEmojis: () => ({
    quickReactions: ['👍', '❤️']
  })
}));

const baseParams = {
  serverId: 'server-1',
  roomId: 'room-1',
  messageEventId: 'message-event-1',
  eventId: 'event-1',
  messageBody: 'Hello'
};

const baseProps = {
  onClose: vi.fn()
};

type ActionOverrides = {
  messageBody?: string;
  permalinkThreadRootEventId?: string | null;
  reactions?: { emoji: string; hasReacted: boolean }[];
  canReact?: boolean;
  canEdit?: boolean;
  canDelete?: boolean;
  replyInRoomLabel?: string;
  replyThreadLabel?: string;
  secondaryReplyInRoomLabel?: string;
  onReplyInRoom?: () => void;
  onReply?: () => void;
  onSecondaryReplyInRoom?: () => void;
};

function buildAction(overrides: ActionOverrides = {}) {
  return buildMessageActionModel({
    actions: mocks.actions,
    params: {
      ...baseParams,
      messageBody: overrides.messageBody ?? baseParams.messageBody,
      permalinkThreadRootEventId: overrides.permalinkThreadRootEventId
    },
    reactions: overrides.reactions ?? [],
    canReact: overrides.canReact ?? false,
    canEdit: overrides.canEdit ?? false,
    canDelete: overrides.canDelete ?? false,
    replyInRoomLabel: overrides.replyInRoomLabel ?? 'Reply',
    replyThreadLabel: overrides.replyThreadLabel ?? 'Reply in thread',
    replyInRoom: overrides.onReplyInRoom,
    replyThread: overrides.onReply,
    secondaryReplyInRoomLabel: overrides.secondaryReplyInRoomLabel,
    secondaryReplyInRoom: overrides.onSecondaryReplyInRoom
  });
}

function renderMenu({
  presentation,
  linkUrl,
  imageUrl,
  onOpenEmojiPicker,
  hasReactions,
  onOpenReactionDetails,
  ...overrides
}: ActionOverrides & {
  presentation?: 'menu' | 'sheet';
  linkUrl?: string | null;
  imageUrl?: string | null;
  onOpenEmojiPicker?: () => void;
  hasReactions?: boolean;
  onOpenReactionDetails?: () => void;
} = {}) {
  return render(MessageActionMenuTestHarness, {
    props: {
      action: buildAction(overrides),
      presentation,
      linkUrl,
      imageUrl,
      onOpenEmojiPicker,
      hasReactions,
      onOpenReactionDetails,
      onClose: baseProps.onClose
    }
  });
}

function actionLabels(container: HTMLElement): string[] {
  return Array.from(container.querySelectorAll<HTMLButtonElement>('.menu-entry'))
    .map((button) => button.textContent?.trim())
    .filter((label): label is string => !!label);
}

beforeEach(() => {
  vi.clearAllMocks();
  baseProps.onClose.mockClear();
});

afterEach(() => {
  vi.restoreAllMocks();
});

describe('MessageActionMenu', () => {
  it('copies the clicked image and keeps the message permalink action', async () => {
    mocks.copyImageToClipboard.mockResolvedValue(undefined);
    const success = vi.spyOn(toast, 'success').mockImplementation(() => 'toast');
    const { container } = renderMenu({ imageUrl: 'https://example.com/image?access=ticket' });

    expect(actionLabels(container)).toEqual(['Copy text', 'Copy image', 'Copy message link']);
    Array.from(container.querySelectorAll<HTMLButtonElement>('[role="menuitem"]'))
      .find((button) => button.textContent?.trim() === 'Copy image')!
      .click();

    await vi.waitFor(() =>
      expect(mocks.copyImageToClipboard).toHaveBeenCalledWith(
        'https://example.com/image?access=ticket'
      )
    );
    expect(success).toHaveBeenCalledWith('Image copied');
    expect(baseProps.onClose).toHaveBeenCalledOnce();
    expect(mocks.actions.copyMessageLink).not.toHaveBeenCalled();
  });

  it('reports an image clipboard failure and closes the menu', async () => {
    mocks.copyImageToClipboard.mockRejectedValue(new Error('Clipboard unavailable'));
    const error = vi.spyOn(toast, 'error').mockImplementation(() => 'toast');
    const { container } = renderMenu({ imageUrl: 'https://example.com/image' });

    Array.from(container.querySelectorAll<HTMLButtonElement>('[role="menuitem"]'))
      .find((button) => button.textContent?.trim() === 'Copy image')!
      .click();

    await vi.waitFor(() => expect(error).toHaveBeenCalledWith('Failed to copy image'));
    expect(baseProps.onClose).toHaveBeenCalledOnce();
  });

  it('copies the clicked link in the desktop menu and keeps the message permalink action', async () => {
    const writeText = vi.spyOn(navigator.clipboard, 'writeText').mockResolvedValue(undefined);
    const success = vi.spyOn(toast, 'success').mockImplementation(() => 'toast');
    const { container } = renderMenu({ linkUrl: 'https://example.com/path?q=1' });

    expect(actionLabels(container)).toEqual(['Copy text', 'Copy link', 'Copy message link']);
    Array.from(container.querySelectorAll<HTMLButtonElement>('[role="menuitem"]'))
      .find((button) => button.textContent?.trim() === 'Copy link')!
      .click();

    await vi.waitFor(() => expect(writeText).toHaveBeenCalledWith('https://example.com/path?q=1'));
    expect(success).toHaveBeenCalledWith('Link copied');
    expect(baseProps.onClose).toHaveBeenCalledOnce();
    expect(mocks.actions.copyMessageLink).not.toHaveBeenCalled();
  });

  it('reports a clipboard failure and closes the menu', async () => {
    vi.spyOn(navigator.clipboard, 'writeText').mockRejectedValue(
      new Error('Clipboard unavailable')
    );
    const error = vi.spyOn(toast, 'error').mockImplementation(() => 'toast');
    const { container } = renderMenu({ linkUrl: 'https://example.com/path' });

    Array.from(container.querySelectorAll<HTMLButtonElement>('[role="menuitem"]'))
      .find((button) => button.textContent?.trim() === 'Copy link')!
      .click();

    await vi.waitFor(() => expect(error).toHaveBeenCalledWith('Failed to copy link'));
    expect(baseProps.onClose).toHaveBeenCalledOnce();
  });

  it('renders reaction buttons when reactions are allowed', async () => {
    const { container } = renderMenu({ canReact: true });

    await expect.element(q(container, '[aria-label="React with 👍"]')).toBeInTheDocument();
    await expect.element(q(container, '[aria-label="React with ❤️"]')).toBeInTheDocument();
  });

  it.each(['menu', 'sheet'] as const)(
    'opens reaction details from the %s when adding reactions is denied',
    (presentation) => {
      const onOpenReactionDetails = vi.fn();
      const { container } = renderMenu({
        presentation,
        canReact: false,
        hasReactions: true,
        onOpenReactionDetails
      });

      const entry = [...container.querySelectorAll<HTMLButtonElement>('.menu-entry')].find(
        (button) => button.textContent?.trim() === 'Reactions'
      );
      expect(entry).toBeDefined();
      entry!.click();
      expect(baseProps.onClose).toHaveBeenCalledOnce();
      expect(onOpenReactionDetails).toHaveBeenCalledOnce();
      expect(container.querySelector('[aria-label="React with 👍"]')).toBeNull();
    }
  );

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
    expect(container.textContent).toContain('Copy message link');
    expect(container.textContent).toContain('Delete');
    expect(
      Array.from(container.querySelectorAll('.menu-section')).map((section) =>
        Array.from(section.querySelectorAll('[role="menuitem"]')).map((button) =>
          button.textContent?.trim()
        )
      )
    ).toEqual([
      ['Reply', 'Reply in thread', 'Edit'],
      ['Copy text', 'Copy message link'],
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

    expect(actionLabels).toEqual([
      'Reply in thread',
      'Open thread',
      'Copy text',
      'Copy message link'
    ]);
    const replyIcon = container.querySelector('[role="menuitem"] .iconify');
    expect(replyIcon?.classList).toContain('icon-[uil--corner-up-left]');
    expect(replyIcon?.classList).toContain('rtl:-scale-x-100');
  });

  it('keeps reply actions in a stable order and puts the room fallback last', () => {
    const { container } = renderMenu({
      onReplyInRoom: vi.fn(),
      onReply: vi.fn(),
      secondaryReplyInRoomLabel: 'Reply in room',
      onSecondaryReplyInRoom: vi.fn()
    });

    expect(actionLabels(container)).toEqual([
      'Reply',
      'Reply in thread',
      'Reply in room',
      'Copy text',
      'Copy message link'
    ]);
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

    expect(actionLabels).toEqual(['Edit', 'Copy text', 'Copy message link', 'Delete']);
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

    expect(actionLabels).toEqual(['Copy text', 'Copy message link']);
  });

  it('omits copy text when the message has no text body', () => {
    const { container } = renderMenu({ messageBody: '' });

    const actionLabels = Array.from(
      container.querySelectorAll<HTMLButtonElement>('[role="menuitem"]')
    )
      .map((button) => button.textContent?.trim())
      .filter(Boolean);

    expect(actionLabels).toEqual(['Copy message link']);
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
      .find((button) => button.textContent?.includes('Copy message link'))!
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
        linkUrl: 'https://example.com/path',
        imageUrl: 'https://example.com/image',
        canReact: true,
        canEdit: true,
        canDelete: true,
        onReply: vi.fn(),
        onReplyInRoom: vi.fn()
      });

      expect(actionLabels(container)).toEqual([
        'Reply',
        'Reply in thread',
        'Edit',
        'Copy text',
        'Copy message link',
        'Delete'
      ]);
      expect(
        Array.from(container.querySelectorAll('.menu-section'))
          .filter((section) => section.querySelector('.menu-entry'))
          .map((section) =>
            Array.from(section.querySelectorAll('button')).map((button) =>
              button.textContent?.trim()
            )
          )
      ).toEqual([
        ['Reply', 'Reply in thread', 'Edit'],
        ['Copy text', 'Copy message link'],
        ['Delete']
      ]);
      expect(container.querySelector('[role="menuitem"]')).toBeNull();
      expect(container.querySelector('.menu-entry')).toHaveClass('menu-entry-sheet');
      expect(q(container, '[aria-label="React with 👍"]')).toHaveClass('rounded-full', 'text-xl');
      expect(
        Array.from(container.querySelectorAll<HTMLButtonElement>('.menu-entry')).find((button) =>
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

      Array.from(container.querySelectorAll<HTMLButtonElement>('.menu-entry'))
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
          action: buildAction(),
          roomId: 'room-1',
          messageEventId: 'message-event-1',
          reactions: [],
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
