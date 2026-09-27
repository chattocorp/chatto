import { tick } from 'svelte';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { testSnippet } from '$lib/test-utils';
import { RealtimeProjectionSyncState } from '$lib/state/server/realtimeSync.svelte';
import { createTestServerScope, type TestServerScope } from '$lib/test-utils/serverScope.svelte';

vi.mock('$app/state', () => ({
  page: { url: new URL('https://example.test/chat/origin/manage/server/permissions') }
}));
vi.mock('$app/paths', () => ({
  resolve: (path: string) => path.replace('[serverId]', 'origin')
}));
vi.mock('$lib/navigation', () => ({ serverIdToSegment: () => 'origin' }));
vi.mock(
  '$lib/state/server/scope.svelte',
  async () => (await import('$lib/test-utils/serverScope.svelte')).serverScopeModule
);

import Layout from './+layout.svelte';

let server: TestServerScope;
let sync: RealtimeProjectionSyncState;

beforeEach(() => {
  sync = new RealtimeProjectionSyncState();
  sync.markCaughtUp('initial');
  server = createTestServerScope({
    serverId: 'origin',
    permissions: { canAdminManageRoles: true },
    store: { realtimeSync: sync }
  });
});

describe('management route admission', () => {
  it('retains an admitted page through refresh and removes it on confirmed access loss', async () => {
    const { container } = render(Layout, {
      props: { children: testSnippet('<input data-testid="filter" />') }
    });
    await tick();
    const filter = container.querySelector<HTMLInputElement>('[data-testid="filter"]')!;
    filter.value = 'message';
    sync.markStale();
    await tick();
    expect(container.querySelector('[data-testid="filter"]')).toBe(filter);
    server.permissions.loaded = true;
    sync.markCaughtUp('refreshed');
    await tick();
    expect(container.querySelector('[data-testid="filter"]')).toBe(filter);
    expect(filter.value).toBe('message');

    sync.markStale();
    await tick();
    server.permissions.canAdminManageRoles = false;
    server.permissions.loaded = true;
    sync.markCaughtUp('revoked');
    await tick();
    expect(container.querySelector('[data-testid="filter"]')).toBeNull();
    await expect.element(container).toHaveTextContent('Access Denied');
  });

  it('does not admit private content while initial permissions are unknown', async () => {
    server.permissions.loaded = false;
    const { container } = render(Layout, {
      props: { children: testSnippet('<input data-testid="filter" />') }
    });
    await tick();
    expect(container.querySelector('[data-testid="filter"]')).toBeNull();
  });
});
// Title composition has separate coverage; these fixtures model permissions only.
vi.mock('$lib/render/pageTitle', () => ({ formatPageTitle: () => 'Chatto' }));
