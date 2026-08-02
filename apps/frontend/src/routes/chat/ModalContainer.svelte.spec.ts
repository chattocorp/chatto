import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { q } from '$lib/test-utils';

const { mocks } = vi.hoisted(() => ({
  mocks: {
    modal: {
      type: 'logout'
    } as Record<string, unknown> | undefined,
    notifyPageState: () => {},
    closeModal: vi.fn(),
    goto: vi.fn(),
    replaceState: vi.fn(),
    refreshAttachmentUrlsForAssets: vi.fn(),
    toastSuccess: vi.fn(),
    toastError: vi.fn(),
    leaveRoom: vi.fn(),
    deleteMessage: vi.fn(),
    deleteAttachment: vi.fn(),
    deleteLinkPreview: vi.fn(),
    mutation: vi.fn(() => ({
      toPromise: () => Promise.resolve({ data: {}, error: null })
    })),
    getClient: vi.fn(),
    activeServer: 'origin',
    serverIdParam: '-' as string | undefined,
    servers: [] as Array<{ id: string; url: string; name: string; token: string | null }>,
    originServer: undefined as
      { id: string; url: string; name: string; token: string | null } | undefined,
    authenticated: {} as Record<string, boolean>,
    beginExplicitSignOutRedirect: vi.fn(),
    signOutServer: vi.fn(),
    signOutServers: vi.fn(),
    hardRedirectAfterSignOut: vi.fn(),
    notifyLogout: vi.fn(),
    clearLastRoom: vi.fn(),
    removeServer: vi.fn(),
    removeAll: vi.fn(),
    clearServerAuthentication: vi.fn(),
    resetToOrigin: vi.fn(),
    signOutAuthling: vi.fn(),
    signOutCurrentAccount: vi.fn(),
    signOutAllAccount: vi.fn()
  }
}));

vi.mock('$app/state', async () => {
  const { createSubscriber } = await import('svelte/reactivity');
  let notify: () => void = () => {};
  const subscribe = createSubscriber((update) => {
    notify = update;
    return () => {
      notify = () => {};
    };
  });
  mocks.notifyPageState = () => notify();

  return {
    page: {
      get state() {
        subscribe();
        return { modal: mocks.modal };
      },
      get params() {
        return mocks.serverIdParam ? { serverId: mocks.serverIdParam } : {};
      },
      url: new URL('https://chat.example.test/chat/-')
    }
  };
});

vi.mock('$app/navigation', () => ({
  goto: mocks.goto,
  replaceState: mocks.replaceState
}));

vi.mock('$app/environment', () => ({ version: '0.5.0-test' }));

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
    getServer: vi.fn((id: string) => mocks.servers.find((server) => server.id === id)),
    isOriginServer: vi.fn((id: string) => mocks.originServer?.id === id),
    isAuthenticated: vi.fn((id: string) => mocks.authenticated[id] === true),
    firstAuthenticatedServerId: vi.fn((excludedId: string) => {
      const originId = mocks.originServer?.id;
      if (originId && originId !== excludedId && mocks.authenticated[originId]) return originId;
      return mocks.servers.find(
        (server) => server.id !== excludedId && mocks.authenticated[server.id]
      )?.id;
    }),
    clearServerAuthentication: mocks.clearServerAuthentication,
    removeServer: mocks.removeServer,
    removeAll: mocks.removeAll,
    resetToOrigin: mocks.resetToOrigin,
    get servers() {
      return mocks.servers;
    },
    get originServer() {
      return mocks.originServer;
    }
  }
}));

vi.mock('$lib/state/server/serverConnection.svelte', () => ({
  serverConnectionManager: {
    getClient: (serverId: string) => {
      mocks.getClient(serverId);
      return {
        serverId,
        connectBaseUrl: `https://${serverId}.example.test/api/connect`,
        bearerToken: null,
        getAPI: (factory: (config: never) => unknown) => factory({} as never),
        client: {
          mutation: mocks.mutation
        }
      };
    }
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
  beginExplicitSignOutRedirect: mocks.beginExplicitSignOutRedirect,
  signOutServer: mocks.signOutServer,
  signOutServers: mocks.signOutServers,
  hardRedirectAfterSignOut: mocks.hardRedirectAfterSignOut
}));

vi.mock('$lib/accountData/signOut', () => ({
  signOutAccountData: mocks.signOutAuthling
}));

vi.mock('$lib/state/clientAccount', () => ({
  clientAccount: {
    signOutCurrentServer: mocks.signOutCurrentAccount,
    signOutAllServers: mocks.signOutAllAccount
  }
}));

vi.mock('$lib/attachments/attachmentUrls', () => ({
  LIGHTBOX_ATTACHMENT_IMAGE_REFRESH: {
    width: 2048,
    height: 2048,
    fit: 'contain'
  },
  refreshAttachmentUrlsForAssets: mocks.refreshAttachmentUrlsForAssets
}));

vi.mock('$lib/api-client/messages', () => ({
  createMessageAPI: () => ({
    deleteMessage: mocks.deleteMessage,
    deleteAttachment: mocks.deleteAttachment,
    deleteLinkPreview: mocks.deleteLinkPreview
  })
}));

vi.mock('$lib/api-client/rooms', () => ({
  createRoomCommandAPI: () => ({
    leaveRoom: mocks.leaveRoom
  })
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

function setModal(modal: Record<string, unknown> | undefined): void {
  mocks.modal = modal;
  mocks.notifyPageState();
}

beforeEach(() => {
  vi.spyOn(window.history, 'back').mockImplementation(() => undefined);
  mocks.modal = {
    type: 'logout'
  };
  mocks.leaveRoom.mockResolvedValue(undefined);
  mocks.deleteMessage.mockResolvedValue(true);
  mocks.deleteAttachment.mockResolvedValue(true);
  mocks.deleteLinkPreview.mockResolvedValue(true);
  mocks.refreshAttachmentUrlsForAssets.mockResolvedValue(new Map());
  mocks.mutation.mockReturnValue({
    toPromise: () => Promise.resolve({ data: {}, error: null })
  });
  mocks.signOutServer.mockResolvedValue(new Response('{}', { status: 200 }));
  mocks.signOutServers.mockResolvedValue(undefined);
  mocks.signOutAuthling.mockResolvedValue(undefined);
  mocks.signOutCurrentAccount.mockImplementation(async (serverId: string) => {
    const server = mocks.servers.find((candidate) => candidate.id === serverId);
    if (!server) return null;
    const origin = mocks.originServer?.id === serverId;
    if (origin) mocks.beginExplicitSignOutRedirect();
    await mocks.signOutServer(server, origin).catch(() => undefined);
    mocks.clearLastRoom(serverId);
    if (origin) {
      mocks.clearServerAuthentication(serverId);
      mocks.notifyLogout();
      const next = mocks.servers.find(
        (candidate) => candidate.id !== serverId && mocks.authenticated[candidate.id]
      );
      return { kind: 'hard' as const, serverId: next?.id };
    }
    mocks.clearServerAuthentication(serverId);
    const next =
      mocks.originServer && mocks.authenticated[mocks.originServer.id]
        ? mocks.originServer
        : mocks.servers.find(
            (candidate) => candidate.id !== serverId && mocks.authenticated[candidate.id]
          );
    return { kind: 'soft' as const, serverId: next?.id };
  });
  mocks.signOutAllAccount.mockImplementation(async () => {
    mocks.beginExplicitSignOutRedirect();
    await mocks.signOutServers(mocks.servers, () => false);
    await mocks.signOutAuthling().catch(() => undefined);
    mocks.resetToOrigin();
    mocks.notifyLogout();
    return { kind: 'hard' as const };
  });
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

afterEach(() => {
  vi.useRealTimers();
});

describe('ModalContainer image viewer', () => {
  it('refreshes compressed display and original URLs independently', async () => {
    vi.useFakeTimers();
    mocks.modal = {
      type: 'imageViewer',
      serverId: 'remote',
      roomId: 'room_1',
      eventId: 'event_1',
      imageItems: [
        {
          id: 'att_1',
          src: '/assets/files/att_1/image/2048x2048/contain?access=old',
          originalSrc: '/assets/files/att_1?access=old',
          filename: 'image.jpg'
        }
      ],
      imageIndex: 0
    };
    mocks.servers.push({
      id: 'remote',
      url: 'https://remote.example.test',
      name: 'Remote',
      token: 'remote-token'
    });
    mocks.refreshAttachmentUrlsForAssets.mockResolvedValue(
      new Map([
        [
          'att_1',
          {
            assetUrl: { url: '/assets/files/att_1?access=fresh' },
            thumbnailAssetUrl: {
              url: '/assets/files/att_1/image/2048x2048/contain?access=fresh'
            }
          }
        ]
      ])
    );

    render(ModalContainer);
    await vi.advanceTimersByTimeAsync(22 * 60 * 60 * 1000);

    expect(mocks.refreshAttachmentUrlsForAssets).toHaveBeenCalledWith(
      expect.anything(),
      'room_1',
      ['att_1'],
      { width: 2048, height: 2048, fit: 'contain' }
    );
    expect(mocks.getClient).toHaveBeenCalledWith('remote');
    expect(mocks.replaceState).toHaveBeenCalledWith('', {
      modal: {
        ...mocks.modal,
        imageItems: [
          {
            id: 'att_1',
            src: 'https://remote.example.test/assets/files/att_1/image/2048x2048/contain?access=fresh',
            originalSrc: 'https://remote.example.test/assets/files/att_1?access=fresh',
            filename: 'image.jpg'
          }
        ],
        imageIndex: 0
      }
    });
  });

  it('preserves an image selected while URL refresh is pending', async () => {
    vi.useFakeTimers();
    mocks.modal = {
      type: 'imageViewer',
      serverId: 'origin',
      roomId: 'room_1',
      eventId: 'event_1',
      imageItems: [
        { id: 'att_1', src: '/assets/files/att_1?access=old', filename: 'first.jpg' },
        { id: 'att_2', src: '/assets/files/att_2?access=old', filename: 'second.jpg' }
      ],
      imageIndex: 0
    };
    let finishRefresh: ((urls: Map<string, unknown>) => void) | undefined;
    mocks.refreshAttachmentUrlsForAssets.mockImplementationOnce(
      () =>
        new Promise((resolve) => {
          finishRefresh = resolve;
        })
    );
    mocks.replaceState.mockImplementationOnce(
      (_url: string, state: { modal?: Record<string, unknown> }) => setModal(state.modal)
    );

    const { container } = render(ModalContainer);
    vi.advanceTimersByTime(22 * 60 * 60 * 1000);
    await vi.waitFor(() => expect(mocks.refreshAttachmentUrlsForAssets).toHaveBeenCalledOnce());

    container.querySelector<HTMLButtonElement>('button[aria-label="Next image"]')?.click();
    await vi.waitFor(() => {
      expect(container.textContent).toContain('2 / 2');
    });

    finishRefresh?.(
      new Map([
        [
          'att_1',
          {
            assetUrl: { url: '/assets/files/att_1?access=fresh' },
            thumbnailAssetUrl: { url: '/assets/files/att_1/thumbnail?access=fresh' }
          }
        ]
      ])
    );

    await vi.waitFor(() => {
      expect(mocks.replaceState).toHaveBeenCalledOnce();
      expect(container.textContent).toContain('2 / 2');
    });
  });

  it('does not apply a late refresh to the same room on another server', async () => {
    vi.useFakeTimers();
    mocks.modal = {
      type: 'imageViewer',
      serverId: 'remote',
      roomId: 'room_1',
      eventId: 'event_1',
      imageItems: [{ id: 'att_1', src: '/assets/files/att_1?access=old' }],
      imageIndex: 0
    };
    let finishRefresh: ((urls: Map<string, unknown>) => void) | undefined;
    mocks.refreshAttachmentUrlsForAssets.mockImplementationOnce(
      () =>
        new Promise((resolve) => {
          finishRefresh = resolve;
        })
    );

    render(ModalContainer);
    vi.advanceTimersByTime(22 * 60 * 60 * 1000);
    await vi.waitFor(() => expect(mocks.getClient).toHaveBeenCalledWith('remote'));

    setModal({
      ...mocks.modal,
      serverId: 'origin'
    });
    finishRefresh?.(
      new Map([
        [
          'att_1',
          {
            assetUrl: { url: '/assets/files/att_1?access=fresh' },
            thumbnailAssetUrl: { url: '/assets/files/att_1/thumbnail?access=fresh' }
          }
        ]
      ])
    );
    await Promise.resolve();
    await Promise.resolve();

    expect(mocks.replaceState).not.toHaveBeenCalled();
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
    expect(findButton(container, 'All Servers').dataset.variant).toBe('danger');
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
      expect(mocks.clearServerAuthentication).toHaveBeenCalledWith(remote.id);
      expect(mocks.removeServer).not.toHaveBeenCalled();
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
      expect(mocks.beginExplicitSignOutRedirect).toHaveBeenCalledOnce();
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
      expect(mocks.beginExplicitSignOutRedirect).toHaveBeenCalledOnce();
      expect(mocks.signOutServers).toHaveBeenCalledWith(mocks.servers, expect.any(Function));
      expect(mocks.signOutAuthling).toHaveBeenCalledOnce();
      expect(mocks.resetToOrigin).toHaveBeenCalledOnce();
      expect(mocks.notifyLogout).toHaveBeenCalledOnce();
      expect(mocks.hardRedirectAfterSignOut).toHaveBeenCalledWith('/');
      expect(mocks.removeServer).not.toHaveBeenCalled();
    });
    expect(mocks.signOutAuthling.mock.invocationCallOrder[0]).toBeLessThan(
      mocks.resetToOrigin.mock.invocationCallOrder[0]
    );
  });

  it('finishes all-server sign-out when Authling cleanup fails', async () => {
    mocks.modal = { type: 'logout' };
    mocks.signOutAuthling.mockRejectedValueOnce(new Error('Authling is unavailable'));

    const { container } = render(ModalContainer);
    clickButton(container, 'All Servers');

    await vi.waitFor(() => {
      expect(mocks.signOutAuthling).toHaveBeenCalledOnce();
      expect(mocks.resetToOrigin).toHaveBeenCalledOnce();
      expect(mocks.notifyLogout).toHaveBeenCalledOnce();
      expect(mocks.hardRedirectAfterSignOut).toHaveBeenCalledWith('/');
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
      expect(mocks.beginExplicitSignOutRedirect).toHaveBeenCalledOnce();
      expect(mocks.signOutServers).toHaveBeenCalledWith([], expect.any(Function));
      expect(mocks.signOutAuthling).toHaveBeenCalledOnce();
      expect(mocks.resetToOrigin).toHaveBeenCalledOnce();
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
      expect(mocks.beginExplicitSignOutRedirect).toHaveBeenCalledOnce();
      expect(mocks.signOutServers).toHaveBeenCalledWith(mocks.servers, expect.any(Function));
      expect(mocks.signOutAuthling).toHaveBeenCalledOnce();
      expect(mocks.resetToOrigin).toHaveBeenCalledOnce();
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

describe('ModalContainer About Chatto modal', () => {
  it('shows the interactive Chatto wordmark', async () => {
    mocks.modal = { type: 'aboutChatto' };

    const { container } = render(ModalContainer);

    expect(q(container, 'dialog')?.getAttribute('aria-label')).toBe('About Chatto');
    expect(container.textContent ?? '').toContain('v0.5.0-test');
    expect(
      container.querySelector('a[href="https://github.com/chattocorp/chatto"]')
    ).not.toBeNull();
    expect(container.querySelector('a[href="https://docs.chatto.run"]')).not.toBeNull();
    await vi.waitFor(
      () => {
        const wordmarkButton = container.querySelector<HTMLButtonElement>(
          'button[aria-label="Fire a ready laser at Chatto"]'
        );
        expect(wordmarkButton).not.toBeNull();
        expect(wordmarkButton?.querySelector('canvas')).not.toBeNull();
      },
      { timeout: 10_000 }
    );
  });
});

describe('ModalContainer remove server modal', () => {
  it('removes an inactive selected server without navigating away from the active server', async () => {
    const remote = {
      id: 'remote',
      url: 'https://remote.example.test',
      name: 'Remote',
      token: 'token'
    };
    mocks.servers = [mocks.originServer!, remote];
    mocks.modal = { type: 'removeServer', serverId: 'remote', spaceName: 'Remote' };

    const { container } = render(ModalContainer);
    await expect
      .element(q(container, '[href="/chat/remote.example.test/settings/account"]'))
      .toHaveTextContent('Account Settings');
    expect(container.textContent).toContain(
      'Your account and data on the server will not be deleted.'
    );
    clickButton(container, 'Remove Server');

    await vi.waitFor(() => {
      expect(mocks.clearLastRoom).toHaveBeenCalledWith('remote');
      expect(mocks.removeServer).toHaveBeenCalledWith('remote');
      expect(window.history.back).toHaveBeenCalledOnce();
    });
    expect(mocks.goto).not.toHaveBeenCalled();
  });

  it('navigates to the origin after removing the active remote server', async () => {
    const remote = {
      id: 'remote',
      url: 'https://remote.example.test',
      name: 'Remote',
      token: 'token'
    };
    mocks.servers = [mocks.originServer!, remote];
    mocks.activeServer = 'remote';
    mocks.modal = { type: 'removeServer', serverId: 'remote', spaceName: 'Remote' };

    const { container } = render(ModalContainer);
    clickButton(container, 'Remove Server');

    await vi.waitFor(() => {
      expect(mocks.removeServer).toHaveBeenCalledWith('remote');
      expect(mocks.goto).toHaveBeenCalledWith('/chat/-');
    });
  });
});

describe('ModalContainer leave room modal', () => {
  it('leaves the room and returns to the modal server', async () => {
    mocks.modal = {
      type: 'leaveRoom',
      serverId: 'remote',
      roomId: 'room-1',
      roomName: 'General'
    };

    const { container } = render(ModalContainer);
    clickButton(container, 'Leave Room');

    await vi.waitFor(() => {
      expect(mocks.leaveRoom).toHaveBeenCalledWith('room-1');
      expect(mocks.getClient).toHaveBeenCalledWith('remote');
      expect(mocks.clearLastRoom).toHaveBeenCalledWith('remote');
      expect(mocks.goto).toHaveBeenCalledWith('/chat/remote.example.test');
    });
  });
});

describe('ModalContainer message mutation modals', () => {
  it('remounts replacement modals and fences stale action completion', async () => {
    let finishDelete: (() => void) | undefined;
    mocks.modal = {
      type: 'deleteMessage',
      serverId: 'origin',
      roomId: 'room-1',
      eventId: 'event-1'
    };
    mocks.deleteMessage.mockImplementation(
      () =>
        new Promise<boolean>((resolve) => {
          finishDelete = () => resolve(true);
        })
    );

    const { container } = render(ModalContainer);
    clickButton(container, 'Delete');

    await vi.waitFor(() => {
      expect(findButton(container, 'Delete').getAttribute('aria-busy')).toBe('true');
    });

    const replacementModal = {
      type: 'deleteMessage',
      serverId: 'origin',
      roomId: 'room-1',
      eventId: 'event-1'
    };
    setModal(replacementModal);

    await vi.waitFor(() => {
      expect(findButton(container, 'Delete').hasAttribute('aria-busy')).toBe(false);
    });

    finishDelete?.();

    await vi.waitFor(() => {
      expect(mocks.toastSuccess).toHaveBeenCalledOnce();
    });
    expect(window.history.back).not.toHaveBeenCalled();
    expect(mocks.modal).toBe(replacementModal);
  });

  it('notifies the visible room after message deletion succeeds', async () => {
    mocks.modal = {
      type: 'deleteMessage',
      serverId: 'remote',
      roomId: 'room-1',
      eventId: 'event-1'
    };
    const listener = vi.fn();
    window.addEventListener('chatto:room-message-mutated', listener);

    try {
      const { container } = render(ModalContainer);
      clickButton(container, 'Delete');

      await vi.waitFor(() => {
        expect(mocks.deleteMessage).toHaveBeenCalledWith('room-1', 'event-1');
        expect(mocks.getClient).toHaveBeenCalledWith('remote');
        expect(listener).toHaveBeenCalledOnce();
      });
      expect((listener.mock.calls[0][0] as CustomEvent).detail).toEqual({
        serverId: 'remote',
        roomId: 'room-1',
        eventId: 'event-1',
        reason: 'message-deleted'
      });
      expect(mocks.toastSuccess).toHaveBeenCalledOnce();
    } finally {
      window.removeEventListener('chatto:room-message-mutated', listener);
    }
  });

  it('notifies the visible room after attachment deletion succeeds', async () => {
    mocks.modal = {
      type: 'deleteAttachment',
      serverId: 'remote',
      roomId: 'room-1',
      eventId: 'event-1',
      attachmentId: 'attachment-1'
    };
    const listener = vi.fn();
    window.addEventListener('chatto:room-message-mutated', listener);

    try {
      const { container } = render(ModalContainer);
      clickButton(container, 'Delete');

      await vi.waitFor(() => {
        expect(mocks.deleteAttachment).toHaveBeenCalledWith('room-1', 'event-1', 'attachment-1');
        expect(mocks.getClient).toHaveBeenCalledWith('remote');
        expect(listener).toHaveBeenCalledOnce();
      });
      expect((listener.mock.calls[0][0] as CustomEvent).detail).toEqual({
        serverId: 'remote',
        roomId: 'room-1',
        eventId: 'event-1',
        reason: 'attachment-deleted'
      });
    } finally {
      window.removeEventListener('chatto:room-message-mutated', listener);
    }
  });

  it('notifies the visible room after link preview deletion succeeds', async () => {
    mocks.modal = {
      type: 'deleteLinkPreview',
      serverId: 'remote',
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
        expect(mocks.getClient).toHaveBeenCalledWith('remote');
        expect(listener).toHaveBeenCalledOnce();
      });
      expect((listener.mock.calls[0][0] as CustomEvent).detail).toEqual({
        serverId: 'remote',
        roomId: 'room-1',
        eventId: 'event-1',
        reason: 'link-preview-deleted'
      });
    } finally {
      window.removeEventListener('chatto:room-message-mutated', listener);
    }
  });
});
