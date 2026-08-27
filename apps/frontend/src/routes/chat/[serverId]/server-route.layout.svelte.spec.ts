import { SvelteMap } from 'svelte/reactivity';
import { tick } from 'svelte';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { testSnippet } from '$lib/test-utils';

type RegisteredState = { reauthRequiredAt: number | null };

const { mocks } = vi.hoisted(() => ({
  mocks: {
    goto: vi.fn(),
    servers: null as SvelteMap<string, RegisteredState> | null,
    store: {
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
});

describe('server route authentication privacy', () => {
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
