import { tick } from 'svelte';
import { SvelteMap } from 'svelte/reactivity';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { testSnippet } from '$lib/test-utils';
import { RealtimeProjectionSyncState } from '$lib/state/server/realtimeSync.svelte';

const mocks = vi.hoisted(() => ({
  state: null as SvelteMap<string, boolean> | null,
  sync: null as RealtimeProjectionSyncState | null
}));

vi.mock('$app/state', () => ({
  page: { url: new URL('https://example.test/chat/origin/manage/server/permissions') }
}));
vi.mock('$app/paths', () => ({
  resolve: (path: string) => path.replace('[serverId]', 'origin')
}));
vi.mock('$lib/navigation', () => ({ serverIdToSegment: () => 'origin' }));
vi.mock('$lib/state/server/scope.svelte', () => ({
  useServerScope: () => ({
    serverId: 'origin',
    store: {
      get realtimeSync() {
        return mocks.sync;
      },
      get permissions() {
        return { loaded: mocks.state!.get('loaded') };
      }
    }
  })
}));
vi.mock('$lib/state/server/chromePermissions.svelte', () => ({
  getChromePermissions: () => () =>
    mocks.state!.get('loaded') ? { canManageRoles: mocks.state!.get('allowed') } : null
}));

import Layout from './+layout.svelte';

beforeEach(() => {
  mocks.state = new SvelteMap([
    ['loaded', true],
    ['allowed', true]
  ]);
  mocks.sync = new RealtimeProjectionSyncState();
  mocks.sync.markCaughtUp('initial');
});

describe('management route admission', () => {
  it('retains an admitted page through refresh and removes it on confirmed access loss', async () => {
    const { container } = render(Layout, {
      props: { children: testSnippet('<input data-testid="filter" />') }
    });
    await tick();
    const filter = container.querySelector<HTMLInputElement>('[data-testid="filter"]')!;
    filter.value = 'message';
    mocks.sync!.markStale();
    await tick();
    expect(container.querySelector('[data-testid="filter"]')).toBe(filter);
    mocks.state!.set('loaded', true);
    mocks.sync!.markCaughtUp('refreshed');
    await tick();
    expect(container.querySelector('[data-testid="filter"]')).toBe(filter);
    expect(filter.value).toBe('message');

    mocks.sync!.markStale();
    await tick();
    mocks.state!.set('allowed', false);
    mocks.state!.set('loaded', true);
    mocks.sync!.markCaughtUp('revoked');
    await tick();
    expect(container.querySelector('[data-testid="filter"]')).toBeNull();
    await expect.element(container).toHaveTextContent('Access Denied');
  });

  it('does not admit private content while initial permissions are unknown', async () => {
    mocks.state!.set('loaded', false);
    const { container } = render(Layout, {
      props: { children: testSnippet('<input data-testid="filter" />') }
    });
    await tick();
    expect(container.querySelector('[data-testid="filter"]')).toBeNull();
  });
});
