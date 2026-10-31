import { beforeAll } from 'vitest';
import { loadLocaleMessages } from '$lib/i18n/messages';

// Standalone management components do not run the route catalog loader.
beforeAll(() => loadLocaleMessages('en-GB'));

import { tick } from 'svelte';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { testSnippet } from '$lib/test-utils';
import { RealtimeProjectionSyncState } from '@chatto/client/server/realtimeSync';
import { createTestServerScope, type TestServerScope } from '$lib/test-utils/serverScope.svelte';

const { page } = vi.hoisted(() => ({
  page: { url: new URL('https://example.test/chat/origin/manage/server/permissions') }
}));
vi.mock('$app/state', () => ({ page }));
vi.mock('$app/paths', () => ({
  base: '',
  assets: '',
  resolve: (path: string) => path.replace('[serverId]', 'origin')
}));
vi.mock('$lib/navigation', () => ({
  segmentToServerId: vi.fn(),
  serverIdToSegment: () => 'origin'
}));
vi.mock(
  '$lib/state/server/scope.svelte',
  async () => (await import('$lib/test-utils/serverScope.svelte')).serverScopeModule
);

import Layout from './+layout.svelte';

let server: TestServerScope;
let sync: RealtimeProjectionSyncState;

beforeEach(() => {
  page.url = new URL('https://example.test/chat/origin/manage/server/permissions');
  sync = new RealtimeProjectionSyncState();
  sync.markCaughtUp('initial');
  server = createTestServerScope({
    serverId: 'origin',
    permissions: { canAdminManageRoles: true },
    store: { realtimeSync: sync }
  });
});

describe('management route admission', () => {
  it.each(['canManageServer'] as const)(
    'admits Registration with %s and denies it after access is removed',
    async (permission) => {
      page.url = new URL('https://example.test/chat/origin/manage/server/registration');
      server.permissions[permission] = true;
      const { container } = render(Layout, {
        props: { children: testSnippet('<p>Registration controls</p>') }
      });
      await tick();
      expect(container.textContent).toContain('Registration controls');
      server.permissions[permission] = false;
      await tick();
      expect(container.textContent).toContain('Access Denied');
    }
  );
  it.each(['canInviteUsers', 'canManageInvites'] as const)(
    'admits Invites, not Registration, with %s',
    async (permission) => {
      server.permissions[permission] = true;
      page.url = new URL('https://example.test/chat/origin/manage/server/invites');
      const { container, unmount } = render(Layout, {
        props: { children: testSnippet('<p>Invite controls</p>') }
      });
      await tick();
      expect(container.textContent).toContain('Invite controls');
      await unmount();
      page.url = new URL('https://example.test/chat/origin/manage/server/registration');
      const registration = render(Layout, {
        props: { children: testSnippet('<p>Registration controls</p>') }
      });
      await tick();
      expect(registration.container.textContent).toContain('Access Denied');
    }
  );
  it.each(['', '/moderator'])(
    'admits role assigners to the role list and roster: %s',
    async (suffix) => {
      page.url = new URL(`https://example.test/chat/origin/manage/server/permissions${suffix}`);
      server.permissions.canAdminManageRoles = false;
      server.permissions.canAssignRoles = true;
      const { container } = render(Layout, {
        props: { children: testSnippet('<p>Role roster</p>') }
      });
      await tick();
      expect(container.textContent).toContain('Role roster');
      server.permissions.canAssignRoles = false;
      await tick();
      expect(container.textContent).not.toContain('Role roster');
      expect(container.textContent).toContain('Access Denied');
    }
  );

  it.each(['/permissions/new', '/members'])(
    'keeps broader tools blocked for role assigners: %s',
    async (suffix) => {
      page.url = new URL(`https://example.test/chat/origin/manage/server${suffix}`);
      server.permissions.canAdminManageRoles = false;
      server.permissions.canAssignRoles = true;
      const { container } = render(Layout, {
        props: { children: testSnippet('<p>Private editor</p>') }
      });
      await tick();
      expect(container.textContent).not.toContain('Private editor');
      expect(container.textContent).toContain('Access Denied');
    }
  );

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
