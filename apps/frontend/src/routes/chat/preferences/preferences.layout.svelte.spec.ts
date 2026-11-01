import { beforeEach, describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import AppPreferencesLayout from './+layout.svelte';
import { desktopUpdates } from '$lib/desktop/updates.svelte';
import { resetRoomGroupCollapseForTests } from '$lib/components/chat/roomGroupCollapse';

const mocks = vi.hoisted(() => ({
  pathname: '/chat/preferences',
  authenticatedServerId: undefined as string | undefined,
  goto: vi.fn()
}));

vi.mock('$lib/client', async () => ({
  ...(await import('$lib/test-utils/clientMock')).clientMockDefaults,
  serverRegistry: {
    isAuthenticated: () => false,
    isOriginServer: () => false,
    getServer: (id: string) => ({ id, url: `https://${id}.example.com` })
  }
}));

vi.mock('$lib/serverCatalogue', () => ({
  firstAuthenticatedServerId: () => mocks.authenticatedServerId
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

describe('App Preferences layout', () => {
  beforeEach(() => {
    mocks.pathname = '/chat/preferences';
    mocks.authenticatedServerId = undefined;
    mocks.goto.mockReset();
    desktopUpdates.status = null;
    localStorage.clear();
    resetRoomGroupCollapseForTests();
  });

  it('uses the shared sidebar shell with section navigation', async () => {
    const { container, getByRole } = render(AppPreferencesLayout);

    expect(container.querySelector('[data-testid="server-sidebar"]')).not.toBeNull();
    expect(container.querySelector('[data-testid="current-user-identity-card"]')).toBeNull();
    await expect.element(getByRole('heading', { name: 'Settings' })).toBeVisible();
    expect(container.querySelectorAll('[data-testid="room-group-section"]')).toHaveLength(1);
    await expect
      .element(getByRole('button', { name: 'App preferences' }))
      .toHaveAttribute('aria-expanded', 'true');
    await expect
      .element(getByRole('link', { name: 'Overview' }))
      .toHaveAttribute('aria-current', 'page');
    await expect
      .element(getByRole('link', { name: 'Appearance' }))
      .toHaveAttribute('href', '/chat/preferences/appearance');
    await expect.element(getByRole('link', { name: 'Language' })).toBeVisible();
    await expect.element(getByRole('link', { name: 'Composer' })).toBeVisible();
  });

  it('collapses only app preferences and retains the choice across mounts', async () => {
    const view = render(AppPreferencesLayout);
    await view.getByRole('button', { name: 'App preferences' }).click();
    await expect
      .element(view.getByRole('button', { name: 'App preferences' }))
      .toHaveAttribute('aria-expanded', 'false');
    await expect.element(view.getByRole('link', { name: 'Appearance' })).not.toBeInTheDocument();
    await expect.element(view.getByRole('link', { name: 'Overview' })).toBeVisible();
    view.unmount();
    resetRoomGroupCollapseForTests();

    const nextView = render(AppPreferencesLayout);
    await expect
      .element(nextView.getByRole('button', { name: 'App preferences' }))
      .toHaveAttribute('aria-expanded', 'false');
    await nextView.getByRole('button', { name: 'App preferences' }).click();
    await expect.element(nextView.getByRole('link', { name: 'Appearance' })).toBeVisible();
  });

  it('marks only the selected preference as the current page', async () => {
    mocks.pathname = '/chat/preferences/appearance';
    const view = render(AppPreferencesLayout);
    await expect
      .element(view.getByRole('link', { name: 'Appearance' }))
      .toHaveAttribute('aria-current', 'page');
    await expect
      .element(view.getByRole('link', { name: 'Overview' }))
      .not.toHaveAttribute('aria-current');
  });

  it.each([
    ['/chat/preferences', '/chat/remote.example.com/settings/overview'],
    ['/chat/preferences/appearance', '/chat/remote.example.com/settings/appearance'],
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

  it('keeps Updates preferences independent of an authenticated server', async () => {
    mocks.pathname = '/chat/preferences/desktop';
    mocks.authenticatedServerId = 'remote';
    desktopUpdates.status = {
      revision: 0,
      version: '0.5.0',
      status: 'unavailable',
      busy: false,
      progress: null,
      availableVersion: null,
      automatic: false,
      channel: 'stable'
    };
    const { getByRole } = render(AppPreferencesLayout);
    await expect.element(getByRole('heading', { name: 'Settings' })).toBeVisible();
    await expect
      .element(getByRole('link', { name: 'Updates' }))
      .toHaveAttribute('href', '/chat/preferences/desktop');
    expect(mocks.goto).not.toHaveBeenCalled();
  });
});
