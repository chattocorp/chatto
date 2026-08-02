import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { flushSync, tick } from 'svelte';
import { render } from 'vitest-browser-svelte';
import { PresenceStatus } from '@chatto/api-types/api/v1/presence_pb';
import { TimelineEventKind, type TimelineEventView } from '$lib/render/timelineEvents';
import { q } from '$lib/test-utils';
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
      replyCount: 0,
      lastReplyAt: null,
      threadParticipants: [],
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
  await vi.waitFor(() => expect(menuButton(container, 'Copy link')).toBeTruthy());
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
  window.getSelection()?.removeAllRanges();
});

describe('MessageEvent action model integration', () => {
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
        threadRootEventId: 'thread-root'
      })
    );

    await openContextMenu(rendered.container);
    menuButton(rendered.container, 'Copy link')!.click();
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
    await rendered.rerender({ event: echo, permalinkThreadRootEventId: null });
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
    await vi.waitFor(() => expect(actionSheetButton(rendered.container, 'Copy link')).toBeTruthy());
    expect(actionSheetButton(rendered.container, 'Edit')).toBeUndefined();
    expect(actionSheetButton(rendered.container, 'Delete')).toBeUndefined();
    expect(q(rendered.container, 'dialog[open] button[aria-label="React with 👍"]')).toBeNull();
  });
});
