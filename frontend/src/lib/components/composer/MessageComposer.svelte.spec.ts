import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { Client } from '@urql/svelte';
import MessageComposer from './MessageComposer.svelte';
import { createMockConnection, createMockGraphqlClient, q } from '$lib/test-utils';

const mutationData = { postMessage: { id: 'msg_123' } };

// Mock instance state
const mockInstanceStores = {
  currentUser: { user: { id: 'test-user', login: 'testuser' }, loading: false },
  instance: {
    maxUploadSize: 25 * 1024 * 1024,
    maxVideoUploadSize: 25 * 1024 * 1024
  },
  roomUnread: {
    setRoomUnread: vi.fn()
  }
};

vi.mock('$lib/state/instance/connection.svelte', () => ({
  useConnection: () => () => createMockConnection({ mutationData })
}));

vi.mock('$lib/state/instance/registry.svelte', () => ({
  instanceRegistry: { getStore: () => mockInstanceStores }
}));

vi.mock('$lib/state/activeInstance.svelte', () => ({
  getActiveInstance: () => () => 'test-instance'
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

function renderMessageComposer(
  props: { spaceId: string; roomId: string },
  context: Map<string, unknown>
) {
  return render(MessageComposer, { props, context });
}

describe('MessageComposer', () => {
  let mockClient: Client;

  beforeEach(() => {
    mockClient = createMockGraphqlClient({ mutationData });
    vi.clearAllMocks();
  });

  describe('form rendering', () => {
    it('renders the TipTap editor', async () => {
      const { container } = renderMessageComposer(
        { spaceId: 'space_123', roomId: 'room_456' },
        new Map([['$$_urql', mockClient]])
      );

      await expect.element(q(container, '[data-testid="message-input"]')).toBeInTheDocument();
    });

    it('renders the attachment button', async () => {
      const { container } = renderMessageComposer(
        { spaceId: 'space_123', roomId: 'room_456' },
        new Map([['$$_urql', mockClient]])
      );

      await expect.element(q(container, 'button[title="Attach file"]')).toBeInTheDocument();
    });

    it('renders hidden file input', async () => {
      const { container } = renderMessageComposer(
        { spaceId: 'space_123', roomId: 'room_456' },
        new Map([['$$_urql', mockClient]])
      );

      const fileInput = q(container, 'input[type="file"]');
      await expect.element(fileInput).toBeInTheDocument();
      await expect.element(fileInput).toHaveClass('hidden');
    });

    it('editor has correct placeholder', async () => {
      const { container } = renderMessageComposer(
        { spaceId: 'space_123', roomId: 'room_456' },
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
        { spaceId: 'space_123', roomId: 'room_456' },
        new Map([['$$_urql', mockClient]])
      );

      await expect
        .element(q(container, 'input[type="file"]'))
        .toHaveAttribute('accept', 'image/*,video/*,audio/*');
    });

    it('allows multiple file selection', async () => {
      const { container } = renderMessageComposer(
        { spaceId: 'space_123', roomId: 'room_456' },
        new Map([['$$_urql', mockClient]])
      );

      await expect.element(q(container, 'input[type="file"]')).toHaveAttribute('multiple');
    });
  });

  describe('initial state', () => {
    it('editor is editable initially', async () => {
      const { container } = renderMessageComposer(
        { spaceId: 'space_123', roomId: 'room_456' },
        new Map([['$$_urql', mockClient]])
      );

      await expect
        .element(q(container, '[data-testid="message-input"]'))
        .toHaveAttribute('contenteditable', 'true');
    });

    it('attachment button is not disabled initially', async () => {
      const { container } = renderMessageComposer(
        { spaceId: 'space_123', roomId: 'room_456' },
        new Map([['$$_urql', mockClient]])
      );

      await expect.element(q(container, 'button[title="Attach file"]')).not.toBeDisabled();
    });

    it('does not show file preview area initially', async () => {
      const { container } = renderMessageComposer(
        { spaceId: 'space_123', roomId: 'room_456' },
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
        { spaceId: 'space_123', roomId: 'room_456' },
        new Map([['$$_urql', mockClient]])
      );

      await expect.element(q(container, 'button[title="Send message"]')).toBeInTheDocument();
    });

    it('send button is disabled when input is empty', async () => {
      const { container } = renderMessageComposer(
        { spaceId: 'space_123', roomId: 'room_456' },
        new Map([['$$_urql', mockClient]])
      );

      await expect.element(q(container, 'button[title="Send message"]')).toBeDisabled();
    });

    it('send button has paper plane icon', async () => {
      const { container } = renderMessageComposer(
        { spaceId: 'space_123', roomId: 'room_456' },
        new Map([['$$_urql', mockClient]])
      );

      const sendButton = q(container, 'button[title="Send message"]');
      const icon = sendButton?.querySelector('.uil--telegram-alt');
      expect(icon).not.toBeNull();
    });
  });

  describe('accessibility', () => {
    it('attachment button has title attribute', async () => {
      const { container } = renderMessageComposer(
        { spaceId: 'space_123', roomId: 'room_456' },
        new Map([['$$_urql', mockClient]])
      );

      await expect
        .element(q(container, 'button[title="Attach file"]'))
        .toHaveAttribute('title', 'Attach file');
    });

    it('send button has title attribute', async () => {
      const { container } = renderMessageComposer(
        { spaceId: 'space_123', roomId: 'room_456' },
        new Map([['$$_urql', mockClient]])
      );

      await expect
        .element(q(container, 'button[title="Send message"]'))
        .toHaveAttribute('title', 'Send message');
    });
  });
});
