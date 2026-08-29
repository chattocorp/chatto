import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { userEvent } from 'vitest/browser';
import { render } from 'vitest-browser-svelte';
import { tick, type ComponentProps } from 'svelte';
import MessageComposer, { type MessageComposerApi } from './MessageComposer.svelte';
import { q } from '$lib/test-utils';
import { getToasts, toast } from '$lib/ui/toast';
import type { QuoteInsertionContent, RoomMember } from '$lib/state/room';
import { PresenceStatus } from '@chatto/api-types/api/v1/presence_pb';

import { TimelineEventKind } from '$lib/render/timelineEvents';
import { renderMarkdown } from '$lib/markdown';
import type { CreateMessageInput } from '$lib/api-client/messages';
import { MentionRolesStore } from '$lib/state/server/mentionRoles.svelte';
import { Code, ConnectError } from '$lib/api-client/connect';
import { userPreferences } from '$lib/state/userPreferences.svelte';

function postedMessageEvent(
  id = 'msg_123',
  roomId = 'room_456',
  threadRootEventId: string | null = null
) {
  return {
    id,
    createdAt: '2026-06-17T10:47:00Z',
    actorId: 'test-user',
    actor: null,
    event: {
      kind: TimelineEventKind.MessagePosted,
      roomId,
      body: 'hello world',
      attachments: [],
      linkPreview: null,
      reactions: [],
      updatedAt: null,
      inReplyTo: null,
      threadRootEventId,
      echoOfEventId: null,
      echoFromThreadRootEventId: null,
      channelEchoEventId: null,
      replyCount: 0,
      lastReplyAt: null,
      threadParticipants: [],
      viewerIsFollowingThread: true
    }
  };
}

const mutationData = { createMessage: postedMessageEvent() };
const updateMutationData = { updateMessage: true };
const prepareFilesMock = vi.hoisted(() => vi.fn());
const mutationMock = vi.hoisted(() => vi.fn());
const queryMock = vi.hoisted(() => vi.fn());
const createMessageConnectMock = vi.hoisted(() => vi.fn());
const updateMessageConnectMock = vi.hoisted(() => vi.fn());
const fetchLinkPreviewConnectMock = vi.hoisted(() => vi.fn());
const listRolesConnectMock = vi.hoisted(() => vi.fn());
const roomStateMock = vi.hoisted(() => ({
  members: [] as RoomMember[],
  editState: {
    eventId: null as string | null,
    originalBody: '',
    threadRootEventId: null as string | null,
    channelEchoEventId: null as string | null,
    canAddChannelEcho: false,
    startEdit: vi.fn(),
    cancelEdit: vi.fn()
  },
  quoteInsertionState: {
    request: null as { id: number; text: QuoteInsertionContent } | null
  },
  lastEditableMessage: {
    getLastEditableMessage: vi.fn(() => null as { eventId: string; body: string } | null),
    setFinder: vi.fn()
  },
  scrollState: {
    scrollRequestCounter: 0,
    requestScrollToBottom: vi.fn(),
    setContainer: vi.fn(),
    setShouldScroll: vi.fn(),
    scrollToBottomIfSticky: vi.fn()
  }
}));

// Mock instance state
let mentionRolesStore = new MentionRolesStore({ listRoles: listRolesConnectMock });
const mockInstanceStores = {
  currentUser: { user: { id: 'test-user', login: 'testuser', settings: null }, loading: false },
  serverInfo: {
    videoProcessingEnabled: false,
    maxUploadSize: 25 * 1024 * 1024,
    maxVideoUploadSize: 25 * 1024 * 1024
  },
  roomUnread: {
    setRoomUnread: vi.fn()
  },
  get mentionRoles() {
    return mentionRolesStore;
  }
};

vi.mock('$lib/state/server/scope.svelte', () => ({
  useServerScope: () => ({
    serverId: 'test-instance',
    store: mockInstanceStores,
    connection: {
      isConnected: true,
      showConnectionLostBanner: false,
      client: {
        query: queryMock,
        mutation: mutationMock,
        subscription: vi.fn()
      },
      connectBaseUrl: 'http://localhost/api/connect',
      bearerToken: null,
      serverId: 'test-instance',
      getAPI: (factory: (config: never) => unknown) => factory({} as never)
    }
  })
}));

vi.mock('$lib/api-client/messages', () => ({
  createMessageAPI: () => ({
    createMessage: createMessageConnectMock,
    updateMessage: updateMessageConnectMock
  })
}));

vi.mock('$lib/api-client/linkPreviews', () => ({
  createLinkPreviewAPI: () => ({
    fetchLinkPreview: fetchLinkPreviewConnectMock
  })
}));

vi.mock('$lib/api-client/roles', () => ({
  createRoleAPI: () => ({
    listRoles: listRolesConnectMock
  })
}));

vi.mock('$lib/attachments/prepareFiles', () => ({
  prepareFiles: prepareFilesMock
}));

vi.mock('$lib/state/room', () => ({
  MessagesStore: class {},
  RoomFilesStore: class {},
  RoomPinsStore: class {},
  getRoomMembers: () => roomStateMock.members,
  getRoomMembersStore: () => ({
    searchMembers: vi.fn(async () => roomStateMock.members)
  }),
  getComposerContext: () => ({
    editState: roomStateMock.editState,
    quoteInsertionState: roomStateMock.quoteInsertionState,
    lastEditableMessage: roomStateMock.lastEditableMessage,
    scrollState: roomStateMock.scrollState
  })
}));

type MessageComposerProps = ComponentProps<typeof MessageComposer>;

function renderMessageComposer(
  props: Partial<MessageComposerProps> & { roomId: string },
  options: { exactRoomId?: boolean } = {}
) {
  const roomId = options.exactRoomId ? props.roomId : `${props.roomId}-${renderId++}`;
  return {
    ...render(MessageComposer, {
      props: { ...props, roomId }
    }),
    roomId
  };
}

let renderId = 0;

function selectFiles(input: HTMLInputElement, files: File[]) {
  Object.defineProperty(input, 'files', {
    value: Object.assign(files, {
      item: (index: number) => files[index] ?? null
    }),
    configurable: true
  });
  input.dispatchEvent(new Event('change', { bubbles: true }));
}

function deferred<T>() {
  let resolve!: (value: T) => void;
  let reject!: (reason?: unknown) => void;
  const promise = new Promise<T>((res, rej) => {
    resolve = res;
    reject = rej;
  });
  return { promise, resolve, reject };
}

function imageFile(name = 'paste.png'): File {
  return new File([new Uint8Array([1, 2, 3])], name, { type: 'image/png' });
}

function roomMember(login: string, displayName = login): RoomMember {
  return {
    id: `user_${login}`,
    login,
    displayName,
    avatarUrl: null,
    presenceStatus: PresenceStatus.OFFLINE
  };
}

function pasteFile(target: HTMLElement, file: File) {
  const dataTransfer = new DataTransfer();
  dataTransfer.items.add(file);
  target.dispatchEvent(
    new ClipboardEvent('paste', {
      bubbles: true,
      cancelable: true,
      clipboardData: dataTransfer
    })
  );
}

function pasteText(target: HTMLElement, text: string, html?: string) {
  const dataTransfer = new DataTransfer();
  dataTransfer.setData('text/plain', text);
  if (html !== undefined) dataTransfer.setData('text/html', html);
  target.dispatchEvent(
    new ClipboardEvent('paste', {
      bubbles: true,
      cancelable: true,
      clipboardData: dataTransfer
    })
  );
}

async function findEditor(container: Element, testid = 'message-input'): Promise<HTMLElement> {
  await vi.waitFor(() => expect(q(container, `[data-testid="${testid}"]`)).toBeTruthy(), {
    timeout: 5000
  });
  return q(container, `[data-testid="${testid}"]`)!;
}

async function typeInEditor(editor: HTMLElement, text: string) {
  editor.focus();
  document.execCommand('selectAll');
  document.execCommand('insertText', false, text);
  await vi.waitFor(() => expect(editor.textContent).toBe(text));
}

async function typeEditorKeys(editor: HTMLElement, text: string) {
  editor.focus();
  document.execCommand('selectAll');
  document.execCommand('delete');
  await userEvent.type(editor, text);
  await tick();
}

async function typeEditorLiteralText(editor: HTMLElement, text: string) {
  editor.focus();
  document.execCommand('selectAll');
  document.execCommand('delete');
  for (const char of text) {
    document.execCommand('insertText', false, char);
    await tick();
  }
}

async function insertEditorLiteralText(editor: HTMLElement, text: string) {
  editor.focus();
  for (const char of text) {
    document.execCommand('insertText', false, char);
    await tick();
  }
}

async function placeCaretAtEditorEnd(editor: HTMLElement) {
  editor.focus();
  const selection = window.getSelection();
  const range = document.createRange();
  range.selectNodeContents(editor);
  range.collapse(false);
  selection?.removeAllRanges();
  selection?.addRange(range);
  await tick();
}

async function selectEditorContents(editor: HTMLElement) {
  editor.focus();
  const selection = window.getSelection();
  const range = document.createRange();
  range.selectNodeContents(editor);
  selection?.removeAllRanges();
  selection?.addRange(range);
  document.dispatchEvent(new Event('selectionchange'));
  await tick();
}

async function pressEditorKey(
  editor: HTMLElement,
  key: string,
  options: {
    ctrlKey?: boolean;
    metaKey?: boolean;
    shiftKey?: boolean;
    isComposing?: boolean;
  } = {}
) {
  editor.dispatchEvent(
    new KeyboardEvent('keydown', { key, bubbles: true, cancelable: true, ...options })
  );
  await tick();
}

function mockTouchPrimaryPointer() {
  vi.spyOn(window, 'matchMedia').mockImplementation(
    (query): MediaQueryList =>
      ({
        matches: query === '(pointer: coarse)',
        media: query,
        onchange: null,
        addListener: vi.fn(),
        removeListener: vi.fn(),
        addEventListener: vi.fn(),
        removeEventListener: vi.fn(),
        dispatchEvent: vi.fn(() => false)
      }) as MediaQueryList
  );
}

async function changeSelectValue(select: HTMLSelectElement, value: string) {
  select.value = value;
  select.dispatchEvent(new Event('change', { bubbles: true }));
  await tick();
}

async function changeInputValue(input: HTMLInputElement, value: string) {
  input.value = value;
  input.dispatchEvent(new InputEvent('input', { bubbles: true, inputType: 'insertText' }));
  input.dispatchEvent(new FocusEvent('blur', { bubbles: true }));
  await tick();
}

function selectFirstAttachment(input: HTMLInputElement, file = imageFile()) {
  selectFiles(input, [file]);
  return file;
}

async function openFormattingShelf(container: HTMLElement) {
  const toggle = q(
    container,
    'button[aria-label="Formatting options"]'
  ) as HTMLButtonElement;
  if (toggle.getAttribute('aria-expanded') !== 'true') await userEvent.click(toggle);
  await vi.waitFor(() =>
    expect(q(container, '[data-testid="composer-formatting-shelf"]')).toBeTruthy()
  );
}

describe('MessageComposer', () => {
  beforeEach(() => {
    userPreferences.composerEditor = 'visual';
    userPreferences.composerSendMode = 'modifier-enter';
    userPreferences.composerFormattingToolbarVisible = false;
    window.getSelection()?.removeAllRanges();
    mockInstanceStores.serverInfo.videoProcessingEnabled = false;
    mockInstanceStores.serverInfo.maxUploadSize = 25 * 1024 * 1024;
    mockInstanceStores.serverInfo.maxVideoUploadSize = 25 * 1024 * 1024;
    mockInstanceStores.roomUnread.setRoomUnread.mockClear();
    roomStateMock.members = [];
    roomStateMock.editState.eventId = null;
    roomStateMock.editState.originalBody = '';
    roomStateMock.editState.threadRootEventId = null;
    roomStateMock.editState.channelEchoEventId = null;
    roomStateMock.editState.canAddChannelEcho = false;
    roomStateMock.editState.startEdit.mockClear();
    roomStateMock.editState.cancelEdit.mockClear();
    roomStateMock.quoteInsertionState.request = null;
    roomStateMock.lastEditableMessage.getLastEditableMessage.mockReset();
    roomStateMock.lastEditableMessage.getLastEditableMessage.mockReturnValue(null);
    roomStateMock.lastEditableMessage.setFinder.mockClear();
    roomStateMock.scrollState.requestScrollToBottom.mockClear();
    roomStateMock.scrollState.scrollToBottomIfSticky.mockClear();
    toast.clear();
    Object.defineProperty(URL, 'createObjectURL', {
      value: vi.fn(() => 'blob:test'),
      configurable: true
    });
    Object.defineProperty(URL, 'revokeObjectURL', {
      value: vi.fn(),
      configurable: true
    });
    prepareFilesMock.mockReset();
    prepareFilesMock.mockImplementation(async (files: File[]) => files);
    mutationMock.mockReset();
    mutationMock.mockImplementation((_mutation, variables) => {
      if (variables?.input?.eventId)
        return Promise.resolve({ data: updateMutationData, error: null });
      return Promise.resolve({ data: mutationData, error: null });
    });
    createMessageConnectMock.mockReset();
    createMessageConnectMock.mockImplementation(async (input) => {
      const response = await mutationMock('connectCreateMessage', {
        input: { ...input, attachments: input.attachments ?? null }
      });
      if (response.error) throw response.error;
      return {
        event: response.data?.createMessage ?? null
      };
    });
    updateMessageConnectMock.mockReset();
    updateMessageConnectMock.mockResolvedValue(true);
    fetchLinkPreviewConnectMock.mockReset();
    fetchLinkPreviewConnectMock.mockResolvedValue(null);
    listRolesConnectMock.mockReset();
    listRolesConnectMock.mockResolvedValue({ roles: [] });
    mentionRolesStore = new MentionRolesStore({ listRoles: listRolesConnectMock });
    queryMock.mockReset();
    queryMock.mockResolvedValue({ data: null, error: null });
    sessionStorage.clear();
    vi.clearAllMocks();
  });

  afterEach(() => {
    vi.useRealTimers();
    window.getSelection()?.removeAllRanges();
    vi.restoreAllMocks();
  });

  describe('form rendering', () => {
    it('renders the TipTap editor', async () => {
      const { container } = renderMessageComposer({ roomId: 'room_456' });

      await expect.element(await findEditor(container)).toBeInTheDocument();
    });

    it('renders the attachment button', async () => {
      const { container } = renderMessageComposer({ roomId: 'room_456' });

      await expect.element(q(container, 'button[title="Attach file"]')).toBeInTheDocument();
    });

    it('renders the timestamp insertion button', async () => {
      const { container } = renderMessageComposer({ roomId: 'room_456' });

      await findEditor(container);
      await expect
        .element(q(container, 'button[aria-label="Insert timestamp"]'))
        .toBeInTheDocument();
    });

    it('keeps the editor and message actions in one compact input row', async () => {
      const { container } = renderMessageComposer({ roomId: 'room_456' });

      const editor = await findEditor(container);
      const surface = q(container, '[data-testid="composer-input-surface"]');
      const editorRow = q(container, '[data-testid="composer-editor-row"]');
      const actions = q(container, '[data-testid="composer-action-toolbar"]');

      expect(editorRow?.contains(editor)).toBe(true);
      expect(surface?.contains(editorRow)).toBe(true);
      expect(actions?.contains(q(container, 'button[title="Attach file"]'))).toBe(true);
      expect(actions?.contains(q(container, 'button[aria-label="Insert timestamp"]'))).toBe(true);
      expect(actions?.contains(q(container, 'button[aria-label="Send message"]'))).toBe(true);
      expect(surface).toHaveClass('composer-surface');
      expect(q(container, '[data-testid="composer-formatting-shelf"]')).toBeNull();
    });

    it('toggles, persists, and restores the formatting shelf without losing editor focus', async () => {
      const first = renderMessageComposer({ roomId: 'formatting-shelf-first' });
      const editor = await findEditor(first.container);
      await userEvent.click(editor);
      const toggle = q(
        first.container,
        'button[aria-label="Formatting options"]'
      ) as HTMLButtonElement;

      await userEvent.click(toggle);

      await expect.element(toggle).toHaveAttribute('aria-expanded', 'true');
      await expect.element(toggle).toHaveAttribute('aria-pressed', 'true');
      expect(toggle.getAttribute('aria-controls')).toBe(
        q(first.container, '[data-testid="composer-formatting-shelf"]')?.id
      );
      expect(document.activeElement).toBe(editor);
      expect(userPreferences.composerFormattingToolbarVisible).toBe(true);
      expect(q(first.container, '[data-testid="composer-formatting-shelf"]')).toHaveClass(
        'composer-surface'
      );

      first.unmount();
      const second = renderMessageComposer({ roomId: 'formatting-shelf-second' });
      await findEditor(second.container);
      expect(q(second.container, '[data-testid="composer-formatting-shelf"]')).toBeTruthy();
    });

    it('preserves an editor selection when a mouse drag ends over composer padding', async () => {
      const { container } = renderMessageComposer({ roomId: 'room-selection-padding' });
      const editor = await findEditor(container);
      const surface = q(container, '[data-testid="composer-input-surface"]')!;
      await typeInEditor(editor, 'keep this selected');
      await selectEditorContents(editor);

      editor.dispatchEvent(
        new PointerEvent('pointerdown', { bubbles: true, button: 0, pointerType: 'mouse' })
      );
      surface.dispatchEvent(
        new PointerEvent('pointerup', { bubbles: true, button: 0, pointerType: 'mouse' })
      );
      surface.dispatchEvent(new MouseEvent('click', { bubbles: true, button: 0 }));
      await tick();

      expect(window.getSelection()?.toString()).toBe('keep this selected');
    });

    it('uses the composer width to control labels and keeps formatting controls on one row', async () => {
      const { container } = renderMessageComposer({ roomId: 'room_456' });

      await findEditor(container);
      await openFormattingShelf(container);

      expect(q(container, '[data-testid="composer-input-surface"]')).toHaveClass('@container');
      expect(q(container, '[data-testid="composer-formatting-toolbar"]')).toHaveClass(
        'flex-nowrap'
      );
    });

    it('hides attachment controls when uploads are not allowed', async () => {
      const { container } = renderMessageComposer({ roomId: 'room_456', canAttach: false });

      await findEditor(container);
      expect(q(container, 'button[title="Attach file"]')).toBeNull();
      expect(q(container, 'input[type="file"]')).toBeNull();
    });

    it('renders hidden file input', async () => {
      const { container } = renderMessageComposer({ roomId: 'room_456' });

      const fileInput = q(container, 'input[type="file"]');
      await expect.element(fileInput).toBeInTheDocument();
      await expect.element(fileInput).toHaveClass('hidden');
    });

    it('editor has correct placeholder', async () => {
      const { container } = renderMessageComposer({ roomId: 'room_456' });

      await findEditor(container);
      // TipTap Placeholder extension sets data-placeholder on the empty paragraph
      await expect
        .element(q(container, 'p.is-editor-empty[data-placeholder="Type a message..."]'))
        .toBeInTheDocument();
    });
  });

  describe('Markdown editor integration', () => {
    beforeEach(() => {
      userPreferences.composerEditor = 'markdown';
    });

    it('mounts CodeMirror without mounting TipTap and restores source drafts', async () => {
      sessionStorage.setItem('chatto:draft:markdown-draft', '**saved** draft');
      const { container } = renderMessageComposer(
        { roomId: 'markdown-draft' },
        { exactRoomId: true }
      );
      const editor = await findEditor(container);

      expect(q(container, '[data-composer-editor="markdown"]')).toBeTruthy();
      expect(q(container, '[data-composer-editor="visual"]')).toBeNull();
      await expect.element(editor).toHaveTextContent('**saved** draft');
    });

    it('formats and submits Markdown source with Ctrl+Enter', async () => {
      const { container, roomId } = renderMessageComposer({ roomId: 'markdown-send' });
      const editor = await findEditor(container);
      await openFormattingShelf(container);
      await typeEditorKeys(editor, 'first');
      await userEvent.click(q(container, 'button[aria-label="Bullet list"]')!);

      await expect.element(editor).toHaveTextContent('- first');
      await expect
        .element(q(container, 'button[aria-label="Bullet list"]'))
        .toHaveAttribute('aria-pressed', 'true');
      await pressEditorKey(editor, 'Enter', { ctrlKey: true });

      await vi.waitFor(() => expect(mutationMock).toHaveBeenCalledOnce());
      expect(mutationMock.mock.calls[0][1].input).toMatchObject({ roomId, body: '- first' });
    });

    it('uses CodeMirror line indentation with the toolbar and Tab', async () => {
      const { container } = renderMessageComposer({ roomId: 'markdown-list-indent' });
      const editor = await findEditor(container);
      await openFormattingShelf(container);
      const indent = q(container, 'button[aria-label="Indent"]') as HTMLButtonElement;
      const outdent = q(container, 'button[aria-label="Outdent"]') as HTMLButtonElement;

      await vi.waitFor(() => expect(indent.disabled).toBe(false));
      expect(outdent.disabled).toBe(false);
      await typeEditorKeys(editor, 'first');
      await pressEditorKey(editor, 'Enter');
      await userEvent.type(editor, 'second');

      await pressEditorKey(editor, 'Tab');
      await vi.waitFor(() =>
        expect([...editor.querySelectorAll('.cm-line')].map((line) => line.textContent)).toEqual([
          'first',
          '  second'
        ])
      );

      await userEvent.click(outdent);
      await vi.waitFor(() =>
        expect([...editor.querySelectorAll('.cm-line')].map((line) => line.textContent)).toEqual([
          'first',
          'second'
        ])
      );
      await userEvent.click(indent);
      await pressEditorKey(editor, 'Tab', { shiftKey: true });
      await vi.waitFor(() =>
        expect([...editor.querySelectorAll('.cm-line')].map((line) => line.textContent)).toEqual([
          'first',
          'second'
        ])
      );
      await pressEditorKey(editor, 'Tab', { shiftKey: true });
      await vi.waitFor(() =>
        expect([...editor.querySelectorAll('.cm-line')].map((line) => line.textContent)).toEqual([
          'first',
          'second'
        ])
      );
    });

    it('lets Escape followed by Tab leave the Markdown composer', async () => {
      const { container } = renderMessageComposer({ roomId: 'markdown-tab-focus' });
      const editor = await findEditor(container);

      await userEvent.click(editor);
      await userEvent.keyboard('{Escape}{Tab}');

      expect(document.activeElement).toBe(q(container, 'button[aria-label="Attach file"]'));
    });

    it('completes mentions before Enter can submit Markdown', async () => {
      roomStateMock.members = [roomMember('alice')];
      const { container, roomId } = renderMessageComposer({ roomId: 'markdown-mention' });
      const editor = await findEditor(container);
      await typeEditorKeys(editor, '@ali');
      await vi.waitFor(() =>
        expect(container.querySelector('[data-testid="mention-autocomplete"]')).toBeTruthy()
      );

      await pressEditorKey(editor, 'Enter');
      await vi.waitFor(() => expect(editor.textContent).toBe('@alice '));
      expect(mutationMock).not.toHaveBeenCalled();
      await pressEditorKey(editor, 'Enter', { ctrlKey: true });

      await vi.waitFor(() => expect(mutationMock).toHaveBeenCalledOnce());
      expect(mutationMock.mock.calls[0][1].input).toMatchObject({ roomId, body: '@alice' });
    });

    it('completes emoji before Enter can submit Markdown', async () => {
      const { container, roomId } = renderMessageComposer({ roomId: 'markdown-emoji' });
      const editor = await findEditor(container);
      await typeEditorKeys(editor, ':fire');
      await vi.waitFor(() =>
        expect(
          [...container.querySelectorAll('button')].some((button) =>
            button.textContent?.includes(':fire:')
          )
        ).toBe(true)
      );

      await pressEditorKey(editor, 'Enter');
      await vi.waitFor(() => expect(editor.textContent).toMatch(/^\p{Extended_Pictographic}\s$/u));
      expect(mutationMock).not.toHaveBeenCalled();
      const completedEmoji = editor.textContent!.trim();
      await pressEditorKey(editor, 'Enter', { ctrlKey: true });

      await vi.waitFor(() => expect(mutationMock).toHaveBeenCalledOnce());
      expect(mutationMock.mock.calls[0][1].input).toMatchObject({
        roomId,
        body: completedEmoji
      });
    });

    it('preserves the draft while switching editor implementations', async () => {
      const { container } = renderMessageComposer({ roomId: 'markdown-switch' });
      let editor = await findEditor(container);
      await typeEditorKeys(editor, '**kept** draft');

      userPreferences.composerEditor = 'visual';
      await vi.waitFor(() => expect(q(container, '[data-composer-editor="visual"]')).toBeTruthy());
      expect(q(container, '[data-composer-editor="markdown"]')).toBeNull();
      editor = await findEditor(container);
      await expect.element(editor).toHaveTextContent('kept draft');

      userPreferences.composerEditor = 'markdown';
      await vi.waitFor(() =>
        expect(q(container, '[data-composer-editor="markdown"]')).toBeTruthy()
      );
      expect(q(container, '[data-composer-editor="visual"]')).toBeNull();
      editor = await findEditor(container);
      await expect.element(editor).toHaveTextContent('**kept** draft');
    });

    it('uses normal newlines and Ctrl+Enter to send', async () => {
      const { container, roomId } = renderMessageComposer({ roomId: 'markdown-shortcut' });
      const editor = await findEditor(container);
      await typeEditorKeys(editor, 'first');
      await pressEditorKey(editor, 'Enter');
      await userEvent.type(editor, 'second');

      expect(mutationMock).not.toHaveBeenCalled();
      await expect.element(editor).toHaveTextContent('firstsecond');
      await pressEditorKey(editor, 'Enter', { ctrlKey: true });

      await vi.waitFor(() => expect(mutationMock).toHaveBeenCalledOnce());
      expect(mutationMock.mock.calls[0][1].input).toMatchObject({
        roomId,
        body: 'first\nsecond'
      });
    });

    it('uses Ctrl+Enter for normal Markdown continuation when Return sends', async () => {
      userPreferences.composerSendMode = 'enter';
      const { container, roomId } = renderMessageComposer({ roomId: 'markdown-enter-send' });
      const editor = await findEditor(container);
      await typeEditorKeys(editor, '- first');
      await pressEditorKey(editor, 'Enter', { ctrlKey: true });
      await userEvent.type(editor, 'second');

      expect(mutationMock).not.toHaveBeenCalled();
      await vi.waitFor(() => expect(editor.querySelectorAll('.cm-line')).toHaveLength(2));
      expect([...editor.querySelectorAll('.cm-line')].map((line) => line.textContent)).toEqual([
        '- first',
        '- second'
      ]);

      await pressEditorKey(editor, 'Enter');
      await vi.waitFor(() => expect(mutationMock).toHaveBeenCalledOnce());
      expect(mutationMock.mock.calls[0][1].input).toMatchObject({
        roomId,
        body: '- first\n- second'
      });
    });

    it('preserves Markdown while editing and supports nested reply quotes', async () => {
      roomStateMock.editState.eventId = 'markdown-edit';
      roomStateMock.editState.originalBody = '**bold** source';
      let composerApi: MessageComposerApi | null = null;
      const { container } = renderMessageComposer({
        roomId: 'markdown-edit-room',
        onReady: (api) => (composerApi = api)
      });
      const editor = await findEditor(container);
      await expect.element(editor).toHaveTextContent('**bold** source');
      await vi.waitFor(() => expect(composerApi).not.toBeNull());

      composerApi!.insertQuote([{ quoteDepth: 1, text: 'nested' }]);
      await expect.element(editor).toHaveTextContent('> > nested');
      (q(container, 'button[aria-label="Send message"]') as HTMLButtonElement).click();

      await vi.waitFor(() => expect(updateMessageConnectMock).toHaveBeenCalledOnce());
      expect(updateMessageConnectMock).toHaveBeenCalledWith({
        roomId: expect.any(String),
        eventId: 'markdown-edit',
        body: '**bold** source\n\n> > nested'
      });
    });

    it('keeps attachments, link previews, slow mode, and focus editor-independent', async () => {
      fetchLinkPreviewConnectMock.mockResolvedValueOnce({
        previewToken: 'preview-token',
        url: 'https://example.com',
        title: 'Example',
        description: null,
        imageUrl: null,
        imageAssetId: null,
        siteName: null,
        embedType: null,
        embedId: null
      });
      const readyApis: MessageComposerApi[] = [];
      const { container } = renderMessageComposer({
        roomId: 'markdown-shared-features',
        slowModeSeconds: 30,
        onReady: (api) => readyApis.push(api)
      });
      const editor = await findEditor(container);
      await typeEditorKeys(editor, 'https://example.com');
      selectFirstAttachment(q(container, 'input[type="file"]') as HTMLInputElement);
      await vi.waitFor(() => expect(q(container, 'img')).toBeTruthy());
      await vi.waitFor(() => expect(readyApis).toHaveLength(1));
      readyApis[0]!.focus();

      await vi.waitFor(() => expect(document.activeElement).toBe(editor));
      await expect
        .element(q(container, '[data-testid="slow-mode-status"]'))
        .toHaveTextContent('Slow Mode: one message every 30 seconds.');
      await vi.waitFor(() => expect(fetchLinkPreviewConnectMock).toHaveBeenCalled());
      await vi.waitFor(() => expect(container.textContent).toContain('Example'));
      (q(container, 'button[aria-label="Send message"]') as HTMLButtonElement).click();
      await vi.waitFor(() => expect(mutationMock).toHaveBeenCalledOnce());
      expect(mutationMock.mock.calls[0][1].input).toMatchObject({
        body: 'https://example.com',
        linkPreviewToken: 'preview-token'
      });
    });
  });

  describe('angle-bracket link preview suppression', () => {
    it('suppresses previews and posts the autolink from the visual editor', async () => {
      const body = '<https://example.com/visual';
      const { container, roomId } = renderMessageComposer({ roomId: 'visual-autolink' });
      const editor = await findEditor(container);

      await typeEditorLiteralText(editor, body);
      await new Promise((resolve) => setTimeout(resolve, 600));

      expect(fetchLinkPreviewConnectMock).not.toHaveBeenCalled();
      (q(container, 'button[aria-label="Send message"]') as HTMLButtonElement).click();
      await vi.waitFor(() => expect(mutationMock).toHaveBeenCalledOnce());
      expect(mutationMock.mock.calls[0][1].input).toMatchObject({
        roomId,
        body,
        linkPreviewToken: null
      });
    });

    it('suppresses previews and posts the autolink from the Markdown editor', async () => {
      userPreferences.composerEditor = 'markdown';
      const body = '<https://example.com/markdown';
      const { container, roomId } = renderMessageComposer({ roomId: 'markdown-autolink' });
      const editor = await findEditor(container);

      await typeEditorKeys(editor, body);
      await new Promise((resolve) => setTimeout(resolve, 600));

      expect(fetchLinkPreviewConnectMock).not.toHaveBeenCalled();
      (q(container, 'button[aria-label="Send message"]') as HTMLButtonElement).click();
      await vi.waitFor(() => expect(mutationMock).toHaveBeenCalledOnce());
      expect(mutationMock.mock.calls[0][1].input).toMatchObject({
        roomId,
        body,
        linkPreviewToken: null
      });
    });
  });

  describe('Slow Mode', () => {
    it('shows ready, waiting, and bypassed status', async () => {
      const ready = renderMessageComposer({ roomId: 'room-ready', slowModeSeconds: 30 });
      await expect
        .element(q(ready.container, '[data-testid="slow-mode-status"]'))
        .toHaveTextContent('Slow Mode: one message every 30 seconds.');

      const nextPostAt = new Date(Date.now() + 65_000).toISOString();
      const waiting = renderMessageComposer({
        roomId: 'room-waiting',
        slowModeSeconds: 120,
        slowModeNextPostAt: nextPostAt
      });
      await expect
        .element(q(waiting.container, '[data-testid="slow-mode-status"]'))
        .toHaveTextContent('Slow Mode: send again in 1:05.');

      const bypassed = renderMessageComposer({
        roomId: 'room-bypassed',
        slowModeSeconds: 60,
        slowModeNextPostAt: nextPostAt,
        slowModeBypassed: true
      });
      await expect
        .element(q(bypassed.container, '[data-testid="slow-mode-status"]'))
        .toHaveTextContent("Slow Mode: 1 minute (you're exempt).");
    });

    it('keeps a waiting draft editable while blocking the keyboard send shortcut', async () => {
      const { container } = renderMessageComposer({
        roomId: 'room-waiting-draft',
        slowModeSeconds: 30,
        slowModeNextPostAt: new Date(Date.now() + 30_000).toISOString()
      });
      const editor = await findEditor(container);
      await typeInEditor(editor, 'preserve this draft');

      await expect.element(q(container, 'button[aria-label="Send message"]')).toBeDisabled();
      const draftHtml = editor.innerHTML;
      await pressEditorKey(editor, 'Enter', { ctrlKey: true });

      expect(createMessageConnectMock).not.toHaveBeenCalled();
      await expect.element(editor).toHaveTextContent('preserve this draft');
      expect(editor.innerHTML).toBe(draftHtml);
    });

    it('starts an optimistic countdown from the successful event timestamp', async () => {
      const createdAt = new Date(Date.now()).toISOString();
      createMessageConnectMock.mockResolvedValueOnce({
        event: { ...postedMessageEvent('slow-message'), createdAt }
      });
      const { container } = renderMessageComposer({
        roomId: 'room-optimistic',
        slowModeSeconds: 30
      });
      const editor = await findEditor(container);
      await typeInEditor(editor, 'first message');
      (q(container, 'button[aria-label="Send message"]') as HTMLButtonElement).click();

      await vi.waitFor(() =>
        expect(q(container, '[data-testid="slow-mode-status"]')?.textContent).toContain(
          'Slow Mode: send again in 0:30.'
        )
      );
    });

    it('does not include the age of an idle composer in the optimistic countdown', async () => {
      vi.useFakeTimers({ toFake: ['Date'] });
      vi.setSystemTime(new Date('2026-08-11T12:00:00.000Z'));
      const onMessageSent = vi.fn();
      const { container } = renderMessageComposer({
        roomId: 'room-long-lived',
        slowModeSeconds: 30,
        onMessageSent
      });
      const editor = await findEditor(container);
      await typeInEditor(editor, 'message after leaving the room open');

      vi.setSystemTime(new Date('2026-08-11T12:05:00.000Z'));
      const createdAt = new Date().toISOString();
      createMessageConnectMock.mockResolvedValueOnce({
        event: { ...postedMessageEvent('slow-message'), createdAt }
      });
      (q(container, 'button[aria-label="Send message"]') as HTMLButtonElement).click();

      await vi.waitFor(() => expect(onMessageSent).toHaveBeenCalledOnce());
      await tick();
      expect(q(container, '[data-testid="slow-mode-status"]')?.textContent).toContain(
        'Slow Mode: send again in 0:30.'
      );
    });

    it('preserves the draft and shows a Slow Mode error after a server race', async () => {
      createMessageConnectMock.mockRejectedValueOnce(
        new ConnectError('slow mode active', Code.ResourceExhausted)
      );
      const { container } = renderMessageComposer({
        roomId: 'room-race',
        slowModeSeconds: 30
      });
      const editor = await findEditor(container);
      await typeInEditor(editor, 'cross-tab draft');
      (q(container, 'button[aria-label="Send message"]') as HTMLButtonElement).click();

      await vi.waitFor(() =>
        expect(getToasts().some((entry) => entry.message.includes('Slow Mode is active'))).toBe(
          true
        )
      );
      await expect.element(editor).toHaveTextContent('cross-tab draft');
    });

    it('allows editing an existing message during the cooldown', async () => {
      roomStateMock.editState.eventId = 'message-to-edit';
      roomStateMock.editState.originalBody = 'existing body';
      const { container } = renderMessageComposer({
        roomId: 'room-editing',
        slowModeSeconds: 30,
        slowModeNextPostAt: new Date(Date.now() + 30_000).toISOString()
      });
      await findEditor(container);

      await expect.element(q(container, 'button[aria-label="Send message"]')).not.toBeDisabled();
    });
  });

  describe('file input configuration', () => {
    it('allows selecting any file type', async () => {
      const { container } = renderMessageComposer({ roomId: 'room_456' });

      await expect.element(q(container, 'input[type="file"]')).not.toHaveAttribute('accept');
    });

    it('stages selected document files', async () => {
      const { container } = renderMessageComposer({ roomId: 'room_456' });
      const input = q(container, 'input[type="file"]') as HTMLInputElement;

      selectFiles(input, [new File(['document'], 'report.pdf', { type: 'application/pdf' })]);

      await expect
        .poll(() => q(container, '[data-testid="file-attachment-preview"]')?.textContent)
        .toBe('pdf');
    });

    it('allows multiple file selection', async () => {
      const { container } = renderMessageComposer({ roomId: 'room_456' });

      await expect.element(q(container, 'input[type="file"]')).toHaveAttribute('multiple');
    });

    it('rejects selected video files when video processing is disabled', async () => {
      const { container } = renderMessageComposer({ roomId: 'room_456' });
      const input = q(container, 'input[type="file"]') as HTMLInputElement;

      selectFiles(input, [new File(['video'], 'clip.mp4', { type: 'video/mp4' })]);

      expect(getToasts().map((t) => t.message)).toContain(
        'Video uploads are disabled on this server.'
      );
      expect(q(container, '[data-testid="video-attachment-preview"]')).toBeNull();
    });

    it('stages selected video files when video processing is enabled', async () => {
      mockInstanceStores.serverInfo.videoProcessingEnabled = true;
      const { container } = renderMessageComposer({ roomId: 'room_456' });
      const input = q(container, 'input[type="file"]') as HTMLInputElement;

      selectFiles(input, [new File(['video'], 'clip.mp4', { type: 'video/mp4' })]);

      await expect
        .poll(() => q(container, '[data-testid="video-attachment-preview"]'))
        .toBeTruthy();
    });

    it('shows selected files in consistently sized cards with their file size', async () => {
      const { container } = renderMessageComposer({ roomId: 'room_456' });
      const input = q(container, 'input[type="file"]') as HTMLInputElement;

      selectFiles(input, [
        new File([new Uint8Array(2 * 1024)], 'a-long-filename-that-needs-truncating.png', {
          type: 'image/png'
        })
      ]);

      await expect
        .poll(() => q(container, '[data-testid="composer-attachment-preview"]'))
        .toBeTruthy();
      const preview = q(container, '[data-testid="composer-attachment-preview"]')!;
      expect(preview.className).toContain('w-72');
      expect(preview.querySelector('img')?.parentElement?.className).toContain('w-12');
      await expect.element(preview).toHaveTextContent('2 KB');
    });

    it('rejects selected files over the server upload size limit', async () => {
      mockInstanceStores.serverInfo.maxUploadSize = 1;
      const { container } = renderMessageComposer({ roomId: 'room_456' });
      const input = q(container, 'input[type="file"]') as HTMLInputElement;

      selectFiles(input, [
        new File([new Uint8Array([1, 2])], 'too-large.png', { type: 'image/png' })
      ]);

      expect(
        getToasts()
          .map((t) => t.message)
          .join('\n')
      ).toContain('too-large.png is too large');
      expect(q(container, 'img')).toBeNull();
      expect(prepareFilesMock).not.toHaveBeenCalled();
    });
  });

  describe('initial state', () => {
    it('publishes its API after initial synchronization and republishes it to replacement callbacks', async () => {
      const firstReady = vi.fn((api: MessageComposerApi) => {
        void api.addFiles([imageFile('ready.png')]);
      });
      const secondReady = vi.fn();
      const rendered = renderMessageComposer(
        { roomId: 'room-ready', onReady: firstReady },
        { exactRoomId: true }
      );

      await findEditor(rendered.container);
      await vi.waitFor(() => expect(firstReady).toHaveBeenCalledOnce());
      await expect.element(rendered.container).toHaveTextContent('ready.png');

      await rendered.rerender({ roomId: 'room-ready', onReady: secondReady });

      await vi.waitFor(() => expect(secondReady).toHaveBeenCalledOnce());
    });

    it('fulfils an API focus request made before the editor is ready', async () => {
      const { container } = renderMessageComposer({
        roomId: 'room-focus-on-ready',
        autoFocus: false,
        onReady: (api) => api.focus()
      });

      await expect.element(await findEditor(container)).toHaveFocus();
    });

    it('editor is editable initially', async () => {
      const { container } = renderMessageComposer({ roomId: 'room_456' });

      await expect.element(await findEditor(container)).toHaveAttribute('contenteditable', 'true');
    });

    it('attachment button is not disabled initially', async () => {
      const { container } = renderMessageComposer({ roomId: 'room_456' });

      await expect.element(q(container, 'button[title="Attach file"]')).not.toBeDisabled();
    });

    it('does not show file preview area initially', async () => {
      const { container } = renderMessageComposer({ roomId: 'room_456' });

      // File preview should only appear when files are selected
      const previewImages = container.querySelectorAll('img');
      expect(previewImages.length).toBe(0);
    });
  });

  describe('send button', () => {
    it('renders the send button', async () => {
      const { container } = renderMessageComposer({ roomId: 'room_456' });

      await expect.element(q(container, 'button[aria-label="Send message"]')).toBeInTheDocument();
    });

    it('send button is disabled when input is empty', async () => {
      const { container } = renderMessageComposer({ roomId: 'room_456' });

      await expect.element(q(container, 'button[aria-label="Send message"]')).toBeDisabled();
    });

    it('send button has paper plane icon', async () => {
      const { container } = renderMessageComposer({ roomId: 'room_456' });

      const sendButton = q(container, 'button[aria-label="Send message"]');
      const icon = sendButton?.querySelector('[class~="icon-[uil--telegram-alt]"]');
      expect(icon).not.toBeNull();
    });

    it('disables the composer and shows per-file upload progress while sending', async () => {
      const pendingSend = deferred<{ event: ReturnType<typeof postedMessageEvent> | null }>();
      let submittedInput!: CreateMessageInput;
      createMessageConnectMock.mockImplementationOnce((input) => {
        submittedInput = input;
        return pendingSend.promise;
      });
      const { container } = renderMessageComposer({ roomId: 'room_456' });
      const editor = await findEditor(container);
      const file = selectFirstAttachment(q(container, 'input[type="file"]') as HTMLInputElement);
      await typeInEditor(editor, 'large upload');
      const progressSlot = q(container, '[data-testid="attachment-upload-progress"]');

      await expect.element(progressSlot).toHaveClass('invisible');
      expect(progressSlot?.getAttribute('role')).toBeNull();

      (q(container, 'button[aria-label="Send message"]') as HTMLButtonElement).click();

      await expect.element(editor).toHaveAttribute('contenteditable', 'false');
      await expect.element(q(container, `button[aria-label="Remove ${file.name}"]`)).toBeDisabled();
      await expect.element(container).toHaveTextContent('Preparing…');

      submittedInput.onAttachmentUploadUpdate?.({
        file,
        phase: 'uploading',
        committedBytes: 1,
        totalBytes: 4
      });

      await expect.element(container).toHaveTextContent('25% uploaded');
      await expect
        .element(q(container, `[role="progressbar"][aria-label="${file.name}"]`))
        .toHaveAttribute('aria-valuenow', '25');
      await expect.element(progressSlot).not.toHaveClass('invisible');

      submittedInput.onAttachmentUploadUpdate?.({ file, phase: 'uploaded' });
      await expect.element(container).toHaveTextContent('Uploaded');

      pendingSend.resolve({ event: postedMessageEvent() });
      await vi.waitFor(() => expect(q(container, 'img')).toBeNull());
    });

    it('does not clear the next room draft when an earlier room send completes', async () => {
      const pendingSend = deferred<{ event: ReturnType<typeof postedMessageEvent> | null }>();
      const onMessageSent = vi.fn();
      createMessageConnectMock.mockReturnValueOnce(pendingSend.promise);
      const rendered = renderMessageComposer(
        { roomId: 'room-uploading', onMessageSent },
        { exactRoomId: true }
      );
      const editor = await findEditor(rendered.container);
      selectFirstAttachment(
        q(rendered.container, 'input[type="file"]') as HTMLInputElement,
        imageFile('room-a.png')
      );
      await typeInEditor(editor, 'room A message');
      (q(rendered.container, 'button[aria-label="Send message"]') as HTMLButtonElement).click();
      await vi.waitFor(() => expect(createMessageConnectMock).toHaveBeenCalledOnce());

      sessionStorage.setItem('chatto:draft:room-next', 'room B draft');
      await rendered.rerender({ roomId: 'room-next', onMessageSent });
      await expect.element(editor).toHaveTextContent('room B draft');

      pendingSend.resolve({ event: postedMessageEvent('msg-a', 'room-uploading') });

      await expect.element(editor).toHaveTextContent('room B draft');
      expect(sessionStorage.getItem('chatto:draft:room-next')).toBe('room B draft');
      expect(onMessageSent).not.toHaveBeenCalled();
      await vi.waitFor(() =>
        expect(sessionStorage.getItem('chatto:draft:room-uploading')).toBeNull()
      );
    });

    it('ignores externally added files while a send is in flight', async () => {
      const pendingSend = deferred<{ event: ReturnType<typeof postedMessageEvent> | null }>();
      const readyApis: MessageComposerApi[] = [];
      createMessageConnectMock.mockReturnValueOnce(pendingSend.promise);
      const { container } = renderMessageComposer({
        roomId: 'room_456',
        onReady: (api) => readyApis.push(api)
      });
      const editor = await findEditor(container);
      selectFirstAttachment(q(container, 'input[type="file"]') as HTMLInputElement);
      await typeInEditor(editor, 'sending');
      (q(container, 'button[aria-label="Send message"]') as HTMLButtonElement).click();
      await vi.waitFor(() => expect(createMessageConnectMock).toHaveBeenCalledOnce());

      await readyApis.at(-1)!.addFiles([imageFile('late.png')]);

      expect(prepareFilesMock).toHaveBeenCalledOnce();
      expect(container.textContent).not.toContain('late.png');
      pendingSend.resolve({ event: postedMessageEvent() });
    });

    it('has no adaptive composer mode or visual ring state', async () => {
      const { container } = renderMessageComposer({ roomId: 'room_456' });
      await findEditor(container);
      const surface = q(container, '[data-testid="composer-input-surface"]');
      expect(surface).not.toHaveAttribute('data-composer-mode');
      expect(surface).not.toHaveClass('composer-mode-surface');
    });

    it('does not show a send shortcut hint', async () => {
      const { container } = renderMessageComposer({ roomId: 'room_456' });
      await findEditor(container);
      const toolbar = q(container, '[data-testid="composer-action-toolbar"]');
      expect(toolbar?.textContent).not.toMatch(/to send/i);
      expect(toolbar?.querySelector('[title*="to send"]')).toBeNull();
    });

    it('treats an empty block element as sendable composer content', async () => {
      const { container, roomId } = renderMessageComposer({ roomId: 'room_456' });
      const editor = await findEditor(container);

      await typeEditorLiteralText(editor, '- ');
      await vi.waitFor(() => expect(editor.querySelector('ul li')).toBeTruthy());

      await expect.element(q(container, 'button[aria-label="Send message"]')).not.toBeDisabled();

      await pressEditorKey(editor, 'Enter', { ctrlKey: true });

      await vi.waitFor(() => expect(mutationMock).toHaveBeenCalledOnce());
      expect(mutationMock.mock.calls[0][1].input).toMatchObject({
        roomId,
        body: '- '
      });
    });
  });

  describe('pasted attachments', () => {
    it('ignores pasted files when uploads are not allowed', async () => {
      const { container } = renderMessageComposer({ roomId: 'room_456', canAttach: false });
      const editor = await findEditor(container);

      pasteFile(editor, imageFile());

      expect(prepareFilesMock).not.toHaveBeenCalled();
      expect(q(container, 'img')).toBeNull();
    });

    it('ignores externally added files when uploads are not allowed', async () => {
      const readyApis: MessageComposerApi[] = [];
      const { container } = renderMessageComposer({
        roomId: 'room_456',
        canAttach: false,
        onReady: (api) => {
          readyApis.push(api);
        }
      });

      await findEditor(container);
      expect(readyApis).not.toHaveLength(0);
      await readyApis.at(-1)!.addFiles([imageFile()]);

      expect(prepareFilesMock).not.toHaveBeenCalled();
      expect(q(container, 'img')).toBeNull();
    });

    it('disables sending typed text while a pasted image is preparing', async () => {
      const file = imageFile();
      const pendingPreparation = deferred<File[]>();
      prepareFilesMock.mockReturnValueOnce(pendingPreparation.promise);
      const { container, roomId } = renderMessageComposer({ roomId: 'room_456' });
      const editor = await findEditor(container);

      pasteFile(editor, file);
      await typeInEditor(editor, 'message with image');
      const sendButton = q(container, 'button[aria-label="Send message"]')! as HTMLButtonElement;
      await expect.element(sendButton).toBeDisabled();

      editor.dispatchEvent(
        new KeyboardEvent('keydown', { key: 'Enter', bubbles: true, cancelable: true })
      );
      expect(mutationMock).not.toHaveBeenCalled();

      pendingPreparation.resolve([file]);

      await expect.element(sendButton).not.toBeDisabled();
      sendButton.click();

      await vi.waitFor(() => expect(mutationMock).toHaveBeenCalledOnce());
      expect(mutationMock.mock.calls[0][1].input).toMatchObject({
        roomId,
        body: 'message with image',
        attachments: [file]
      });
    });

    it('disables image-only send until a pasted image preview appears', async () => {
      const file = imageFile();
      const pendingPreparation = deferred<File[]>();
      prepareFilesMock.mockReturnValueOnce(pendingPreparation.promise);
      const { container, roomId } = renderMessageComposer({ roomId: 'room_456' });
      const editor = await findEditor(container);
      const sendButton = q(container, 'button[aria-label="Send message"]')! as HTMLButtonElement;

      pasteFile(editor, file);
      await expect.element(sendButton).toBeDisabled();
      sendButton.click();

      expect(mutationMock).not.toHaveBeenCalled();

      pendingPreparation.resolve([file]);

      await expect.element(sendButton).not.toBeDisabled();
      sendButton.click();

      await vi.waitFor(() => expect(mutationMock).toHaveBeenCalledOnce());
      expect(mutationMock.mock.calls[0][1].input).toMatchObject({
        roomId,
        body: '',
        attachments: [file]
      });
    });

    it('clears disabled send state when pasted image preparation fails', async () => {
      const pendingPreparation = deferred<File[]>();
      prepareFilesMock.mockReturnValueOnce(pendingPreparation.promise);
      const { container } = renderMessageComposer({ roomId: 'room_456' });
      const editor = await findEditor(container);
      const sendButton = q(container, 'button[aria-label="Send message"]')! as HTMLButtonElement;

      pasteFile(editor, imageFile());
      await expect.element(sendButton).toBeDisabled();
      sendButton.click();

      pendingPreparation.reject(new Error('prepare failed'));

      await vi.waitFor(() => expect(mutationMock).not.toHaveBeenCalled());
      await expect.element(sendButton).toBeDisabled();
      expect(container.querySelector('.sending')).toBeNull();
    });
  });

  describe('draft lifecycle', () => {
    it('renders saved markdown drafts as rich editor content', async () => {
      sessionStorage.setItem(
        'chatto:draft:room_markdown_draft',
        '**bold**\n\n```ts\nconst answer = 42;\n```'
      );

      const { container } = renderMessageComposer(
        { roomId: 'room_markdown_draft' },
        { exactRoomId: true }
      );

      const editor = await findEditor(container);
      await vi.waitFor(() => expect(editor.querySelector('strong')?.textContent).toBe('bold'));
      await vi.waitFor(() =>
        expect(editor.querySelector('pre code')?.textContent).toContain('const answer = 42;')
      );
    });

    it('loads and persists a room text draft in sessionStorage', async () => {
      sessionStorage.setItem('chatto:draft:room_draft', 'saved draft');

      const { container } = renderMessageComposer({ roomId: 'room_draft' }, { exactRoomId: true });
      const editor = await findEditor(container);

      await expect.element(editor).toHaveTextContent('saved draft');

      await typeInEditor(editor, 'saved draft + more');

      await vi.waitFor(() =>
        expect(sessionStorage.getItem('chatto:draft:room_draft')).toBe('saved draft + more')
      );
    });

    it('preserves literal HTML-looking text when restoring a draft', async () => {
      const body = '<script>alert(1)</script> & <b>bold?</b>';
      const editedBody = `${body}!`;
      sessionStorage.setItem('chatto:draft:room_html_draft', body);

      const { container } = renderMessageComposer(
        { roomId: 'room_html_draft' },
        { exactRoomId: true }
      );
      const editor = await findEditor(container);

      await expect.element(editor).toHaveTextContent(body);
      editor.focus();
      document.execCommand('insertText', false, '!');

      await vi.waitFor(() =>
        expect(sessionStorage.getItem('chatto:draft:room_html_draft')).toBe(editedBody)
      );
    });

    it('preserves literal entity-looking text when restoring a draft', async () => {
      const body = 'AT&amp;T &gt; MCI';
      const editedBody = `${body}!`;
      sessionStorage.setItem('chatto:draft:room_entity_draft', body);

      const { container } = renderMessageComposer(
        { roomId: 'room_entity_draft' },
        { exactRoomId: true }
      );
      const editor = await findEditor(container);

      await expect.element(editor).toHaveTextContent(body);
      editor.focus();
      document.execCommand('insertText', false, '!');

      await vi.waitFor(() =>
        expect(sessionStorage.getItem('chatto:draft:room_entity_draft')).toBe(editedBody)
      );
    });

    it('preserves literal less-than entities when restoring a draft', async () => {
      const body = '&lt;x&gt;';
      const editedBody = `${body}!`;
      sessionStorage.setItem('chatto:draft:room_less_than_entity_draft', body);

      const { container } = renderMessageComposer(
        { roomId: 'room_less_than_entity_draft' },
        { exactRoomId: true }
      );
      const editor = await findEditor(container);

      await expect.element(editor).toHaveTextContent(body);
      editor.focus();
      document.execCommand('insertText', false, '!');

      await vi.waitFor(() =>
        expect(sessionStorage.getItem('chatto:draft:room_less_than_entity_draft')).toBe(editedBody)
      );
    });

    it('preserves ampersands in restored markdown link URLs', async () => {
      const body = '[search](https://example.com/?a=1&b=2)';
      const editedBody = '[search](https://example.com/?a=1&b=3)';
      sessionStorage.setItem('chatto:draft:room_link_draft', body);

      const { container } = renderMessageComposer(
        { roomId: 'room_link_draft' },
        { exactRoomId: true }
      );
      const editor = await findEditor(container);

      await vi.waitFor(() => {
        const link = editor.querySelector('a');
        expect(link?.textContent).toBe('search');
        expect(link?.getAttribute('href')).toBe('https://example.com/?a=1&b=2');
      });

      const hrefInput = q(container, 'input[aria-label="Link URL"]') as HTMLInputElement;
      await expect.element(hrefInput).toHaveValue('https://example.com/?a=1&b=2');
      await changeInputValue(hrefInput, 'https://example.com/?a=1&b=3');

      await vi.waitFor(() =>
        expect(sessionStorage.getItem('chatto:draft:room_link_draft')).toBe(editedBody)
      );
    });

    it('preserves literal HTML-looking text in restored indented code blocks', async () => {
      sessionStorage.setItem('chatto:draft:room_indented_code_draft', '    <b>x</b>');

      const { container } = renderMessageComposer(
        { roomId: 'room_indented_code_draft' },
        { exactRoomId: true }
      );
      const editor = await findEditor(container);

      await vi.waitFor(() =>
        expect(editor.querySelector('pre code')?.textContent).toBe('<b>x</b>')
      );
      editor.focus();
      document.execCommand('insertText', false, '!');

      await vi.waitFor(() => {
        const draft = sessionStorage.getItem('chatto:draft:room_indented_code_draft') ?? '';
        expect(draft).toContain('<b>x</b>');
        expect(draft).not.toContain('&lt;b>x&lt;/b>');
      });
    });

    it('escapes indented paragraph continuation lines as normal markdown text', async () => {
      const body = 'before\n    <b>x</b>';
      sessionStorage.setItem('chatto:draft:room_indented_continuation_draft', body);

      const { container } = renderMessageComposer(
        { roomId: 'room_indented_continuation_draft' },
        { exactRoomId: true }
      );
      const editor = await findEditor(container);

      await expect.element(editor).toHaveTextContent('before <b>x</b>');
      expect(editor.querySelector('strong')).toBeNull();
      await placeCaretAtEditorEnd(editor);
      document.execCommand('insertText', false, '!');

      await vi.waitFor(() => {
        const draft = sessionStorage.getItem('chatto:draft:room_indented_continuation_draft') ?? '';
        expect(draft).toContain('    <b>x</b>!');
        expect(draft).not.toContain('**x**');
      });
    });

    it('canonically escapes an unmatched backtick while preserving its literal text', async () => {
      const body = '` <b>literal</b>';
      const editedBody = `${body}!`;
      const serializedBody = `\\${editedBody}`;
      sessionStorage.setItem('chatto:draft:room_unmatched_backtick_draft', body);

      const { container } = renderMessageComposer(
        { roomId: 'room_unmatched_backtick_draft' },
        { exactRoomId: true }
      );
      const editor = await findEditor(container);

      await expect.element(editor).toHaveTextContent(body);
      editor.focus();
      document.execCommand('insertText', false, '!');

      await vi.waitFor(() =>
        expect(sessionStorage.getItem('chatto:draft:room_unmatched_backtick_draft')).toBe(
          serializedBody
        )
      );
    });

    it('does not escape code after a non-closing fence marker line', async () => {
      const body = '```md\n``` not a closing fence\n<b>code</b>\n```';
      sessionStorage.setItem('chatto:draft:room_non_closing_fence_draft', body);

      const { container } = renderMessageComposer(
        { roomId: 'room_non_closing_fence_draft' },
        { exactRoomId: true }
      );
      const editor = await findEditor(container);

      await vi.waitFor(() =>
        expect(editor.querySelector('pre code')?.textContent).toBe(
          '``` not a closing fence\n<b>code</b>'
        )
      );
    });

    it('does not escape code inside blockquoted fenced code blocks', async () => {
      const body = '> ```\n> <b>x</b>\n> ```';
      sessionStorage.setItem('chatto:draft:room_blockquoted_fence_draft', body);

      const { container } = renderMessageComposer(
        { roomId: 'room_blockquoted_fence_draft' },
        { exactRoomId: true }
      );
      const editor = await findEditor(container);

      await vi.waitFor(() =>
        expect(editor.querySelector('pre code')?.textContent).toBe('<b>x</b>')
      );
    });

    it('does not escape multiline inline code spans', async () => {
      const body = '`<b>\n</b>`';
      sessionStorage.setItem('chatto:draft:room_multiline_inline_code_draft', body);

      const { container } = renderMessageComposer(
        { roomId: 'room_multiline_inline_code_draft' },
        { exactRoomId: true }
      );
      const editor = await findEditor(container);

      await vi.waitFor(() => expect(editor.querySelector('code')?.textContent).toContain('<b>'));
      expect(editor.querySelector('code')?.textContent).toContain('</b>');
    });

    it('canonically escapes an unmatched closing bracket without creating a link', async () => {
      const body = 'not a link](<b>x</b>)';
      const editedBody = `${body}!`;
      const serializedBody = editedBody.replace(']', '\\]');
      sessionStorage.setItem('chatto:draft:room_fake_link_draft', body);

      const { container } = renderMessageComposer(
        { roomId: 'room_fake_link_draft' },
        { exactRoomId: true }
      );
      const editor = await findEditor(container);

      await expect.element(editor).toHaveTextContent(body);
      expect(editor.querySelector('strong')).toBeNull();
      editor.focus();
      document.execCommand('insertText', false, '!');

      await vi.waitFor(() =>
        expect(sessionStorage.getItem('chatto:draft:room_fake_link_draft')).toBe(serializedBody)
      );
    });

    it('restores markdown autolinks with ampersands intact', async () => {
      const body = '<https://example.com/?a=1&b=2>';
      sessionStorage.setItem('chatto:draft:room_autolink_text_draft', body);

      const { container } = renderMessageComposer(
        { roomId: 'room_autolink_text_draft' },
        { exactRoomId: true }
      );
      const editor = await findEditor(container);

      await vi.waitFor(() =>
        expect(editor.querySelector('a')?.getAttribute('href')).toBe('https://example.com/?a=1&b=2')
      );
    });

    it('uses a separate thread draft key', async () => {
      sessionStorage.setItem('chatto:draft:room_draft', 'room draft');
      sessionStorage.setItem('chatto:draft:room_draft:thread:msg_root', 'thread draft');

      const { container } = renderMessageComposer(
        { roomId: 'room_draft', inThread: 'msg_root' },
        { exactRoomId: true }
      );

      await expect
        .element(await findEditor(container, 'thread-reply-input'))
        .toHaveTextContent('thread draft');
      expect(sessionStorage.getItem('chatto:draft:room_draft')).toBe('room draft');
    });

    it('clears the active text draft after a successful send', async () => {
      const { container, roomId } = renderMessageComposer({ roomId: 'room_456' });
      const editor = await findEditor(container);

      await typeInEditor(editor, 'send and clear draft');
      await vi.waitFor(() =>
        expect(sessionStorage.getItem(`chatto:draft:${roomId}`)).toBe('send and clear draft')
      );

      (q(container, 'button[aria-label="Send message"]') as HTMLButtonElement).click();

      await vi.waitFor(() => expect(mutationMock).toHaveBeenCalledOnce());
      await vi.waitFor(() => expect(sessionStorage.getItem(`chatto:draft:${roomId}`)).toBeNull());
    });
  });

  describe('edit mode transitions', () => {
    it('keeps an existing message editable when new posting is unavailable', async () => {
      roomStateMock.editState.eventId = 'evt_historical_thread_edit';
      roomStateMock.editState.originalBody = 'historical reply';
      const { container } = renderMessageComposer({ roomId: 'room_456', canPost: false });
      const editor = await findEditor(container);

      await expect.element(editor).toHaveAttribute('contenteditable', 'true');
      (q(container, 'button[aria-label="Send message"]') as HTMLButtonElement).click();

      await vi.waitFor(() => expect(updateMessageConnectMock).toHaveBeenCalledOnce());
      expect(updateMessageConnectMock).toHaveBeenCalledWith({
        roomId: expect.any(String),
        eventId: 'evt_historical_thread_edit',
        body: 'historical reply'
      });
    });

    it('does not start editing on ArrowUp when no editable message is available', async () => {
      const { container } = renderMessageComposer({ roomId: 'room_456' });
      const editor = await findEditor(container);

      await pressEditorKey(editor, 'ArrowUp');

      expect(roomStateMock.lastEditableMessage.getLastEditableMessage).toHaveBeenCalledOnce();
      expect(roomStateMock.editState.startEdit).not.toHaveBeenCalled();
      await expect.element(editor).toHaveTextContent('');
    });

    it('prefills edit text, hides attachment controls, and cancels on Escape', async () => {
      roomStateMock.editState.eventId = 'evt_edit';
      roomStateMock.editState.originalBody = 'original body';
      const { container } = renderMessageComposer({ roomId: 'room_456' });
      const editor = await findEditor(container);

      await expect.element(editor).toHaveTextContent('original body');
      expect(q(container, 'button[title="Attach file"]')).toBeNull();

      await pressEditorKey(editor, 'Escape');

      expect(roomStateMock.editState.cancelEdit).toHaveBeenCalledOnce();
      expect(mutationMock).not.toHaveBeenCalled();
      expect(updateMessageConnectMock).not.toHaveBeenCalled();
    });

    it('closes mention autocomplete when cancelling an edit', async () => {
      roomStateMock.members = [roomMember('golden_fox07')];
      roomStateMock.editState.eventId = 'evt_edit';
      roomStateMock.editState.originalBody = 'original body';
      const { container } = renderMessageComposer({ roomId: 'room_456' });
      const editor = await findEditor(container);

      await typeEditorLiteralText(editor, '@gold');
      await vi.waitFor(() =>
        expect(container.querySelector('[data-testid="mention-autocomplete"]')).toBeTruthy()
      );

      const cancelButton = Array.from(container.querySelectorAll('button')).find(
        (button) => button.textContent?.trim() === 'Cancel'
      ) as HTMLButtonElement | undefined;
      expect(cancelButton).toBeTruthy();
      cancelButton!.click();

      expect(roomStateMock.editState.cancelEdit).toHaveBeenCalledOnce();
      await vi.waitFor(() =>
        expect(container.querySelector('[data-testid="mention-autocomplete"]')).toBeNull()
      );
    });

    it('closes mention autocomplete after saving an edit', async () => {
      roomStateMock.members = [roomMember('golden_fox07')];
      roomStateMock.editState.eventId = 'evt_edit';
      roomStateMock.editState.originalBody = 'original body';
      const { container } = renderMessageComposer({ roomId: 'room_456' });
      const editor = await findEditor(container);

      await typeEditorLiteralText(editor, '@gold');
      await vi.waitFor(() =>
        expect(container.querySelector('[data-testid="mention-autocomplete"]')).toBeTruthy()
      );
      (q(container, 'button[aria-label="Send message"]') as HTMLButtonElement).click();

      await vi.waitFor(() => expect(updateMessageConnectMock).toHaveBeenCalledOnce());
      expect(updateMessageConnectMock).toHaveBeenCalledWith({
        roomId: expect.any(String),
        eventId: 'evt_edit',
        body: '@gold'
      });
      await vi.waitFor(() =>
        expect(container.querySelector('[data-testid="mention-autocomplete"]')).toBeNull()
      );
    });

    it('sends a plain text edit with Ctrl+Enter', async () => {
      roomStateMock.editState.eventId = 'evt_edit';
      roomStateMock.editState.originalBody = 'original body';
      const { container } = renderMessageComposer({ roomId: 'room_456' });
      const editor = await findEditor(container);

      await vi.waitFor(() => expect(editor.textContent).toBe('original body'));
      expect(container.textContent).not.toMatch(/(?:Cmd|Ctrl)\+Return to Send/);
      await pressEditorKey(editor, 'Enter', { ctrlKey: true });

      await vi.waitFor(() => expect(updateMessageConnectMock).toHaveBeenCalledOnce());
      expect(updateMessageConnectMock).toHaveBeenCalledWith({
        roomId: expect.any(String),
        eventId: 'evt_edit',
        body: 'original body'
      });
    });

    it('uses normal Enter and Ctrl+Enter to send while editing', async () => {
      roomStateMock.editState.eventId = 'evt_edit';
      roomStateMock.editState.originalBody = 'original body';
      const { container } = renderMessageComposer({ roomId: 'room_456' });
      const editor = await findEditor(container);

      await vi.waitFor(() => expect(editor.textContent).toBe('original body'));
      await pressEditorKey(editor, 'Enter');
      expect(mutationMock).not.toHaveBeenCalled();
      expect(updateMessageConnectMock).not.toHaveBeenCalled();
      await vi.waitFor(() => expect(editor.querySelectorAll(':scope > p')).toHaveLength(2));

      await pressEditorKey(editor, 'Enter', { ctrlKey: true });

      await vi.waitFor(() => expect(updateMessageConnectMock).toHaveBeenCalledOnce());
      expect(updateMessageConnectMock).toHaveBeenCalledWith({
        roomId: expect.any(String),
        eventId: 'evt_edit',
        body: 'original body'
      });
    });

    it('preserves literal HTML-looking text when restoring and saving an edit', async () => {
      const body = '<script>alert(1)</script> & <b>bold?</b> &#45;';
      const editedBody = `${body}!`;
      roomStateMock.editState.eventId = 'evt_edit';
      roomStateMock.editState.originalBody = body;
      const { container } = renderMessageComposer({ roomId: 'room_456' });
      const editor = await findEditor(container);

      await expect.element(editor).toHaveTextContent(body);
      await placeCaretAtEditorEnd(editor);
      document.execCommand('insertText', false, '!');
      await vi.waitFor(() => expect(editor.textContent).toBe(editedBody));

      (q(container, 'button[aria-label="Send message"]') as HTMLButtonElement).click();

      await vi.waitFor(() => expect(updateMessageConnectMock).toHaveBeenCalledOnce());
      expect(updateMessageConnectMock).toHaveBeenCalledWith({
        roomId: expect.any(String),
        eventId: 'evt_edit',
        body: editedBody
      });
    });

    it('preserves a labelled channel link across repeated edit and save cycles', async () => {
      const url = 'https://chat.chatto.run/chat/-/REEi0LIuxqwQl3F';
      let body = `[#announcements](${url})`;

      for (let cycle = 1; cycle <= 3; cycle += 1) {
        const eventId = `evt_channel_link_${cycle}`;
        roomStateMock.editState.eventId = eventId;
        roomStateMock.editState.originalBody = body;
        const rendered = renderMessageComposer({ roomId: 'room_456' });
        const editor = await findEditor(rendered.container);

        await vi.waitFor(() => {
          const link = editor.querySelector('a');
          expect(link?.textContent).toBe('#announcements');
          expect(link?.getAttribute('href')).toBe(url);
        });
        await placeCaretAtEditorEnd(editor);
        document.execCommand('insertText', false, '!');
        const editedBody = `${body}!`;
        await vi.waitFor(() =>
          expect(editor.textContent).toBe(`#announcements${'!'.repeat(cycle)}`)
        );

        (q(rendered.container, 'button[aria-label="Send message"]') as HTMLButtonElement).click();

        await vi.waitFor(() => expect(updateMessageConnectMock).toHaveBeenCalledOnce());
        expect(updateMessageConnectMock).toHaveBeenCalledWith({
          roomId: expect.any(String),
          eventId,
          body: editedBody
        });

        body = editedBody;
        rendered.unmount();
        updateMessageConnectMock.mockClear();
      }
    });

    it('keeps an existing GFM table renderable after editing and saving', async () => {
      const body = '| Name | Role |\n| --- | --- |\n| Ada | Admin |';
      roomStateMock.editState.eventId = 'evt_table';
      roomStateMock.editState.originalBody = body;
      const { container } = renderMessageComposer({ roomId: 'room_456' });
      const editor = await findEditor(container);

      await expect.element(editor).toHaveTextContent('| Name | Role |');
      await placeCaretAtEditorEnd(editor);
      document.execCommand('insertText', false, '!');
      await vi.waitFor(() => expect(editor.textContent).toContain('| Ada | Admin |!'));

      (q(container, 'button[aria-label="Send message"]') as HTMLButtonElement).click();

      await vi.waitFor(() => expect(updateMessageConnectMock).toHaveBeenCalledOnce());
      const submittedBody = updateMessageConnectMock.mock.calls[0][0].body as string;
      expect(updateMessageConnectMock).toHaveBeenCalledWith({
        roomId: expect.any(String),
        eventId: 'evt_table',
        body: submittedBody
      });
      expect(submittedBody).toBe(`${body}!`);
      expect(await renderMarkdown(submittedBody)).toContain('<table>');
    });

    it('clears staged attachments when edit mode is active at mount', async () => {
      const roomId = 'room_edit_attachments';
      const firstRender = renderMessageComposer(
        { roomId },
        {
          exactRoomId: true
        }
      );
      const file = selectFirstAttachment(
        q(firstRender.container, 'input[type="file"]') as HTMLInputElement
      );
      await expect.poll(() => q(firstRender.container, 'img')).toBeTruthy();
      firstRender.unmount();

      // Stash an attachment draft for the same room, then mount directly into edit mode.
      // The composer should discard attachments because editMessage only supports text.
      roomStateMock.editState.eventId = 'evt_edit';
      roomStateMock.editState.originalBody = 'editable';
      const { container } = renderMessageComposer(
        { roomId },
        {
          exactRoomId: true
        }
      );
      expect(q(container, 'button[title="Attach file"]')).toBeNull();
      expect(file.name).toBe('paste.png');
      expect(q(container, 'img')).toBeNull();
    });

    it('converts a new typed code fence after an edited terminal code block', async () => {
      roomStateMock.editState.eventId = 'evt_edit';
      roomStateMock.editState.originalBody = '```ts\nconst existing = true;\n```';
      const { container } = renderMessageComposer({ roomId: 'room_456' });
      const editor = await findEditor(container);

      await vi.waitFor(() => expect(editor.querySelectorAll('pre code')).toHaveLength(1));
      await placeCaretAtEditorEnd(editor);
      await insertEditorLiteralText(editor, '```js ');

      await vi.waitFor(() => expect(editor.querySelectorAll('pre code')).toHaveLength(2));
      document.execCommand('insertText', false, 'console.log("second");');
      await vi.waitFor(() =>
        expect(editor.querySelectorAll('pre code')[1]?.textContent).toContain(
          'console.log("second");'
        )
      );
      (q(container, 'button[aria-label="Send message"]') as HTMLButtonElement).click();

      await vi.waitFor(() => expect(updateMessageConnectMock).toHaveBeenCalledOnce());
      expect(updateMessageConnectMock).toHaveBeenCalledWith({
        roomId: expect.any(String),
        eventId: 'evt_edit',
        body: '```ts\nconst existing = true;\n```\n\n```js\nconsole.log("second");\n```'
      });
    });
  });

  describe('submit behavior', () => {
    it('inserts a raw timestamp token from the picker before sending', async () => {
      const { container } = renderMessageComposer({ roomId: 'room_456' });
      const editor = await findEditor(container);

      await typeInEditor(editor, 'Call');
      await userEvent.click(q(container, 'button[aria-label="Insert timestamp"]')!);
      const dateTimeInput = document.querySelector(
        'input[type="datetime-local"]'
      ) as HTMLInputElement;
      const timezoneInput = document.querySelector(
        `input[list^="timestamp-timezones-"]`
      ) as HTMLInputElement;
      expect(dateTimeInput).toBeTruthy();
      expect(timezoneInput).toBeTruthy();
      await vi.waitFor(() => expect(document.activeElement).toBe(dateTimeInput));

      await changeInputValue(dateTimeInput, '2025-04-27T14:30');
      await changeInputValue(timezoneInput, 'UTC');
      await userEvent.click(document.querySelector('button[type="submit"]')!);

      await vi.waitFor(() => expect(editor.textContent).toContain('<t:1745764200:F>'));
      await pressEditorKey(editor, 'Enter', { ctrlKey: true });

      await vi.waitFor(() => expect(mutationMock).toHaveBeenCalled());
      expect(mutationMock.mock.calls[0][1].input).toMatchObject({
        body: 'Call <t:1745764200:F>'
      });
    });

    it('uses Enter to complete an active mention before Ctrl+Enter can send', async () => {
      roomStateMock.members = [roomMember('alice')];
      const { container, roomId } = renderMessageComposer({ roomId: 'room_456' });
      const editor = await findEditor(container);

      await typeEditorLiteralText(editor, '@ali');
      await vi.waitFor(() =>
        expect(container.querySelector('[data-testid="mention-autocomplete"]')).toBeTruthy()
      );

      await pressEditorKey(editor, 'Enter');

      await vi.waitFor(() => expect(editor.textContent).toBe('@alice '));
      expect(mutationMock).not.toHaveBeenCalled();

      await pressEditorKey(editor, 'Enter', { ctrlKey: true });

      await vi.waitFor(() => expect(mutationMock).toHaveBeenCalledOnce());
      expect(mutationMock.mock.calls[0][1].input).toMatchObject({
        roomId,
        body: '@alice'
      });
    });

    it('sends plain text with Ctrl+Enter', async () => {
      const { container, roomId } = renderMessageComposer({ roomId: 'room_456' });
      const editor = await findEditor(container);

      await typeEditorLiteralText(editor, 'hello from shortcut');
      await pressEditorKey(editor, 'Enter', { ctrlKey: true });

      await vi.waitFor(() => expect(mutationMock).toHaveBeenCalledOnce());
      expect(mutationMock.mock.calls[0][1].input).toMatchObject({
        roomId,
        body: 'hello from shortcut'
      });
    });

    it('lets bare Enter insert a line break on touch-primary devices', async () => {
      mockTouchPrimaryPointer();
      userPreferences.composerSendMode = 'enter';
      const { container, roomId } = renderMessageComposer({ roomId: 'room_456' });
      const editor = await findEditor(container);

      await typeEditorLiteralText(editor, 'first line');
      await pressEditorKey(editor, 'Enter');

      expect(mutationMock).not.toHaveBeenCalled();
      await vi.waitFor(() => expect(editor.querySelectorAll(':scope > p')).toHaveLength(2));

      await insertEditorLiteralText(editor, 'second line');
      (q(container, 'button[aria-label="Send message"]') as HTMLButtonElement).click();

      await vi.waitFor(() => expect(mutationMock).toHaveBeenCalledOnce());
      expect(mutationMock.mock.calls[0][1].input).toMatchObject({
        roomId,
        body: 'first line\n\nsecond line'
      });
    });

    it('inserts selected reply quotes without replacing draft text', async () => {
      let composerApi: MessageComposerApi | null = null;
      const { container } = renderMessageComposer({
        roomId: 'room_456',
        onReady: (api) => {
          composerApi = api;
        }
      });
      const editor = await findEditor(container);

      await typeInEditor(editor, 'draft');
      await vi.waitFor(() => expect(composerApi).not.toBeNull());
      composerApi!.insertQuote('quoted text');

      await vi.waitFor(() => expect(editor.querySelector('blockquote')).toBeTruthy());
      expect(editor.textContent).toContain('draft');
      expect(editor.querySelector('blockquote')?.textContent).toBe('quoted text');
    });

    it('submits inserted selected reply quotes as blockquote markdown', async () => {
      let composerApi: MessageComposerApi | null = null;
      const { container, roomId } = renderMessageComposer({
        roomId: 'room_456',
        onReady: (api) => {
          composerApi = api;
        }
      });
      const editor = await findEditor(container);

      await typeInEditor(editor, 'draft');
      await vi.waitFor(() => expect(composerApi).not.toBeNull());
      composerApi!.insertQuote('quoted text');
      await vi.waitFor(() => expect(editor.querySelector('blockquote')).toBeTruthy());

      (q(container, 'button[aria-label="Send message"]') as HTMLButtonElement).click();

      await vi.waitFor(() => expect(mutationMock).toHaveBeenCalledOnce());
      expect(mutationMock.mock.calls[0][1].input).toMatchObject({
        roomId,
        body: 'draft\n\n> quoted text'
      });
    });

    it('preserves line breaks inside inserted reply quotes', async () => {
      let composerApi: MessageComposerApi | null = null;
      const { container } = renderMessageComposer({
        roomId: 'room_456',
        onReady: (api) => {
          composerApi = api;
        }
      });
      const editor = await findEditor(container);

      await vi.waitFor(() => expect(composerApi).not.toBeNull());
      composerApi!.insertQuote('first line\nsecond line');

      await vi.waitFor(() => expect(editor.querySelector('blockquote')).toBeTruthy());
      expect(editor.querySelectorAll('blockquote p')).toHaveLength(2);
      expect(editor.querySelector('blockquote')?.textContent).toBe('first linesecond line');
    });

    it('renders structured selected reply quotes as nested blockquotes', async () => {
      let composerApi: MessageComposerApi | null = null;
      const { container } = renderMessageComposer({
        roomId: 'room_456',
        onReady: (api) => {
          composerApi = api;
        }
      });
      const editor = await findEditor(container);

      await vi.waitFor(() => expect(composerApi).not.toBeNull());
      composerApi!.insertQuote([
        { quoteDepth: 0, text: 'a' },
        { quoteDepth: 1, text: 'nice, love it' },
        { quoteDepth: 0, text: ':D' }
      ]);

      await vi.waitFor(() => expect(editor.querySelector('blockquote blockquote')).toBeTruthy());
      const outerQuote = editor.querySelector('blockquote')!;
      const outerParagraphs = Array.from(outerQuote.children).filter(
        (child): child is HTMLParagraphElement => child instanceof HTMLParagraphElement
      );
      expect(outerParagraphs.map((paragraph) => paragraph.textContent)).toEqual(['a', ':D']);
      expect(editor.querySelector('blockquote blockquote p')?.textContent).toBe('nice, love it');
    });

    it('submits structured selected reply quotes as nested blockquote markdown', async () => {
      let composerApi: MessageComposerApi | null = null;
      const { container, roomId } = renderMessageComposer({
        roomId: 'room_456',
        onReady: (api) => {
          composerApi = api;
        }
      });
      const editor = await findEditor(container);

      await typeInEditor(editor, 'draft');
      await vi.waitFor(() => expect(composerApi).not.toBeNull());
      composerApi!.insertQuote([{ quoteDepth: 1, text: 'quoted text' }]);
      await vi.waitFor(() => expect(editor.querySelector('blockquote blockquote')).toBeTruthy());

      (q(container, 'button[aria-label="Send message"]') as HTMLButtonElement).click();

      await vi.waitFor(() => expect(mutationMock).toHaveBeenCalledOnce());
      expect(mutationMock.mock.calls[0][1].input).toMatchObject({
        roomId,
        body: 'draft\n\n> > quoted text'
      });
    });

    it('inserts a normal newline and sends with Ctrl+Enter', async () => {
      const { container, roomId } = renderMessageComposer({ roomId: 'room_456' });
      const editor = await findEditor(container);

      await typeEditorLiteralText(editor, 'hello from shortcut');
      await pressEditorKey(editor, 'Enter');
      expect(mutationMock).not.toHaveBeenCalled();
      await vi.waitFor(() => expect(editor.querySelectorAll(':scope > p')).toHaveLength(2));

      await pressEditorKey(editor, 'Enter', { ctrlKey: true });

      await vi.waitFor(() => expect(mutationMock).toHaveBeenCalledOnce());
      expect(mutationMock.mock.calls[0][1].input).toMatchObject({
        roomId,
        body: 'hello from shortcut'
      });
    });

    it('uses Ctrl+Enter for a structural paragraph break when Return sends', async () => {
      userPreferences.composerSendMode = 'enter';
      const { container, roomId } = renderMessageComposer({ roomId: 'room-enter-send' });
      const editor = await findEditor(container);

      await typeEditorLiteralText(editor, 'first');
      await pressEditorKey(editor, 'Enter', { ctrlKey: true });
      expect(mutationMock).not.toHaveBeenCalled();
      await vi.waitFor(() => expect(editor.querySelectorAll(':scope > p')).toHaveLength(2));
      await insertEditorLiteralText(editor, 'second');

      await pressEditorKey(editor, 'Enter');
      await vi.waitFor(() => expect(mutationMock).toHaveBeenCalledOnce());
      expect(mutationMock.mock.calls[0][1].input).toMatchObject({
        roomId,
        body: 'first\n\nsecond'
      });
    });

    it('keeps Shift+Enter as a visual hard break when Return sends', async () => {
      userPreferences.composerSendMode = 'enter';
      const { container } = renderMessageComposer({ roomId: 'room-hard-break' });
      const editor = await findEditor(container);

      await typeEditorLiteralText(editor, 'first');
      await pressEditorKey(editor, 'Enter', { shiftKey: true });
      await insertEditorLiteralText(editor, 'second');

      expect(mutationMock).not.toHaveBeenCalled();
      expect(editor.querySelectorAll(':scope > p')).toHaveLength(1);
      expect(editor.querySelector('p > br')).toBeTruthy();
    });

    it('does not send while an input method is composing text', async () => {
      userPreferences.composerSendMode = 'enter';
      const { container } = renderMessageComposer({ roomId: 'room-composing-enter' });
      const editor = await findEditor(container);

      await typeEditorLiteralText(editor, 'composing');
      await pressEditorKey(editor, 'Enter', { isComposing: true });

      expect(mutationMock).not.toHaveBeenCalled();
    });

    it('posts markdown after TipTap formatting shortcuts are applied', async () => {
      const { container, roomId } = renderMessageComposer({ roomId: 'room_456' });
      const editor = await findEditor(container);

      await typeEditorKeys(editor, '**bold**');
      await vi.waitFor(() => expect(editor.querySelector('strong')?.textContent).toBe('bold'));
      (q(container, 'button[aria-label="Send message"]') as HTMLButtonElement).click();

      await vi.waitFor(() => expect(mutationMock).toHaveBeenCalledOnce());
      expect(mutationMock.mock.calls[0][1].input).toMatchObject({
        roomId,
        body: '**bold**'
      });
    });

    it('posts markdown after composer formatting buttons are applied', async () => {
      const { container, roomId } = renderMessageComposer({ roomId: 'room_456' });
      const editor = await findEditor(container);
      await openFormattingShelf(container);
      const boldButton = q(container, 'button[aria-label="Bold"]') as HTMLButtonElement;

      await userEvent.click(boldButton);
      await expect.element(boldButton).toHaveAttribute('aria-pressed', 'true');
      await insertEditorLiteralText(editor, 'bold');
      await vi.waitFor(() => expect(editor.querySelector('strong')?.textContent).toBe('bold'));
      (q(container, 'button[aria-label="Send message"]') as HTMLButtonElement).click();

      await vi.waitFor(() => expect(mutationMock).toHaveBeenCalledOnce());
      expect(mutationMock.mock.calls[0][1].input).toMatchObject({
        roomId,
        body: '**bold**'
      });
    });

    it('clears inline code formatting after an inline-code-only draft is deleted', async () => {
      const { container, roomId } = renderMessageComposer({ roomId: 'room_456' });
      const editor = await findEditor(container);

      await typeEditorKeys(editor, '`code`');
      await vi.waitFor(() => expect(editor.querySelector('p code')?.textContent).toBe('code'));

      document.execCommand('selectAll');
      document.execCommand('delete');
      await tick();
      await vi.waitFor(() => expect(editor.textContent).toBe(''));

      await insertEditorLiteralText(editor, 'plain');
      await vi.waitFor(() => {
        expect(editor.textContent).toBe('plain');
        expect(editor.querySelector('p code')).toBeNull();
      });

      (q(container, 'button[aria-label="Send message"]') as HTMLButtonElement).click();

      await vi.waitFor(() => expect(mutationMock).toHaveBeenCalledOnce());
      expect(mutationMock.mock.calls[0][1].input).toMatchObject({
        roomId,
        body: 'plain'
      });
    });

    it('posts markdown after typed markdown link syntax is applied', async () => {
      const { container, roomId } = renderMessageComposer({ roomId: 'room_456' });
      const editor = await findEditor(container);

      await typeEditorLiteralText(editor, '[example](https://example.com)');
      await vi.waitFor(() => {
        const link = editor.querySelector('a');
        expect(link?.textContent).toBe('example');
        expect(link?.getAttribute('href')).toBe('https://example.com');
      });
      (q(container, 'button[aria-label="Send message"]') as HTMLButtonElement).click();

      await vi.waitFor(() => expect(mutationMock).toHaveBeenCalledOnce());
      expect(mutationMock.mock.calls[0][1].input).toMatchObject({
        roomId,
        body: '[example](https://example.com)'
      });
    });

    it('round-trips a pasted labelled Markdown channel link', async () => {
      const body = '[#announcements](https://chat.chatto.run/chat/-/REEi0LIuxqwQl3F)';
      const { container, roomId } = renderMessageComposer({ roomId: 'room_456' });
      const editor = await findEditor(container);

      editor.focus();
      pasteText(editor, body);

      await vi.waitFor(() => {
        const link = editor.querySelector('a');
        expect(link?.textContent).toBe('#announcements');
        expect(link?.getAttribute('href')).toBe('https://chat.chatto.run/chat/-/REEi0LIuxqwQl3F');
      });
      (q(container, 'button[aria-label="Send message"]') as HTMLButtonElement).click();

      await vi.waitFor(() => expect(mutationMock).toHaveBeenCalledOnce());
      expect(mutationMock.mock.calls[0][1].input).toMatchObject({ roomId, body });
    });

    it('round-trips a pasted Markdown channel link surrounded by prose', async () => {
      const url = 'https://chat.chatto.run/chat/-/REEi0LIuxqwQl3F';
      const body = `See [#announcements](${url}) for details`;
      const { container, roomId } = renderMessageComposer({ roomId: 'room_456' });
      const editor = await findEditor(container);

      editor.focus();
      pasteText(editor, body);

      await vi.waitFor(() => {
        expect(editor.textContent).toBe('See #announcements for details');
        const link = editor.querySelector('a');
        expect(link?.textContent).toBe('#announcements');
        expect(link?.getAttribute('href')).toBe(url);
      });
      (q(container, 'button[aria-label="Send message"]') as HTMLButtonElement).click();

      await vi.waitFor(() => expect(mutationMock).toHaveBeenCalledOnce());
      expect(mutationMock.mock.calls[0][1].input).toMatchObject({ roomId, body });
    });

    it('uses the Markdown link from plain text when pasted HTML is also available', async () => {
      const url = 'https://chat.chatto.run/chat/-/REEi0LIuxqwQl3F';
      const body = `[#announcements](${url})`;
      const { container, roomId } = renderMessageComposer({ roomId: 'room_456' });
      const editor = await findEditor(container);

      editor.focus();
      pasteText(editor, body, `<a href="https://wrong.example/">wrong label</a>`);

      await vi.waitFor(() => {
        const link = editor.querySelector('a');
        expect(link?.textContent).toBe('#announcements');
        expect(link?.getAttribute('href')).toBe(url);
      });
      (q(container, 'button[aria-label="Send message"]') as HTMLButtonElement).click();

      await vi.waitFor(() => expect(mutationMock).toHaveBeenCalledOnce());
      expect(mutationMock.mock.calls[0][1].input).toMatchObject({ roomId, body });
    });

    it('preserves a single pasted line break without creating separate paragraphs', async () => {
      const { container, roomId } = renderMessageComposer({ roomId: 'room_456' });
      const editor = await findEditor(container);

      editor.focus();
      pasteText(editor, 'test line one\ntest line two');

      await vi.waitFor(() => {
        expect(editor.querySelectorAll(':scope > p')).toHaveLength(1);
        expect(editor.querySelectorAll('br')).toHaveLength(1);
      });

      (q(container, 'button[aria-label="Send message"]') as HTMLButtonElement).click();

      await vi.waitFor(() => expect(mutationMock).toHaveBeenCalledOnce());
      expect(mutationMock.mock.calls[0][1].input).toMatchObject({
        roomId,
        body: 'test line one  \ntest line two'
      });
    });

    it('submits pasted GFM table syntax in a renderable form', async () => {
      const body = '| Name | Role |\n| --- | --- |\n| Ada | Admin |';
      const { container, roomId } = renderMessageComposer({ roomId: 'room_456' });
      const editor = await findEditor(container);

      editor.focus();
      pasteText(editor, body);

      await vi.waitFor(() => expect(editor.querySelectorAll('br')).toHaveLength(2));
      (q(container, 'button[aria-label="Send message"]') as HTMLButtonElement).click();

      await vi.waitFor(() => expect(mutationMock).toHaveBeenCalledOnce());
      const submittedBody = mutationMock.mock.calls[0][1].input.body as string;
      expect(mutationMock.mock.calls[0][1].input).toMatchObject({ roomId });
      expect(await renderMarkdown(submittedBody)).toContain('<table>');
    });

    it('converts pasted blockquote Markdown and posts it as a rendered quote', async () => {
      const body = '> moo';
      const { container, roomId } = renderMessageComposer({ roomId: 'room_456' });
      const editor = await findEditor(container);

      editor.focus();
      pasteText(editor, body);

      await vi.waitFor(() => {
        expect(editor.querySelector('blockquote')?.textContent).toBe('moo');
      });
      (q(container, 'button[aria-label="Send message"]') as HTMLButtonElement).click();

      await vi.waitFor(() => expect(mutationMock).toHaveBeenCalledOnce());
      expect(mutationMock.mock.calls[0][1].input).toMatchObject({ roomId, body });
    });

    it('preserves active inline formatting when pasting plain text', async () => {
      const { container, roomId } = renderMessageComposer({ roomId: 'room_456' });
      const editor = await findEditor(container);

      editor.focus();
      await pressEditorKey(
        editor,
        'b',
        navigator.platform.startsWith('Mac') ? { metaKey: true } : { ctrlKey: true }
      );
      pasteText(editor, 'pasted text');

      await vi.waitFor(() => {
        expect(editor.querySelector('strong')?.textContent).toBe('pasted text');
      });
      (q(container, 'button[aria-label="Send message"]') as HTMLButtonElement).click();

      await vi.waitFor(() => expect(mutationMock).toHaveBeenCalledOnce());
      expect(mutationMock.mock.calls[0][1].input).toMatchObject({
        roomId,
        body: '**pasted text**'
      });
    });

    it('combines active inline formatting with a pasted autolink', async () => {
      const url = 'https://example.com/';
      const { container } = renderMessageComposer({ roomId: 'room_456' });
      const editor = await findEditor(container);

      editor.focus();
      await pressEditorKey(
        editor,
        'b',
        navigator.platform.startsWith('Mac') ? { metaKey: true } : { ctrlKey: true }
      );
      pasteText(editor, url);

      await vi.waitFor(() => {
        expect(editor.querySelector('a')?.textContent).toBe(url);
        expect(editor.querySelector('strong')?.textContent).toBe(url);
      });
    });

    it('pastes unsupported Markdown syntax literally over selected text', async () => {
      const { container } = renderMessageComposer({ roomId: 'room_456' });
      const editor = await findEditor(container);

      await typeInEditor(editor, 'replace me');
      await selectEditorContents(editor);
      pasteText(editor, '---');

      await vi.waitFor(() => {
        expect(editor.textContent).toBe('---');
        expect(editor.querySelector('hr')).toBeNull();
      });
    });

    it('preserves an intentional blank line in pasted text', async () => {
      const body = 'first paragraph\n\nsecond paragraph';
      const { container, roomId } = renderMessageComposer({ roomId: 'room_456' });
      const editor = await findEditor(container);

      editor.focus();
      pasteText(editor, body);

      await vi.waitFor(() => expect(editor.querySelectorAll(':scope > p')).toHaveLength(2));
      (q(container, 'button[aria-label="Send message"]') as HTMLButtonElement).click();

      await vi.waitFor(() => expect(mutationMock).toHaveBeenCalledOnce());
      expect(mutationMock.mock.calls[0][1].input).toMatchObject({ roomId, body });
    });

    it('preserves pasted fenced code without adding line breaks between source lines', async () => {
      const body = [
        '```go',
        'type Conn struct {',
        '\trwc     io.ReadWriteCloser',
        '\terr     error',
        '\tr, w, x sync.Mutex',
        '}',
        '```'
      ].join('\n');
      const { container, roomId } = renderMessageComposer({ roomId: 'room_456' });
      const editor = await findEditor(container);

      editor.focus();
      pasteText(editor, body);

      await vi.waitFor(() => {
        expect(editor.querySelector('pre code')?.textContent).toBe(
          'type Conn struct {\n\trwc     io.ReadWriteCloser\n\terr     error\n\tr, w, x sync.Mutex\n}'
        );
      });
      (q(container, 'button[aria-label="Send message"]') as HTMLButtonElement).click();

      await vi.waitFor(() => expect(mutationMock).toHaveBeenCalledOnce());
      expect(mutationMock.mock.calls[0][1].input).toMatchObject({ roomId, body });
    });

    it('prefers plain text when pasted clipboard data also contains HTML', async () => {
      const { container, roomId } = renderMessageComposer({ roomId: 'room_456' });
      const editor = await findEditor(container);

      editor.focus();
      pasteText(editor, 'plain line one\nplain line two', '<p>wrong HTML</p><p>content</p>');

      await vi.waitFor(() => {
        expect(editor.textContent).toBe('plain line oneplain line two');
        expect(editor.querySelectorAll(':scope > p')).toHaveLength(1);
      });
      (q(container, 'button[aria-label="Send message"]') as HTMLButtonElement).click();

      await vi.waitFor(() => expect(mutationMock).toHaveBeenCalledOnce());
      expect(mutationMock.mock.calls[0][1].input).toMatchObject({
        roomId,
        body: 'plain line one  \nplain line two'
      });
    });

    it('preserves multiline text pasted while editing', async () => {
      roomStateMock.editState.eventId = 'evt_edit';
      roomStateMock.editState.originalBody = 'original body';
      const { container } = renderMessageComposer({ roomId: 'room_456' });
      const editor = await findEditor(container);

      await vi.waitFor(() => expect(editor.textContent).toBe('original body'));
      await placeCaretAtEditorEnd(editor);
      pasteText(editor, 'edited line one\nedited line two');

      await vi.waitFor(() => expect(editor.querySelectorAll('br')).toHaveLength(1));
      (q(container, 'button[aria-label="Send message"]') as HTMLButtonElement).click();

      await vi.waitFor(() => expect(updateMessageConnectMock).toHaveBeenCalledOnce());
      expect(updateMessageConnectMock).toHaveBeenCalledWith({
        roomId: expect.any(String),
        eventId: 'evt_edit',
        body: 'original bodyedited line one  \nedited line two'
      });
    });

    it('pastes multiline text literally inside an active code block', async () => {
      const { container, roomId } = renderMessageComposer({ roomId: 'room_456' });
      const editor = await findEditor(container);

      await typeEditorLiteralText(editor, '```go ');
      await vi.waitFor(() => expect(editor.querySelector('pre code')).toBeTruthy());
      pasteText(editor, 'line one\nline two');

      await vi.waitFor(() => {
        expect(editor.querySelector('pre code')?.textContent).toBe('line one\nline two');
      });
      (q(container, 'button[aria-label="Send message"]') as HTMLButtonElement).click();

      await vi.waitFor(() => expect(mutationMock).toHaveBeenCalledOnce());
      expect(mutationMock.mock.calls[0][1].input).toMatchObject({
        roomId,
        body: '```go\nline one\nline two\n```'
      });
    });

    it('keeps typed space after a pasted autolink outside the link', async () => {
      const url = 'https://www.spiegel.de/';
      const { container } = renderMessageComposer({ roomId: 'room_456' });
      const editor = await findEditor(container);

      editor.focus();
      pasteText(editor, url);
      await vi.waitFor(() => {
        const link = editor.querySelector('a');
        expect(link?.textContent).toBe(url);
        expect(link?.getAttribute('href')).toBe(url);
      });

      await insertEditorLiteralText(editor, ' after');

      await vi.waitFor(() => {
        const links = editor.querySelectorAll('a');
        expect(links).toHaveLength(1);
        expect(links[0]?.textContent).toBe(url);
        expect(editor.textContent).toBe(`${url} after`);
      });
    });

    it('posts fresh literal HTML-looking text without entity corruption', async () => {
      const body = '<script>alert(1)</script> & <b>bold?</b> &lt;x&gt;';
      const { container, roomId } = renderMessageComposer({ roomId: 'room_456' });
      const editor = await findEditor(container);

      await typeInEditor(editor, body);
      (q(container, 'button[aria-label="Send message"]') as HTMLButtonElement).click();

      await vi.waitFor(() => expect(mutationMock).toHaveBeenCalledOnce());
      expect(mutationMock.mock.calls[0][1].input).toMatchObject({
        roomId,
        body
      });
    });

    it('posts fresh plain less-than text without entity corruption', async () => {
      const body = 'x < 5';
      const { container, roomId } = renderMessageComposer({ roomId: 'room_456' });
      const editor = await findEditor(container);

      await typeInEditor(editor, body);
      (q(container, 'button[aria-label="Send message"]') as HTMLButtonElement).click();

      await vi.waitFor(() => expect(mutationMock).toHaveBeenCalledOnce());
      expect(mutationMock.mock.calls[0][1].input).toMatchObject({
        roomId,
        body
      });
    });

    it('posts fresh plain greater-than text without entity corruption', async () => {
      const body = 'x > 5';
      const { container, roomId } = renderMessageComposer({ roomId: 'room_456' });
      const editor = await findEditor(container);

      await typeInEditor(editor, body);
      (q(container, 'button[aria-label="Send message"]') as HTMLButtonElement).click();

      await vi.waitFor(() => expect(mutationMock).toHaveBeenCalledOnce());
      expect(mutationMock.mock.calls[0][1].input).toMatchObject({
        roomId,
        body
      });
    });

    it('escapes fresh leading blockquote markers typed as literal text', async () => {
      const { container, roomId } = renderMessageComposer({ roomId: 'room_456' });
      const editor = await findEditor(container);

      await typeInEditor(editor, '> not a quote');
      (q(container, 'button[aria-label="Send message"]') as HTMLButtonElement).click();

      await vi.waitFor(() => expect(mutationMock).toHaveBeenCalledOnce());
      expect(mutationMock.mock.calls[0][1].input).toMatchObject({
        roomId,
        body: '&gt; not a quote'
      });
    });

    it('edits the active markdown link href from the composer controls', async () => {
      const { container, roomId } = renderMessageComposer({ roomId: 'room_456' });
      const editor = await findEditor(container);

      await typeEditorLiteralText(editor, '[example](https://example.com)');
      await vi.waitFor(() => expect(editor.querySelector('a')?.textContent).toBe('example'));

      const hrefInput = q(container, 'input[aria-label="Link URL"]') as HTMLInputElement;
      await expect.element(hrefInput).toHaveValue('https://example.com');
      await changeInputValue(hrefInput, 'https://chatto.test/docs');

      await vi.waitFor(() =>
        expect(editor.querySelector('a')?.getAttribute('href')).toBe('https://chatto.test/docs')
      );
      (q(container, 'button[aria-label="Send message"]') as HTMLButtonElement).click();

      await vi.waitFor(() => expect(mutationMock).toHaveBeenCalledOnce());
      expect(mutationMock.mock.calls[0][1].input).toMatchObject({
        roomId,
        body: '[example](https://chatto.test/docs)'
      });
    });

    it('removes the active markdown link from the composer controls', async () => {
      const { container, roomId } = renderMessageComposer({ roomId: 'room_456' });
      const editor = await findEditor(container);

      await typeEditorLiteralText(editor, '[example](https://example.com)');
      await vi.waitFor(() => expect(editor.querySelector('a')?.textContent).toBe('example'));

      (q(container, 'button[title="Remove link"]') as HTMLButtonElement).click();
      await vi.waitFor(() => expect(editor.querySelector('a')).toBeNull());
      (q(container, 'button[aria-label="Send message"]') as HTMLButtonElement).click();

      await vi.waitFor(() => expect(mutationMock).toHaveBeenCalledOnce());
      expect(mutationMock.mock.calls[0][1].input).toMatchObject({
        roomId,
        body: 'example'
      });
    });

    it('posts fenced markdown after typed code fence syntax is applied', async () => {
      const { container, roomId } = renderMessageComposer({ roomId: 'room_456' });
      const editor = await findEditor(container);

      await typeEditorLiteralText(editor, '```ts ');
      await vi.waitFor(() => expect(editor.querySelector('pre code')).toBeTruthy());

      document.execCommand('insertText', false, 'const answer = 42;');
      await vi.waitFor(() =>
        expect(editor.querySelector('pre code')?.textContent).toContain('const answer = 42;')
      );
      (q(container, 'button[aria-label="Send message"]') as HTMLButtonElement).click();

      await vi.waitFor(() => expect(mutationMock).toHaveBeenCalledOnce());
      expect(mutationMock.mock.calls[0][1].input).toMatchObject({
        roomId,
        body: '```ts\nconst answer = 42;\n```'
      });
    });

    it('converts a code fence on the current visual line after normal text', async () => {
      const { container, roomId } = renderMessageComposer({ roomId: 'room_456' });
      const editor = await findEditor(container);

      await typeEditorLiteralText(editor, 'or this:');
      await pressEditorKey(editor, 'Enter');
      await insertEditorLiteralText(editor, '```go ');

      await vi.waitFor(() => expect(editor.querySelector('pre code')).toBeTruthy());
      document.execCommand('insertText', false, 'IO.puts("moo")');
      await vi.waitFor(() =>
        expect(editor.querySelector('pre code')?.textContent).toContain('IO.puts("moo")')
      );

      (q(container, 'button[aria-label="Send message"]') as HTMLButtonElement).click();

      await vi.waitFor(() => expect(mutationMock).toHaveBeenCalledOnce());
      expect(mutationMock.mock.calls[0][1].input).toMatchObject({
        roomId,
        body: 'or this:\n\n```go\nIO.puts("moo")\n```'
      });
    });

    it('converts a second code fence after an existing code block and normal text', async () => {
      roomStateMock.editState.eventId = 'evt_edit';
      roomStateMock.editState.originalBody = '```text\nmoo\nquack\n```';
      const { container } = renderMessageComposer({ roomId: 'room_456' });
      const editor = await findEditor(container);

      await vi.waitFor(() => expect(editor.querySelectorAll('pre code')).toHaveLength(1));
      await placeCaretAtEditorEnd(editor);
      await insertEditorLiteralText(editor, 'or this:');
      await pressEditorKey(editor, 'Enter');
      await insertEditorLiteralText(editor, '```go ');
      await vi.waitFor(() => expect(editor.querySelectorAll('pre code')).toHaveLength(2));

      document.execCommand('insertText', false, 'IO.puts("moo")');
      await vi.waitFor(() =>
        expect(editor.querySelectorAll('pre code')[1]?.textContent).toContain('IO.puts("moo")')
      );
      (q(container, 'button[aria-label="Send message"]') as HTMLButtonElement).click();

      await vi.waitFor(() => expect(updateMessageConnectMock).toHaveBeenCalledOnce());
      expect(updateMessageConnectMock).toHaveBeenCalledWith({
        roomId: expect.any(String),
        eventId: 'evt_edit',
        body: '```text\nmoo\nquack\n```\n\nor this:\n\n```go\nIO.puts("moo")\n```'
      });
    });

    it('uses native Enter for a structural newline inside an active code block', async () => {
      const { container, roomId } = renderMessageComposer({ roomId: 'room_456' });
      const editor = await findEditor(container);

      await typeEditorLiteralText(editor, '```ts ');
      await vi.waitFor(() => expect(editor.querySelector('pre code')).toBeTruthy());

      document.execCommand('insertText', false, 'const first = 1;');
      await vi.waitFor(() =>
        expect(editor.querySelector('pre code')?.textContent).toContain('const first = 1;')
      );
      await pressEditorKey(editor, 'Enter');
      document.execCommand('insertText', false, 'const second = 2;');

      await vi.waitFor(() =>
        expect(editor.querySelector('pre code')?.textContent).toContain(
          'const first = 1;\nconst second = 2;'
        )
      );
      expect(editor.querySelectorAll('pre code')).toHaveLength(1);

      (q(container, 'button[aria-label="Send message"]') as HTMLButtonElement).click();

      await vi.waitFor(() => expect(mutationMock).toHaveBeenCalledOnce());
      expect(mutationMock.mock.calls[0][1].input).toMatchObject({
        roomId,
        body: '```ts\nconst first = 1;\nconst second = 2;\n```'
      });
    });

    it('lets native Enter insert a structural paragraph break without submitting', async () => {
      const { container, roomId } = renderMessageComposer({ roomId: 'room_456' });
      const editor = await findEditor(container);

      await typeEditorLiteralText(editor, 'first');
      await pressEditorKey(editor, 'Enter');
      expect(mutationMock).not.toHaveBeenCalled();
      document.execCommand('insertText', false, 'second');
      await vi.waitFor(() => expect(editor.querySelectorAll(':scope > p')).toHaveLength(2));

      (q(container, 'button[aria-label="Send message"]') as HTMLButtonElement).click();

      await vi.waitFor(() => expect(mutationMock).toHaveBeenCalledOnce());
      expect(mutationMock.mock.calls[0][1].input).toMatchObject({
        roomId,
        body: 'first\n\nsecond'
      });
    });

    it('sends with Cmd+Enter inside an active code block', async () => {
      const { container, roomId } = renderMessageComposer({ roomId: 'room_456' });
      const editor = await findEditor(container);

      await typeEditorLiteralText(editor, '```ts ');
      await vi.waitFor(() => expect(editor.querySelector('pre code')).toBeTruthy());
      document.execCommand('insertText', false, 'const answer = 42;');
      await vi.waitFor(() =>
        expect(editor.querySelector('pre code')?.textContent).toContain('const answer = 42;')
      );

      await pressEditorKey(editor, 'Enter', { metaKey: true });

      await vi.waitFor(() => expect(mutationMock).toHaveBeenCalledOnce());
      expect(mutationMock.mock.calls[0][1].input).toMatchObject({
        roomId,
        body: '```ts\nconst answer = 42;\n```'
      });
    });

    it('lets native Enter create another bullet list item instead of submitting', async () => {
      const { container, roomId } = renderMessageComposer({ roomId: 'room_456' });
      const editor = await findEditor(container);

      await typeEditorLiteralText(editor, '- first');
      await vi.waitFor(() => expect(editor.querySelector('ul li')?.textContent).toBe('first'));
      await pressEditorKey(editor, 'Enter');
      expect(mutationMock).not.toHaveBeenCalled();

      document.execCommand('insertText', false, 'second');
      await vi.waitFor(() => expect(editor.querySelectorAll('ul li')).toHaveLength(2));
      await vi.waitFor(() =>
        expect(editor.querySelectorAll('ul li')[1]?.textContent).toBe('second')
      );

      (q(container, 'button[aria-label="Send message"]') as HTMLButtonElement).click();

      await vi.waitFor(() => expect(mutationMock).toHaveBeenCalledOnce());
      expect(mutationMock.mock.calls[0][1].input).toMatchObject({
        roomId,
        body: '- first\n- second'
      });
    });

    it('indents visual list items with both the toolbar and Tab', async () => {
      const { container } = renderMessageComposer({ roomId: 'visual-list-indent' });
      const editor = await findEditor(container);
      await openFormattingShelf(container);
      const indent = q(container, 'button[aria-label="Indent"]') as HTMLButtonElement;
      const outdent = q(container, 'button[aria-label="Outdent"]') as HTMLButtonElement;

      await typeEditorLiteralText(editor, '- first');
      await pressEditorKey(editor, 'Enter');
      document.execCommand('insertText', false, 'second');
      await vi.waitFor(() => expect(indent.disabled).toBe(false));

      await userEvent.click(indent);
      await vi.waitFor(() => expect(editor.querySelectorAll('ul ul li')).toHaveLength(1));
      await pressEditorKey(editor, 'Tab', { shiftKey: true });
      await vi.waitFor(() => expect(editor.querySelectorAll(':scope > ul > li')).toHaveLength(2));

      await pressEditorKey(editor, 'Tab');
      await vi.waitFor(() => expect(editor.querySelectorAll('ul ul li')).toHaveLength(1));
      await userEvent.click(outdent);
      await vi.waitFor(() => expect(editor.querySelectorAll(':scope > ul > li')).toHaveLength(2));
    });

    it('sends with Ctrl+Enter from the visible trailing paragraph after leaving a bullet list', async () => {
      const { container, roomId } = renderMessageComposer({ roomId: 'room_456' });
      const editor = await findEditor(container);

      await typeEditorLiteralText(editor, '- first');
      await vi.waitFor(() => expect(editor.querySelector('ul li')?.textContent).toBe('first'));
      await pressEditorKey(editor, 'Enter');
      await vi.waitFor(() => expect(editor.querySelectorAll('ul li')).toHaveLength(2));
      await pressEditorKey(editor, 'Enter');
      expect(mutationMock).not.toHaveBeenCalled();
      await vi.waitFor(() => expect(editor.querySelectorAll('ul li')).toHaveLength(1));

      await pressEditorKey(editor, 'Enter', { ctrlKey: true });

      await vi.waitFor(() => expect(mutationMock).toHaveBeenCalledOnce());
      expect(mutationMock.mock.calls[0][1].input).toMatchObject({
        roomId,
        body: '- first'
      });
    });

    it('sends with Cmd+Enter inside a bullet list', async () => {
      const { container, roomId } = renderMessageComposer({ roomId: 'room_456' });
      const editor = await findEditor(container);

      await typeEditorLiteralText(editor, '- first');
      await vi.waitFor(() => expect(editor.querySelector('ul li')?.textContent).toBe('first'));
      await pressEditorKey(editor, 'Enter', { metaKey: true });

      await vi.waitFor(() => expect(mutationMock).toHaveBeenCalledOnce());
      expect(mutationMock.mock.calls[0][1].input).toMatchObject({
        roomId,
        body: '- first'
      });
    });

    it('starts a bullet list from a visual line after structural breaks', async () => {
      const { container, roomId } = renderMessageComposer({ roomId: 'room_456' });
      const editor = await findEditor(container);

      await typeEditorLiteralText(editor, 'Things I hate:');
      await pressEditorKey(editor, 'Enter');
      await pressEditorKey(editor, 'Enter');
      await insertEditorLiteralText(editor, '- ');
      await vi.waitFor(() => expect(editor.querySelector('ul li')).toBeTruthy());
      expect(editor.querySelectorAll(':scope > p')).toHaveLength(2);

      document.execCommand('insertText', false, 'lists');
      await vi.waitFor(() => expect(editor.querySelector('ul li')?.textContent).toBe('lists'));

      (q(container, 'button[aria-label="Send message"]') as HTMLButtonElement).click();

      await vi.waitFor(() => expect(mutationMock).toHaveBeenCalledOnce());
      expect(mutationMock.mock.calls[0][1].input).toMatchObject({
        roomId,
        body: 'Things I hate:\n\n- lists'
      });
    });

    it('starts an ordered list from a visual line after a structural break', async () => {
      const { container, roomId } = renderMessageComposer({ roomId: 'room_456' });
      const editor = await findEditor(container);

      await typeEditorLiteralText(editor, 'Things I like:');
      await pressEditorKey(editor, 'Enter');
      await insertEditorLiteralText(editor, '1. ');
      await vi.waitFor(() => expect(editor.querySelector('ol li')).toBeTruthy());

      document.execCommand('insertText', false, 'lists');
      await vi.waitFor(() => expect(editor.querySelector('ol li')?.textContent).toBe('lists'));

      (q(container, 'button[aria-label="Send message"]') as HTMLButtonElement).click();

      await vi.waitFor(() => expect(mutationMock).toHaveBeenCalledOnce());
      expect(mutationMock.mock.calls[0][1].input).toMatchObject({
        roomId,
        body: 'Things I like:\n\n1. lists'
      });
    });

    it('lets native Enter leave a heading without submitting', async () => {
      const { container, roomId } = renderMessageComposer({ roomId: 'room_456' });
      const editor = await findEditor(container);

      await typeEditorLiteralText(editor, '# Heading');
      await vi.waitFor(() => expect(editor.querySelector('h1')?.textContent).toBe('Heading'));
      expect(Array.from(editor.children).map((child) => child.tagName)).toEqual(['H1']);
      await pressEditorKey(editor, 'Enter');
      expect(mutationMock).not.toHaveBeenCalled();

      document.execCommand('insertText', false, 'body');
      await vi.waitFor(() => expect(editor.querySelector('p')?.textContent).toBe('body'));
      expect(getComputedStyle(editor.querySelector('p')!).marginTop).not.toBe('0px');
      await pressEditorKey(editor, 'Enter');
      expect(mutationMock).not.toHaveBeenCalled();

      await pressEditorKey(editor, 'Enter', { ctrlKey: true });

      await vi.waitFor(() => expect(mutationMock).toHaveBeenCalledOnce());
      expect(mutationMock.mock.calls[0][1].input).toMatchObject({
        roomId,
        body: '# Heading\n\nbody'
      });
    });

    it('preserves literal trailing hashes in heading text when sending', async () => {
      const { container, roomId } = renderMessageComposer({ roomId: 'room_456' });
      const editor = await findEditor(container);

      await typeEditorLiteralText(editor, '# test #');
      await vi.waitFor(() => expect(editor.querySelector('h1')?.textContent).toBe('test #'));
      expect(Array.from(editor.children).map((child) => child.tagName)).toEqual(['H1']);

      (q(container, 'button[aria-label="Send message"]') as HTMLButtonElement).click();

      await vi.waitFor(() => expect(mutationMock).toHaveBeenCalledOnce());
      expect(mutationMock.mock.calls[0][1].input).toMatchObject({
        roomId,
        body: '# test &#35;'
      });
    });

    it('preserves multiple literal trailing hashes in heading text when sending', async () => {
      const { container, roomId } = renderMessageComposer({ roomId: 'room_456' });
      const editor = await findEditor(container);

      await typeEditorLiteralText(editor, '# test ##');
      await vi.waitFor(() => expect(editor.querySelector('h1')?.textContent).toBe('test ##'));
      expect(Array.from(editor.children).map((child) => child.tagName)).toEqual(['H1']);

      (q(container, 'button[aria-label="Send message"]') as HTMLButtonElement).click();

      await vi.waitFor(() => expect(mutationMock).toHaveBeenCalledOnce());
      expect(mutationMock.mock.calls[0][1].input).toMatchObject({
        roomId,
        body: '# test &#35;&#35;'
      });
    });

    it('updates the active code block language from the composer controls', async () => {
      const { container, roomId } = renderMessageComposer({ roomId: 'room_456' });
      const editor = await findEditor(container);

      await typeEditorLiteralText(editor, '```ts ');
      await vi.waitFor(() => expect(editor.querySelector('pre code')).toBeTruthy());
      await vi.waitFor(() =>
        expect(editor.querySelector('pre')).toHaveAttribute('data-language', 'ts')
      );
      document.execCommand('insertText', false, 'const answer = 42;');

      const languageSelect = q(
        container,
        'select[aria-label="Code language"]'
      ) as HTMLSelectElement;
      await expect.element(languageSelect).toHaveValue('ts');
      await changeSelectValue(languageSelect, 'js');

      await vi.waitFor(() =>
        expect(editor.querySelector('code')?.classList.contains('language-js')).toBe(true)
      );
      await vi.waitFor(() =>
        expect(editor.querySelector('pre')).toHaveAttribute('data-language', 'js')
      );
      expect(editor.querySelector('pre code span')).toBeTruthy();
      (q(container, 'button[aria-label="Send message"]') as HTMLButtonElement).click();

      await vi.waitFor(() => expect(mutationMock).toHaveBeenCalledOnce());
      expect(mutationMock.mock.calls[0][1].input).toMatchObject({
        roomId,
        body: '```js\nconst answer = 42;\n```'
      });
    });

    it('posts normalized body and all thread/reply options', async () => {
      const onCancelReply = vi.fn();
      const onMessageSent = vi.fn();
      const { container, roomId } = renderMessageComposer({
        roomId: 'room_456',
        inThread: 'evt_thread_root',
        inReplyTo: 'evt_reply_to',
        showAlsoSendToChannel: true,
        onCancelReply,
        onMessageSent
      });
      const editor = await findEditor(container, 'thread-reply-input');

      await typeInEditor(editor, 'hello world');
      const echoToggle = q(
        container,
        'button[aria-label="Also send to channel"]'
      ) as HTMLButtonElement;
      expect(echoToggle.closest('[data-testid="composer-action-toolbar"]')).toBeTruthy();
      expect(echoToggle).toHaveTextContent('Echo');
      expect(echoToggle.querySelector('.iconify')).toHaveClass('icon-[uil--megaphone]');
      expect(echoToggle.querySelector('span:not(.iconify)')).toHaveClass(
        'hidden',
        '@min-[560px]:inline'
      );
      expect(echoToggle).not.toHaveClass('active:scale-[0.96]');
      echoToggle.click();
      const sendButton = q(container, 'button[aria-label="Send message"]') as HTMLButtonElement;
      expect(sendButton).toHaveTextContent('Send');
      expect(sendButton.querySelector('span:not(.iconify)')).toHaveClass(
        'hidden',
        '@min-[560px]:inline'
      );
      sendButton.click();

      await vi.waitFor(() => expect(mutationMock).toHaveBeenCalledOnce());
      expect(mutationMock.mock.calls[0][1].input).toMatchObject({
        roomId,
        body: 'hello world',
        attachments: null,
        threadRootEventId: 'evt_thread_root',
        inReplyTo: 'evt_reply_to',
        alsoSendToChannel: true
      });
      await vi.waitFor(() => expect(onCancelReply).toHaveBeenCalledOnce());
      expect(onMessageSent).toHaveBeenCalledWith(
        expect.objectContaining({
          id: 'msg_123',
          event: expect.objectContaining({ kind: TimelineEventKind.MessagePosted })
        })
      );
      expect(mockInstanceStores.roomUnread.setRoomUnread).toHaveBeenCalledWith(roomId, false);
      expect(roomStateMock.scrollState.requestScrollToBottom).toHaveBeenCalledOnce();
    });

    it('reports the root ID after a room-level post creates a thread', async () => {
      const onMessageSent = vi.fn();
      const onThreadCreated = vi.fn();
      const { container, roomId } = renderMessageComposer({
        roomId: 'room_456',
        showCreateThread: true,
        onMessageSent,
        onThreadCreated
      });
      const editor = await findEditor(container);
      const threadToggle = q(container, 'button[aria-label="Post as thread"]') as HTMLButtonElement;

      await expect.element(threadToggle).toHaveAttribute('aria-pressed', 'false');
      expect(threadToggle.closest('[data-testid="composer-action-toolbar"]')).toBeTruthy();
      expect(threadToggle).toHaveTextContent('Thread');
      expect(threadToggle.querySelector('span:not(.iconify)')).toHaveClass(
        'hidden',
        '@min-[560px]:inline'
      );
      expect(threadToggle).not.toHaveClass('active:scale-[0.96]');
      await typeInEditor(editor, 'discuss this');
      await userEvent.click(threadToggle);
      (q(container, 'button[aria-label="Send message"]') as HTMLButtonElement).click();

      await vi.waitFor(() => expect(mutationMock).toHaveBeenCalledOnce());
      expect(mutationMock.mock.calls[0][1].input).toMatchObject({
        roomId,
        body: 'discuss this',
        createThread: true
      });
      await vi.waitFor(() => expect(onMessageSent).toHaveBeenCalledOnce());
      expect(onMessageSent).toHaveBeenCalledWith(expect.objectContaining({ id: 'msg_123' }));
      expect(onThreadCreated).toHaveBeenCalledWith('msg_123');
      await expect.element(threadToggle).toHaveAttribute('aria-pressed', 'false');
    });

    it('defaults Thread on for Encouraged drafts while preserving an explicit opt-out', async () => {
      const rendered = renderMessageComposer({
        roomId: 'room_456',
        showCreateThread: true,
        createThreadDefault: true,
        threadsEncouraged: true
      });
      const threadToggle = q(
        rendered.container,
        'button[aria-label="Post as thread"]'
      ) as HTMLButtonElement;

      await expect.element(threadToggle).toHaveAttribute('aria-pressed', 'true');
      await userEvent.click(threadToggle);
      await expect.element(threadToggle).toHaveAttribute('aria-pressed', 'false');

      await rendered.rerender({ threadsEncouraged: true });
      await expect.element(threadToggle).toHaveAttribute('aria-pressed', 'false');

      const editor = await findEditor(rendered.container);
      await typeInEditor(editor, 'flat by choice');
      await userEvent.click(
        q(rendered.container, 'button[aria-label="Send message"]') as HTMLButtonElement
      );
      await vi.waitFor(() => expect(mutationMock).toHaveBeenCalledOnce());
      expect(mutationMock.mock.calls[0][1].input).toMatchObject({ createThread: false });
      await expect.element(threadToggle).toHaveAttribute('aria-pressed', 'true');
    });

    it('offers a recent thread before upload and routes the draft into it', async () => {
      const onMessageSent = vi.fn();
      const onThreadMessageSent = vi.fn();
      const getRecentThreadRootCandidate = vi.fn(() => ({
        threadRootEventId: 'previous-root'
      }));
      const { container, getByRole } = renderMessageComposer({
        roomId: 'room_456',
        showCreateThread: true,
        createThreadDefault: true,
        getRecentThreadRootCandidate,
        onMessageSent,
        onThreadMessageSent
      });
      const editor = await findEditor(container);
      await typeInEditor(editor, 'continue this thought');
      await userEvent.click(q(container, 'button[aria-label="Send message"]') as HTMLButtonElement);

      await expect
        .element(getByRole('dialog', { name: 'Continue your previous thread?' }))
        .toBeInTheDocument();
      expect(mutationMock).not.toHaveBeenCalled();
      await userEvent.click(getByRole('button', { name: 'Continue in thread' }));

      await vi.waitFor(() => expect(mutationMock).toHaveBeenCalledOnce());
      expect(mutationMock.mock.calls[0][1].input).toMatchObject({
        body: 'continue this thought',
        threadRootEventId: 'previous-root',
        createThread: false
      });
      expect(onMessageSent).not.toHaveBeenCalled();
      expect(onThreadMessageSent).toHaveBeenCalledWith(
        'previous-root',
        expect.objectContaining({ id: 'msg_123' })
      );
    });

    it('keeps the prepared root unchanged when the user confirms a new message', async () => {
      const onMessageSent = vi.fn();
      const { container, getByRole } = renderMessageComposer({
        roomId: 'room_456',
        showCreateThread: true,
        createThreadDefault: true,
        getRecentThreadRootCandidate: () => ({ threadRootEventId: 'previous-root' }),
        onMessageSent
      });
      const editor = await findEditor(container);
      await typeInEditor(editor, 'a distinct topic');
      await userEvent.click(q(container, 'button[aria-label="Send message"]') as HTMLButtonElement);
      await userEvent.click(getByRole('button', { name: 'Post as new message' }));

      await vi.waitFor(() => expect(mutationMock).toHaveBeenCalledOnce());
      expect(mutationMock.mock.calls[0][1].input).toMatchObject({
        body: 'a distinct topic',
        threadRootEventId: null,
        createThread: true
      });
      expect(onMessageSent).toHaveBeenCalledOnce();
    });

    it('keeps the draft and attachments untouched when the destination choice is cancelled', async () => {
      const { container, getByRole } = renderMessageComposer({
        roomId: 'room_456',
        getRecentThreadRootCandidate: () => ({ threadRootEventId: 'previous-root' })
      });
      const editor = await findEditor(container);
      const file = selectFirstAttachment(q(container, 'input[type="file"]') as HTMLInputElement);

      await expect.poll(() => q(container, 'img')).toBeTruthy();
      await typeInEditor(editor, 'keep this destination undecided');
      await userEvent.click(q(container, 'button[aria-label="Send message"]') as HTMLButtonElement);

      const dialog = getByRole('dialog', { name: 'Continue your previous thread?' });
      await expect.element(dialog).toBeInTheDocument();
      expect(mutationMock).not.toHaveBeenCalled();

      await userEvent.click(getByRole('button', { name: 'Cancel' }));

      await expect.element(dialog).not.toBeInTheDocument();
      await expect.element(editor).toHaveTextContent('keep this destination undecided');
      await expect.poll(() => q(container, 'img')).toBeTruthy();
      expect(mutationMock).not.toHaveBeenCalled();
      expect(file.name).toBe('paste.png');
    });

    it('keeps Required thread creation visible, locked on, and reactive to policy changes', async () => {
      const rendered = renderMessageComposer({
        roomId: 'room_456',
        showCreateThread: true,
        createThreadRequired: false
      });
      const editor = await findEditor(rendered.container);
      const threadToggle = q(
        rendered.container,
        'button[aria-label="Post as thread"]'
      ) as HTMLButtonElement;

      await expect.element(threadToggle).toHaveAttribute('aria-pressed', 'false');
      expect(threadToggle.disabled).toBe(false);

      await rendered.rerender({ createThreadRequired: true });
      await expect.element(threadToggle).toHaveAttribute('aria-pressed', 'true');
      expect(threadToggle.disabled).toBe(true);

      await typeInEditor(editor, 'required thread root');
      (q(rendered.container, 'button[aria-label="Send message"]') as HTMLButtonElement).click();
      await vi.waitFor(() => expect(mutationMock).toHaveBeenCalledOnce());
      expect(mutationMock.mock.calls[0][1].input).toMatchObject({
        roomId: rendered.roomId,
        body: 'required thread root',
        createThread: true
      });

      await rendered.rerender({ createThreadRequired: false });
      await expect.element(threadToggle).toHaveAttribute('aria-pressed', 'false');
      expect(threadToggle.disabled).toBe(false);
    });

    it('clears hidden thread creation state when navigating to another room', async () => {
      const rendered = renderMessageComposer(
        { roomId: 'channel-room', showCreateThread: true },
        { exactRoomId: true }
      );
      const editor = await findEditor(rendered.container);
      const threadToggle = q(
        rendered.container,
        'button[aria-label="Post as thread"]'
      ) as HTMLButtonElement;

      threadToggle.click();
      await rendered.rerender({ roomId: 'dm-room', showCreateThread: false });
      expect(q(rendered.container, 'button[aria-label="Post as thread"]')).toBeNull();

      await typeInEditor(editor, 'hello from the DM');
      (q(rendered.container, 'button[aria-label="Send message"]') as HTMLButtonElement).click();

      await vi.waitFor(() => expect(mutationMock).toHaveBeenCalledOnce());
      expect(mutationMock.mock.calls[0][1].input).toMatchObject({
        roomId: 'dm-room',
        body: 'hello from the DM',
        createThread: false
      });
    });

    it('asks for confirmation before sending a virtual role mention', async () => {
      mutationMock.mockResolvedValueOnce({ data: mutationData, error: null });

      const { container, getByRole, getByText } = renderMessageComposer({ roomId: 'room_456' });
      const editor = await findEditor(container);

      await typeInEditor(editor, '@all hello');
      (q(container, 'button[aria-label="Send message"]') as HTMLButtonElement).click();

      await expect.element(getByRole('dialog', { name: 'Send mention?' })).toBeInTheDocument();
      await expect
        .element(getByText('This message contains a mention that may notify multiple people.'))
        .toBeInTheDocument();
      expect(mutationMock).not.toHaveBeenCalled();

      await userEvent.click(getByRole('button', { name: 'Send Anyway' }));

      await vi.waitFor(() => expect(mutationMock).toHaveBeenCalledOnce());
      expect(mutationMock.mock.calls[0][1].input).toMatchObject({
        body: '@all hello',
        attachments: null
      });
      expect(mutationMock.mock.calls[0][1].input).not.toHaveProperty('mentionConfirmationToken');
    });

    it('asks for confirmation before sending a known role mention', async () => {
      listRolesConnectMock.mockResolvedValueOnce({
        roles: [{ name: 'mods', isSystem: false, position: 10, pingable: false }]
      });

      const { container, getByRole } = renderMessageComposer({ roomId: 'room_456' });
      const editor = await findEditor(container);

      await vi.waitFor(() => expect(listRolesConnectMock).toHaveBeenCalledOnce());
      await tick();
      await typeInEditor(editor, '@mods hello');
      (q(container, 'button[aria-label="Send message"]') as HTMLButtonElement).click();

      await expect.element(getByRole('dialog', { name: 'Send mention?' })).toBeInTheDocument();
      expect(mutationMock).not.toHaveBeenCalled();
    });

    it('waits for the role catalog before deciding whether a role mention needs confirmation', async () => {
      const roleLoad = deferred<{
        roles: { name: string; isSystem: boolean; position: number; pingable: boolean }[];
      }>();
      listRolesConnectMock.mockReturnValueOnce(roleLoad.promise);

      const { container, getByRole } = renderMessageComposer({ roomId: 'room_456' });
      const editor = await findEditor(container);

      await typeInEditor(editor, '@mods hello');
      (q(container, 'button[aria-label="Send message"]') as HTMLButtonElement).click();

      expect(mutationMock).not.toHaveBeenCalled();
      expect(q(document.body, '[role="dialog"]')).toBeNull();

      roleLoad.resolve({
        roles: [{ name: 'mods', isSystem: false, position: 10, pingable: false }]
      });

      await expect.element(getByRole('dialog', { name: 'Send mention?' })).toBeInTheDocument();
      expect(mutationMock).not.toHaveBeenCalled();
    });

    it('asks for confirmation on parsed mentions when the role catalog fails to load', async () => {
      listRolesConnectMock.mockRejectedValueOnce(new Error('roles unavailable'));

      const { container, getByRole } = renderMessageComposer({ roomId: 'room_456' });
      const editor = await findEditor(container);

      await vi.waitFor(() => expect(listRolesConnectMock).toHaveBeenCalledOnce());
      await typeInEditor(editor, '@mods hello');
      (q(container, 'button[aria-label="Send message"]') as HTMLButtonElement).click();

      await expect.element(getByRole('dialog', { name: 'Send mention?' })).toBeInTheDocument();
      expect(mutationMock).not.toHaveBeenCalled();
    });

    it('does not ask for confirmation for the implicit everyone role handle', async () => {
      listRolesConnectMock.mockResolvedValueOnce({
        roles: [{ name: 'everyone', isSystem: true, position: 0, pingable: false }]
      });

      const { container } = renderMessageComposer({ roomId: 'room_456' });
      const editor = await findEditor(container);

      await vi.waitFor(() => expect(listRolesConnectMock).toHaveBeenCalledOnce());
      await tick();
      await typeInEditor(editor, '@everyone hello');
      (q(container, 'button[aria-label="Send message"]') as HTMLButtonElement).click();

      await vi.waitFor(() => expect(mutationMock).toHaveBeenCalledOnce());
      expect(mutationMock.mock.calls[0][1].input.body).toBe('@everyone hello');
    });

    it('leaves text and attachments in place when cancelling a role mention send', async () => {
      const { container, getByRole } = renderMessageComposer({ roomId: 'room_456' });
      const editor = await findEditor(container);
      const file = selectFirstAttachment(q(container, 'input[type="file"]') as HTMLInputElement);

      await expect.poll(() => q(container, 'img')).toBeTruthy();
      await typeInEditor(editor, '@all with attachment');
      (q(container, 'button[aria-label="Send message"]') as HTMLButtonElement).click();

      await expect.element(getByRole('dialog', { name: 'Send mention?' })).toBeInTheDocument();
      expect(mutationMock).not.toHaveBeenCalled();

      await userEvent.click(getByRole('button', { name: 'Cancel' }));

      await expect.element(editor).toHaveTextContent('@all with attachment');
      await expect.poll(() => q(container, 'img')).toBeTruthy();
      expect(mutationMock).not.toHaveBeenCalled();
      expect(file.name).toBe('paste.png');
    });

    it('restores text and attachments after a failed confirmed role mention send', async () => {
      mutationMock.mockResolvedValueOnce({ data: null, error: new Error('still nope') });

      const { container, getByRole } = renderMessageComposer({ roomId: 'room_456' });
      const editor = await findEditor(container);
      const file = selectFirstAttachment(q(container, 'input[type="file"]') as HTMLInputElement);

      await expect.poll(() => q(container, 'img')).toBeTruthy();
      await typeInEditor(editor, '@all will retry');
      (q(container, 'button[aria-label="Send message"]') as HTMLButtonElement).click();

      await expect.element(getByRole('dialog', { name: 'Send mention?' })).toBeInTheDocument();
      await userEvent.click(getByRole('button', { name: 'Send Anyway' }));

      await vi.waitFor(() => expect(mutationMock).toHaveBeenCalledOnce());
      await expect.element(editor).toHaveTextContent('@all will retry');
      await expect.poll(() => q(container, 'img')).toBeTruthy();
      expect(mutationMock.mock.calls[0][1].input.attachments).toEqual([file]);
      expect(getToasts().map((t) => t.message)).toContain('Failed to send message');
    });

    it('restores text and attachments after a failed post', async () => {
      mutationMock.mockResolvedValueOnce({ data: null, error: new Error('nope') });
      const { container } = renderMessageComposer({ roomId: 'room_456' });
      const editor = await findEditor(container);
      const file = selectFirstAttachment(q(container, 'input[type="file"]') as HTMLInputElement);

      await expect.poll(() => q(container, 'img')).toBeTruthy();
      await typeInEditor(editor, 'will retry');
      (q(container, 'button[aria-label="Send message"]') as HTMLButtonElement).click();

      await vi.waitFor(() => expect(mutationMock).toHaveBeenCalledOnce());
      await expect.element(editor).toHaveTextContent('will retry');
      await expect.poll(() => q(container, 'img')).toBeTruthy();
      expect(mutationMock.mock.calls[0][1].input.attachments).toEqual([file]);
      expect(getToasts().map((t) => t.message)).toContain('Failed to send message');
    });
  });

  describe('link preview composer behavior', () => {
    function mockLinkPreview(url: string) {
      queryMock.mockResolvedValueOnce({ data: { server: { roles: [] } }, error: null });
      fetchLinkPreviewConnectMock.mockResolvedValueOnce({
        url,
        previewToken: 'cht_LPpreviewtoken',
        title: 'Preview title',
        description: 'Preview description',
        imageUrl: null,
        siteName: 'Preview site',
        embedType: null,
        embedId: null,
        imageAssetId: 'asset_preview'
      });
    }

    it('fetches a non-message-link preview and sends it with the post mutation', async () => {
      const url = 'https://example.com/story';
      mockLinkPreview(url);
      const { container } = renderMessageComposer({ roomId: 'room_456' });
      const editor = await findEditor(container);

      await typeInEditor(editor, `Look ${url}`);

      await vi.waitFor(() => expect(fetchLinkPreviewConnectMock).toHaveBeenCalledOnce(), {
        timeout: 1000
      });
      await expect.element(q(container, '[data-testid="link-preview-card"]')).toBeInTheDocument();

      (q(container, 'button[aria-label="Send message"]') as HTMLButtonElement).click();

      await vi.waitFor(() => expect(mutationMock).toHaveBeenCalledOnce());
      expect(mutationMock.mock.calls[0][1].input.linkPreviewToken).toBe('cht_LPpreviewtoken');
    });

    it('dismisses a fetched preview so it is not attached to the outgoing message', async () => {
      const url = 'https://example.com/dismiss';
      mockLinkPreview(url);
      const { container } = renderMessageComposer({ roomId: 'room_456' });
      const editor = await findEditor(container);

      await typeInEditor(editor, `Dismiss ${url}`);
      await vi.waitFor(() => expect(fetchLinkPreviewConnectMock).toHaveBeenCalledOnce(), {
        timeout: 1000
      });
      (q(container, 'button[aria-label="Dismiss preview"]') as HTMLButtonElement).click();

      (q(container, 'button[aria-label="Send message"]') as HTMLButtonElement).click();

      await vi.waitFor(() => expect(mutationMock).toHaveBeenCalledOnce());
      expect(mutationMock.mock.calls[0][1].input.linkPreviewToken).toBeNull();
    });
  });

  describe('attachment object URL lifecycle', () => {
    it('revokes object URLs when removing staged files', async () => {
      const { container } = renderMessageComposer({ roomId: 'room_456' });
      selectFirstAttachment(q(container, 'input[type="file"]') as HTMLInputElement);
      await expect.poll(() => q(container, 'img')).toBeTruthy();

      (q(container, 'button[aria-label="Remove paste.png"]') as HTMLButtonElement).click();

      expect(URL.revokeObjectURL).toHaveBeenCalledWith('blob:test');
      await vi.waitFor(() => expect(q(container, 'img')).toBeNull());
    });

    it('revokes object URLs after a successful send', async () => {
      const { container } = renderMessageComposer({ roomId: 'room_456' });
      const editor = await findEditor(container);
      selectFirstAttachment(q(container, 'input[type="file"]') as HTMLInputElement);
      await typeInEditor(editor, 'with file');

      (q(container, 'button[aria-label="Send message"]') as HTMLButtonElement).click();

      await vi.waitFor(() => expect(mutationMock).toHaveBeenCalledOnce());
      await vi.waitFor(() => expect(URL.revokeObjectURL).toHaveBeenCalledWith('blob:test'));
      await vi.waitFor(() => expect(q(container, 'img')).toBeNull());
    });
  });

  describe('accessibility', () => {
    it('attachment button has title attribute', async () => {
      const { container } = renderMessageComposer({ roomId: 'room_456' });

      await expect
        .element(q(container, 'button[title="Attach file"]'))
        .toHaveAttribute('title', 'Attach file');
    });

    it('send button title describes the action', async () => {
      const { container } = renderMessageComposer({ roomId: 'room_456' });
      await findEditor(container);

      expect(q(container, 'button[aria-label="Send message"]')?.getAttribute('title')).toBe(
        'Send message'
      );
    });
  });
});
