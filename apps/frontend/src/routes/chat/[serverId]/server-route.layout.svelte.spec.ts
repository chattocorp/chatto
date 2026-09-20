import { SvelteMap } from 'svelte/reactivity';
import { tick } from 'svelte';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { page, userEvent } from 'vitest/browser';
import { testSnippet } from '$lib/test-utils';
import { RealtimeProjectionSyncState } from '$lib/state/server/realtimeSync.svelte';

type RegisteredState = { reauthRequiredAt: number | null; checkingPermissions?: boolean };

const { mocks } = vi.hoisted(() => ({
  mocks: {
    goto: vi.fn(),
    servers: null as SvelteMap<string, RegisteredState> | null,
    store: {
      get checkingPermissions(): boolean {
        return mocks.servers?.get('origin')?.checkingPermissions ?? false;
      },
      realtimeSync: null as RealtimeProjectionSyncState | null,
      currentUser: {
        loading: false,
        user: { id: 'viewer-1' }
      }
    }
  }
}));

vi.mock('$app/environment', () => ({ browser: true }));

vi.mock('$app/navigation', () => ({
  goto: mocks.goto
}));

vi.mock('$app/paths', () => ({
  resolve: (path: string) => path
}));

vi.mock('$app/state', () => ({
  page: {
    params: { serverId: '-' },
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
    getServer: (serverId: string) => mocks.servers?.get(serverId)
  }
}));

vi.mock('$lib/state/server/serverConnection.svelte', () => ({
  serverConnectionManager: {
    getClient: () => ({ queryScope: 'layout-test' })
  }
}));

vi.mock('$lib/state/server/scope.svelte', () => ({
  provideServerScope: vi.fn()
}));

vi.mock('$lib/components/chat/Chrome.svelte', async () => {
  const { default: ChromeMock } = await import('./ServerLayoutChromeMock.svelte');
  return { default: ChromeMock };
});

import Layout from './+layout.svelte';

beforeEach(() => {
  vi.clearAllMocks();
  mocks.servers = new SvelteMap([['origin', { reauthRequiredAt: null }]]);
  mocks.store.realtimeSync = new RealtimeProjectionSyncState();
  mocks.store.realtimeSync.markCaughtUp('ready');
});

describe('server route authentication privacy', () => {
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
  it('unmounts private route content when reauthentication becomes required', async () => {
    const { container } = render(Layout, {
      props: {
        children: testSnippet('<main data-testid="private-route">Private member data</main>')
      }
    });
    expect(container.querySelector('[data-testid="private-route"]')).not.toBeNull();

    mocks.servers!.set('origin', { reauthRequiredAt: Date.now() });
    await tick();

    expect(container.querySelector('[data-testid="server-chrome"]')).not.toBeNull();
    expect(container.querySelector('[data-testid="private-route"]')).toBeNull();
  });
});
