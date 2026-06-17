import { beforeEach, describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { q } from '$lib/test-utils';
import MessageContextMenu from './MessageContextMenu.svelte';

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
  messageBody: 'Hello',
  onClose: vi.fn()
};

function renderMenu(props: Record<string, unknown> = {}) {
  return render(MessageContextMenu, {
    props: {
      ...baseProps,
      ...props
    }
  });
}

beforeEach(() => {
  vi.clearAllMocks();
  baseProps.onClose.mockClear();
});

describe('MessageContextMenu', () => {
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
    expect(container.textContent).toContain('Delete');
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
    expect(container.querySelectorAll('.menu-section')).toHaveLength(2);
  });

  it('closes after invoking menu actions', async () => {
    const onReply = vi.fn();
    const { container } = renderMenu({
      canReact: true,
      canEdit: true,
      canDelete: true,
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
    (Array.from(container.querySelectorAll<HTMLButtonElement>('[role="menuitem"]')).find(
      (button) => button.textContent?.trim() === 'Reply in thread'
    )!).click();
    expect(onReply).toHaveBeenCalledOnce();
    expect(baseProps.onClose).toHaveBeenCalledOnce();

    baseProps.onClose.mockClear();
    (Array.from(container.querySelectorAll<HTMLButtonElement>('[role="menuitem"]')).find(
      (button) => button.textContent?.includes('Edit')
    )!).click();
    expect(mocks.actions.startEdit).toHaveBeenCalledWith(
      expect.objectContaining({ eventId: 'event-1', messageBody: 'Hello' })
    );
    expect(baseProps.onClose).toHaveBeenCalledOnce();

    baseProps.onClose.mockClear();
    (Array.from(container.querySelectorAll<HTMLButtonElement>('[role="menuitem"]')).find(
      (button) => button.textContent?.includes('Delete')
    )!).click();
    expect(mocks.actions.openDeleteConfirmation).toHaveBeenCalledWith(
      expect.objectContaining({ eventId: 'event-1' })
    );
    expect(baseProps.onClose).toHaveBeenCalledOnce();
  });
});
