import { describe, expect, it, vi } from 'vitest';
import { flushSync } from 'svelte';
import { render } from 'vitest-browser-svelte';
import { q } from '$lib/test-utils';
import MessageMetaBar from './MessageMetaBar.svelte';

vi.mock('$app/paths', () => ({
  resolve: (path: string, params?: Record<string, string>) =>
    path
      .replace('[serverId]', params?.serverId ?? '')
      .replace('[roomId]', params?.roomId ?? '')
      .replace('[threadId]', params?.threadId ?? '')
}));

vi.mock('$lib/gql', () => ({
  graphql: (source: string) => source
}));

vi.mock('$lib/gql/fragment-masking', () => ({
  useFragment: (_document: unknown, value: unknown) => value
}));

vi.mock('$lib/state/server/connection.svelte', () => ({
  useConnection: () => () => ({
    client: {
      mutation: vi.fn().mockResolvedValue({ error: null })
    }
  })
}));

const baseProps = {
  roomId: 'room-1',
  messageEventId: 'thread-1',
  serverSegment: '-',
  threadRootEventId: 'thread-1',
  reactions: [],
  onOpenThread: vi.fn()
};

function reaction(
  overrides: Partial<{
    emoji: string;
    count: number;
    hasReacted: boolean;
    users: { id: string; displayName: string }[];
  }> = {}
) {
  return {
    emoji: 'thumbsup',
    count: 2,
    hasReacted: false,
    users: [
      { id: 'user-1', displayName: 'Alice' },
      { id: 'user-2', displayName: 'Bob' }
    ],
    ...overrides
  };
}

describe('MessageMetaBar', () => {
  it('renders the reply count badge as a native thread link', async () => {
    const { container } = render(MessageMetaBar, {
      props: {
        ...baseProps,
        replyCount: 2
      }
    });

    const link = q(container, 'a[href="/chat/-/room-1/thread-1"]') as HTMLAnchorElement;

    await expect.element(link).toBeInTheDocument();
    expect(link.textContent?.replace(/\s+/g, ' ').trim()).toContain('2 replies');
  });

  it('renders the echo thread badge as a native thread link', async () => {
    const { container } = render(MessageMetaBar, {
      props: {
        ...baseProps,
        isEchoEvent: true
      }
    });

    const link = q(container, 'a[href="/chat/-/room-1/thread-1"]') as HTMLAnchorElement;

    await expect.element(link).toBeInTheDocument();
    expect(link.textContent).toContain('Thread');
  });

  it('opens the thread through the existing callback for plain primary clicks', () => {
    const onOpenThread = vi.fn();
    const { container } = render(MessageMetaBar, {
      props: {
        ...baseProps,
        onOpenThread,
        replyCount: 1
      }
    });

    const link = q(container, 'a[href="/chat/-/room-1/thread-1"]') as HTMLAnchorElement;
    const event = new MouseEvent('click', { bubbles: true, cancelable: true, button: 0 });

    const allowed = link.dispatchEvent(event);

    expect(allowed).toBe(false);
    expect(event.defaultPrevented).toBe(true);
    expect(onOpenThread).toHaveBeenCalledOnce();
  });

  it('leaves modified clicks to native link behavior', () => {
    const onOpenThread = vi.fn();
    const { container } = render(MessageMetaBar, {
      props: {
        ...baseProps,
        onOpenThread,
        replyCount: 1
      }
    });

    const link = q(container, 'a[href="/chat/-/room-1/thread-1"]') as HTMLAnchorElement;
    const event = new MouseEvent('click', {
      bubbles: true,
      cancelable: true,
      button: 0,
      metaKey: true
    });

    const allowed = link.dispatchEvent(event);

    expect(allowed).toBe(true);
    expect(event.defaultPrevented).toBe(false);
    expect(onOpenThread).not.toHaveBeenCalled();
  });

  it('does not bubble press-start gestures to the message row', () => {
    const { container } = render(MessageMetaBar, {
      props: {
        ...baseProps,
        replyCount: 1
      }
    });
    const touchStart = vi.fn();
    const mouseDown = vi.fn();
    container.addEventListener('touchstart', touchStart);
    container.addEventListener('mousedown', mouseDown);

    const link = q(container, 'a[href="/chat/-/room-1/thread-1"]') as HTMLAnchorElement;
    const touchEvent = new Event('touchstart', { bubbles: true, cancelable: true });
    const mouseEvent = new MouseEvent('mousedown', { bubbles: true, cancelable: true, button: 0 });

    expect(link.dispatchEvent(touchEvent)).toBe(true);
    expect(touchEvent.defaultPrevented).toBe(false);
    expect(touchStart).not.toHaveBeenCalled();

    expect(link.dispatchEvent(mouseEvent)).toBe(true);
    expect(mouseEvent.defaultPrevented).toBe(false);
    expect(mouseDown).not.toHaveBeenCalled();
  });

  it('keeps follow toggles as buttons', () => {
    const { container } = render(MessageMetaBar, {
      props: {
        ...baseProps,
        replyCount: 1,
        isFollowingThread: true,
        onToggleThreadFollow: vi.fn()
      }
    });

    const followButton = q(container, 'button[title="Unfollow thread"]');

    expect(followButton).not.toBeNull();
    expect(followButton?.closest('a')).toBeNull();
  });

  it('shows reaction tooltips with the readable reaction name and reacting users', () => {
    const { container } = render(MessageMetaBar, {
      props: {
        ...baseProps,
        reactions: [reaction()]
      }
    });

    const wrapper = q(container, 'button[aria-label="Add 👍 reaction (2)"]')!
      .parentElement as HTMLElement;

    wrapper.dispatchEvent(new MouseEvent('mouseenter'));
    flushSync();

    const tooltip = q(container, '[role="tooltip"]')!;
    const reactionName = q(tooltip, 'strong')!;

    expect(tooltip.textContent?.trim()).toBe('Thumbs up · Alice, Bob');
    expect(reactionName.textContent?.trim()).toBe('Thumbs up');
    expect(reactionName.classList.contains('font-semibold')).toBe(true);
  });

  it('keeps the reaction tooltip available when the reaction button is disabled', () => {
    const { container } = render(MessageMetaBar, {
      props: {
        ...baseProps,
        reactions: [reaction({ emoji: 'heart', count: 1, users: [{ id: 'user-1', displayName: 'Alice' }] })],
        canReact: false
      }
    });

    const button = q(container, 'button[aria-label="Add ❤️ reaction (1)"]')! as HTMLButtonElement;
    const wrapper = button.parentElement as HTMLElement;

    expect(button.disabled).toBe(true);

    wrapper.dispatchEvent(new MouseEvent('mouseenter'));
    flushSync();

    const tooltip = q(container, '[role="tooltip"]')!;
    expect(tooltip.textContent?.trim()).toBe('Heart · Alice');
  });
});
