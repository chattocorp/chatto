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
    closeModal: vi.fn(),
    goto: vi.fn(),
    toastSuccess: vi.fn(),
    toastError: vi.fn(),
    joinRoom: vi.fn(),
    deleteMessage: vi.fn(),
    deleteAttachment: vi.fn(),
    deleteLinkPreview: vi.fn(),
    refreshRooms: vi.fn(),
    mutation: vi.fn(() => ({
      toPromise: () => Promise.resolve({ data: {}, error: null })
    })),
    activeServer: 'origin',
    serverIdParam: '-' as string | undefined,
    servers: [] as Array<{ id: string; url: string; name: string; token: string | null }>,
    originServer: undefined as
      | { id: string; url: string; name: string; token: string | null }
      | undefined,
    authenticated: {} as Record<string, boolean>,
    signOutServer: vi.fn(),
    signOutServers: vi.fn(),
    hardRedirectAfterSignOut: vi.fn(),
    notifyLogout: vi.fn(),
    clearLastRoom: vi.fn(),
    removeServer: vi.fn(),
    removeAll: vi.fn(),
    clearServerAuthentication: vi.fn()
  }
}));

vi.mock('$app/state', () => ({
  page: {
    get state() {
      return { modal: mocks.modal };
    },
    get params() {
      return mocks.serverIdParam ? { serverId: mocks.serverIdParam } : {};
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
    path.replace('[serverId]', params?.serverId ?? '').replace('[roomId]', params?.roomId ?? '')
}));

vi.mock('$lib/navigation', () => ({
  serverIdToSegment: (serverId: string) =>
    serverId === 'origin' ? '-' : `${serverId}.example.test`,
  segmentToServerId: (segment: string) =>
    segment === '-' ? 'origin' : segment.endsWith('.example.test') ? segment.slice(0, -13) : null
}));

vi.mock('$lib/state/activeServer.svelte', () => ({
  getActiveServer: () => mocks.activeServer
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
    getServer: vi.fn((id: string) => mocks.servers.find((server) => server.id === id)),
    isOriginServer: vi.fn((id: string) => mocks.originServer?.id === id),
    isAuthenticated: vi.fn((id: string) => mocks.authenticated[id] === true),
    clearServerAuthentication: mocks.clearServerAuthentication,
    removeServer: mocks.removeServer,
    removeAll: mocks.removeAll,
    get servers() {
      return mocks.servers;
    },
    get originServer() {
      return mocks.originServer;
    }
  }
}));

vi.mock('$lib/state/server/graphqlClient.svelte', () => ({
  graphqlClientManager: {
    getClient: vi.fn(() => ({
      serverId: 'origin',
      connectBaseUrl: 'https://origin.example.test/api/connect',
      bearerToken: null,
      client: {
        mutation: mocks.mutation
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
  clearLastRoom: mocks.clearLastRoom
}));

vi.mock('$lib/auth/sessionChannel', () => ({
  notifyLogout: mocks.notifyLogout
}));

vi.mock('$lib/auth/signOut', () => ({
  signOutServer: mocks.signOutServer,
  signOutServers: mocks.signOutServers,
  hardRedirectAfterSignOut: mocks.hardRedirectAfterSignOut
}));

vi.mock('$lib/attachments/attachmentUrls', () => ({
  refreshAttachmentUrlsForMessage: vi.fn()
}));

vi.mock('$lib/CreateRoom.svelte', () => ({
  default: {}
}));

vi.mock('$lib/api/messages', () => ({
  createMessageAPI: () => ({
    deleteMessage: mocks.deleteMessage,
    deleteAttachment: mocks.deleteAttachment,
    deleteLinkPreview: mocks.deleteLinkPreview
  })
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
import SignOutDialog from './SignOutDialog.svelte';

function findButton(container: HTMLElement, label: string): HTMLButtonElement {
  const button = [...container.querySelectorAll('button')].find(
    (candidate) => candidate.textContent?.trim() === label
  );
  if (!(button instanceof HTMLButtonElement)) {
    throw new Error(`Button not found: ${label}`);
  }
  return button;
}

function clickButton(container: HTMLElement, label: string): void {
  const button = findButton(container, label);
  button.click();
}

beforeEach(() => {
  vi.spyOn(window.history, 'back').mockImplementation(() => undefined);
  mocks.modal = {
    type: 'joinRoom',
    roomId: 'room-1',
    roomName: 'general',
    viewerCanJoinRoom: true
  };
  mocks.joinRoom.mockResolvedValue({ ok: true, room: { id: 'room-1', name: 'general' } });
  mocks.deleteMessage.mockResolvedValue(true);
  mocks.deleteAttachment.mockResolvedValue(true);
  mocks.deleteLinkPreview.mockResolvedValue(true);
  mocks.mutation.mockReturnValue({
    toPromise: () => Promise.resolve({ data: {}, error: null })
  });
  mocks.refreshRooms.mockResolvedValue(undefined);
  mocks.signOutServer.mockResolvedValue(new Response('{}', { status: 200 }));
  mocks.signOutServers.mockResolvedValue(undefined);
  mocks.activeServer = 'origin';
  mocks.serverIdParam = '-';
  mocks.originServer = {
    id: 'origin',
    url: 'https://origin.example.test',
    name: 'Origin',
    token: null
  };
  mocks.servers = [mocks.originServer];
  mocks.authenticated = { origin: true };
  vi.clearAllMocks();
});

describe('ModalContainer join room modal', () => {
  it('joins and navigates from a joinable room modal', async () => {
    const { container } = render(ModalContainer);

    await expect
      .element(q(container, 'dialog'))
      .toHaveTextContent('Join #general to read and participate in this room.');
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

    await expect
      .element(q(container, 'dialog'))
      .toHaveTextContent('You do not have permission to join this room.');
    expect(
      [...container.querySelectorAll('button')].map((button) => button.textContent?.trim())
    ).toEqual(['Got it']);
    (q(container, 'button') as HTMLButtonElement).click();

    expect(mocks.joinRoom).not.toHaveBeenCalled();
  });
});

describe('ModalContainer sign out modal', () => {
  it('shows current-server and all-server choices', async () => {
    mocks.modal = { type: 'logout' };

    const { container } = render(ModalContainer);

    await expect
      .element(q(container, 'dialog'))
      .toHaveTextContent('Sign out of only the selected server');
    expect(
      [...container.querySelectorAll('button')].map((button) => button.textContent?.trim())
    ).toEqual(['Cancel', 'Current Server', 'All Servers']);
  });

  it('signs out of only the active remote server', async () => {
    const remote = {
      id: 'remote',
      url: 'https://remote.example.test',
      name: 'Remote',
      token: 'remote-token'
    };
    mocks.modal = { type: 'logout' };
    mocks.activeServer = remote.id;
    mocks.servers = [mocks.originServer!, remote];
    mocks.authenticated = { origin: true, remote: true };

    const { container } = render(ModalContainer);
    clickButton(container, 'Current Server');

    await vi.waitFor(() => {
      expect(mocks.signOutServer).toHaveBeenCalledWith(remote, false);
      expect(mocks.clearLastRoom).toHaveBeenCalledWith(remote.id);
      expect(mocks.removeServer).toHaveBeenCalledWith(remote.id);
      expect(mocks.removeAll).not.toHaveBeenCalled();
      expect(mocks.notifyLogout).not.toHaveBeenCalled();
      expect(mocks.goto).toHaveBeenCalledWith('/chat/-');
    });
  });

  it('clears origin authentication when signing out of the current origin server', async () => {
    const remote = {
      id: 'remote',
      url: 'https://remote.example.test',
      name: 'Remote',
      token: 'remote-token'
    };
    mocks.modal = { type: 'logout' };
    mocks.activeServer = 'origin';
    mocks.servers = [mocks.originServer!, remote];
    mocks.authenticated = { origin: true, remote: true };

    const { container } = render(ModalContainer);
    clickButton(container, 'Current Server');

    await vi.waitFor(() => {
      expect(mocks.signOutServer).toHaveBeenCalledWith(mocks.originServer, true);
      expect(mocks.clearServerAuthentication).toHaveBeenCalledWith('origin');
      expect(mocks.removeServer).not.toHaveBeenCalled();
      expect(mocks.notifyLogout).toHaveBeenCalledOnce();
      expect(mocks.hardRedirectAfterSignOut).toHaveBeenCalledWith('/chat/remote.example.test');
    });
  });

  it('signs out of all registered servers', async () => {
    const remote = {
      id: 'remote',
      url: 'https://remote.example.test',
      name: 'Remote',
      token: 'remote-token'
    };
    mocks.modal = { type: 'logout' };
    mocks.servers = [mocks.originServer!, remote];

    const { container } = render(ModalContainer);
    clickButton(container, 'All Servers');

    await vi.waitFor(() => {
      expect(mocks.signOutServers).toHaveBeenCalledWith(mocks.servers, expect.any(Function));
      expect(mocks.removeAll).toHaveBeenCalledOnce();
      expect(mocks.notifyLogout).toHaveBeenCalledOnce();
      expect(mocks.hardRedirectAfterSignOut).toHaveBeenCalledWith('/');
      expect(mocks.removeServer).not.toHaveBeenCalled();
    });
  });

  it('keeps the all-server escape path when the active server is missing', async () => {
    mocks.modal = { type: 'logout' };
    mocks.activeServer = 'missing';
    mocks.serverIdParam = undefined;
    mocks.originServer = undefined;
    mocks.servers = [];
    mocks.authenticated = {};

    const { container } = render(ModalContainer);

    await expect.element(q(container, 'dialog')).toHaveTextContent('All Servers');
    expect(findButton(container, 'Current Server')).toBeDisabled();
    expect(findButton(container, 'All Servers')).not.toBeDisabled();
    clickButton(container, 'All Servers');

    await vi.waitFor(() => {
      expect(mocks.signOutServers).toHaveBeenCalledWith([], expect.any(Function));
      expect(mocks.removeAll).toHaveBeenCalledOnce();
      expect(mocks.hardRedirectAfterSignOut).toHaveBeenCalledWith('/');
    });
  });

  it('keeps all-server sign-out available outside a server route', async () => {
    mocks.modal = { type: 'logout' };
    mocks.activeServer = 'origin';
    mocks.serverIdParam = undefined;
    mocks.authenticated = { origin: true };

    const { container } = render(ModalContainer);

    expect(findButton(container, 'Current Server')).toBeDisabled();
    expect(findButton(container, 'All Servers')).not.toBeDisabled();
    clickButton(container, 'Current Server');
    expect(mocks.signOutServer).not.toHaveBeenCalled();

    clickButton(container, 'All Servers');

    await vi.waitFor(() => {
      expect(mocks.signOutServers).toHaveBeenCalledWith(mocks.servers, expect.any(Function));
      expect(mocks.removeAll).toHaveBeenCalledOnce();
      expect(mocks.hardRedirectAfterSignOut).toHaveBeenCalledWith('/');
    });
  });

  it('does not reuse busy state when the logout dialog is opened again', async () => {
    let finishSignOut: ((response: Response) => void) | undefined;
    mocks.modal = { type: 'logout' };
    mocks.signOutServer.mockImplementation(
      () =>
        new Promise<Response>((resolve) => {
          finishSignOut = resolve;
        })
    );

    const first = render(SignOutDialog, { props: { onclose: mocks.closeModal } });
    clickButton(first.container, 'Current Server');

    await vi.waitFor(() => {
      expect(findButton(first.container, 'Current Server').getAttribute('aria-busy')).toBe('true');
    });

    const second = render(SignOutDialog, { props: { onclose: mocks.closeModal } });

    expect(findButton(second.container, 'Current Server').hasAttribute('aria-busy')).toBe(false);
    expect(findButton(second.container, 'Current Server')).not.toBeDisabled();
    expect(findButton(second.container, 'All Servers')).not.toBeDisabled();

    finishSignOut?.(new Response('{}', { status: 200 }));
  });
});

describe('ModalContainer message mutation modals', () => {
  it('notifies the visible room after link preview deletion succeeds', async () => {
    mocks.modal = {
      type: 'deleteLinkPreview',
      roomId: 'room-1',
      eventId: 'event-1',
      previewUrl: 'https://example.test/article'
    };
    const listener = vi.fn();
    window.addEventListener('chatto:room-message-mutated', listener);

    try {
      const { container } = render(ModalContainer);
      clickButton(container, 'Delete');

      await vi.waitFor(() => {
        expect(mocks.deleteLinkPreview).toHaveBeenCalledWith(
          'room-1',
          'event-1',
          'https://example.test/article'
        );
        expect(listener).toHaveBeenCalledOnce();
      });
      expect((listener.mock.calls[0][0] as CustomEvent).detail).toEqual({
        roomId: 'room-1',
        eventId: 'event-1',
        reason: 'link-preview-deleted'
      });
    } finally {
      window.removeEventListener('chatto:room-message-mutated', listener);
    }
  });
});
