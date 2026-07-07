import { describe, expect, it, vi } from 'vitest';
import { flushSync } from 'svelte';
import { render } from 'vitest-browser-svelte';
import { q } from '$lib/test-utils';
import MessageMetaBar from './MessageMetaBar.svelte';

vi.mock('$app/paths', () => ({
  assets: '',
  base: '',
  resolve: (path: string, params?: Record<string, string>) =>
    path
      .replace('[serverId]', params?.serverId ?? '')
      .replace('[roomId]', params?.roomId ?? '')
      .replace('[threadId]', params?.threadId ?? '')
}));

vi.mock('$lib/render/data', () => ({
  useRenderData: (_document: unknown, value: unknown) => value
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
    let preventedByComponent: boolean | undefined;
    link.addEventListener('click', (event) => {
      preventedByComponent = event.defaultPrevented;
      event.preventDefault();
    });
    const event = new MouseEvent('click', {
      bubbles: true,
      cancelable: true,
      button: 0,
      metaKey: true
    });

    link.dispatchEvent(event);

    expect(preventedByComponent).toBe(false);
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
    const userNames = Array.from(
      tooltip.querySelectorAll<HTMLElement>('[data-testid="reaction-tooltip-user"]')
    ).map((el) => el.textContent?.trim());

    expect(reactionName.textContent?.trim()).toBe('Thumbs up');
    expect(userNames).toEqual(['Alice', 'Bob']);
    expect(tooltip.classList.contains('menu')).toBe(true);
    expect(q(tooltip, '.menu-section')).not.toBeNull();
    expect(
      Array.from(
        tooltip.querySelectorAll<HTMLElement>('[data-testid="reaction-tooltip-user"]')
      ).every((el) => el.classList.contains('break-words'))
    ).toBe(true);
    expect(reactionName.classList.contains('font-semibold')).toBe(true);
    expect(tooltip.innerHTML).not.toContain('whitespace-nowrap');
  });

  it('caps long reacting user lists and summarizes the remaining users', () => {
    const { container } = render(MessageMetaBar, {
      props: {
        ...baseProps,
        reactions: [
          reaction({
            count: 72,
            users: [
              { id: 'user-1', displayName: 'Azerbaijan' },
              { id: 'user-2', displayName: 'German_Noob_With_An_Absurdly_Long_Name' },
              { id: 'user-3', displayName: '2tap2b' },
              { id: 'user-4', displayName: 'muchtin' },
              { id: 'user-5', displayName: 'patry' }
            ]
          })
        ]
      }
    });

    const wrapper = q(container, 'button[aria-label="Add 👍 reaction (72)"]')!
      .parentElement as HTMLElement;

    wrapper.dispatchEvent(new MouseEvent('mouseenter'));
    flushSync();

    const tooltip = q(container, '[role="tooltip"]')!;
    const content = q(tooltip, '.menu-section')!;
    const reactingUsers = q(tooltip, 'span.text-muted')!;
    const userNames = Array.from(
      tooltip.querySelectorAll<HTMLElement>('[data-testid="reaction-tooltip-user"]')
    ).map((el) => el.textContent?.trim());

    expect(content.classList.contains('min-w-0')).toBe(true);
    expect(tooltip.classList.contains('menu')).toBe(true);
    expect(tooltip.classList.contains('w-64')).toBe(true);
    expect(reactingUsers.classList.contains('min-w-0')).toBe(true);
    expect(userNames).toEqual([
      'Azerbaijan',
      'German_Noob_With_An_Absurdly_Long_Name',
      '2tap2b',
      'muchtin',
      'patry'
    ]);
    expect(reactingUsers.textContent).toContain('+ 67 more');
  });

  it('keeps the reaction tooltip available when the reaction button is disabled', () => {
    const { container } = render(MessageMetaBar, {
      props: {
        ...baseProps,
        reactions: [
          reaction({ emoji: 'heart', count: 1, users: [{ id: 'user-1', displayName: 'Alice' }] })
        ],
        canReact: false
      }
    });

    const button = q(container, 'button[aria-label="Add ❤️ reaction (1)"]')! as HTMLButtonElement;
    const wrapper = button.parentElement as HTMLElement;

    expect(button.disabled).toBe(true);

    wrapper.dispatchEvent(new MouseEvent('mouseenter'));
    flushSync();

    const tooltip = q(container, '[role="tooltip"]')!;
    expect(q(tooltip, 'strong')?.textContent?.trim()).toBe('Heart');
    expect(q(tooltip, '[data-testid="reaction-tooltip-user"]')?.textContent?.trim()).toBe('Alice');
  });
});
