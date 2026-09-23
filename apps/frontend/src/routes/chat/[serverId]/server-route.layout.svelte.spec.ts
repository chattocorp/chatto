import { SvelteMap } from 'svelte/reactivity';
import { tick } from 'svelte';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { page, userEvent } from 'vitest/browser';
import { testSnippet } from '$lib/test-utils';
import { RealtimeProjectionSyncState } from '$lib/state/server/realtimeSync.svelte';

type RegisteredState = {
  reauthRequiredAt: number | null;
  checkingPermissions?: boolean;
  userId?: string;
  connectionStatus?: 'connected' | 'disconnected';
};

const { mocks } = vi.hoisted(() => ({
  mocks: {
    goto: vi.fn(),
    servers: null as SvelteMap<string, RegisteredState> | null,
    store: {
      get checkingPermissions(): boolean {
        return mocks.servers?.get('origin')?.checkingPermissions ?? false;
      },
      realtimeSync: null as RealtimeProjectionSyncState | null,
      savedView: null as null | {
        version: 1; serverId: string; userId: string; serverName: string; savedAt: number;
        rooms: Array<{ id: string; name: string; messages: Array<{ id: string; createdAt: string; author: string; body: string }> }>;
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
    params: { serverId: '-', roomId: 'room' },
    url: new URL('https://chat.example.test/chat/-/manage/server/members')
  }
}));

vi.mock('$lib/auth/returnNavigation', () => ({
  saveReturnUrl: vi.fn()
}));

vi.mock('$lib/state/activeServer.svelte', () => ({
  getActiveServer: () => 'origin'
}));

vi.mock('$lib/state/server/registry.svelte', () => ({
  serverRegistry: {
    originProbed: true,
    originServer: { id: 'origin' },
    tryGetStore: () => mocks.store,
    getStore: () => mocks.store,
    isOriginServer: (serverId: string) => serverId === 'origin',
    getServer: (serverId: string) => mocks.servers?.get(serverId)
  }
}));

vi.mock('$lib/state/server/serverConnection.svelte', () => ({
  serverConnectionManager: {
    getClient: () => ({
      queryScope: 'layout-test',
      get status() { return mocks.servers?.get('origin')?.connectionStatus ?? 'connected'; }
    })
  }
}));

vi.mock('$lib/state/server/scope.svelte', () => ({
  provideServerScope: vi.fn()
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
  mocks.servers = new SvelteMap([['origin', { reauthRequiredAt: null, userId: 'viewer-1' }]]);
  mocks.store.realtimeSync = new RealtimeProjectionSyncState();
  mocks.store.savedView = null;
  mocks.store.realtimeSync.markCaughtUp('ready');
});

describe('server route authentication privacy', () => {
  it('keeps the normal chat view and its draft visible during snapshot replacement', async () => {
    const savedView = {
      version: 1 as const,
      serverId: 'origin',
      userId: 'viewer-1',
      serverName: 'Home',
      savedAt: Date.now(),
      rooms: [{ id: 'room', name: 'Room', messages: [{ id: 'm', createdAt: '', author: 'Member', body: 'Earlier message' }] }]
    };
    mocks.store.savedView = savedView;
    const { container } = render(Layout, {
      props: {
        children: testSnippet('<input data-testid="draft" />')
      }
    });
    const draft = container.querySelector<HTMLInputElement>('[data-testid="draft"]')!;
    draft.value = 'Unsent draft';
    mocks.store.realtimeSync!.acceptProjectionEvent(undefined, true);
    await tick();
    expect(container.querySelector('[data-testid="saved-view-overlay"]')).toBeNull();
    expect(container.querySelector('[data-testid="draft"]')).toBe(draft);
    expect(getComputedStyle(draft).visibility).toBe('visible');
    expect(draft.closest('[inert]')).toBeNull();
    mocks.store.realtimeSync!.markCaughtUp('replacement');
    await tick();
    expect(container.querySelector('[data-testid="saved-view-overlay"]')).toBeNull();
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
    const savedView = {
      version: 1 as const,
      serverId: 'origin',
      userId: 'viewer-1',
      serverName: 'Home',
      savedAt: Date.now(),
      rooms: [{ id: 'room', name: 'Room', messages: [] }]
    };
    mocks.store.savedView = savedView;
    const { container } = render(Layout, {
      props: { children: testSnippet('<main data-testid="normal-chat">Chat</main>') }
    });
    mocks.servers!.set('origin', { reauthRequiredAt: null, userId: 'viewer-1', connectionStatus: 'disconnected' });
    await tick();
    const chat = container.querySelector('[data-testid="normal-chat"]');
    expect(chat).not.toBeNull();
    expect(getComputedStyle(chat!).visibility).toBe('visible');
    expect(chat!.closest('[inert]')).toBeNull();
    expect(container.querySelector('[role="status"]')).toBeNull();
    expect(container.querySelector('[data-testid="saved-view-overlay"]')).toBeNull();
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
    expect(container.querySelector('[data-testid="saved-view-overlay"]')).toBeNull();
  });
});
