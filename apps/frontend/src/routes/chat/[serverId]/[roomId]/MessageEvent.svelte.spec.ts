import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { flushSync, tick } from 'svelte';
import { render } from 'vitest-browser-svelte';
import { PresenceStatus } from '@chatto/api-types/api/v1/presence_pb';
import { TimelineEventKind, type TimelineEventView } from '$lib/render/timelineEvents';
import { q } from '$lib/test-utils';
import { RoomThreadingMode } from '$lib/roomThreading';
import MessageEventTestHarness from './MessageEventTestHarness.svelte';

const mocks = vi.hoisted(() => ({
  actions: {
    addReaction: vi.fn(),
    removeReaction: vi.fn(),
    toggleReaction: vi.fn(),
    startEdit: vi.fn(),
    openDeleteConfirmation: vi.fn(),
    copyMessageText: vi.fn(),
    copyMessageLink: vi.fn()
  }
}));

vi.mock('$lib/hooks', async (importOriginal) => {
  const actual = await importOriginal<typeof import('$lib/hooks')>();
  return { ...actual, useMessageActions: () => mocks.actions };
});

vi.mock('$lib/components/messages/MessageView.svelte', async () => {
  const { default: MessageViewActionTestSurface } =
    await import('./MessageViewActionTestSurface.svelte');
  return { default: MessageViewActionTestSurface };
});

vi.mock('$lib/utils/inputCapabilities', async (importOriginal) => {
  const actual = await importOriginal<typeof import('$lib/utils/inputCapabilities')>();
  return {
    ...actual,
    prefersTouchActions: () => false,
    supportsHoverActions: () => true
  };
});

vi.mock('$app/paths', () => ({
  assets: '',
  base: '',
  resolve: (path: string, params?: Record<string, string>) =>
    path
      .replace('[serverId]', params?.serverId ?? '')
      .replace('[roomId]', params?.roomId ?? '')
      .replace('[threadId]', params?.threadId ?? '')
      .replace('[messageId]', params?.messageId ?? '')
}));

type MessageOverrides = Partial<{
  id: string;
  actorId: string;
  body: string;
  threadRootEventId: string | null;
  echoOfEventId: string | null;
  echoFromThreadRootEventId: string | null;
  channelEchoEventId: string | null;
  threadExists: boolean;
  replyCount: number;
}>;

function messageEvent(overrides: MessageOverrides = {}): TimelineEventView {
  const actorId = overrides.actorId ?? 'viewer';
  return {
    id: overrides.id ?? 'regular-message',
    actorId,
    actor: {
      id: actorId,
      login: actorId,
      displayName: actorId,
      deleted: false,
      avatarUrl: null,
      presenceStatus: PresenceStatus.OFFLINE
    },
    createdAt: new Date().toISOString(),
    event: {
      kind: TimelineEventKind.MessagePosted,
      roomId: 'room-1',
      body: overrides.body ?? 'Hello from this message',
      attachments: [],
      linkPreview: null,
      reactions: [
        {
          emoji: 'thumbsup',
          count: 1,
          hasReacted: true,
          users: [{ id: 'viewer', displayName: 'viewer' }]
        }
      ],
      updatedAt: null,
      inReplyTo: null,
      threadRootEventId: overrides.threadRootEventId ?? null,
      echoOfEventId: overrides.echoOfEventId ?? null,
      echoFromThreadRootEventId: overrides.echoFromThreadRootEventId ?? null,
      channelEchoEventId: overrides.channelEchoEventId ?? null,
      replyCount: overrides.replyCount ?? 0,
      lastReplyAt: null,
      threadParticipants: [],
      threadExists: overrides.threadExists ?? false,
      viewerIsFollowingThread: false
    }
  } as TimelineEventView;
}

function menuButton(container: HTMLElement, label: string): HTMLButtonElement | undefined {
  return Array.from(container.querySelectorAll<HTMLButtonElement>('button')).find(
    (button) => button.textContent?.trim() === label
  );
}

function actionSheetButton(container: HTMLElement, label: string): HTMLButtonElement | undefined {
  return Array.from(container.querySelectorAll<HTMLButtonElement>('dialog[open] button')).find(
    (button) => button.textContent?.trim() === label
  );
}

async function openContextMenu(container: HTMLElement): Promise<void> {
  q(container, '[data-testid="message-row"]')!.dispatchEvent(
    new MouseEvent('contextmenu', {
      bubbles: true,
      cancelable: true,
      clientX: 80,
      clientY: 120
    })
  );
  await vi.waitFor(() => expect(menuButton(container, 'Copy message link')).toBeTruthy());
}

async function selectPickerEmoji(
  container: HTMLElement,
  query: string,
  title: string
): Promise<void> {
  const input = q(container, 'input[placeholder="Search emojis..."]') as HTMLInputElement;
  input.value = query;
  input.dispatchEvent(new Event('input', { bubbles: true }));
  flushSync();
  await tick();
  (q(container, `button[title="${title}"]`) as HTMLButtonElement).click();
}

beforeEach(() => {
  vi.clearAllMocks();
  for (const action of Object.values(mocks.actions)) action.mockResolvedValue(undefined);
});

afterEach(() => {
  vi.useRealTimers();
  vi.restoreAllMocks();
  window.getSelection()?.removeAllRanges();
});

describe('MessageEvent action model integration', () => {
  it('shows Copy Link only for a right-clicked message-body link', async () => {
    const writeText = vi.spyOn(navigator.clipboard, 'writeText').mockResolvedValue(undefined);
    const rendered = render(MessageEventTestHarness, { props: { event: messageEvent() } });
    const body = q(rendered.container, '[data-testid="message-body"]')!;
    body.innerHTML = '<a href="/linked/path"><strong>Linked text</strong></a>';
    const link = q(body, 'a') as HTMLAnchorElement;
    const click = new MouseEvent('contextmenu', {
      bubbles: true,
      cancelable: true,
      clientX: 80,
      clientY: 120
    });

    q(body, 'strong')!.dispatchEvent(click);

    expect(click.defaultPrevented).toBe(true);
    await vi.waitFor(() => expect(menuButton(rendered.container, 'Copy link')).toBeTruthy());
    expect(menuButton(rendered.container, 'Copy message link')).toBeTruthy();

    menuButton(rendered.container, 'Copy link')!.click();
    await vi.waitFor(() => expect(writeText).toHaveBeenCalledWith(link.href));
    await vi.waitFor(() => expect(menuButton(rendered.container, 'Copy link')).toBeUndefined());

    await openContextMenu(rendered.container);
    expect(menuButton(rendered.container, 'Copy link')).toBeUndefined();

    q(body, 'strong')!.dispatchEvent(
      new MouseEvent('contextmenu', { bubbles: true, cancelable: true })
    );
    await vi.waitFor(() => expect(menuButton(rendered.container, 'Copy link')).toBeTruthy());
    (q(rendered.container, 'button[aria-label="More actions"]') as HTMLButtonElement).click();
    await vi.waitFor(() => expect(menuButton(rendered.container, 'Copy link')).toBeUndefined());

    expect(link.href).toBe(new URL('/linked/path', window.location.href).href);
  });

  it('drops a clicked link when a virtualized row changes message', async () => {
    const firstMessage = messageEvent();
    const rendered = render(MessageEventTestHarness, { props: { event: firstMessage } });
    const body = q(rendered.container, '[data-testid="message-body"]')!;
    body.innerHTML = '<a href="https://example.com"><span>Link</span></a>';
    q(body, 'span')!.dispatchEvent(
      new MouseEvent('contextmenu', { bubbles: true, cancelable: true })
    );
    await vi.waitFor(() => expect(menuButton(rendered.container, 'Copy link')).toBeTruthy());

    await rendered.rerender({ event: messageEvent({ id: 'next-message' }) });

    await vi.waitFor(() => expect(menuButton(rendered.container, 'Copy link')).toBeUndefined());
    expect(menuButton(rendered.container, 'Copy message link')).toBeTruthy();

    await rendered.rerender({ event: firstMessage });
    expect(menuButton(rendered.container, 'Copy link')).toBeUndefined();
  });

  it('shows an Echo link only for an echoed reply in the thread pane', async () => {
    const reply = messageEvent({
      id: 'thread-reply',
      threadRootEventId: 'thread-root',
      channelEchoEventId: 'echo-wrapper'
    });
    const rendered = render(MessageEventTestHarness, {
      props: { event: reply, permalinkThreadRootEventId: 'thread-root' }
    });

    const echoLinkSelector = 'a[href$="/room-1/m/echo-wrapper"]';
    await expect.element(q(rendered.container, echoLinkSelector)).toHaveTextContent('Echo');

    await rendered.rerender({
      event: messageEvent({ id: 'thread-reply', threadRootEventId: 'thread-root' }),
      permalinkThreadRootEventId: 'thread-root'
    });
    await expect.element(q(rendered.container, echoLinkSelector)).not.toBeInTheDocument();

    const echo = messageEvent({
      id: 'echo-wrapper',
      echoOfEventId: 'thread-reply',
      echoFromThreadRootEventId: 'thread-root'
    });
    await rendered.rerender({ event: echo, permalinkThreadRootEventId: null });
    await expect.element(q(rendered.container, echoLinkSelector)).not.toBeInTheDocument();
  });

  it('orders and constrains reply actions for each threading mode', async () => {
    const onOpenThread = vi.fn();
    const event = messageEvent({ threadExists: true });
    const rendered = render(MessageEventTestHarness, {
      props: { event, onOpenThread, threadingMode: RoomThreadingMode.REQUIRED }
    });

    const toolbarReplyLabels = () =>
      Array.from(
        q(rendered.container, '[role="toolbar"]')!.querySelectorAll<HTMLButtonElement>('button')
      )
        .map((button) => button.getAttribute('aria-label'))
        .filter((label) => label?.startsWith('Reply') || label === 'Open thread');

    expect(toolbarReplyLabels()).toEqual(['Reply', 'Reply in thread']);
    (q(rendered.container, 'button[aria-label="Reply"]') as HTMLButtonElement).click();
    expect(onOpenThread).toHaveBeenLastCalledWith(
      event.id,
      expect.objectContaining({
        reply: expect.objectContaining({ eventId: event.id })
      })
    );

    await rendered.rerender({
      event,
      onOpenThread,
      threadingMode: RoomThreadingMode.ENCOURAGED
    });
    expect(toolbarReplyLabels()).toEqual(['Reply', 'Reply in thread']);
    (q(rendered.container, 'button[aria-label="Reply"]') as HTMLButtonElement).click();
    expect(onOpenThread).toHaveBeenLastCalledWith(
      event.id,
      expect.objectContaining({ reply: expect.objectContaining({ eventId: event.id }) })
    );
    await openContextMenu(rendered.container);
    expect(menuButton(rendered.container, 'Reply in room')).toBeTruthy();
    menuButton(rendered.container, 'Reply in room')!.click();
    await expect
      .element(q(rendered.container, '[data-testid="active-reply-target"]'))
      .toHaveTextContent(event.id);

    await rendered.rerender({
      event,
      onOpenThread,
      threadingMode: RoomThreadingMode.ENABLED
    });
    expect(toolbarReplyLabels()).toEqual(['Reply', 'Reply in thread']);

    await rendered.rerender({
      event,
      onOpenThread,
      threadingMode: RoomThreadingMode.DISABLED
    });
    expect(toolbarReplyLabels()).toEqual(['Reply', 'Open thread']);

    await rendered.rerender({
      event,
      onOpenThread,
      permalinkThreadRootEventId: event.id,
      threadingMode: RoomThreadingMode.DISABLED
    });
    expect(toolbarReplyLabels()).toEqual([]);
  });

  it.each([RoomThreadingMode.ENABLED, RoomThreadingMode.ENCOURAGED, RoomThreadingMode.REQUIRED])(
    'starts an attributed thread reply from the context menu in %s mode',
    async (threadingMode) => {
      const onOpenThread = vi.fn();
      const event = messageEvent({ id: 'reply-target', body: 'Selected thread quote' });
      const { container } = render(MessageEventTestHarness, {
        props: { event, onOpenThread, threadingMode }
      });
      const range = document.createRange();
      range.selectNodeContents(q(container, '[data-testid="message-body"]')!);
      window.getSelection()!.addRange(range);
      q(container, '[data-testid="message-row"]')!.dispatchEvent(
        new MouseEvent('mousedown', { bubbles: true, cancelable: true, button: 2 })
      );

      await openContextMenu(container);
      menuButton(container, 'Reply in thread')!.click();

      expect(onOpenThread).toHaveBeenCalledWith(
        event.id,
        expect.objectContaining({
          quoteText: 'Selected thread quote',
          reply: expect.objectContaining({
            eventId: event.id,
            actorDisplayName: 'viewer',
            excerpt: 'Selected thread quote'
          })
        })
      );
    }
  );

  it('starts an attributed thread reply from the hover toolbar', () => {
    const onOpenThread = vi.fn();
    const event = messageEvent({ id: 'toolbar-target' });
    const { container } = render(MessageEventTestHarness, { props: { event, onOpenThread } });

    (q(container, 'button[aria-label="Reply in thread"]') as HTMLButtonElement).click();

    expect(onOpenThread).toHaveBeenCalledWith(
      event.id,
      expect.objectContaining({ reply: expect.objectContaining({ eventId: event.id }) })
    );
  });

  it('starts an attributed thread reply from the touch sheet', async () => {
    const onOpenThread = vi.fn();
    const event = messageEvent({ id: 'touch-target' });
    const { container } = render(MessageEventTestHarness, { props: { event, onOpenThread } });

    vi.useFakeTimers();
    q(container, '[data-testid="message-row"]')!.dispatchEvent(
      new Event('touchstart', { bubbles: true, cancelable: true })
    );
    vi.advanceTimersByTime(500);
    flushSync();
    vi.useRealTimers();
    await vi.waitFor(() => expect(actionSheetButton(container, 'Reply in thread')).toBeTruthy());
    actionSheetButton(container, 'Reply in thread')!.click();

    expect(onOpenThread).toHaveBeenCalledWith(
      event.id,
      expect.objectContaining({ reply: expect.objectContaining({ eventId: event.id }) })
    );
  });

  it('opens Disabled threads, thread badges, and echoes without a reply target', async () => {
    const onOpenThread = vi.fn();
    const event = messageEvent({ id: 'thread-root', threadExists: true });
    const rendered = render(MessageEventTestHarness, {
      props: { event, onOpenThread, threadingMode: RoomThreadingMode.DISABLED }
    });

    await openContextMenu(rendered.container);
    menuButton(rendered.container, 'Open thread')!.click();
    expect(onOpenThread).toHaveBeenLastCalledWith(event.id);

    const threadBadge = Array.from(
      rendered.container.querySelectorAll<HTMLAnchorElement>('a')
    ).find((link) => link.textContent?.trim() === 'Thread');
    expect(threadBadge).toBeTruthy();
    threadBadge!.click();
    expect(onOpenThread).toHaveBeenLastCalledWith(event.id);

    const echo = messageEvent({
      id: 'echo-wrapper',
      echoOfEventId: 'echoed-reply',
      echoFromThreadRootEventId: event.id
    });
    await rendered.rerender({ event: echo, threadingMode: RoomThreadingMode.ENABLED });
    await openContextMenu(rendered.container);
    menuButton(rendered.container, 'Open thread')!.click();
    expect(onOpenThread).toHaveBeenLastCalledWith(event.id);
  });

  it('keeps reply attribution available inside Required threads with thread-only permission', async () => {
    const event = messageEvent({ id: 'thread-reply', threadRootEventId: 'thread-root' });
    const { container } = render(MessageEventTestHarness, {
      props: {
        event,
        permalinkThreadRootEventId: 'thread-root',
        threadingMode: RoomThreadingMode.REQUIRED,
        canPostMessage: false,
        canPostInThread: true
      }
    });

    const reply = q(container, 'button[aria-label="Reply"]') as HTMLButtonElement;
    expect(reply).toBeTruthy();
    reply.click();
    await expect
      .element(q(container, '[data-testid="active-reply-target"]'))
      .toHaveTextContent(event.id);
  });

  it('falls back to an in-room reply in Encouraged mode without thread permission', async () => {
    const event = messageEvent();
    const onOpenThread = vi.fn();
    const { container } = render(MessageEventTestHarness, {
      props: {
        event,
        onOpenThread,
        threadingMode: RoomThreadingMode.ENCOURAGED,
        canPostMessage: true,
        canPostInThread: false
      }
    });

    expect(q(container, 'button[aria-label="Reply"]')).toBeTruthy();
    expect(q(container, 'button[aria-label="Reply in thread"]')).toBeNull();
    (q(container, 'button[aria-label="Reply"]') as HTMLButtonElement).click();
    expect(onOpenThread).not.toHaveBeenCalled();
    await expect
      .element(q(container, '[data-testid="active-reply-target"]'))
      .toHaveTextContent(event.id);
  });

  it.each([true, false])('uses resolved interaction reply authority: %s', (allowed) => {
    const event = messageEvent();
    if (event.event.kind !== TimelineEventKind.MessagePosted) throw new Error('Expected message');
    event.event.canReplyInThread = allowed;
    const { container } = render(MessageEventTestHarness, {
      props: {
        event,
        onOpenThread: vi.fn(),
        threadingMode: RoomThreadingMode.REQUIRED,
        canPostMessage: false,
        canPostInThread: !allowed
      }
    });
    expect(!!q(container, 'button[aria-label="Reply in thread"]')).toBe(allowed);
    expect(!!q(container, 'button[aria-label="Reply"]')).toBe(allowed);
  });

  it('rebinds every action surface when a virtualized row changes message shape', async () => {
    const regular = messageEvent();
    const rendered = render(MessageEventTestHarness, { props: { event: regular } });

    (
      q(rendered.container, 'button[aria-label="Remove 👍 reaction (1)"]') as HTMLButtonElement
    ).click();
    await vi.waitFor(() =>
      expect(mocks.actions.toggleReaction).toHaveBeenLastCalledWith(
        expect.objectContaining({
          serverId: 'remote-server',
          roomId: 'room-1',
          messageEventId: 'regular-message',
          eventId: 'regular-message',
          deleteEventId: 'regular-message'
        }),
        'thumbsup',
        true
      )
    );

    const threadReply = messageEvent({
      id: 'thread-reply',
      body: 'Thread reply',
      threadRootEventId: 'thread-root'
    });
    await rendered.rerender({
      event: threadReply,
      permalinkThreadRootEventId: 'thread-root'
    });
    (q(rendered.container, 'button[aria-label="Edit message"]') as HTMLButtonElement).click();
    expect(mocks.actions.startEdit).toHaveBeenLastCalledWith(
      expect.objectContaining({
        messageEventId: 'thread-reply',
        eventId: 'thread-reply',
        deleteEventId: 'thread-reply',
        permalinkThreadRootEventId: 'thread-root',
        threadRootEventId: 'thread-root',
        canAddChannelEcho: true
      })
    );

    await rendered.rerender({
      event: threadReply,
      permalinkThreadRootEventId: 'thread-root',
      threadingMode: RoomThreadingMode.DISABLED
    });
    (q(rendered.container, 'button[aria-label="Edit message"]') as HTMLButtonElement).click();
    expect(mocks.actions.startEdit).toHaveBeenLastCalledWith(
      expect.objectContaining({
        eventId: 'thread-reply',
        threadRootEventId: 'thread-root',
        canAddChannelEcho: false
      })
    );

    await openContextMenu(rendered.container);
    menuButton(rendered.container, 'Copy message link')!.click();
    await vi.waitFor(() =>
      expect(mocks.actions.copyMessageLink).toHaveBeenLastCalledWith(
        expect.objectContaining({
          messageEventId: 'thread-reply',
          permalinkThreadRootEventId: 'thread-root'
        })
      )
    );

    const echo = messageEvent({
      id: 'echo-wrapper',
      body: 'Channel echo',
      echoOfEventId: 'original-thread-message',
      echoFromThreadRootEventId: 'thread-root'
    });
    await rendered.rerender({
      event: echo,
      permalinkThreadRootEventId: null,
      threadingMode: RoomThreadingMode.DISABLED
    });
    (q(rendered.container, 'button[aria-label="Edit message"]') as HTMLButtonElement).click();
    expect(mocks.actions.startEdit).toHaveBeenLastCalledWith(
      expect.objectContaining({
        messageEventId: 'echo-wrapper',
        eventId: 'original-thread-message',
        deleteEventId: 'echo-wrapper',
        threadRootEventId: 'thread-root',
        channelEchoEventId: 'echo-wrapper',
        canAddChannelEcho: true
      })
    );

    await openContextMenu(rendered.container);
    menuButton(rendered.container, 'Delete')!.click();
    expect(mocks.actions.openDeleteConfirmation).toHaveBeenLastCalledWith(
      expect.objectContaining({ eventId: 'original-thread-message', deleteEventId: 'echo-wrapper' })
    );

    (q(rendered.container, 'button[aria-label="Add reaction"]') as HTMLButtonElement).click();
    await vi.waitFor(() =>
      expect(q(rendered.container, 'input[placeholder="Search emojis..."]')).toBeTruthy()
    );
    await selectPickerEmoji(rendered.container, 'check', 'white_check_mark');
    await vi.waitFor(() =>
      expect(mocks.actions.toggleReaction).toHaveBeenLastCalledWith(
        expect.objectContaining({
          messageEventId: 'echo-wrapper',
          eventId: 'original-thread-message'
        }),
        '✅',
        false
      )
    );
  });

  it('keeps selected-text replies current and updates permissions in the touch surface', async () => {
    const onOpenThread = vi.fn();
    const echo = messageEvent({
      id: 'echo-wrapper',
      body: 'Quote this selection',
      echoOfEventId: 'original-thread-message',
      echoFromThreadRootEventId: 'thread-root'
    });
    const rendered = render(MessageEventTestHarness, { props: { event: echo, onOpenThread } });
    const body = q(rendered.container, '[data-testid="message-body"]')!;
    const range = document.createRange();
    range.selectNodeContents(body);
    window.getSelection()!.addRange(range);

    q(rendered.container, '[data-testid="message-row"]')!.dispatchEvent(
      new MouseEvent('mousedown', { bubbles: true, cancelable: true, button: 2 })
    );
    await openContextMenu(rendered.container);
    menuButton(rendered.container, 'Reply in thread')!.click();
    expect(onOpenThread).toHaveBeenCalledWith(
      'thread-root',
      expect.objectContaining({
        highlightEventId: 'original-thread-message',
        quoteText: 'Quote this selection',
        reply: expect.objectContaining({ eventId: 'original-thread-message' })
      })
    );

    const otherUsersMessage = messageEvent({ id: 'other-message', actorId: 'other-user' });
    await rendered.rerender({ event: otherUsersMessage, canReact: false, onOpenThread });

    expect(q(rendered.container, 'button[aria-label="Edit message"]')).toBeNull();
    expect(
      (q(rendered.container, 'button[aria-label="Remove 👍 reaction (1)"]') as HTMLButtonElement)
        .disabled
    ).toBe(true);

    vi.useFakeTimers();
    q(rendered.container, '[data-testid="message-row"]')!.dispatchEvent(
      new Event('touchstart', { bubbles: true, cancelable: true })
    );
    vi.advanceTimersByTime(500);
    flushSync();
    vi.useRealTimers();
    await vi.waitFor(() => expect(actionSheetButton(rendered.container, 'Copy message link')).toBeTruthy());
    expect(actionSheetButton(rendered.container, 'Edit')).toBeUndefined();
    expect(actionSheetButton(rendered.container, 'Delete')).toBeUndefined();
    expect(q(rendered.container, 'dialog[open] button[aria-label="React with 👍"]')).toBeNull();
  });

  it('updates the pin indicator for viewers who can view but cannot manage room pins', async () => {
    const event = messageEvent({ id: 'remotely-pinned-message' });
    const rendered = render(MessageEventTestHarness, {
      props: { event, canViewPinnedMessages: true, canPinMessages: false, pinStatus: false }
    });

    expect(q(rendered.container, '[role="img"][aria-label="Pinned message"]')).toBeNull();

    await rendered.rerender({
      event,
      canViewPinnedMessages: true,
      canPinMessages: false,
      pinStatus: true
    });
    await vi.waitFor(() =>
      expect(q(rendered.container, '[role="img"][aria-label="Pinned message"]')).not.toBeNull()
    );
    expect(q(rendered.container, 'button[aria-label="Unpin message"]')).toBeNull();

    await rendered.rerender({
      event,
      canViewPinnedMessages: true,
      canPinMessages: false,
      pinStatus: false
    });
    await vi.waitFor(() =>
      expect(q(rendered.container, '[role="img"][aria-label="Pinned message"]')).toBeNull()
    );
  });
});
