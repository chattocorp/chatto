import { SvelteMap } from 'svelte/reactivity';

// Title composition has separate coverage; these fixtures model route access only.
vi.mock('$lib/client', async () => ({
  ...(await import('$lib/test-utils/clientMock')).clientMockDefaults,
  serverRegistry: {
    originProbed: true,
    originServer: { id: 'origin' },
    tryGetStore: () => mocks.store,
    getStore: () => mocks.store,
    isOriginServer: (serverId: string) => serverId === 'origin',
    getServer: (serverId: string) => mocks.servers?.get(serverId),
    recoverServer: mocks.recoverServer
  },
  serverConnectionManager: {
    getClient: () => ({
      queryScope: 'layout-test',
      get status() {
        return mocks.servers?.get('origin')?.connectionStatus ?? 'connected';
      }
    })
  }
}));

vi.mock('$lib/render/pageTitle', () => ({ formatPageTitle: () => 'Chatto' }));
import { tick } from 'svelte';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { page, userEvent } from 'vitest/browser';
import { testSnippet } from '$lib/test-utils';
import { RealtimeProjectionSyncState } from '@chatto/client/server/realtimeSync';
import type { ServerCompatibilityProblem } from '@chatto/client/server/compatibility';

type RegisteredState = {
  reauthRequiredAt: number | null;
  token?: string | null;
  name?: string;
  url?: string;
  iconUrl?: string | null;
  checkingPermissions?: boolean;
  userId?: string;
  connectionStatus?: 'connected' | 'disconnected';
  compatibilityProblem?: ServerCompatibilityProblem;
};

const { mocks } = vi.hoisted(() => ({
  mocks: {
    goto: vi.fn(),
    recoverServer: vi.fn(),
    activeServerId: 'origin',
    routeId: '/chat/[serverId]/[roomId]',
    servers: null as SvelteMap<string, RegisteredState> | null,
    store: {
      get checkingPermissions(): boolean {
        return mocks.servers?.get('origin')?.checkingPermissions ?? false;
      },
      realtimeSync: null as RealtimeProjectionSyncState | null,
      serverInfo: {
        name: 'Old Server',
        version: '0.5.0-beta.7',
        iconUrl: null,
        get compatibilityProblem(): ServerCompatibilityProblem | null {
          return mocks.servers?.get('origin')?.compatibilityProblem ?? null;
        }
      },
      currentUser: {
        loading: false,
        user: { id: 'viewer-1' }
      }
    }
  }
}));

vi.mock('$app/environment', () => ({ browser: true, dev: true, building: false, version: 'test' }));

vi.mock('$app/navigation', () => ({
  goto: mocks.goto,
  pushState: vi.fn(),
  replaceState: vi.fn()
}));

vi.mock('$app/paths', () => ({
  resolve: (path: string) => path
}));

vi.mock('$app/state', () => ({
  page: {
    route: {
      get id() {
        return mocks.routeId;
      }
    },
    params: { serverId: '-', roomId: 'room' },
    url: new URL('https://chat.example.test/chat/-/manage/server/members')
  }
}));

vi.mock('$lib/auth/reauth', () => ({
  startRemoteReauthentication: vi.fn()
}));

vi.mock('$lib/auth/returnNavigation', () => ({
  saveReturnUrl: vi.fn()
}));

vi.mock('$lib/state/activeServer.svelte', () => ({
  getActiveServer: () => mocks.activeServerId
}));

vi.mock('$lib/state/server/scope.svelte', () => ({
  provideServerScope: vi.fn(),
  useServerScope: () => ({ serverId: 'origin', store: mocks.store })
}));

vi.mock('$lib/components/chat/Chrome.svelte', async () => {
  const { default: ChromeMock } = await import('./ServerLayoutChromeMock.svelte');
  return { default: ChromeMock };
});

vi.mock('$lib/components/ServerSidebar.svelte', async () => {
  const { default: SidebarMock } = await import('./ServerLayoutChromeMock.svelte');
  return { default: SidebarMock };
});

import Layout from './+layout.svelte';

beforeEach(() => {
  vi.clearAllMocks();
  mocks.routeId = '/chat/[serverId]/[roomId]';
  mocks.activeServerId = 'origin';
  mocks.servers = new SvelteMap([['origin', { reauthRequiredAt: null, userId: 'viewer-1' }]]);
  mocks.store.realtimeSync = new RealtimeProjectionSyncState();
  mocks.store.realtimeSync.markCaughtUp('ready');
});

describe('server route authentication privacy', () => {
  it('keeps the normal chat view and its draft visible during snapshot replacement', async () => {
    const { container } = render(Layout, {
      props: {
        children: testSnippet('<input data-testid="draft" />')
      }
    });
    const draft = container.querySelector<HTMLInputElement>('[data-testid="draft"]')!;
    draft.value = 'Unsent draft';
    mocks.store.realtimeSync!.acceptProjectionEvent(undefined, true);
    await tick();
    expect(container.querySelector('[data-testid="draft"]')).toBe(draft);
    expect(getComputedStyle(draft).visibility).toBe('visible');
    expect(draft.closest('[inert]')).toBeNull();
    mocks.store.realtimeSync!.markCaughtUp('replacement');
    await tick();
    expect(container.querySelector('[data-testid="draft"]')).toBe(draft);
    expect(draft.value).toBe('Unsent draft');
  });

  it('keeps the normal route visible and interactive throughout warm snapshot recovery', async () => {
    const { container } = render(Layout, {
      props: { children: testSnippet('<input data-testid="retained-filter" />') }
    });
    const input = container.querySelector<HTMLInputElement>('input')!;
    input.value = 'retained filter';
    mocks.store.realtimeSync!.acceptProjectionEvent(undefined, true);
    await tick();
    expect(container.querySelector('input')).toBe(input);
    expect(getComputedStyle(input).visibility).toBe('visible');
    expect(input.closest('[inert]')).toBeNull();
    mocks.store.realtimeSync!.markStale();
    mocks.store.realtimeSync!.acceptProjectionEvent(undefined, true);
    await tick();
    expect(getComputedStyle(input).visibility).toBe('visible');
    mocks.store.realtimeSync!.markCaughtUp('replacement');
    await tick();
    expect(container.querySelector('input')).toBe(input);
    expect(input.value).toBe('retained filter');
    expect(getComputedStyle(input).visibility).toBe('visible');
    expect(input.closest('[inert]')).toBeNull();
  });

  it('keeps the normal chat view visible during a transport disconnect', async () => {
    const { container } = render(Layout, {
      props: { children: testSnippet('<main data-testid="normal-chat">Chat</main>') }
    });
    mocks.servers!.set('origin', {
      reauthRequiredAt: null,
      userId: 'viewer-1',
      connectionStatus: 'disconnected'
    });
    await tick();
    const chat = container.querySelector('[data-testid="normal-chat"]');
    expect(chat).not.toBeNull();
    expect(getComputedStyle(chat!).visibility).toBe('visible');
    expect(chat!.closest('[inert]')).toBeNull();
    expect(container.querySelector('[role="status"]')).toBeNull();
  });
  it('keeps the page mounted, focused, and interactive during an authority check', async () => {
    const { container } = render(Layout, {
      props: { children: testSnippet('<input data-testid="filter" />') }
    });
    const filter = container.querySelector<HTMLInputElement>('[data-testid="filter"]')!;
    filter.value = 'message';
    filter.focus();
    mocks.servers!.set('origin', { reauthRequiredAt: null, checkingPermissions: true });
    await tick();
    expect(container.querySelector('[data-testid="filter"]')).toBe(filter);
    expect(filter.closest('[inert]')).toBeNull();
    expect(filter.closest('[aria-busy="true"]')).toBeNull();
    expect(getComputedStyle(filter).visibility).toBe('visible');
    expect(document.activeElement).toBe(filter);
    await userEvent.fill(page.getByTestId('filter'), 'room');
    expect(filter.value).toBe('room');
    mocks.servers!.set('origin', { reauthRequiredAt: null, checkingPermissions: false });
    await tick();
    expect(filter.closest('[inert]')).toBeNull();
    expect(container.querySelector('[data-testid="filter"]')).toBe(filter);
    expect(filter.value).toBe('room');
    expect(document.activeElement).toBe(filter);
  });

  it('hides private content throughout a reset and failed reconnect', async () => {
    const { container } = render(Layout, {
      props: { children: testSnippet('<main data-testid="private-route">Private data</main>') }
    });
    expect(container.querySelector('[data-testid="private-route"]')).not.toBeNull();
    mocks.store.realtimeSync!.reset();
    await tick();
    expect(container.querySelector('[data-testid="private-route"]')).toBeNull();
    mocks.store.realtimeSync!.beginCatchUp();
    mocks.store.realtimeSync!.markStale();
    await tick();
    expect(container.querySelector('[data-testid="private-route"]')).toBeNull();
    mocks.store.realtimeSync!.markCaughtUp('replacement');
    await tick();
    expect(container.querySelector('[data-testid="private-route"]')).not.toBeNull();
  });
  it('keeps the normal route visible when reauthentication becomes required', async () => {
    const { container } = render(Layout, {
      props: {
        children: testSnippet('<main data-testid="private-route">Private member data</main>')
      }
    });
    expect(container.querySelector('[data-testid="private-route"]')).not.toBeNull();

    mocks.servers!.set('origin', { reauthRequiredAt: Date.now() });
    await tick();

    expect(container.querySelector('[data-testid="server-chrome"]')).not.toBeNull();
    expect(container.querySelector('[data-testid="private-route"]')).not.toBeNull();
  });

  it('explains an unusable server instead of rendering its chrome and routes', async () => {
    mocks.servers!.set('origin', {
      reauthRequiredAt: null,
      userId: 'viewer-1',
      compatibilityProblem: 'server-too-old'
    });
    mocks.store.realtimeSync = new RealtimeProjectionSyncState();
    const { container } = render(Layout, {
      props: { children: testSnippet('<main data-testid="private-route">Rooms</main>') }
    });

    await expect.element(page.getByTestId('server-unavailable')).toBeVisible();
    expect(container.querySelector('[data-testid="server-chrome"]')).toBeNull();
    expect(container.querySelector('[data-testid="private-route"]')).toBeNull();

    await page.getByRole('button', { name: 'Check Again' }).click();
    expect(mocks.recoverServer).toHaveBeenCalledWith('origin');

    // A supported result after the retry shows the normal server chrome.
    mocks.servers!.set('origin', { reauthRequiredAt: null, userId: 'viewer-1' });
    await expect.element(page.getByTestId('server-chrome')).toBeInTheDocument();
    expect(container.querySelector('[data-testid="server-unavailable"]')).toBeNull();
  });
  it('explains a signed-out remote server instead of rendering its chrome and routes', async () => {
    mocks.activeServerId = 'remote';
    mocks.servers!.set('remote', {
      reauthRequiredAt: null,
      token: null,
      name: 'Remote Server',
      url: 'https://remote.example.test',
      iconUrl: null
    });
    const { container } = render(Layout, {
      props: { children: testSnippet('<main data-testid="private-route">Rooms</main>') }
    });

    await expect.element(page.getByTestId('server-signed-out')).toBeVisible();
    await expect.element(page.getByRole('button', { name: 'Log in to this server' })).toBeVisible();
    expect(container.querySelector('[data-testid="server-chrome"]')).toBeNull();
    expect(container.querySelector('[data-testid="private-route"]')).toBeNull();
  });

  it('keeps the chrome for a remote server that needs reauthentication over loaded data', async () => {
    mocks.activeServerId = 'remote';
    mocks.servers!.set('remote', {
      reauthRequiredAt: Date.now(),
      token: 'expired-token',
      name: 'Remote Server',
      url: 'https://remote.example.test',
      iconUrl: null
    });
    const { container } = render(Layout, {
      props: { children: testSnippet('<main data-testid="private-route">Rooms</main>') }
    });

    await expect.element(page.getByTestId('server-chrome')).toBeInTheDocument();
    expect(container.querySelector('[data-testid="server-signed-out"]')).toBeNull();
  });

  it('explains a remote server that needs reauthentication before any data loads', async () => {
    mocks.activeServerId = 'remote';
    mocks.servers!.set('remote', {
      reauthRequiredAt: Date.now(),
      token: 'rejected-token',
      name: 'Remote Server',
      url: 'https://remote.example.test',
      iconUrl: null
    });
    mocks.store.realtimeSync = new RealtimeProjectionSyncState();
    const { container } = render(Layout, {
      props: { children: testSnippet('<main data-testid="private-route">Rooms</main>') }
    });

    await expect.element(page.getByTestId('server-signed-out')).toBeVisible();
    expect(container.querySelector('[data-testid="server-chrome"]')).toBeNull();
  });
});
