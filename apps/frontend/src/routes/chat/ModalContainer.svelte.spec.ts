import { beforeEach, describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { q } from '$lib/test-utils';

const { mocks } = vi.hoisted(() => ({
  mocks: {
    modal: {
      type: 'aboutChatto'
    } as Record<string, unknown> | undefined,
    notifyPageState: () => {},
    goto: vi.fn(),
    toastSuccess: vi.fn(),
    toastError: vi.fn(),
    leaveRoom: vi.fn(),
    leaveRoomDirectory: [] as Array<{ id: string; viewerCanRejoin?: boolean }>,
    deleteMessage: vi.fn(),
    deleteAttachment: vi.fn(),
    deleteLinkPreview: vi.fn(),
    applyLocalMessageMutation: vi.fn(),
    tryGetStore: vi.fn(),
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
    clearLastRoom: vi.fn(),
    removeServer: vi.fn(),
    unsubscribePushBeforeLeaving: vi.fn()
  }
}));

vi.mock('$lib/client', async () => ({
  ...(await import('$lib/test-utils/clientMock')).clientMockDefaults,
  serverRegistry: {
    getServer: vi.fn((id: string) => mocks.servers.find((server) => server.id === id)),
    isOriginServer: vi.fn((id: string) => mocks.originServer?.id === id),
    isAuthenticated: vi.fn((id: string) => mocks.authenticated[id] === true),
    tryGetStore: mocks.tryGetStore,
    removeServer: mocks.removeServer,
    get servers() {
      return mocks.servers;
    },
    get originServer() {
      return mocks.originServer;
    }
  },
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

vi.mock('$lib/serverCatalogue', () => ({
  firstAuthenticatedServerId: vi.fn((excludedId: string) => {
    const originId = mocks.originServer?.id;
    if (originId && originId !== excludedId && mocks.authenticated[originId]) return originId;
    return mocks.servers.find(
      (server) => server.id !== excludedId && mocks.authenticated[server.id]
    )?.id;
  })
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
  goto: mocks.goto
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

vi.mock('$lib/ui/toast', () => ({
  toast: {
    success: mocks.toastSuccess,
    error: mocks.toastError
  }
}));

vi.mock('$lib/storage/lastRoom', () => ({
  clearLastRoom: mocks.clearLastRoom
}));

vi.mock('$lib/notifications/pushNotifications', async (importOriginal) => ({
  ...(await importOriginal<typeof import('$lib/notifications/pushNotifications')>()),
  unsubscribeBeforeLeaving: mocks.unsubscribePushBeforeLeaving
}));

vi.mock('@chatto/client/api/messages', () => ({
  createMessageAPI: () => ({
    deleteMessage: mocks.deleteMessage,
    deleteAttachment: mocks.deleteAttachment,
    deleteLinkPreview: mocks.deleteLinkPreview
  })
}));

vi.mock('@chatto/client/api/rooms', () => ({
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

vi.mock('$lib/ui', async () => {
  const [{ default: ConfirmDialog }, { default: Dialog }] = await Promise.all([
    import('./ModalContainerConfirmDialogMock.svelte'),
    import('./ModalContainerDialogMock.svelte')
  ]);
  return {
    MarkdownHtml: (await import('$lib/ui/MarkdownHtml.svelte')).default,
    ConfirmDialog,
    Dialog
  };
});

vi.mock('$lib/ui/form', async () => {
  const { default: ButtonMock } = await import('./ModalContainerButtonMock.svelte');
  return { Button: ButtonMock };
});

vi.mock('./modals/EditAttachmentDescriptionModal.svelte', async () => {
  const { default: DialogMock } = await import('./ModalContainerDialogMock.svelte');
  return { default: DialogMock };
});

import ModalContainer from './ModalContainer.svelte';

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
    type: 'aboutChatto'
  };
  mocks.leaveRoom.mockResolvedValue(undefined);
  mocks.leaveRoomDirectory = [];
  mocks.deleteMessage.mockResolvedValue(true);
  mocks.deleteAttachment.mockResolvedValue(true);
  mocks.deleteLinkPreview.mockResolvedValue(true);
  mocks.tryGetStore.mockReturnValue({
    roomList: {
      get rooms() {
        return mocks.leaveRoomDirectory;
      }
    },
    applyLocalMessageMutation: mocks.applyLocalMessageMutation
  });
  mocks.mutation.mockReturnValue({
    toPromise: () => Promise.resolve({ data: {}, error: null })
  });
  mocks.activeServer = 'origin';
  mocks.unsubscribePushBeforeLeaving.mockResolvedValue(undefined);
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

describe('ModalContainer recommendation info', () => {
  it('shows every cached recommendation source', () => {
    mocks.modal = {
      type: 'serverRecommendation',
      name: 'Community',
      origin: 'https://community.example',
      sources: [
        { name: 'First server', origin: 'https://first.example' },
        { name: 'Second server', origin: 'https://second.example' },
        { name: 'Third server', origin: 'https://third.example' }
      ]
    };
    const { container } = render(ModalContainer);
    expect(container.querySelector('dialog')?.getAttribute('aria-label')).toBe('About Community');
    for (const host of ['first.example', 'second.example', 'third.example'])
      expect(container.textContent).toContain(host);
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
    mocks.unsubscribePushBeforeLeaving.mockResolvedValueOnce(undefined);

    const { container } = render(ModalContainer);
    await expect
      .element(q(container, '[href="/chat/remote.example.test/settings/account"]'))
      .toHaveTextContent('Account Settings');
    expect(container.textContent).toContain(
      'Your account and data on the server will not be deleted.'
    );
    const removeButton = findButton(container, 'Remove Server');
    removeButton.click();
    removeButton.click();

    await vi.waitFor(() => {
      expect(mocks.clearLastRoom).toHaveBeenCalledWith('remote');
      expect(mocks.removeServer).toHaveBeenCalledWith('remote');
      expect(window.history.back).toHaveBeenCalledOnce();
    });
    expect(mocks.goto).not.toHaveBeenCalled();
    expect(mocks.unsubscribePushBeforeLeaving).toHaveBeenCalledWith('remote');
    expect(mocks.unsubscribePushBeforeLeaving).toHaveBeenCalledOnce();
    expect(mocks.removeServer).toHaveBeenCalledOnce();
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

  it('keeps a server registered when push cleanup cannot establish a delivery fence', async () => {
    const remote = {
      id: 'remote',
      url: 'https://remote.example.test',
      name: 'Remote',
      token: 'token'
    };
    mocks.servers = [mocks.originServer!, remote];
    mocks.modal = { type: 'removeServer', serverId: 'remote', spaceName: 'Remote' };
    mocks.unsubscribePushBeforeLeaving.mockRejectedValueOnce(new Error('push cleanup unavailable'));

    const { container } = render(ModalContainer);
    clickButton(container, 'Remove Server');

    await vi.waitFor(() => {
      expect(mocks.toastError).toHaveBeenCalledWith('Network error. Please try again.');
      expect(findButton(container, 'Remove Server').hasAttribute('aria-busy')).toBe(false);
    });
    expect(mocks.clearLastRoom).not.toHaveBeenCalled();
    expect(mocks.removeServer).not.toHaveBeenCalled();
    expect(mocks.goto).not.toHaveBeenCalled();
  });
});

describe('ModalContainer leave room modal', () => {
  it.each([true, false, undefined])('explains rejoining with permission=%s', async (canRejoin) => {
    mocks.modal = { type: 'leaveRoom', serverId: 'remote', roomId: 'room-1', roomName: 'General' };
    mocks.leaveRoomDirectory = [{ id: 'room-1', viewerCanRejoin: canRejoin }];
    const { container } = render(ModalContainer);
    await vi.waitFor(() =>
      expect(container.textContent).toContain(
        canRejoin === undefined
          ? 'You may need a new invitation to rejoin.'
          : canRejoin
            ? 'You can rejoin later.'
            : 'You will need a new invitation to join this room.'
      )
    );
    expect(mocks.leaveRoom).not.toHaveBeenCalled();
  });

  it('leaves the room and returns to the modal server', async () => {
    mocks.modal = {
      type: 'leaveRoom',
      serverId: 'remote',
      roomId: 'room-1',
      roomName: 'General'
    };

    const { container } = render(ModalContainer);
    clickButton(container, 'Leave room');

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

  it('applies a message deletion to the server store after it succeeds', async () => {
    mocks.modal = {
      type: 'deleteMessage',
      serverId: 'remote',
      roomId: 'room-1',
      eventId: 'event-1'
    };

    const { container } = render(ModalContainer);
    clickButton(container, 'Delete');

    await vi.waitFor(() => {
      expect(mocks.deleteMessage).toHaveBeenCalledWith('room-1', 'event-1');
      expect(mocks.getClient).toHaveBeenCalledWith('remote');
      expect(mocks.applyLocalMessageMutation).toHaveBeenCalledOnce();
    });
    expect(mocks.tryGetStore).toHaveBeenCalledWith('remote');
    expect(mocks.applyLocalMessageMutation).toHaveBeenCalledWith(
      'room-1',
      'event-1',
      'message-deleted'
    );
    expect(mocks.toastSuccess).toHaveBeenCalledOnce();
  });

  it('applies an attachment deletion to the server store after it succeeds', async () => {
    mocks.modal = {
      type: 'deleteAttachment',
      serverId: 'remote',
      roomId: 'room-1',
      eventId: 'event-1',
      attachmentId: 'attachment-1'
    };

    const { container } = render(ModalContainer);
    clickButton(container, 'Delete');

    await vi.waitFor(() => {
      expect(mocks.deleteAttachment).toHaveBeenCalledWith('room-1', 'event-1', 'attachment-1');
      expect(mocks.getClient).toHaveBeenCalledWith('remote');
      expect(mocks.applyLocalMessageMutation).toHaveBeenCalledOnce();
    });
    expect(mocks.tryGetStore).toHaveBeenCalledWith('remote');
    expect(mocks.applyLocalMessageMutation).toHaveBeenCalledWith(
      'room-1',
      'event-1',
      'attachment-deleted'
    );
  });

  it('applies a link preview deletion to the server store after it succeeds', async () => {
    mocks.modal = {
      type: 'deleteLinkPreview',
      serverId: 'remote',
      roomId: 'room-1',
      eventId: 'event-1',
      previewUrl: 'https://example.test/article'
    };

    const { container } = render(ModalContainer);
    clickButton(container, 'Delete');

    await vi.waitFor(() => {
      expect(mocks.deleteLinkPreview).toHaveBeenCalledWith(
        'room-1',
        'event-1',
        'https://example.test/article'
      );
      expect(mocks.getClient).toHaveBeenCalledWith('remote');
      expect(mocks.applyLocalMessageMutation).toHaveBeenCalledOnce();
    });
    expect(mocks.tryGetStore).toHaveBeenCalledWith('remote');
    expect(mocks.applyLocalMessageMutation).toHaveBeenCalledWith(
      'room-1',
      'event-1',
      'link-preview-deleted'
    );
  });

  it('does not apply a local mutation when the deletion fails', async () => {
    const consoleError = vi.spyOn(console, 'error').mockImplementation(() => {});
    mocks.deleteMessage.mockRejectedValue(new Error('delete failed'));
    mocks.modal = {
      type: 'deleteMessage',
      serverId: 'remote',
      roomId: 'room-1',
      eventId: 'event-1'
    };

    try {
      const { container } = render(ModalContainer);
      clickButton(container, 'Delete');

      await vi.waitFor(() => expect(mocks.toastError).toHaveBeenCalledOnce());
      expect(mocks.applyLocalMessageMutation).not.toHaveBeenCalled();
    } finally {
      consoleError.mockRestore();
    }
  });
});
