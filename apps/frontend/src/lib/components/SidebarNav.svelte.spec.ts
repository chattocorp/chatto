import { beforeEach, describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import SidebarNav from './SidebarNav.svelte';

const mocks = vi.hoisted(() => ({ pathname: '/settings/time' }));

vi.mock('$app/state', () => ({
  page: {
    get url() {
      return new URL(`https://chatto.test${mocks.pathname}`);
    }
  }
}));

describe('SidebarNav', () => {
  beforeEach(() => {
    mocks.pathname = '/settings/time';
    localStorage.clear();
  });

  it('renders collapsible navigation groups and marks the active item', async () => {
    const { container, getByText } = render(SidebarNav, {
      props: {
        title: 'Settings',
        subtitle: 'Chatto Test',
        backHref: '/chat/test',
        groups: [
          {
            label: 'App preferences',
            persistKey: 'test:sidebar-nav:app-preferences',
            items: [
              { href: '/settings/appearance', label: 'Appearance', icon: 'icon-appearance' },
              { href: '/settings/language', label: 'Language', icon: 'icon-language' },
              { href: '/settings/composer', label: 'Composer', icon: 'icon-composer' }
            ]
          },
          {
            label: 'Your account',
            persistKey: 'test:sidebar-nav:your-account',
            items: [
              { href: '/settings/account', label: 'Account', icon: 'icon-account' },
              { href: '/settings/profile', label: 'Profile', icon: 'icon-profile' },
              { href: '/settings/time', label: 'Time & region', icon: 'icon-time' }
            ]
          },
          {
            label: 'Server',
            persistKey: 'test:sidebar-nav:server-configuration',
            items: [{ href: '/manage/general', label: 'General', icon: 'icon-general' }]
          }
        ]
      }
    });

    const groups = container.querySelectorAll('[data-testid="room-group-section"]');
    expect(groups).toHaveLength(3);
    expect(groups[0]?.textContent).toContain('App preferences');
    expect(groups[1]?.textContent).toContain('Your account');
    expect(groups[2]?.textContent).toContain('Server');
    expect(container.querySelector('nav a[href="/manage/general"]')).not.toBeNull();
    await expect.element(getByText('App preferences')).toBeVisible();
    await expect.element(getByText('Appearance')).toBeVisible();
    await expect.element(getByText('Language')).toBeVisible();
    await expect.element(getByText('Composer')).toBeVisible();
    const activeItem = container.querySelector<HTMLElement>('a[href="/settings/time"]');
    await expect.element(activeItem).toHaveAttribute('aria-current', 'page');
    expect(activeItem?.classList.contains('sidebar-item')).toBe(true);
    expect(activeItem?.classList.contains('bg-surface')).toBe(false);

    const toggle = container.querySelector<HTMLButtonElement>('button[aria-expanded]');
    await expect.element(toggle).toHaveAttribute('aria-expanded', 'true');
    toggle?.click();
    await expect.element(toggle).toHaveAttribute('aria-expanded', 'false');
  });

  it.each([
    { pathname: '/settings/appearance', expectedHref: '/settings/appearance' },
    { pathname: '/settings/account', expectedHref: '/settings/account' },
    { pathname: '/settings/profile', expectedHref: '/settings/profile' }
  ])('marks only the most-specific item for $pathname', async ({ pathname, expectedHref }) => {
    mocks.pathname = pathname;

    const { container } = render(SidebarNav, {
      props: {
        title: 'Settings',
        items: [
          { href: '/settings/appearance', label: 'Appearance', icon: 'icon-appearance' },
          { href: '/settings/account', label: 'Account', icon: 'icon-account' },
          { href: '/settings/profile', label: 'Profile', icon: 'icon-profile' }
        ]
      }
    });

    const currentItems = Array.from(
      container.querySelectorAll<HTMLAnchorElement>('a[aria-current="page"]')
    );
    expect(currentItems.map((item) => item.getAttribute('href'))).toEqual([expectedHref]);
  });
});
