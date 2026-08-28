import { beforeEach, describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import AppPreferencesLayout from './+layout.svelte';

const mocks = vi.hoisted(() => ({
  pathname: '/chat/preferences',
  authenticatedServerId: undefined as string | undefined,
  goto: vi.fn()
}));

vi.mock('$app/state', () => ({
  page: {
    get url() {
      return new URL(`https://chatto.test${mocks.pathname}`);
    }
  }
}));
vi.mock('$app/environment', () => ({ browser: true, version: '' }));
vi.mock('$app/navigation', () => ({ goto: mocks.goto, pushState: vi.fn() }));
vi.mock('$lib/state/activeServer.svelte', () => ({ getActiveServer: () => '' }));
vi.mock('$lib/state/server/registry.svelte', () => ({
  serverRegistry: {
    isAuthenticated: () => false,
    firstAuthenticatedServerId: () => mocks.authenticatedServerId,
    isOriginServer: () => false,
    getServer: (id: string) => ({ id, url: `https://${id}.example.com` })
  }
}));

describe('App Preferences layout', () => {
  beforeEach(() => {
    mocks.pathname = '/chat/preferences';
    mocks.authenticatedServerId = undefined;
    mocks.goto.mockReset();
  });

  it('uses the shared sidebar shell with section navigation', async () => {
    const { container, getByRole } = render(AppPreferencesLayout);

    expect(container.querySelector('[data-testid="server-sidebar"]')).not.toBeNull();
    expect(container.querySelector('[data-testid="current-user-identity-card"]')).toBeNull();
    await expect.element(getByRole('heading', { name: 'App Preferences' })).toBeVisible();
    await expect
      .element(getByRole('link', { name: 'Appearance' }))
      .toHaveAttribute('aria-current', 'page');
    await expect.element(getByRole('link', { name: 'Language' })).toBeVisible();
    await expect.element(getByRole('link', { name: 'Composer' })).toBeVisible();
  });

  it.each([
    ['/chat/preferences', '/chat/remote.example.com/settings/appearance'],
    ['/chat/preferences/language', '/chat/remote.example.com/settings/language'],
    ['/chat/preferences/composer', '/chat/remote.example.com/settings/composer']
  ])('replace-navigates %s to the matching unified page', async (pathname, destination) => {
    mocks.pathname = pathname;
    mocks.authenticatedServerId = 'remote';

    const { container } = render(AppPreferencesLayout);

    await expect.poll(() => mocks.goto.mock.calls.length).toBe(1);
    expect(mocks.goto).toHaveBeenCalledWith(destination, { replaceState: true });
    expect(container.querySelector('[data-testid="server-sidebar"]')).toBeNull();
  });
});
