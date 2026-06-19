import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { userEvent } from 'vitest/browser';
import { render } from 'vitest-browser-svelte';
import { tick, type ComponentProps } from 'svelte';
import MessageComposer, { type MessageComposerApi } from './MessageComposer.svelte';
import { createMockGraphqlClient, q } from '$lib/test-utils';
import { getToasts, toast } from '$lib/ui/toast';
import type { QuoteInsertionContent, RoomMember } from '$lib/state/room';
import { PresenceStatus } from '$lib/gql/graphql';

function postedMessageEvent(
  id = 'msg_123',
  roomId = 'room_456',
  threadRootEventId: string | null = null
) {
  return {
    __typename: 'Event',
    id,
    createdAt: '2026-06-17T10:47:00Z',
    actorId: 'test-user',
    actor: null,
    event: {
      __typename: 'MessagePostedEvent',
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

const mutationData = { postMessage: postedMessageEvent() };
const updateMutationData = { updateMessage: true };
const prepareFilesMock = vi.hoisted(() => vi.fn());
const mutationMock = vi.hoisted(() => vi.fn());
const queryMock = vi.hoisted(() => vi.fn());
const roomStateMock = vi.hoisted(() => ({
  members: [] as RoomMember[],
  editState: {
    eventId: null as string | null,
    originalBody: '',
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
const mockInstanceStores = {
  currentUser: { user: { id: 'test-user', login: 'testuser' }, loading: false },
  serverInfo: {
    videoProcessingEnabled: false,
    maxUploadSize: 25 * 1024 * 1024,
    maxVideoUploadSize: 25 * 1024 * 1024
  },
  roomUnread: {
    setRoomUnread: vi.fn()
  }
};

vi.mock('$lib/state/server/connection.svelte', () => ({
  useConnection: () => () => ({
    isConnected: true,
    showConnectionLostBanner: false,
    client: {
      query: queryMock,
      mutation: mutationMock,
      subscription: vi.fn()
    }
  })
}));

vi.mock('$lib/attachments/prepareFiles', () => ({
  prepareFiles: prepareFilesMock
}));

vi.mock('$lib/state/server/registry.svelte', () => ({
  serverRegistry: {
    getStore: () => mockInstanceStores,
    getServer: () => ({ id: 'test-instance', url: 'http://localhost' }),
    isOriginServer: () => true,
    originServer: { id: 'test-instance', url: 'http://localhost' },
    servers: [{ id: 'test-instance', url: 'http://localhost' }]
  }
}));

vi.mock('$lib/state/activeServer.svelte', () => ({
  getActiveServer: () => () => 'test-instance'
}));

vi.mock('$lib/state/room', () => ({
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
  context: Map<string, unknown>,
  options: { exactRoomId?: boolean } = {}
) {
  const roomId = options.exactRoomId ? props.roomId : `${props.roomId}-${renderId++}`;
  return {
    ...render(MessageComposer, {
      props: { ...props, roomId },
      context
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
    presenceStatus: PresenceStatus.Offline
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

function pasteText(target: HTMLElement, text: string) {
  const dataTransfer = new DataTransfer();
  dataTransfer.setData('text/plain', text);
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

async function pressEditorKey(
  editor: HTMLElement,
  key: string,
  options: { ctrlKey?: boolean; metaKey?: boolean; shiftKey?: boolean } = {}
) {
  editor.dispatchEvent(
    new KeyboardEvent('keydown', { key, bubbles: true, cancelable: true, ...options })
  );
  await tick();
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

describe('MessageComposer', () => {
  let mockClient: ReturnType<typeof createMockGraphqlClient>;

  beforeEach(() => {
    window.getSelection()?.removeAllRanges();
    mockClient = createMockGraphqlClient({ mutationData });
    mockInstanceStores.serverInfo.videoProcessingEnabled = false;
    mockInstanceStores.serverInfo.maxUploadSize = 25 * 1024 * 1024;
    mockInstanceStores.serverInfo.maxVideoUploadSize = 25 * 1024 * 1024;
    mockInstanceStores.roomUnread.setRoomUnread.mockClear();
    roomStateMock.members = [];
    roomStateMock.editState.eventId = null;
    roomStateMock.editState.originalBody = '';
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
    queryMock.mockReset();
    queryMock.mockResolvedValue({ data: null, error: null });
    sessionStorage.clear();
    vi.clearAllMocks();
  });

  afterEach(() => {
    window.getSelection()?.removeAllRanges();
  });

  describe('form rendering', () => {
    it('renders the TipTap editor', async () => {
      const { container } = renderMessageComposer(
        { roomId: 'room_456' },
        new Map([['$$_urql', mockClient]])
      );

      await expect.element(await findEditor(container)).toBeInTheDocument();
    });

    it('renders the attachment button', async () => {
      const { container } = renderMessageComposer(
        { roomId: 'room_456' },
        new Map([['$$_urql', mockClient]])
      );

      await expect.element(q(container, 'button[title="Attach file"]')).toBeInTheDocument();
    });

    it('hides attachment controls when uploads are not allowed', async () => {
      const { container } = renderMessageComposer(
        { roomId: 'room_456', canAttach: false },
        new Map([['$$_urql', mockClient]])
      );

      await findEditor(container);
      expect(q(container, 'button[title="Attach file"]')).toBeNull();
      expect(q(container, 'input[type="file"]')).toBeNull();
    });

    it('renders hidden file input', async () => {
      const { container } = renderMessageComposer(
        { roomId: 'room_456' },
        new Map([['$$_urql', mockClient]])
      );

      const fileInput = q(container, 'input[type="file"]');
      await expect.element(fileInput).toBeInTheDocument();
      await expect.element(fileInput).toHaveClass('hidden');
    });

    it('editor has correct placeholder', async () => {
      const { container } = renderMessageComposer(
        { roomId: 'room_456' },
        new Map([['$$_urql', mockClient]])
      );

      await findEditor(container);
      // TipTap Placeholder extension sets data-placeholder on the empty paragraph
      await expect
        .element(q(container, 'p.is-editor-empty[data-placeholder="Type a message..."]'))
        .toBeInTheDocument();
    });
  });

  describe('file input configuration', () => {
    it('accepts image and audio files when video processing is disabled', async () => {
      const { container } = renderMessageComposer(
        { roomId: 'room_456' },
        new Map([['$$_urql', mockClient]])
      );

      await expect
        .element(q(container, 'input[type="file"]'))
        .toHaveAttribute('accept', 'image/*,audio/*');
    });

    it('accepts image, video, and audio files when video processing is enabled', async () => {
      mockInstanceStores.serverInfo.videoProcessingEnabled = true;
      const { container } = renderMessageComposer(
        { roomId: 'room_456' },
        new Map([['$$_urql', mockClient]])
      );

      await expect
        .element(q(container, 'input[type="file"]'))
        .toHaveAttribute('accept', 'image/*,video/*,audio/*');
    });

    it('allows multiple file selection', async () => {
      const { container } = renderMessageComposer(
        { roomId: 'room_456' },
        new Map([['$$_urql', mockClient]])
      );

      await expect.element(q(container, 'input[type="file"]')).toHaveAttribute('multiple');
    });

    it('rejects selected video files when video processing is disabled', async () => {
      const { container } = renderMessageComposer(
        { roomId: 'room_456' },
        new Map([['$$_urql', mockClient]])
      );
      const input = q(container, 'input[type="file"]') as HTMLInputElement;

      selectFiles(input, [new File(['video'], 'clip.mp4', { type: 'video/mp4' })]);

      expect(getToasts().map((t) => t.message)).toContain(
        'Video uploads are disabled on this server.'
      );
      expect(q(container, '[data-testid="video-attachment-preview"]')).toBeNull();
    });

    it('stages selected video files when video processing is enabled', async () => {
      mockInstanceStores.serverInfo.videoProcessingEnabled = true;
      const { container } = renderMessageComposer(
        { roomId: 'room_456' },
        new Map([['$$_urql', mockClient]])
      );
      const input = q(container, 'input[type="file"]') as HTMLInputElement;

      selectFiles(input, [new File(['video'], 'clip.mp4', { type: 'video/mp4' })]);

      await expect
        .poll(() => q(container, '[data-testid="video-attachment-preview"]'))
        .toBeTruthy();
    });

    it('rejects selected files over the server upload size limit', async () => {
      mockInstanceStores.serverInfo.maxUploadSize = 1;
      const { container } = renderMessageComposer(
        { roomId: 'room_456' },
        new Map([['$$_urql', mockClient]])
      );
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
    it('editor is editable initially', async () => {
      const { container } = renderMessageComposer(
        { roomId: 'room_456' },
        new Map([['$$_urql', mockClient]])
      );

      await expect.element(await findEditor(container)).toHaveAttribute('contenteditable', 'true');
    });

    it('attachment button is not disabled initially', async () => {
      const { container } = renderMessageComposer(
        { roomId: 'room_456' },
        new Map([['$$_urql', mockClient]])
      );

      await expect.element(q(container, 'button[title="Attach file"]')).not.toBeDisabled();
    });

    it('does not show file preview area initially', async () => {
      const { container } = renderMessageComposer(
        { roomId: 'room_456' },
        new Map([['$$_urql', mockClient]])
      );

      // File preview should only appear when files are selected
      const previewImages = container.querySelectorAll('img');
      expect(previewImages.length).toBe(0);
    });
  });

  describe('send button', () => {
    it('renders the send button', async () => {
      const { container } = renderMessageComposer(
        { roomId: 'room_456' },
        new Map([['$$_urql', mockClient]])
      );

      await expect.element(q(container, 'button[aria-label="Send message"]')).toBeInTheDocument();
    });

    it('send button is disabled when input is empty', async () => {
      const { container } = renderMessageComposer(
        { roomId: 'room_456' },
        new Map([['$$_urql', mockClient]])
      );

      await expect.element(q(container, 'button[aria-label="Send message"]')).toBeDisabled();
    });

    it('send button has paper plane icon', async () => {
      const { container } = renderMessageComposer(
        { roomId: 'room_456' },
        new Map([['$$_urql', mockClient]])
      );

      const sendButton = q(container, 'button[aria-label="Send message"]');
      const icon = sendButton?.querySelector('.uil--telegram-alt');
      expect(icon).not.toBeNull();
    });

    it('hides the keyboard shortcut hint in simple mode', async () => {
      const { container } = renderMessageComposer(
        { roomId: 'room_456' },
        new Map([['$$_urql', mockClient]])
      );
      const editor = await findEditor(container);

      expect(q(container, '[title$="Return to Send"]')).toBeNull();
      await typeEditorLiteralText(editor, 'hint me');

      expect(q(container, '[title$="Return to Send"]')).toBeNull();
    });

    it('shows the enter-again hint after manually activating rich mode', async () => {
      const { container } = renderMessageComposer(
        { roomId: 'room_456' },
        new Map([['$$_urql', mockClient]])
      );
      const editor = await findEditor(container);

      await typeEditorLiteralText(editor, 'hint me');
      await pressEditorKey(editor, 'Enter', { ctrlKey: true });

      await vi.waitFor(() => expect(editor.querySelectorAll(':scope > p')).toHaveLength(2));
      await vi.waitFor(() => {
        const hint = q(container, '[title$="to Send"]');
        expect(hint?.textContent).toMatch(/^(Return|Enter) again to Send$/);
      });
    });

    it('treats an empty block element as sendable composer content', async () => {
      const { container, roomId } = renderMessageComposer(
        { roomId: 'room_456' },
        new Map([['$$_urql', mockClient]])
      );
      const editor = await findEditor(container);

      await typeEditorLiteralText(editor, '- ');
      await vi.waitFor(() => expect(editor.querySelector('ul li')).toBeTruthy());

      await expect.element(q(container, 'button[aria-label="Send message"]')).not.toBeDisabled();

      await vi.waitFor(() => {
        const hint = q(container, '[title$="Return to Send"]');
        expect(hint?.textContent).toMatch(/^(Cmd|Ctrl)\+Return to Send$/);
      });

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
      const { container } = renderMessageComposer(
        { roomId: 'room_456', canAttach: false },
        new Map([['$$_urql', mockClient]])
      );
      const editor = await findEditor(container);

      pasteFile(editor, imageFile());

      expect(prepareFilesMock).not.toHaveBeenCalled();
      expect(q(container, 'img')).toBeNull();
    });

    it('ignores externally added files when uploads are not allowed', async () => {
      const readyApis: MessageComposerApi[] = [];
      const { container } = renderMessageComposer(
        {
          roomId: 'room_456',
          canAttach: false,
          onReady: (api) => {
            readyApis.push(api);
          }
        },
        new Map([['$$_urql', mockClient]])
      );

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
      const { container, roomId } = renderMessageComposer(
        { roomId: 'room_456' },
        new Map([['$$_urql', mockClient]])
      );
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
      const { container, roomId } = renderMessageComposer(
        { roomId: 'room_456' },
        new Map([['$$_urql', mockClient]])
      );
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
        body: null,
        attachments: [file]
      });
    });

    it('clears disabled send state when pasted image preparation fails', async () => {
      const pendingPreparation = deferred<File[]>();
      prepareFilesMock.mockReturnValueOnce(pendingPreparation.promise);
      const { container } = renderMessageComposer(
        { roomId: 'room_456' },
        new Map([['$$_urql', mockClient]])
      );
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
        new Map([['$$_urql', mockClient]]),
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

      const { container } = renderMessageComposer(
        { roomId: 'room_draft' },
        new Map([['$$_urql', mockClient]]),
        { exactRoomId: true }
      );
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
        new Map([['$$_urql', mockClient]]),
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
        new Map([['$$_urql', mockClient]]),
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
        new Map([['$$_urql', mockClient]]),
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
        new Map([['$$_urql', mockClient]]),
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
        new Map([['$$_urql', mockClient]]),
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
        new Map([['$$_urql', mockClient]]),
        { exactRoomId: true }
      );
      const editor = await findEditor(container);

      await expect.element(editor).toHaveTextContent('before <b>x</b>');
      expect(editor.querySelector('strong')).toBeNull();
      editor.focus();
      document.execCommand('insertText', false, '!');

      await vi.waitFor(() => {
        const draft = sessionStorage.getItem('chatto:draft:room_indented_continuation_draft') ?? '';
        expect(draft).toContain('    <b>x</b>!');
        expect(draft).not.toContain('**x**');
      });
    });

    it('preserves literal HTML-looking text after unmatched backticks', async () => {
      const body = '` <b>literal</b>';
      const editedBody = `${body}!`;
      sessionStorage.setItem('chatto:draft:room_unmatched_backtick_draft', body);

      const { container } = renderMessageComposer(
        { roomId: 'room_unmatched_backtick_draft' },
        new Map([['$$_urql', mockClient]]),
        { exactRoomId: true }
      );
      const editor = await findEditor(container);

      await expect.element(editor).toHaveTextContent(body);
      editor.focus();
      document.execCommand('insertText', false, '!');

      await vi.waitFor(() =>
        expect(sessionStorage.getItem('chatto:draft:room_unmatched_backtick_draft')).toBe(
          editedBody
        )
      );
    });

    it('does not escape code after a non-closing fence marker line', async () => {
      const body = '```md\n``` not a closing fence\n<b>code</b>\n```';
      sessionStorage.setItem('chatto:draft:room_non_closing_fence_draft', body);

      const { container } = renderMessageComposer(
        { roomId: 'room_non_closing_fence_draft' },
        new Map([['$$_urql', mockClient]]),
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
        new Map([['$$_urql', mockClient]]),
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
        new Map([['$$_urql', mockClient]]),
        { exactRoomId: true }
      );
      const editor = await findEditor(container);

      await vi.waitFor(() => expect(editor.querySelector('code')?.textContent).toContain('<b>'));
      expect(editor.querySelector('code')?.textContent).toContain('</b>');
    });

    it('does not treat unmatched closing link syntax as a markdown link destination', async () => {
      const body = 'not a link](<b>x</b>)';
      const editedBody = `${body}!`;
      sessionStorage.setItem('chatto:draft:room_fake_link_draft', body);

      const { container } = renderMessageComposer(
        { roomId: 'room_fake_link_draft' },
        new Map([['$$_urql', mockClient]]),
        { exactRoomId: true }
      );
      const editor = await findEditor(container);

      await expect.element(editor).toHaveTextContent(body);
      expect(editor.querySelector('strong')).toBeNull();
      editor.focus();
      document.execCommand('insertText', false, '!');

      await vi.waitFor(() =>
        expect(sessionStorage.getItem('chatto:draft:room_fake_link_draft')).toBe(editedBody)
      );
    });

    it('restores markdown autolinks with ampersands intact', async () => {
      const body = '<https://example.com/?a=1&b=2>';
      sessionStorage.setItem('chatto:draft:room_autolink_text_draft', body);

      const { container } = renderMessageComposer(
        { roomId: 'room_autolink_text_draft' },
        new Map([['$$_urql', mockClient]]),
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
        new Map([['$$_urql', mockClient]]),
        { exactRoomId: true }
      );

      await expect
        .element(await findEditor(container, 'thread-reply-input'))
        .toHaveTextContent('thread draft');
      expect(sessionStorage.getItem('chatto:draft:room_draft')).toBe('room draft');
    });

    it('clears the active text draft after a successful send', async () => {
      const { container, roomId } = renderMessageComposer(
        { roomId: 'room_456' },
        new Map([['$$_urql', mockClient]])
      );
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
    it('does not start editing on ArrowUp when no editable message is available', async () => {
      const { container } = renderMessageComposer(
        { roomId: 'room_456' },
        new Map([['$$_urql', mockClient]])
      );
      const editor = await findEditor(container);

      await pressEditorKey(editor, 'ArrowUp');

      expect(roomStateMock.lastEditableMessage.getLastEditableMessage).toHaveBeenCalledOnce();
      expect(roomStateMock.editState.startEdit).not.toHaveBeenCalled();
      await expect.element(editor).toHaveTextContent('');
    });

    it('prefills edit text, hides attachment controls, and cancels on Escape', async () => {
      roomStateMock.editState.eventId = 'evt_edit';
      roomStateMock.editState.originalBody = 'original body';
      const { container } = renderMessageComposer(
        { roomId: 'room_456' },
        new Map([['$$_urql', mockClient]])
      );
      const editor = await findEditor(container);

      await expect.element(editor).toHaveTextContent('original body');
      expect(q(container, 'button[title="Attach file"]')).toBeNull();

      await pressEditorKey(editor, 'Escape');

      expect(roomStateMock.editState.cancelEdit).toHaveBeenCalledOnce();
      expect(mutationMock).not.toHaveBeenCalled();
    });

    it('closes mention autocomplete when cancelling an edit', async () => {
      roomStateMock.members = [roomMember('golden_fox07')];
      roomStateMock.editState.eventId = 'evt_edit';
      roomStateMock.editState.originalBody = 'original body';
      const { container } = renderMessageComposer(
        { roomId: 'room_456' },
        new Map([['$$_urql', mockClient]])
      );
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
      const { container } = renderMessageComposer(
        { roomId: 'room_456' },
        new Map([['$$_urql', mockClient]])
      );
      const editor = await findEditor(container);

      await typeEditorLiteralText(editor, '@gold');
      await vi.waitFor(() =>
        expect(container.querySelector('[data-testid="mention-autocomplete"]')).toBeTruthy()
      );
      (q(container, 'button[aria-label="Send message"]') as HTMLButtonElement).click();

      await vi.waitFor(() => expect(mutationMock).toHaveBeenCalledOnce());
      expect(mutationMock.mock.calls[0][1].input).toMatchObject({
        eventId: 'evt_edit',
        body: '@gold'
      });
      await vi.waitFor(() =>
        expect(container.querySelector('[data-testid="mention-autocomplete"]')).toBeNull()
      );
    });

    it('keeps edit mode on rich keyboard behavior', async () => {
      roomStateMock.editState.eventId = 'evt_edit';
      roomStateMock.editState.originalBody = 'original body';
      const { container } = renderMessageComposer(
        { roomId: 'room_456' },
        new Map([['$$_urql', mockClient]])
      );
      const editor = await findEditor(container);

      await vi.waitFor(() => expect(container.textContent).toMatch(/(?:Cmd|Ctrl)\+Return to Send/));
      await pressEditorKey(editor, 'Enter');
      expect(mutationMock).not.toHaveBeenCalled();

      await pressEditorKey(editor, 'Enter', { ctrlKey: true });

      await vi.waitFor(() => expect(mutationMock).toHaveBeenCalledOnce());
      expect(mutationMock.mock.calls[0][1].input).toMatchObject({
        eventId: 'evt_edit',
        body: 'original body'
      });
    });

    it('preserves literal HTML-looking text when restoring and saving an edit', async () => {
      const body = '<script>alert(1)</script> & <b>bold?</b>';
      const editedBody = `${body}!`;
      roomStateMock.editState.eventId = 'evt_edit';
      roomStateMock.editState.originalBody = body;
      const { container } = renderMessageComposer(
        { roomId: 'room_456' },
        new Map([['$$_urql', mockClient]])
      );
      const editor = await findEditor(container);

      await expect.element(editor).toHaveTextContent(body);
      await placeCaretAtEditorEnd(editor);
      document.execCommand('insertText', false, '!');
      await vi.waitFor(() => expect(editor.textContent).toBe(editedBody));

      (q(container, 'button[aria-label="Send message"]') as HTMLButtonElement).click();

      await vi.waitFor(() => expect(mutationMock).toHaveBeenCalledOnce());
      expect(mutationMock.mock.calls[0][1].input).toMatchObject({
        eventId: 'evt_edit',
        body: editedBody
      });
    });

    it('clears staged attachments when edit mode is active at mount', async () => {
      const roomId = 'room_edit_attachments';
      const firstRender = renderMessageComposer({ roomId }, new Map([['$$_urql', mockClient]]), {
        exactRoomId: true
      });
      const file = selectFirstAttachment(
        q(firstRender.container, 'input[type="file"]') as HTMLInputElement
      );
      await expect.poll(() => q(firstRender.container, 'img')).toBeTruthy();
      firstRender.unmount();

      // Stash an attachment draft for the same room, then mount directly into edit mode.
      // The composer should discard attachments because editMessage only supports text.
      roomStateMock.editState.eventId = 'evt_edit';
      roomStateMock.editState.originalBody = 'editable';
      const { container } = renderMessageComposer({ roomId }, new Map([['$$_urql', mockClient]]), {
        exactRoomId: true
      });
      expect(q(container, 'button[title="Attach file"]')).toBeNull();
      expect(file.name).toBe('paste.png');
      expect(q(container, 'img')).toBeNull();
    });

    it('converts a new typed code fence after an edited terminal code block', async () => {
      roomStateMock.editState.eventId = 'evt_edit';
      roomStateMock.editState.originalBody = '```ts\nconst existing = true;\n```';
      const { container } = renderMessageComposer(
        { roomId: 'room_456' },
        new Map([['$$_urql', mockClient]])
      );
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

      await vi.waitFor(() => expect(mutationMock).toHaveBeenCalledOnce());
      expect(mutationMock.mock.calls[0][1].input).toMatchObject({
        eventId: 'evt_edit',
        body: '```ts\nconst existing = true;\n```\n\n```js\nconsole.log("second");\n```'
      });
    });
  });

  describe('submit behavior', () => {
    it('uses Enter to complete an active mention before plain Enter can send', async () => {
      roomStateMock.members = [roomMember('alice')];
      const { container, roomId } = renderMessageComposer(
        { roomId: 'room_456' },
        new Map([['$$_urql', mockClient]])
      );
      const editor = await findEditor(container);

      await typeEditorLiteralText(editor, '@ali');
      await vi.waitFor(() =>
        expect(container.querySelector('[data-testid="mention-autocomplete"]')).toBeTruthy()
      );

      await pressEditorKey(editor, 'Enter');

      await vi.waitFor(() => expect(editor.textContent).toBe('@alice '));
      expect(mutationMock).not.toHaveBeenCalled();

      await pressEditorKey(editor, 'Enter');

      await vi.waitFor(() => expect(mutationMock).toHaveBeenCalledOnce());
      expect(mutationMock.mock.calls[0][1].input).toMatchObject({
        roomId,
        body: '@alice'
      });
    });

    it('sends plain text with Enter in simple mode', async () => {
      const { container, roomId } = renderMessageComposer(
        { roomId: 'room_456' },
        new Map([['$$_urql', mockClient]])
      );
      const editor = await findEditor(container);

      await typeEditorLiteralText(editor, 'hello from shortcut');
      await pressEditorKey(editor, 'Enter');

      await vi.waitFor(() => expect(mutationMock).toHaveBeenCalledOnce());
      expect(mutationMock.mock.calls[0][1].input).toMatchObject({
        roomId,
        body: 'hello from shortcut'
      });
    });

    it('inserts selected reply quotes without replacing draft text', async () => {
      let composerApi: MessageComposerApi | null = null;
      const { container } = renderMessageComposer(
        {
          roomId: 'room_456',
          onReady: (api) => {
            composerApi = api;
          }
        },
        new Map([['$$_urql', mockClient]])
      );
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
      const { container, roomId } = renderMessageComposer(
        {
          roomId: 'room_456',
          onReady: (api) => {
            composerApi = api;
          }
        },
        new Map([['$$_urql', mockClient]])
      );
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
      const { container } = renderMessageComposer(
        {
          roomId: 'room_456',
          onReady: (api) => {
            composerApi = api;
          }
        },
        new Map([['$$_urql', mockClient]])
      );
      const editor = await findEditor(container);

      await vi.waitFor(() => expect(composerApi).not.toBeNull());
      composerApi!.insertQuote('first line\nsecond line');

      await vi.waitFor(() => expect(editor.querySelector('blockquote')).toBeTruthy());
      expect(editor.querySelectorAll('blockquote p')).toHaveLength(2);
      expect(editor.querySelector('blockquote')?.textContent).toBe('first linesecond line');
    });

    it('renders structured selected reply quotes as nested blockquotes', async () => {
      let composerApi: MessageComposerApi | null = null;
      const { container } = renderMessageComposer(
        {
          roomId: 'room_456',
          onReady: (api) => {
            composerApi = api;
          }
        },
        new Map([['$$_urql', mockClient]])
      );
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
      const { container, roomId } = renderMessageComposer(
        {
          roomId: 'room_456',
          onReady: (api) => {
            composerApi = api;
          }
        },
        new Map([['$$_urql', mockClient]])
      );
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

    it('activates rich mode with Ctrl+Enter in simple mode', async () => {
      const { container, roomId } = renderMessageComposer(
        { roomId: 'room_456' },
        new Map([['$$_urql', mockClient]])
      );
      const editor = await findEditor(container);

      await typeEditorLiteralText(editor, 'hello from shortcut');
      await pressEditorKey(editor, 'Enter', { ctrlKey: true });
      expect(mutationMock).not.toHaveBeenCalled();
      await vi.waitFor(() => expect(editor.querySelectorAll(':scope > p')).toHaveLength(2));
      await vi.waitFor(() =>
        expect(container.textContent).toMatch(/(?:Return|Enter) again to Send/)
      );

      await pressEditorKey(editor, 'Enter', { ctrlKey: true });

      await vi.waitFor(() => expect(mutationMock).toHaveBeenCalledOnce());
      expect(mutationMock.mock.calls[0][1].input).toMatchObject({
        roomId,
        body: 'hello from shortcut'
      });
    });

    it('posts markdown after TipTap formatting shortcuts are applied', async () => {
      const { container, roomId } = renderMessageComposer(
        { roomId: 'room_456' },
        new Map([['$$_urql', mockClient]])
      );
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

    it('posts markdown after typed markdown link syntax is applied', async () => {
      const { container, roomId } = renderMessageComposer(
        { roomId: 'room_456' },
        new Map([['$$_urql', mockClient]])
      );
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

    it('keeps typed space after a pasted autolink outside the link', async () => {
      const url = 'https://www.spiegel.de/';
      const { container } = renderMessageComposer(
        { roomId: 'room_456' },
        new Map([['$$_urql', mockClient]])
      );
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
      const { container, roomId } = renderMessageComposer(
        { roomId: 'room_456' },
        new Map([['$$_urql', mockClient]])
      );
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
      const { container, roomId } = renderMessageComposer(
        { roomId: 'room_456' },
        new Map([['$$_urql', mockClient]])
      );
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
      const { container, roomId } = renderMessageComposer(
        { roomId: 'room_456' },
        new Map([['$$_urql', mockClient]])
      );
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
      const { container, roomId } = renderMessageComposer(
        { roomId: 'room_456' },
        new Map([['$$_urql', mockClient]])
      );
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
      const { container, roomId } = renderMessageComposer(
        { roomId: 'room_456' },
        new Map([['$$_urql', mockClient]])
      );
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
      const { container, roomId } = renderMessageComposer(
        { roomId: 'room_456' },
        new Map([['$$_urql', mockClient]])
      );
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
      const { container, roomId } = renderMessageComposer(
        { roomId: 'room_456' },
        new Map([['$$_urql', mockClient]])
      );
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
      const { container, roomId } = renderMessageComposer(
        { roomId: 'room_456' },
        new Map([['$$_urql', mockClient]])
      );
      const editor = await findEditor(container);

      await typeEditorLiteralText(editor, 'or this:');
      await pressEditorKey(editor, 'Enter', { ctrlKey: true });
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
      const { container } = renderMessageComposer(
        { roomId: 'room_456' },
        new Map([['$$_urql', mockClient]])
      );
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

      await vi.waitFor(() => expect(mutationMock).toHaveBeenCalledOnce());
      expect(mutationMock.mock.calls[0][1].input).toMatchObject({
        eventId: 'evt_edit',
        body: '```text\nmoo\nquack\n```\n\nor this:\n\n```go\nIO.puts("moo")\n```'
      });
    });

    it('keeps Enter inside an active code block', async () => {
      const { container, roomId } = renderMessageComposer(
        { roomId: 'room_456' },
        new Map([['$$_urql', mockClient]])
      );
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

    it('lets Shift+Enter insert a hard break without submitting', async () => {
      const { container, roomId } = renderMessageComposer(
        { roomId: 'room_456' },
        new Map([['$$_urql', mockClient]])
      );
      const editor = await findEditor(container);

      await typeEditorLiteralText(editor, 'first');
      await pressEditorKey(editor, 'Enter', { shiftKey: true });
      expect(mutationMock).not.toHaveBeenCalled();
      document.execCommand('insertText', false, 'second');
      await vi.waitFor(() => expect(editor.textContent).toContain('firstsecond'));
      expect(editor.querySelector('br')).toBeTruthy();

      (q(container, 'button[aria-label="Send message"]') as HTMLButtonElement).click();

      await vi.waitFor(() => expect(mutationMock).toHaveBeenCalledOnce());
      expect(mutationMock.mock.calls[0][1].input).toMatchObject({
        roomId,
        body: 'first  \nsecond'
      });
    });

    it('sends with Cmd+Enter inside an active code block', async () => {
      const { container, roomId } = renderMessageComposer(
        { roomId: 'room_456' },
        new Map([['$$_urql', mockClient]])
      );
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

    it('lets Enter create another bullet list item instead of submitting', async () => {
      const { container, roomId } = renderMessageComposer(
        { roomId: 'room_456' },
        new Map([['$$_urql', mockClient]])
      );
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

    it('sends with Enter from the visible trailing paragraph after leaving a bullet list', async () => {
      const { container, roomId } = renderMessageComposer(
        { roomId: 'room_456' },
        new Map([['$$_urql', mockClient]])
      );
      const editor = await findEditor(container);

      await typeEditorLiteralText(editor, '- first');
      await vi.waitFor(() => expect(editor.querySelector('ul li')?.textContent).toBe('first'));
      await pressEditorKey(editor, 'Enter');
      await vi.waitFor(() => expect(editor.querySelectorAll('ul li')).toHaveLength(2));
      await pressEditorKey(editor, 'Enter');
      expect(mutationMock).not.toHaveBeenCalled();
      await vi.waitFor(() => expect(editor.querySelectorAll('ul li')).toHaveLength(1));
      await vi.waitFor(() =>
        expect(container.textContent).toMatch(/(?:Return|Enter) again to Send/)
      );

      await pressEditorKey(editor, 'Enter');

      await vi.waitFor(() => expect(mutationMock).toHaveBeenCalledOnce());
      expect(mutationMock.mock.calls[0][1].input).toMatchObject({
        roomId,
        body: '- first'
      });
    });

    it('sends with Cmd+Enter inside a bullet list', async () => {
      const { container, roomId } = renderMessageComposer(
        { roomId: 'room_456' },
        new Map([['$$_urql', mockClient]])
      );
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

    it('starts a bullet list from a visual line after hard breaks', async () => {
      const { container, roomId } = renderMessageComposer(
        { roomId: 'room_456' },
        new Map([['$$_urql', mockClient]])
      );
      const editor = await findEditor(container);

      await typeEditorLiteralText(editor, 'Things I hate:');
      await pressEditorKey(editor, 'Enter', { shiftKey: true });
      await pressEditorKey(editor, 'Enter', { shiftKey: true });
      await insertEditorLiteralText(editor, '- ');
      await vi.waitFor(() => expect(editor.querySelector('ul li')).toBeTruthy());
      expect(editor.querySelector('p br')).toBeTruthy();

      document.execCommand('insertText', false, 'lists');
      await vi.waitFor(() => expect(editor.querySelector('ul li')?.textContent).toBe('lists'));

      (q(container, 'button[aria-label="Send message"]') as HTMLButtonElement).click();

      await vi.waitFor(() => expect(mutationMock).toHaveBeenCalledOnce());
      expect(mutationMock.mock.calls[0][1].input).toMatchObject({
        roomId,
        body: 'Things I hate:\n\n- lists'
      });
    });

    it('starts an ordered list from a visual line after hard breaks', async () => {
      const { container, roomId } = renderMessageComposer(
        { roomId: 'room_456' },
        new Map([['$$_urql', mockClient]])
      );
      const editor = await findEditor(container);

      await typeEditorLiteralText(editor, 'Things I like:');
      await pressEditorKey(editor, 'Enter', { shiftKey: true });
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

    it('lets Enter leave a heading without submitting', async () => {
      const { container, roomId } = renderMessageComposer(
        { roomId: 'room_456' },
        new Map([['$$_urql', mockClient]])
      );
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
      await vi.waitFor(() =>
        expect(container.textContent).toMatch(/(?:Return|Enter) again to Send/)
      );

      await pressEditorKey(editor, 'Enter');

      await vi.waitFor(() => expect(mutationMock).toHaveBeenCalledOnce());
      expect(mutationMock.mock.calls[0][1].input).toMatchObject({
        roomId,
        body: '# Heading\n\nbody'
      });
    });

    it('preserves literal trailing hashes in heading text when sending', async () => {
      const { container, roomId } = renderMessageComposer(
        { roomId: 'room_456' },
        new Map([['$$_urql', mockClient]])
      );
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
      const { container, roomId } = renderMessageComposer(
        { roomId: 'room_456' },
        new Map([['$$_urql', mockClient]])
      );
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
      const { container, roomId } = renderMessageComposer(
        { roomId: 'room_456' },
        new Map([['$$_urql', mockClient]])
      );
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
      const { container, roomId } = renderMessageComposer(
        {
          roomId: 'room_456',
          inThread: 'evt_thread_root',
          inReplyTo: 'evt_reply_to',
          showAlsoSendToChannel: true,
          onCancelReply,
          onMessageSent
        },
        new Map([['$$_urql', mockClient]])
      );
      const editor = await findEditor(container, 'thread-reply-input');

      await typeInEditor(editor, 'hello world');
      (q(container, 'input[type="checkbox"]') as HTMLInputElement).click();
      (q(container, 'button[aria-label="Send message"]') as HTMLButtonElement).click();

      await vi.waitFor(() => expect(mutationMock).toHaveBeenCalledOnce());
      expect(mutationMock.mock.calls[0][1].input).toMatchObject({
        roomId,
        body: 'hello world',
        attachments: null,
        threadRootEventId: 'evt_thread_root',
        inReplyTo: 'evt_reply_to',
        alsoSendToChannel: true
      });
      expect(onCancelReply).toHaveBeenCalledOnce();
      expect(onMessageSent).toHaveBeenCalledWith(
        expect.objectContaining({
          id: 'msg_123',
          event: expect.objectContaining({ __typename: 'MessagePostedEvent' })
        })
      );
      expect(mockInstanceStores.roomUnread.setRoomUnread).toHaveBeenCalledWith(roomId, false);
      expect(roomStateMock.scrollState.requestScrollToBottom).toHaveBeenCalledOnce();
    });

    it('retries large mention sends with the confirmation token', async () => {
      mutationMock
        .mockResolvedValueOnce({
          data: null,
          error: {
            graphQLErrors: [
              {
                extensions: {
                  code: 'MENTION_CONFIRMATION_REQUIRED',
                  recipientCount: 12,
                  mentionConfirmationToken: 'jwt.confirmation.token'
                }
              }
            ]
          }
        })
        .mockResolvedValueOnce({ data: mutationData, error: null });

      const { container, getByRole, getByText } = renderMessageComposer(
        { roomId: 'room_456' },
        new Map([['$$_urql', mockClient]])
      );
      const editor = await findEditor(container);

      await typeInEditor(editor, '@all hello');
      (q(container, 'button[aria-label="Send message"]') as HTMLButtonElement).click();

      await expect.element(getByRole('dialog', { name: 'Notify 12 people?' })).toBeInTheDocument();
      await expect
        .element(getByText('This message will notify 12 people. Send it anyway?'))
        .toBeInTheDocument();
      expect(mutationMock).toHaveBeenCalledOnce();

      await userEvent.click(getByRole('button', { name: 'Send Anyway' }));

      await vi.waitFor(() => expect(mutationMock).toHaveBeenCalledTimes(2));
      expect(mutationMock.mock.calls[0][1].input.mentionConfirmationToken).toBeNull();
      expect(mutationMock.mock.calls[1][1].input.mentionConfirmationToken).toBe(
        'jwt.confirmation.token'
      );
    });

    it('restores text and attachments when cancelling a large mention send', async () => {
      mutationMock.mockResolvedValueOnce({
        data: null,
        error: {
          graphQLErrors: [
            {
              extensions: {
                code: 'MENTION_CONFIRMATION_REQUIRED',
                recipientCount: 12,
                mentionConfirmationToken: 'jwt.confirmation.token'
              }
            }
          ]
        }
      });

      const { container, getByRole } = renderMessageComposer(
        { roomId: 'room_456' },
        new Map([['$$_urql', mockClient]])
      );
      const editor = await findEditor(container);
      const file = selectFirstAttachment(q(container, 'input[type="file"]') as HTMLInputElement);

      await expect.poll(() => q(container, 'img')).toBeTruthy();
      await typeInEditor(editor, '@all with attachment');
      (q(container, 'button[aria-label="Send message"]') as HTMLButtonElement).click();

      await expect.element(getByRole('dialog', { name: 'Notify 12 people?' })).toBeInTheDocument();
      expect(mutationMock).toHaveBeenCalledOnce();

      await userEvent.click(getByRole('button', { name: 'Cancel' }));

      await expect.element(editor).toHaveTextContent('@all with attachment');
      await expect.poll(() => q(container, 'img')).toBeTruthy();
      expect(mutationMock).toHaveBeenCalledOnce();
      expect(mutationMock.mock.calls[0][1].input.attachments).toEqual([file]);
    });

    it('restores text and attachments after a failed large mention confirmation retry', async () => {
      mutationMock
        .mockResolvedValueOnce({
          data: null,
          error: {
            graphQLErrors: [
              {
                extensions: {
                  code: 'MENTION_CONFIRMATION_REQUIRED',
                  recipientCount: 12,
                  mentionConfirmationToken: 'jwt.confirmation.token'
                }
              }
            ]
          }
        })
        .mockResolvedValueOnce({ data: null, error: new Error('still nope') });

      const { container, getByRole } = renderMessageComposer(
        { roomId: 'room_456' },
        new Map([['$$_urql', mockClient]])
      );
      const editor = await findEditor(container);
      const file = selectFirstAttachment(q(container, 'input[type="file"]') as HTMLInputElement);

      await expect.poll(() => q(container, 'img')).toBeTruthy();
      await typeInEditor(editor, '@all will retry');
      (q(container, 'button[aria-label="Send message"]') as HTMLButtonElement).click();

      await expect.element(getByRole('dialog', { name: 'Notify 12 people?' })).toBeInTheDocument();
      await userEvent.click(getByRole('button', { name: 'Send Anyway' }));

      await vi.waitFor(() => expect(mutationMock).toHaveBeenCalledTimes(2));
      await expect.element(editor).toHaveTextContent('@all will retry');
      await expect.poll(() => q(container, 'img')).toBeTruthy();
      expect(mutationMock.mock.calls[1][1].input.mentionConfirmationToken).toBe(
        'jwt.confirmation.token'
      );
      expect(mutationMock.mock.calls[1][1].input.attachments).toEqual([file]);
      expect(getToasts().map((t) => t.message)).toContain('Failed to send message');
    });

    it('restores text and attachments after a failed post', async () => {
      mutationMock.mockResolvedValueOnce({ data: null, error: new Error('nope') });
      const { container } = renderMessageComposer(
        { roomId: 'room_456' },
        new Map([['$$_urql', mockClient]])
      );
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
      queryMock
        .mockResolvedValueOnce({ data: { server: { roles: [] } }, error: null })
        .mockResolvedValueOnce({
          data: {
            linkPreview: {
              url,
              title: 'Preview title',
              description: 'Preview description',
              imageUrl: null,
              siteName: 'Preview site',
              embedType: null,
              embedId: null,
              imageAssetId: 'asset_preview'
            }
          },
          error: null
        });
    }

    it('fetches a non-message-link preview and sends it with the post mutation', async () => {
      const url = 'https://example.com/story';
      mockLinkPreview(url);
      const { container } = renderMessageComposer(
        { roomId: 'room_456' },
        new Map([['$$_urql', mockClient]])
      );
      const editor = await findEditor(container);

      await typeInEditor(editor, `Look ${url}`);

      await vi.waitFor(() => expect(queryMock).toHaveBeenCalledTimes(2), { timeout: 1000 });
      await expect.element(q(container, '[data-testid="link-preview-card"]')).toBeInTheDocument();

      (q(container, 'button[aria-label="Send message"]') as HTMLButtonElement).click();

      await vi.waitFor(() => expect(mutationMock).toHaveBeenCalledOnce());
      expect(mutationMock.mock.calls[0][1].input.linkPreview).toMatchObject({
        url,
        title: 'Preview title',
        description: 'Preview description',
        siteName: 'Preview site',
        imageAssetId: 'asset_preview'
      });
    });

    it('dismisses a fetched preview so it is not attached to the outgoing message', async () => {
      const url = 'https://example.com/dismiss';
      mockLinkPreview(url);
      const { container } = renderMessageComposer(
        { roomId: 'room_456' },
        new Map([['$$_urql', mockClient]])
      );
      const editor = await findEditor(container);

      await typeInEditor(editor, `Dismiss ${url}`);
      await vi.waitFor(() => expect(queryMock).toHaveBeenCalledTimes(2), { timeout: 1000 });
      (q(container, 'button[aria-label="Dismiss preview"]') as HTMLButtonElement).click();

      (q(container, 'button[aria-label="Send message"]') as HTMLButtonElement).click();

      await vi.waitFor(() => expect(mutationMock).toHaveBeenCalledOnce());
      expect(mutationMock.mock.calls[0][1].input.linkPreview).toBeNull();
    });
  });

  describe('attachment object URL lifecycle', () => {
    it('revokes object URLs when removing staged files', async () => {
      const { container } = renderMessageComposer(
        { roomId: 'room_456' },
        new Map([['$$_urql', mockClient]])
      );
      selectFirstAttachment(q(container, 'input[type="file"]') as HTMLInputElement);
      await expect.poll(() => q(container, 'img')).toBeTruthy();

      (q(container, 'button.absolute') as HTMLButtonElement).click();

      expect(URL.revokeObjectURL).toHaveBeenCalledWith('blob:test');
      await vi.waitFor(() => expect(q(container, 'img')).toBeNull());
    });

    it('revokes object URLs after a successful send', async () => {
      const { container } = renderMessageComposer(
        { roomId: 'room_456' },
        new Map([['$$_urql', mockClient]])
      );
      const editor = await findEditor(container);
      selectFirstAttachment(q(container, 'input[type="file"]') as HTMLInputElement);
      await typeInEditor(editor, 'with file');

      (q(container, 'button[aria-label="Send message"]') as HTMLButtonElement).click();

      await vi.waitFor(() => expect(mutationMock).toHaveBeenCalledOnce());
      expect(URL.revokeObjectURL).toHaveBeenCalledWith('blob:test');
      expect(q(container, 'img')).toBeNull();
    });
  });

  describe('accessibility', () => {
    it('attachment button has title attribute', async () => {
      const { container } = renderMessageComposer(
        { roomId: 'room_456' },
        new Map([['$$_urql', mockClient]])
      );

      await expect
        .element(q(container, 'button[title="Attach file"]'))
        .toHaveAttribute('title', 'Attach file');
    });

    it('send button title follows composer mode', async () => {
      const { container } = renderMessageComposer(
        { roomId: 'room_456' },
        new Map([['$$_urql', mockClient]])
      );
      const editor = await findEditor(container);

      await expect
        .element(q(container, 'button[aria-label="Send message"]'))
        .toHaveAttribute('title', 'Send message (Enter)');

      await typeEditorLiteralText(editor, '- first');
      await vi.waitFor(() =>
        expect(q(container, 'button[aria-label="Send message"]')).toHaveAttribute(
          'title',
          'Send message (Ctrl/Cmd+Enter)'
        )
      );
    });
  });
});
