import { beforeEach, describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { q } from '$lib/test-utils';

const { mocks } = vi.hoisted(() => ({
  mocks: {
    modal: {
      type: 'joinRoom',
      roomId: 'room-1',
      roomName: 'general',
      viewerCanJoinRoom: true
    } as Record<string, unknown> | undefined,
    goto: vi.fn(),
    toastSuccess: vi.fn(),
    toastError: vi.fn(),
    joinRoom: vi.fn(),
    refreshRooms: vi.fn()
  }
}));

vi.mock('$app/state', () => ({
  page: {
    get state() {
      return { modal: mocks.modal };
    },
    url: new URL('https://chat.example.test/chat/-')
  }
}));

vi.mock('$app/navigation', () => ({
  goto: mocks.goto,
  replaceState: vi.fn()
}));

vi.mock('$app/paths', () => ({
  resolve: (path: string, params?: Record<string, string>) =>
    path
      .replace('[serverId]', params?.serverId ?? '')
      .replace('[roomId]', params?.roomId ?? '')
}));

vi.mock('$lib/navigation', () => ({
  serverIdToSegment: () => '-'
}));

vi.mock('$lib/state/activeServer.svelte', () => ({
  getActiveServer: () => 'origin'
}));

vi.mock('$lib/state/server/registry.svelte', () => ({
  serverRegistry: {
    getStore: vi.fn(() => ({
      roomDirectory: {
        joinRoom: mocks.joinRoom
      },
      rooms: {
        refresh: mocks.refreshRooms
      }
    })),
    removeServer: vi.fn(),
    removeAll: vi.fn(),
    originServer: { id: 'origin', token: null }
  }
}));

vi.mock('$lib/state/server/graphqlClient.svelte', () => ({
  graphqlClientManager: {
    getClient: vi.fn(() => ({
      client: {
        mutation: vi.fn(() => ({
          toPromise: () => Promise.resolve({ data: {}, error: null })
        }))
      }
    }))
  }
}));

vi.mock('$lib/ui/toast', () => ({
  toast: {
    success: mocks.toastSuccess,
    error: mocks.toastError
  }
}));

vi.mock('$lib/storage/lastRoom', () => ({
  clearLastRoom: vi.fn()
}));

vi.mock('$lib/auth/sessionChannel', () => ({
  notifyLogout: vi.fn()
}));

vi.mock('$lib/auth/csrf', () => ({
  csrfFetch: vi.fn()
}));

vi.mock('$lib/attachments/attachmentUrls', () => ({
  refreshAttachmentUrlsForMessage: vi.fn()
}));

vi.mock('$lib/CreateRoom.svelte', () => ({
  default: {}
}));

vi.mock('$lib/ui/ImageModal.svelte', () => ({
  default: {}
}));

vi.mock('$lib/ui/ConfirmDialog.svelte', async () => {
  const { default: ConfirmDialogMock } = await import('./ModalContainerConfirmDialogMock.svelte');
  return { default: ConfirmDialogMock };
});

vi.mock('$lib/ui/Dialog.svelte', async () => {
  const { default: DialogMock } = await import('./ModalContainerDialogMock.svelte');
  return { default: DialogMock };
});

vi.mock('$lib/ui/form', async () => {
  const { default: ButtonMock } = await import('./ModalContainerButtonMock.svelte');
  return { Button: ButtonMock };
});

import ModalContainer from './ModalContainer.svelte';

beforeEach(() => {
  vi.spyOn(window.history, 'back').mockImplementation(() => undefined);
  mocks.modal = {
    type: 'joinRoom',
    roomId: 'room-1',
    roomName: 'general',
    viewerCanJoinRoom: true
  };
  mocks.joinRoom.mockResolvedValue({ ok: true, room: { id: 'room-1', name: 'general' } });
  mocks.refreshRooms.mockResolvedValue(undefined);
  vi.clearAllMocks();
});

describe('ModalContainer join room modal', () => {
  it('joins and navigates from a joinable room modal', async () => {
    const { container } = render(ModalContainer);

    await expect.element(q(container, 'dialog')).toHaveTextContent(
      'Join #general to read and participate in this room.'
    );
    (q(container, 'button[type="submit"]') as HTMLButtonElement).click();

    await vi.waitFor(() => {
      expect(mocks.joinRoom).toHaveBeenCalledWith('room-1');
      expect(mocks.refreshRooms).toHaveBeenCalledOnce();
      expect(mocks.toastSuccess).toHaveBeenCalledWith('Joined #general');
      expect(mocks.goto).toHaveBeenCalledWith('/chat/-/room-1');
    });
  });

  it('shows an error toast when joining fails', async () => {
    mocks.joinRoom.mockResolvedValue({ ok: false, error: new Error('denied') });

    const { container } = render(ModalContainer);
    (q(container, 'button[type="submit"]') as HTMLButtonElement).click();

    await vi.waitFor(() => {
      expect(mocks.toastError).toHaveBeenCalledWith('Failed to join room');
      expect(mocks.refreshRooms).not.toHaveBeenCalled();
      expect(mocks.goto).not.toHaveBeenCalled();
    });
  });

  it('renders a non-mutating access message for non-joinable rooms', async () => {
    mocks.modal = {
      type: 'joinRoom',
      roomId: 'room-1',
      roomName: 'private',
      viewerCanJoinRoom: false
    };

    const { container } = render(ModalContainer);

    await expect.element(q(container, 'dialog')).toHaveTextContent(
      'You do not have permission to join this room.'
    );
    expect([...container.querySelectorAll('button')].map((button) => button.textContent?.trim())).toEqual([
      'Got it'
    ]);
    (q(container, 'button') as HTMLButtonElement).click();

    expect(mocks.joinRoom).not.toHaveBeenCalled();
  });
});
