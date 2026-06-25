import { beforeEach, describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { q } from '$lib/test-utils';
import MessageHoverBar from './MessageHoverBar.svelte';

const mocks = vi.hoisted(() => ({
  actions: {
    toggleReaction: vi.fn(),
    startEdit: vi.fn(),
    openDeleteConfirmation: vi.fn()
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
  messageBody: 'Hello'
};

type MessageHoverBarProps = {
  serverId: string;
  roomId: string;
  messageEventId: string;
  eventId: string;
  deleteEventId?: string;
  messageBody: string;
  threadRootEventId?: string | null;
  channelEchoEventId?: string | null;
  canAddChannelEcho?: boolean;
  reactions?: { emoji: string; hasReacted: boolean }[];
  canReact?: boolean;
  canEdit?: boolean;
  forceVisible?: boolean;
  onReplyInRoom?: () => void;
  onReply?: () => void;
  onOpenEmojiPicker?: (e: MouseEvent) => void;
  onOpenMenu?: (e: MouseEvent) => void;
};

function renderBar(props: Partial<MessageHoverBarProps> = {}) {
  return render(MessageHoverBar, {
    props: {
      ...baseProps,
      ...props
    }
  });
}

beforeEach(() => {
  vi.clearAllMocks();
});

describe('MessageHoverBar', () => {
  it('renders quick reactions and action buttons for an author', async () => {
    const { container } = renderBar({
      canReact: true,
      canEdit: true,
      onReply: vi.fn(),
      onOpenMenu: vi.fn()
    });

    await expect.element(q(container, '[aria-label="React with 👍"]')).toBeInTheDocument();
    await expect.element(q(container, '[aria-label="React with ❤️"]')).toBeInTheDocument();
    await expect.element(q(container, '[aria-label="Reply in thread"]')).toBeInTheDocument();
    await expect.element(q(container, '[aria-label="Edit message"]')).toBeInTheDocument();
    await expect.element(q(container, '[aria-label="More actions"]')).toBeInTheDocument();
  });

  it('hides edit when the viewer cannot edit', async () => {
    const { container } = renderBar({
      canReact: true,
      canEdit: false,
      onReply: vi.fn(),
      onOpenMenu: vi.fn()
    });

    await expect.element(q(container, '[aria-label="React with 👍"]')).toBeInTheDocument();
    await expect.element(q(container, '[aria-label="Reply in thread"]')).toBeInTheDocument();
    expect(q(container, '[aria-label="Edit message"]')).toBeNull();
  });

  it('marks already-reacted quick reactions as removable', async () => {
    const { container } = renderBar({
      canReact: true,
      reactions: [{ emoji: 'thumbsup', hasReacted: true }]
    });

    await expect.element(q(container, '[aria-label="Remove 👍"]')).toBeInTheDocument();
  });

  it('keeps the toolbar visible while forceVisible is set', async () => {
    const { container } = renderBar({ forceVisible: true });

    await expect.element(q(container, '[role="toolbar"]')).toHaveClass('!visible');
  });

  it('routes quick actions to callbacks and message actions', async () => {
    const onReplyInRoom = vi.fn();
    const onReply = vi.fn();
    const onOpenMenu = vi.fn();
    const { container } = renderBar({
      canReact: true,
      canEdit: true,
      onReplyInRoom,
      onReply,
      onOpenMenu
    });

    (q(container, '[aria-label="React with 👍"]') as HTMLButtonElement).click();
    await vi.waitFor(() => {
      expect(mocks.actions.toggleReaction).toHaveBeenCalledWith(
        expect.objectContaining({
          roomId: 'room-1',
          messageEventId: 'message-event-1',
          eventId: 'event-1',
          messageBody: 'Hello'
        }),
        '👍',
        false
      );
    });

    (q(container, '[aria-label="Reply"]') as HTMLButtonElement).click();
    expect(onReplyInRoom).toHaveBeenCalledOnce();

    (q(container, '[aria-label="Reply in thread"]') as HTMLButtonElement).click();
    expect(onReply).toHaveBeenCalledOnce();

    (q(container, '[aria-label="Edit message"]') as HTMLButtonElement).click();
    expect(mocks.actions.startEdit).toHaveBeenCalledWith(
      expect.objectContaining({ eventId: 'event-1', messageBody: 'Hello' })
    );

    (q(container, '[aria-label="More actions"]') as HTMLButtonElement).click();
    expect(onOpenMenu).toHaveBeenCalledOnce();
  });
});
