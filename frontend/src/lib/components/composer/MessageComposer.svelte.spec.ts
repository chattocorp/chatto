import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { tick, type ComponentProps } from 'svelte';
import MessageComposer from './MessageComposer.svelte';
import { createMockGraphqlClient, q } from '$lib/test-utils';
import { getToasts, toast } from '$lib/ui/toast';
import type { RoomMember } from '$lib/state/room';

const mutationData = { postMessage: { id: 'msg_123' } };
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
  getComposerContext: () => ({
    editState: roomStateMock.editState,
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

async function typeInEditor(editor: HTMLElement, text: string) {
  editor.focus();
  document.execCommand('selectAll');
  document.execCommand('insertText', false, text);
  await vi.waitFor(() => expect(editor.textContent).toBe(text));
}

async function pressEditorKey(editor: HTMLElement, key: string) {
  editor.dispatchEvent(new KeyboardEvent('keydown', { key, bubbles: true, cancelable: true }));
  await tick();
}

function selectFirstAttachment(input: HTMLInputElement, file = imageFile()) {
  selectFiles(input, [file]);
  return file;
}

describe('MessageComposer', () => {
  let mockClient: ReturnType<typeof createMockGraphqlClient>;

  beforeEach(() => {
    mockClient = createMockGraphqlClient({ mutationData });
    mockInstanceStores.serverInfo.videoProcessingEnabled = false;
    mockInstanceStores.roomUnread.setRoomUnread.mockClear();
    roomStateMock.members = [];
    roomStateMock.editState.eventId = null;
    roomStateMock.editState.originalBody = '';
    roomStateMock.editState.startEdit.mockClear();
    roomStateMock.editState.cancelEdit.mockClear();
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
      if (variables?.input?.eventId) return Promise.resolve({ data: updateMutationData, error: null });
      return Promise.resolve({ data: mutationData, error: null });
    });
    queryMock.mockReset();
    queryMock.mockResolvedValue({ data: null, error: null });
    sessionStorage.clear();
    vi.clearAllMocks();
  });

  describe('form rendering', () => {
    it('renders the TipTap editor', async () => {
      const { container } = renderMessageComposer(
        { roomId: 'room_456' },
        new Map([['$$_urql', mockClient]])
      );

      await expect.element(q(container, '[data-testid="message-input"]')).toBeInTheDocument();
    });

    it('renders the attachment button', async () => {
      const { container } = renderMessageComposer(
        { roomId: 'room_456' },
        new Map([['$$_urql', mockClient]])
      );

      await expect.element(q(container, 'button[title="Attach file"]')).toBeInTheDocument();
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
  });

  describe('initial state', () => {
    it('editor is editable initially', async () => {
      const { container } = renderMessageComposer(
        { roomId: 'room_456' },
        new Map([['$$_urql', mockClient]])
      );

      await expect
        .element(q(container, '[data-testid="message-input"]'))
        .toHaveAttribute('contenteditable', 'true');
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

      await expect.element(q(container, 'button[title="Send message"]')).toBeInTheDocument();
    });

    it('send button is disabled when input is empty', async () => {
      const { container } = renderMessageComposer(
        { roomId: 'room_456' },
        new Map([['$$_urql', mockClient]])
      );

      await expect.element(q(container, 'button[title="Send message"]')).toBeDisabled();
    });

    it('send button has paper plane icon', async () => {
      const { container } = renderMessageComposer(
        { roomId: 'room_456' },
        new Map([['$$_urql', mockClient]])
      );

      const sendButton = q(container, 'button[title="Send message"]');
      const icon = sendButton?.querySelector('.uil--telegram-alt');
      expect(icon).not.toBeNull();
    });
  });

  describe('pasted attachments', () => {
    it('disables sending typed text while a pasted image is preparing', async () => {
      const file = imageFile();
      const pendingPreparation = deferred<File[]>();
      prepareFilesMock.mockReturnValueOnce(pendingPreparation.promise);
      const { container, roomId } = renderMessageComposer(
        { roomId: 'room_456' },
        new Map([['$$_urql', mockClient]])
      );
      const editor = q(container, '[data-testid="message-input"]')!;

      pasteFile(editor, file);
      await typeInEditor(editor, 'message with image');
      const sendButton = q(container, 'button[title="Send message"]')! as HTMLButtonElement;
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
      const editor = q(container, '[data-testid="message-input"]')!;
      const sendButton = q(container, 'button[title="Send message"]')! as HTMLButtonElement;

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
      const editor = q(container, '[data-testid="message-input"]')!;
      const sendButton = q(container, 'button[title="Send message"]')! as HTMLButtonElement;

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
    it('loads and persists a room text draft in sessionStorage', async () => {
      sessionStorage.setItem('chatto:draft:room_draft', 'saved draft');

      const { container } = renderMessageComposer(
        { roomId: 'room_draft' },
        new Map([['$$_urql', mockClient]]),
        { exactRoomId: true }
      );
      const editor = q(container, '[data-testid="message-input"]')!;

      await expect.element(editor).toHaveTextContent('saved draft');

      await typeInEditor(editor, 'saved draft + more');

      await vi.waitFor(() =>
        expect(sessionStorage.getItem('chatto:draft:room_draft')).toBe('saved draft + more')
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
        .element(q(container, '[data-testid="thread-reply-input"]'))
        .toHaveTextContent('thread draft');
      expect(sessionStorage.getItem('chatto:draft:room_draft')).toBe('room draft');
    });

    it('clears the active text draft after a successful send', async () => {
      const { container, roomId } = renderMessageComposer(
        { roomId: 'room_456' },
        new Map([['$$_urql', mockClient]])
      );
      const editor = q(container, '[data-testid="message-input"]')!;

      await typeInEditor(editor, 'send and clear draft');
      await vi.waitFor(() =>
        expect(sessionStorage.getItem(`chatto:draft:${roomId}`)).toBe('send and clear draft')
      );

      (q(container, 'button[title="Send message"]') as HTMLButtonElement).click();

      await vi.waitFor(() => expect(mutationMock).toHaveBeenCalledOnce());
      await vi.waitFor(() => expect(sessionStorage.getItem(`chatto:draft:${roomId}`)).toBeNull());
    });
  });

  describe('edit mode transitions', () => {
    it('prefills edit text, hides attachment controls, and cancels on Escape', async () => {
      roomStateMock.editState.eventId = 'evt_edit';
      roomStateMock.editState.originalBody = 'original body';
      const { container } = renderMessageComposer(
        { roomId: 'room_456' },
        new Map([['$$_urql', mockClient]])
      );
      const editor = q(container, '[data-testid="message-input"]')!;

      await expect.element(editor).toHaveTextContent('original body');
      expect(q(container, 'button[title="Attach file"]')).toBeNull();

      await pressEditorKey(editor, 'Escape');

      expect(roomStateMock.editState.cancelEdit).toHaveBeenCalledOnce();
      expect(mutationMock).not.toHaveBeenCalled();
    });

    it('clears staged attachments when edit mode is active at mount', async () => {
      const roomId = 'room_edit_attachments';
      const firstRender = renderMessageComposer(
        { roomId },
        new Map([['$$_urql', mockClient]]),
        { exactRoomId: true }
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
        new Map([['$$_urql', mockClient]]),
        { exactRoomId: true }
      );
      expect(q(container, 'button[title="Attach file"]')).toBeNull();
      expect(file.name).toBe('paste.png');
      expect(q(container, 'img')).toBeNull();
    });
  });

  describe('submit behavior', () => {
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
      const editor = q(container, '[data-testid="thread-reply-input"]')!;

      await typeInEditor(editor, 'hello world');
      (q(container, 'input[type="checkbox"]') as HTMLInputElement).click();
      (q(container, 'button[title="Send message"]') as HTMLButtonElement).click();

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
      expect(onMessageSent).toHaveBeenCalledOnce();
      expect(mockInstanceStores.roomUnread.setRoomUnread).toHaveBeenCalledWith(roomId, false);
      expect(roomStateMock.scrollState.requestScrollToBottom).toHaveBeenCalledOnce();
    });

    it('restores text and attachments after a failed post', async () => {
      mutationMock.mockResolvedValueOnce({ data: null, error: new Error('nope') });
      const { container } = renderMessageComposer(
        { roomId: 'room_456' },
        new Map([['$$_urql', mockClient]])
      );
      const editor = q(container, '[data-testid="message-input"]')!;
      const file = selectFirstAttachment(q(container, 'input[type="file"]') as HTMLInputElement);

      await expect.poll(() => q(container, 'img')).toBeTruthy();
      await typeInEditor(editor, 'will retry');
      (q(container, 'button[title="Send message"]') as HTMLButtonElement).click();

      await vi.waitFor(() => expect(mutationMock).toHaveBeenCalledOnce());
      await expect.element(editor).toHaveTextContent('will retry');
      await expect.poll(() => q(container, 'img')).toBeTruthy();
      expect(mutationMock.mock.calls[0][1].input.attachments).toEqual([file]);
      expect(getToasts().map((t) => t.message)).toContain('Failed to send message');
    });
  });

  describe('link preview composer behavior', () => {
    function mockLinkPreview(url: string) {
      queryMock.mockResolvedValueOnce({
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
      const editor = q(container, '[data-testid="message-input"]')!;

      await typeInEditor(editor, `Look ${url}`);

      await vi.waitFor(() => expect(queryMock).toHaveBeenCalledOnce(), { timeout: 1000 });
      await expect.element(q(container, '[data-testid="link-preview-card"]')).toBeInTheDocument();

      (q(container, 'button[title="Send message"]') as HTMLButtonElement).click();

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
      const editor = q(container, '[data-testid="message-input"]')!;

      await typeInEditor(editor, `Dismiss ${url}`);
      await vi.waitFor(() => expect(queryMock).toHaveBeenCalledOnce(), { timeout: 1000 });
      (q(container, 'button[aria-label="Dismiss preview"]') as HTMLButtonElement).click();

      (q(container, 'button[title="Send message"]') as HTMLButtonElement).click();

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
      const editor = q(container, '[data-testid="message-input"]')!;
      selectFirstAttachment(q(container, 'input[type="file"]') as HTMLInputElement);
      await typeInEditor(editor, 'with file');

      (q(container, 'button[title="Send message"]') as HTMLButtonElement).click();

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

    it('send button has title attribute', async () => {
      const { container } = renderMessageComposer(
        { roomId: 'room_456' },
        new Map([['$$_urql', mockClient]])
      );

      await expect
        .element(q(container, 'button[title="Send message"]'))
        .toHaveAttribute('title', 'Send message');
    });
  });
});
