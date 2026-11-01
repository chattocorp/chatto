import { beforeEach, describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { createTestServerScope, type TestServerScope } from '$lib/test-utils/serverScope.svelte';
import { SvelteMap } from 'svelte/reactivity';

vi.mock('$lib/navigation', () => ({
  serverIdToSegment: (serverId: string) => serverId,
  segmentToServerId: (segment: string) => segment
}));

vi.mock(
  '$lib/state/server/scope.svelte',
  async () => (await import('$lib/test-utils/serverScope.svelte')).serverScopeModule
);
import Overview from './+page.svelte';

let scope: TestServerScope;
const mode = new SvelteMap<string, boolean>();

beforeEach(() => {
  mode.clear();
  scope = createTestServerScope({
    serverId: 'remote.example.com',
    store: {
      projection: {
        get viewer() {
          return mode.has('available')
            ? { privilegedMode: { available: mode.get('available'), active: mode.get('active') } }
            : undefined;
        }
      }
    }
  });
});

describe('settings Overview', () => {
  it('groups links and updates server pages when effective permissions change', async () => {
    const view = render(Overview);
    expect(view.container.querySelectorAll('h2')).toHaveLength(3);
    for (const link of view.container.querySelectorAll('nav a')) {
      const description = document.getElementById(link.getAttribute('aria-describedby')!);
      expect(description?.textContent?.trim()).toBeTruthy();
    }
    await expect
      .element(view.getByRole('link', { name: 'Appearance' }))
      .toHaveAttribute('href', '/chat/remote.example.com/settings/appearance');
    await expect
      .element(view.getByRole('link', { name: 'Account', exact: true }))
      .toHaveAttribute('href', '/chat/remote.example.com/settings/account');
    await expect.element(view.getByRole('link', { name: 'Bots', exact: true })).toBeVisible();
    await expect
      .element(view.getByRole('link', { name: 'General', exact: true }))
      .not.toBeInTheDocument();
    scope.permissions = { ...scope.permissions, canManageServer: true };
    await expect.element(view.getByRole('link', { name: 'General', exact: true })).toBeVisible();
    scope.permissions = { ...scope.permissions, canManageServer: false };
    await expect
      .element(view.getByRole('link', { name: 'General', exact: true }))
      .not.toBeInTheDocument();
  });

  it('shows the notice in Server only for available inactive Shield Mode, including state changes', async () => {
    const view = render(Overview);
    const notice = view.getByText(/To use your additional permissions/);
    await expect.element(notice).not.toBeInTheDocument();
    mode.set('available', false);
    await expect.element(notice).not.toBeInTheDocument();
    mode.set('available', true);
    await expect.element(notice).toBeVisible();
    await expect
      .element(view.getByRole('region', { name: 'Server', exact: true }))
      .toHaveTextContent('To use your additional permissions');
    mode.set('active', true);
    await expect.element(notice).not.toBeInTheDocument();
    mode.set('active', false);
    await expect.element(notice).toBeVisible();
    mode.set('available', false);
    await expect.element(notice).not.toBeInTheDocument();
  });

  it('keeps personal settings available while server permissions load', async () => {
    scope.permissions = { ...scope.permissions, loaded: false };
    const view = render(Overview);
    await expect.element(view.getByRole('link', { name: 'Appearance' })).toBeVisible();
    await expect
      .element(view.getByRole('heading', { name: 'Server', exact: true }))
      .not.toBeInTheDocument();
    scope.permissions = { ...scope.permissions, loaded: true };
    await expect.element(view.getByRole('link', { name: 'Bots', exact: true })).toBeVisible();
  });
});
