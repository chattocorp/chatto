import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render } from 'vitest-browser-svelte';
import MessageComposer from './MessageComposer.svelte';
import { createMockGraphqlClient, q } from '$lib/test-utils';

const mutationData = { postMessage: { id: 'msg_123' } };
const prepareFilesMock = vi.hoisted(() => vi.fn());
const mutationMock = vi.hoisted(() => vi.fn());
const queryMock = vi.hoisted(() => vi.fn());

// Mock instance state
const mockInstanceStores = {
  currentUser: { user: { id: 'test-user', login: 'testuser' }, loading: false },
  serverInfo: {
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
  serverRegistry: { getStore: () => mockInstanceStores }
}));

vi.mock('$lib/state/activeServer.svelte', () => ({
  getActiveServer: () => () => 'test-instance'
}));

vi.mock('$lib/state/room', () => ({
  getRoomMembers: () => [],
  getComposerContext: () => ({
    editState: {
      eventId: null,
      originalBody: '',
      startEdit: vi.fn(),
      cancelEdit: vi.fn()
    },
    lastEditableMessage: {
      getLastEditableMessage: () => null,
      setFinder: vi.fn()
    },
    scrollState: {
      scrollRequestCounter: 0,
      requestScrollToBottom: vi.fn(),
      setContainer: vi.fn(),
      setShouldScroll: vi.fn(),
      scrollToBottomIfSticky: vi.fn()
    }
  })
}));

function renderMessageComposer(props: { roomId: string }, context: Map<string, unknown>) {
  return render(MessageComposer, { props, context });
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
  document.execCommand('insertText', false, text);
  await vi.waitFor(() => expect(editor.textContent).toBe(text));
}

describe('MessageComposer', () => {
  let mockClient: ReturnType<typeof createMockGraphqlClient>;

  beforeEach(() => {
    mockClient = createMockGraphqlClient({ mutationData });
    prepareFilesMock.mockReset();
    prepareFilesMock.mockImplementation(async (files: File[]) => files);
    mutationMock.mockReset();
    mutationMock.mockResolvedValue({ data: mutationData, error: null });
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
    it('accepts image, video, and audio files', async () => {
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
      const { container } = renderMessageComposer(
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
        roomId: 'room_456',
        body: 'message with image',
        attachments: [file]
      });
    });

    it('disables image-only send until a pasted image preview appears', async () => {
      const file = imageFile();
      const pendingPreparation = deferred<File[]>();
      prepareFilesMock.mockReturnValueOnce(pendingPreparation.promise);
      const { container } = renderMessageComposer(
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
        roomId: 'room_456',
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
