import { MessageService } from '@chatto/api-types/api/v1/messages_connect';
import { RoomService } from '@chatto/api-types/api/v1/rooms_connect';
import { ThreadService } from '@chatto/api-types/api/v1/threads_connect';
import { UserService } from '@chatto/api-types/api/v1/user_service_connect';
import { MessagesStore } from '@chatto/client/room/messages/MessagesStore';
import { mockService, receivedRequest } from '@chatto/client/testing/fakeServer';
import { createTestServerScope } from '$lib/test-utils/serverScope.svelte';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { flushSync, tick } from 'svelte';
import { render } from 'vitest-browser-svelte';
import { TimelineEventKind } from '@chatto/client/timeline/timelineEvents';
import type { MessageAttachmentView } from '@chatto/client/timeline/messageAttachments';
import { q } from '$lib/test-utils';
import { toast } from '$lib/ui/toast';
import { RoomThreadingMode } from '@chatto/client/util/roomThreading';
import MessageEventTestHarness from './MessageEventTestHarness.svelte';
import { messageEvent } from './messageEventFixture';
import { MessageActionOverlayState } from './messageActionOverlayState.svelte';

const mocks = vi.hoisted(() => ({
  touchPrimary: false,
  copyImageToClipboard: vi.fn(),
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

vi.mock('$lib/attachments/copyImage', () => ({
  copyImageToClipboard: mocks.copyImageToClipboard
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
    prefersTouchActions: () => mocks.touchPrimary,
    supportsHoverActions: () => !mocks.touchPrimary
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
  mocks.touchPrimary = false;
  vi.clearAllMocks();
  for (const action of Object.values(mocks.actions)) action.mockResolvedValue(undefined);
});

afterEach(() => {
  vi.useRealTimers();
  // A long press under fake timers leaves its opening-click guard; a new press clears it.
  window.dispatchEvent(new Event('pointerdown'));
  vi.restoreAllMocks();
  window.getSelection()?.removeAllRanges();
});

describe('MessageEvent action model integration', () => {
  it('copies the exact selected text above full-message copy after focus loss and row unmount', async () => {
    const writeText = vi.spyOn(navigator.clipboard, 'writeText').mockResolvedValue(undefined);
    const event = messageEvent({ body: 'Before  selected text  after' });
    const rendered = render(MessageEventTestHarness, { props: { event } });
    const body = q(rendered.container, '[data-testid="message-body"]')!;
    const range = document.createRange();
    range.setStart(body.firstChild!, 6);
    range.setEnd(body.firstChild!, 23);
    window.getSelection()!.addRange(range);
    const selectedText = window.getSelection()!.toString();
    expect(selectedText).toBe(' selected text ');

    q(rendered.container, '[data-testid="message-row"]')!.dispatchEvent(
      new MouseEvent('mousedown', { bubbles: true, button: 2 })
    );
    window.getSelection()!.removeAllRanges();
    await openContextMenu(rendered.container);
    await rendered.rerender({ event, showMessage: false });

    const labels = Array.from(rendered.container.querySelectorAll('.menu-entry')).map((button) =>
      button.textContent?.trim()
    );
    const index = labels.indexOf('Copy selected text');
    expect(index).toBeGreaterThanOrEqual(0);
    expect(labels[index + 1]).toBe('Copy message text');
    menuButton(rendered.container, 'Copy selected text')!.click();
    await vi.waitFor(() => expect(writeText).toHaveBeenCalledExactlyOnceWith(selectedText));
    expect(mocks.actions.copyMessageText).not.toHaveBeenCalled();
    await vi.waitFor(() =>
      expect(menuButton(rendered.container, 'Copy selected text')).toBeUndefined()
    );
  });

  it.each(['none', 'collapsed', 'other message', 'cross-message', 'metadata', 'changed selection'])(
    'hides selected-text copy for a %s selection',
    async (kind) => {
      const rendered = render(MessageEventTestHarness, { props: { event: messageEvent() } });
      const other = render(MessageEventTestHarness, {
        props: { event: messageEvent({ id: 'other-message', body: 'Other message' }) }
      });
      const body = q(rendered.container, '[data-testid="message-body"]')!;
      const otherBody = q(other.container, '[data-testid="message-body"]')!;
      if (kind === 'changed selection') {
        const range = document.createRange();
        range.selectNodeContents(body);
        window.getSelection()!.addRange(range);
        q(rendered.container, '[data-testid="message-row"]')!.dispatchEvent(
          new MouseEvent('mouseup', { bubbles: true, button: 0 })
        );
        window.getSelection()!.removeAllRanges();
      }
      if (kind !== 'none') {
        const range = document.createRange();
        range.selectNodeContents(
          kind === 'other message' || kind === 'changed selection' ? otherBody : body
        );
        if (kind === 'collapsed') range.collapse(true);
        if (kind === 'cross-message') range.setEnd(otherBody.firstChild!, 5);
        if (kind === 'metadata') {
          range.selectNodeContents(q(rendered.container, '[data-testid="message-row"]')!);
        }
        window.getSelection()!.addRange(range);
      }

      await openContextMenu(rendered.container);
      expect(menuButton(rendered.container, 'Copy selected text')).toBeUndefined();
      expect(menuButton(rendered.container, 'Copy message text')).toBeTruthy();
    }
  );

  it('reports a selected-text clipboard failure and closes the menu', async () => {
    vi.spyOn(navigator.clipboard, 'writeText').mockRejectedValue(new Error('Clipboard denied'));
    const showError = vi.spyOn(toast, 'error');
    const rendered = render(MessageEventTestHarness, { props: { event: messageEvent() } });
    const range = document.createRange();
    range.selectNodeContents(q(rendered.container, '[data-testid="message-body"]')!);
    window.getSelection()!.addRange(range);
    await openContextMenu(rendered.container);

    menuButton(rendered.container, 'Copy selected text')!.click();
    await vi.waitFor(() => expect(showError).toHaveBeenCalledOnce());
    await vi.waitFor(() =>
      expect(menuButton(rendered.container, 'Copy selected text')).toBeUndefined()
    );
  });

  it('copies a selection from the touch action sheet', async () => {
    const writeText = vi.spyOn(navigator.clipboard, 'writeText').mockResolvedValue(undefined);
    const { container } = render(MessageEventTestHarness, {
      props: { event: messageEvent({ body: 'Touch selection' }) }
    });
    const range = document.createRange();
    range.selectNodeContents(q(container, '[data-testid="message-body"]')!);
    window.getSelection()!.addRange(range);
    vi.useFakeTimers();
    q(container, '[data-testid="message-row"]')!.dispatchEvent(
      new Event('touchstart', { bubbles: true, cancelable: true })
    );
    vi.advanceTimersByTime(500);
    flushSync();
    vi.useRealTimers();
    await vi.waitFor(() => expect(actionSheetButton(container, 'Copy selected text')).toBeTruthy());
    actionSheetButton(container, 'Copy selected text')!.click();
    await vi.waitFor(() => expect(writeText).toHaveBeenCalledExactlyOnceWith('Touch selection'));
  });

  it('does not reuse a selected-text snapshot when the menu reopens', async () => {
    const rendered = render(MessageEventTestHarness, { props: { event: messageEvent() } });
    const range = document.createRange();
    range.selectNodeContents(q(rendered.container, '[data-testid="message-body"]')!);
    window.getSelection()!.addRange(range);
    await openContextMenu(rendered.container);
    expect(menuButton(rendered.container, 'Copy selected text')).toBeTruthy();
    window.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape', bubbles: true }));
    await vi.waitFor(() =>
      expect(menuButton(rendered.container, 'Copy message link')).toBeUndefined()
    );

    await openContextMenu(rendered.container);
    expect(menuButton(rendered.container, 'Copy selected text')).toBeUndefined();
  });

  it('preserves full-message copy when text is selected', async () => {
    const body = '**Full message** with a selection';
    const rendered = render(MessageEventTestHarness, { props: { event: messageEvent({ body }) } });
    const range = document.createRange();
    range.selectNodeContents(q(rendered.container, '[data-testid="message-body"]')!);
    window.getSelection()!.addRange(range);
    await openContextMenu(rendered.container);

    menuButton(rendered.container, 'Copy message text')!.click();
    expect(mocks.actions.copyMessageText).toHaveBeenCalledWith(
      expect.objectContaining({ messageBody: body })
    );
  });

  it('keeps selected-text copy available from the toolbar after selection collapse', async () => {
    const writeText = vi.spyOn(navigator.clipboard, 'writeText').mockResolvedValue(undefined);
    const rendered = render(MessageEventTestHarness, {
      props: { event: messageEvent({ body: 'Toolbar selection' }) }
    });
    const range = document.createRange();
    range.selectNodeContents(q(rendered.container, '[data-testid="message-body"]')!);
    window.getSelection()!.addRange(range);
    q(rendered.container, '[data-testid="message-row"]')!.dispatchEvent(
      new MouseEvent('mouseup', { bubbles: true, button: 0 })
    );
    window.getSelection()!.removeAllRanges();
    (q(rendered.container, 'button[aria-label="More actions"]') as HTMLButtonElement).click();
    await vi.waitFor(() =>
      expect(menuButton(rendered.container, 'Copy selected text')).toBeTruthy()
    );
    menuButton(rendered.container, 'Copy selected text')!.click();
    await vi.waitFor(() => expect(writeText).toHaveBeenCalledWith('Toolbar selection'));
  });

  it('shows an image label after lazily loading an image-only reply target', async () => {
    const messages = mockService(MessageService);
    const rooms = mockService(RoomService);
    const users = mockService(UserService);
    rooms.getRoomEvents.mockReturnValue({});
    messages.getMessage.mockReturnValue({
      message: {
        id: 'image-target',
        roomId: 'room-1',
        actorId: 'viewer',
        body: '',
        attachments: [{ id: 'image-attachment', filename: 'upload.png', contentType: 'image/png' }]
      }
    });
    users.batchGetUsers.mockReturnValue({
      users: [{ user: { id: 'viewer', login: 'isotopp', displayName: 'Isotopp' } }]
    });
    const server = createTestServerScope({
      routes: (router) =>
        router
          .service(MessageService, messages)
          .service(RoomService, rooms)
          .service(UserService, users)
    });
    const messageStore = new MessagesStore(server.scope.connection, () => null, {
      roomId: 'room-1'
    });
    const reply = messageEvent({ id: 'image-reply' });
    if (reply.event.kind !== TimelineEventKind.MessagePosted) throw new Error('Expected a message');
    reply.event.inReplyTo = 'image-target';
    const rendered = render(MessageEventTestHarness, { props: { event: reply, messageStore } });
    try {
      await expect
        .element(rendered.getByRole('button', { name: 'in reply to Image', exact: true }))
        .toBeInTheDocument();
      await expect
        .element(q(rendered.container, '[data-testid="reply-attribution"]'))
        .toHaveTextContent('Isotopp');
      expect(messages.getMessage).toHaveBeenCalledOnce();
      expect(receivedRequest(messages.getMessage)).toMatchObject({
        roomId: 'room-1',
        eventId: 'image-target'
      });
      expect(q(rendered.container, '[data-testid="reply-attribution"] img')).toBeNull();
    } finally {
      await rendered.unmount();
      messageStore.dispose();
    }
  });

  it('lazily resolves an echo reply to a non-root message in an unopened thread', async () => {
    const messages = mockService(MessageService);
    const rooms = mockService(RoomService);
    const threads = mockService(ThreadService);
    const users = mockService(UserService);
    rooms.getRoomEvents.mockReturnValue({});
    let finishRead!: () => void;
    messages.getMessage.mockImplementation(
      () =>
        new Promise((resolve) => {
          finishRead = () =>
            resolve({
              message: {
                id: 'thread-target',
                roomId: 'room-1',
                threadRootEventId: 'thread-root',
                actorId: 'target-user',
                body: 'The original thread reply'
              }
            });
        })
    );
    users.batchGetUsers.mockReturnValue({
      users: [{ user: { id: 'target-user', login: 'target', displayName: 'Target User' } }]
    });
    const server = createTestServerScope({
      routes: (router) =>
        router
          .service(MessageService, messages)
          .service(RoomService, rooms)
          .service(ThreadService, threads)
          .service(UserService, users)
    });
    const messageStore = new MessagesStore(server.scope.connection, () => null, {
      roomId: 'room-1'
    });
    const echo = messageEvent({
      id: 'echo',
      echoOfEventId: 'echoed-reply',
      echoFromThreadRootEventId: 'thread-root'
    });
    if (echo.event.kind !== TimelineEventKind.MessagePosted) throw new Error('Expected a message');
    echo.event.inReplyTo = 'thread-target';
    const onOpenThread = vi.fn();
    const rendered = render(MessageEventTestHarness, {
      props: { event: echo, messageStore, onOpenThread }
    });
    try {
      await expect
        .element(q(rendered.container, '[aria-label="in reply to a message"]'))
        .toBeInTheDocument();
      await vi.waitFor(() => expect(messages.getMessage).toHaveBeenCalledOnce());
      finishRead();
      await expect
        .element(q(rendered.container, '[data-testid="reply-attribution"]'))
        .toHaveTextContent('The original thread reply');
      await expect
        .element(q(rendered.container, '[data-testid="reply-attribution"]'))
        .toHaveTextContent('Target User');
      expect(receivedRequest(messages.getMessage)).toMatchObject({
        roomId: 'room-1',
        eventId: 'thread-target'
      });
      expect(rooms.getRoomEventsAround).not.toHaveBeenCalled();
      expect(threads.getThreadEvents).not.toHaveBeenCalled();
      expect(threads.getThreadEventsAround).not.toHaveBeenCalled();
      expect(messageStore.events).toEqual([]);
      q(rendered.container, '[aria-label="in reply to The original thread reply"]')!.click();
      expect(onOpenThread).toHaveBeenCalledWith('thread-root', {
        highlightEventId: 'thread-target'
      });
    } finally {
      await rendered.unmount();
      messageStore.dispose();
    }
  });

  it('opens the target user menu on a mention right-click', async () => {
    const rendered = render(MessageEventTestHarness, { props: { event: messageEvent() } });
    const body = q(rendered.container, '[data-testid="message-body"]')!;
    body.innerHTML =
      '<span class="mention" data-user-id="target-user"><bdi>@Target User</bdi></span>';
    const contextMenu = new MouseEvent('contextmenu', { bubbles: true, cancelable: true });

    q(body, 'bdi')!.dispatchEvent(contextMenu);

    expect(contextMenu.defaultPrevented).toBe(true);
    await vi.waitFor(() =>
      expect(document.querySelector('[role="dialog"]')?.textContent).toContain('Target User')
    );
    expect(menuButton(rendered.container, 'Copy message link')).toBeUndefined();
  });

  it('keeps the user menu open when its source message unmounts', async () => {
    const event = messageEvent();
    const rendered = render(MessageEventTestHarness, { props: { event } });
    const body = q(rendered.container, '[data-testid="message-body"]')!;
    body.innerHTML = '<span class="mention" data-user-id="target-user">@Target User</span>';
    q(body, '.mention')!.dispatchEvent(
      new MouseEvent('contextmenu', { bubbles: true, cancelable: true })
    );
    await vi.waitFor(() =>
      expect(document.querySelector('[role="dialog"]')?.textContent).toContain('Target User')
    );

    await rendered.rerender({ event, showMessage: false });

    expect(rendered.container.querySelector('[data-testid="message-row"]')).toBeNull();
    expect(document.querySelector('[role="dialog"]')?.textContent).toContain('Target User');
  });

  it('keeps the emoji picker open when its row unmounts', async () => {
    const event = messageEvent();
    const rendered = render(MessageEventTestHarness, { props: { event } });

    (q(rendered.container, 'button[aria-label="Add reaction"]') as HTMLButtonElement).click();
    await vi.waitFor(() =>
      expect(q(rendered.container, 'input[placeholder="Search emojis..."]')).toBeTruthy()
    );

    // The virtualizer unmounts the row, for example when the keyboard shrinks the timeline.
    await rendered.rerender({ event, showMessage: false });
    expect(rendered.container.querySelector('[data-testid="message-row"]')).toBeNull();

    await selectPickerEmoji(rendered.container, 'check', 'white_check_mark');
    await vi.waitFor(() =>
      expect(mocks.actions.toggleReaction).toHaveBeenLastCalledWith(
        expect.objectContaining({ messageEventId: 'regular-message' }),
        '✅',
        false
      )
    );
  });

  it('opens reaction details from the message menu', async () => {
    const rendered = render(MessageEventTestHarness, { props: { event: messageEvent() } });

    await openContextMenu(rendered.container);
    menuButton(rendered.container, 'View reactions')!.click();

    await vi.waitFor(() =>
      expect(document.querySelector('dialog[open]')?.querySelector('[role="tablist"]')).toBeTruthy()
    );
    expect(menuButton(rendered.container, 'Copy message link')).toBeUndefined();
  });

  it('quotes the selection captured at open after the row unmounts', async () => {
    const onOpenThread = vi.fn();
    const event = messageEvent({ body: 'Quote this selection' });
    const rendered = render(MessageEventTestHarness, { props: { event, onOpenThread } });
    const range = document.createRange();
    range.selectNodeContents(q(rendered.container, '[data-testid="message-body"]')!);
    window.getSelection()!.addRange(range);

    q(rendered.container, '[data-testid="message-row"]')!.dispatchEvent(
      new MouseEvent('mousedown', { bubbles: true, cancelable: true, button: 2 })
    );
    await openContextMenu(rendered.container);
    await rendered.rerender({ event, onOpenThread, showMessage: false });
    menuButton(rendered.container, 'Reply in thread')!.click();

    expect(onOpenThread).toHaveBeenCalledWith(
      event.id,
      expect.objectContaining({ quoteText: 'Quote this selection' })
    );
  });

  it('clears the selected text when the message menu closes', async () => {
    const rendered = render(MessageEventTestHarness, { props: { event: messageEvent() } });
    const range = document.createRange();
    range.selectNodeContents(q(rendered.container, '[data-testid="message-body"]')!);
    window.getSelection()!.addRange(range);

    await openContextMenu(rendered.container);
    window.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape', bubbles: true }));

    await vi.waitFor(() =>
      expect(menuButton(rendered.container, 'Copy message link')).toBeUndefined()
    );
    expect(window.getSelection()!.isCollapsed).toBe(true);
  });

  it('ignores the context menu event of a touch long press', async () => {
    const { container } = render(MessageEventTestHarness, { props: { event: messageEvent() } });
    const row = q(container, '[data-testid="message-row"]')!;

    vi.useFakeTimers();
    row.dispatchEvent(new Event('touchstart', { bubbles: true, cancelable: true }));
    vi.advanceTimersByTime(500);
    row.dispatchEvent(new MouseEvent('contextmenu', { bubbles: true, cancelable: true }));
    flushSync();
    vi.useRealTimers();

    await vi.waitFor(() => expect(actionSheetButton(container, 'Copy message link')).toBeTruthy());
    expect(container.querySelector('[role="menu"]')).toBeNull();
  });

  it.each([
    '<span data-link-preview-trigger><a href="#">Link</a></span>',
    '<dialog><button>Preview action</button></dialog>',
    '<div popover><button>Preview action</button></div>'
  ])('keeps message gestures out of child-owned surfaces: %s', async (html) => {
    mocks.touchPrimary = true;
    const actionOverlays = new MessageActionOverlayState();
    const { container } = render(MessageEventTestHarness, {
      props: { event: messageEvent(), actionOverlays }
    });
    const body = q(container, '[data-testid="message-body"]')!;
    body.innerHTML = html;
    const child = body.querySelector('a, button')!;
    vi.useFakeTimers();
    child.dispatchEvent(new Event('touchstart', { bubbles: true }));
    child.dispatchEvent(new MouseEvent('mousedown', { bubbles: true, button: 0 }));
    vi.advanceTimersByTime(600);
    flushSync();
    expect(actionOverlays.current).toBeNull();
    // The message body itself still owns a normal long press.
    body.dispatchEvent(new Event('touchstart', { bubbles: true }));
    vi.advanceTimersByTime(500);
    flushSync();
    expect(actionOverlays.current?.kind).toBe('sheet');
    vi.useRealTimers();
  });

  it('closes the touch sheet when it is dismissed natively', async () => {
    const actionOverlays = new MessageActionOverlayState();
    const { container } = render(MessageEventTestHarness, {
      props: { event: messageEvent(), actionOverlays }
    });

    vi.useFakeTimers();
    q(container, '[data-testid="message-row"]')!.dispatchEvent(
      new Event('touchstart', { bubbles: true, cancelable: true })
    );
    vi.advanceTimersByTime(500);
    flushSync();
    vi.useRealTimers();
    await vi.waitFor(() => expect(actionSheetButton(container, 'Copy message link')).toBeTruthy());

    container.querySelector<HTMLDialogElement>('dialog[open]')!.close();
    await vi.waitFor(() => expect(actionOverlays.current).toBeNull());
  });

  it.each(['dialog', 'div popover'])(
    'ignores context menus retargeted into a child %s',
    async (surface) => {
      const actionOverlays = new MessageActionOverlayState();
      const { container } = render(MessageEventTestHarness, {
        props: { event: messageEvent(), actionOverlays }
      });
      const body = q(container, '[data-testid="message-body"]')!;
      const tag = surface.split(' ')[0];
      body.innerHTML = `<${surface}><span>Preview content</span></${tag}>`;
      const nativeMenu = new MouseEvent('contextmenu', { bubbles: true, cancelable: true });
      body.querySelector('span')!.dispatchEvent(nativeMenu);
      flushSync();
      expect(nativeMenu.defaultPrevented).toBe(true);
      expect(actionOverlays.current).toBeNull();
    }
  );

  it('keeps the message menu for a mention without a current member', async () => {
    const rendered = render(MessageEventTestHarness, { props: { event: messageEvent() } });
    const body = q(rendered.container, '[data-testid="message-body"]')!;
    body.innerHTML = '<span class="mention" data-user-id="missing">@Missing</span>';

    q(body, '.mention')!.dispatchEvent(
      new MouseEvent('contextmenu', { bubbles: true, cancelable: true })
    );

    await vi.waitFor(() =>
      expect(menuButton(rendered.container, 'Copy message link')).toBeTruthy()
    );
  });

  it('shows Copy Image only for a right-clicked image attachment', async () => {
    const imageUrl = 'data:image/gif;base64,R0lGODlhAQABAIAAAAAAAP///ywAAAAAAQABAAACAUwAOw==';
    mocks.copyImageToClipboard.mockResolvedValue(undefined);
    const image: MessageAttachmentView = {
      id: 'image-1',
      filename: 'image.jpg',
      contentType: 'image/jpeg',
      width: 800,
      height: 600,
      assetUrl: { url: imageUrl, expiresAt: '2027-05-29T15:00:00Z' },
      thumbnailAssetUrl: null,
      videoProcessing: null
    };
    const firstMessage = messageEvent({ attachments: [image] });
    const rendered = render(MessageEventTestHarness, { props: { event: firstMessage } });
    const imageElement = q(rendered.container, '[data-message-image-attachment] img')!;
    const click = new MouseEvent('contextmenu', { bubbles: true, cancelable: true });

    imageElement.dispatchEvent(click);

    expect(click.defaultPrevented).toBe(true);
    await vi.waitFor(() => expect(menuButton(rendered.container, 'Copy image')).toBeTruthy());
    expect(menuButton(rendered.container, 'Copy message link')).toBeTruthy();
    expect(menuButton(rendered.container, 'Copy link')).toBeUndefined();

    menuButton(rendered.container, 'Copy image')!.click();
    await vi.waitFor(() => expect(mocks.copyImageToClipboard).toHaveBeenCalledWith(imageUrl));
    await vi.waitFor(() => expect(menuButton(rendered.container, 'Copy image')).toBeUndefined());

    await openContextMenu(rendered.container);
    expect(menuButton(rendered.container, 'Copy image')).toBeUndefined();

    imageElement.dispatchEvent(new MouseEvent('contextmenu', { bubbles: true, cancelable: true }));
    await vi.waitFor(() => expect(menuButton(rendered.container, 'Copy image')).toBeTruthy());
    (q(rendered.container, 'button[aria-label="More actions"]') as HTMLButtonElement).click();
    await vi.waitFor(() => expect(menuButton(rendered.container, 'Copy image')).toBeUndefined());

    imageElement.dispatchEvent(new MouseEvent('contextmenu', { bubbles: true, cancelable: true }));
    await vi.waitFor(() => expect(menuButton(rendered.container, 'Copy image')).toBeTruthy());
    await rendered.rerender({ event: messageEvent({ id: 'next-message' }) });
    await vi.waitFor(() => expect(menuButton(rendered.container, 'Copy image')).toBeUndefined());

    await rendered.rerender({ event: firstMessage });
    expect(menuButton(rendered.container, 'Copy image')).toBeUndefined();
  });

  it('shows Copy Link only for a right-clicked message-body link', async () => {
    const writeText = vi.spyOn(navigator.clipboard, 'writeText').mockResolvedValue(undefined);
    const rendered = render(MessageEventTestHarness, { props: { event: messageEvent() } });
    const body = q(rendered.container, '[data-testid="message-body"]')!;
    body.innerHTML = '<a href="/linked/path"><strong>Linked text</strong></a>';
    const link = q(body, 'a') as HTMLAnchorElement;
    const nativeMenu = new MouseEvent('contextmenu', {
      bubbles: true,
      cancelable: true,
      shiftKey: true
    });
    q(body, 'strong')!.dispatchEvent(nativeMenu);
    expect(nativeMenu.defaultPrevented).toBe(false);
    expect(menuButton(rendered.container, 'Copy link')).toBeUndefined();
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

  it('closes a menu when its message leaves and does not carry its link to another message', async () => {
    const firstMessage = messageEvent();
    const rendered = render(MessageEventTestHarness, { props: { event: firstMessage } });
    const body = q(rendered.container, '[data-testid="message-body"]')!;
    body.innerHTML = '<a href="https://example.com"><span>Link</span></a>';
    q(body, 'span')!.dispatchEvent(
      new MouseEvent('contextmenu', { bubbles: true, cancelable: true })
    );
    await vi.waitFor(() => expect(menuButton(rendered.container, 'Copy link')).toBeTruthy());

    await rendered.rerender({ event: messageEvent({ id: 'next-message' }) });
    await vi.waitFor(() =>
      expect(menuButton(rendered.container, 'Copy message link')).toBeUndefined()
    );

    await openContextMenu(rendered.container);
    expect(menuButton(rendered.container, 'Copy link')).toBeUndefined();

    await rendered.rerender({ event: firstMessage });
    await vi.waitFor(() =>
      expect(menuButton(rendered.container, 'Copy message link')).toBeUndefined()
    );
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
    'opens and focuses the thread without root attribution from the context menu in %s mode',
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

      expect(onOpenThread).toHaveBeenCalledWith(event.id, {
        quoteText: 'Selected thread quote',
        focusComposer: true
      });
    }
  );

  it('keeps the full reply excerpt for CSS truncation in the room composer', async () => {
    const body = 'A reply preview should use all available space. '.repeat(5);
    const { container } = render(MessageEventTestHarness, {
      props: { event: messageEvent({ body }) }
    });

    (q(container, 'button[aria-label="Reply"]') as HTMLButtonElement).click();

    await expect
      .element(q(container, '[data-testid="active-reply-excerpt"]'))
      .toHaveTextContent(body.trim());
  });

  it('keeps the full reply excerpt for CSS truncation when replying to an echo', async () => {
    const body = 'A reply preview should use all available space. '.repeat(5);
    const onOpenThread = vi.fn();
    const { container } = render(MessageEventTestHarness, {
      props: {
        event: messageEvent({ body, echoOfEventId: 'reply', echoFromThreadRootEventId: 'root' }),
        onOpenThread
      }
    });

    await openContextMenu(container);
    menuButton(container, 'Reply in thread')!.click();

    expect(onOpenThread).toHaveBeenCalledWith(
      'root',
      expect.objectContaining({ reply: expect.objectContaining({ excerpt: body }) })
    );
  });

  it('opens and focuses the thread without root attribution from the hover toolbar', () => {
    const onOpenThread = vi.fn();
    const event = messageEvent({ id: 'toolbar-target' });
    const { container } = render(MessageEventTestHarness, { props: { event, onOpenThread } });

    (q(container, 'button[aria-label="Reply in thread"]') as HTMLButtonElement).click();

    expect(onOpenThread).toHaveBeenCalledWith(event.id, { focusComposer: true });
  });

  it('opens and focuses the thread without root attribution from the touch sheet', async () => {
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

    expect(onOpenThread).toHaveBeenCalledWith(event.id, { focusComposer: true });
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

  it('permits moderators to remove existing echoes without permitting additions', async () => {
    const event = messageEvent({
      id: 'bot-reply',
      actorId: 'bot',
      threadRootEventId: 'thread-root',
      channelEchoEventId: 'echo'
    });
    const rendered = render(MessageEventTestHarness, {
      props: {
        event,
        canManageOthersMessage: true,
        canPostMessage: false,
        threadingMode: RoomThreadingMode.DISABLED
      }
    });
    (q(rendered.container, 'button[aria-label="Edit message"]') as HTMLButtonElement).click();
    expect(mocks.actions.startEdit).toHaveBeenLastCalledWith(
      expect.objectContaining({
        eventId: 'bot-reply',
        canAddChannelEcho: false,
        canRemoveChannelEcho: true
      })
    );

    await rendered.rerender({ event, canManageOthersMessage: false });
    expect(q(rendered.container, 'button[aria-label="Edit message"]')).toBeNull();

    await rendered.rerender({
      event: messageEvent({ actorId: 'bot', threadRootEventId: 'thread-root' }),
      canManageOthersMessage: true
    });
    (q(rendered.container, 'button[aria-label="Edit message"]') as HTMLButtonElement).click();
    expect(mocks.actions.startEdit).toHaveBeenLastCalledWith(
      expect.objectContaining({
        canAddChannelEcho: false,
        canRemoveChannelEcho: false
      })
    );
  });

  it('rebinds every action surface when a virtualized row changes message shape', async () => {
    const regular = messageEvent();
    const rendered = render(MessageEventTestHarness, { props: { event: regular } });

    (q(rendered.container, 'button[aria-label="👍 1"]') as HTMLButtonElement).click();
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
        canAddChannelEcho: false,
        canRemoveChannelEcho: true
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
    expect((q(rendered.container, 'button[aria-label="👍 1"]') as HTMLButtonElement).disabled).toBe(
      true
    );

    vi.useFakeTimers();
    q(rendered.container, '[data-testid="message-row"]')!.dispatchEvent(
      new Event('touchstart', { bubbles: true, cancelable: true })
    );
    vi.advanceTimersByTime(500);
    flushSync();
    vi.useRealTimers();
    await vi.waitFor(() =>
      expect(actionSheetButton(rendered.container, 'Copy message link')).toBeTruthy()
    );
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
