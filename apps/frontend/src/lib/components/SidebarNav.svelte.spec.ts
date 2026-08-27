import { beforeEach, describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import SidebarNav from './SidebarNav.svelte';

const mocks = vi.hoisted(() => ({ pathname: '/settings/preferences' }));

vi.mock('$app/state', () => ({
  page: {
    get url() {
      return new URL(`https://chatto.test${mocks.pathname}`);
    }
  }
}));

describe('SidebarNav', () => {
  beforeEach(() => {
    mocks.pathname = '/settings/preferences';
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
              { href: '/settings/app', label: 'Appearance', icon: 'icon-appearance' },
              { href: '/settings/language', label: 'Language', icon: 'icon-language' },
              { href: '/settings/composer', label: 'Composer', icon: 'icon-composer' }
            ]
          },
          {
            label: 'Your account',
            persistKey: 'test:sidebar-nav:your-account',
            items: [
              { href: '/settings', label: 'Profile', icon: 'icon-profile' },
              { href: '/settings/preferences', label: 'Time & region', icon: 'icon-time' }
            ]
          },
          {
            label: 'Server Configuration',
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
    expect(groups[2]?.textContent).toContain('Server Configuration');
    expect(container.querySelector('nav a[href="/manage/general"]')).not.toBeNull();
    await expect.element(getByText('App preferences')).toBeVisible();
    await expect.element(getByText('Appearance')).toBeVisible();
    await expect.element(getByText('Language')).toBeVisible();
    await expect.element(getByText('Composer')).toBeVisible();
    await expect
      .element(container.querySelector<HTMLElement>('a[href="/settings/preferences"]'))
      .toHaveAttribute('aria-current', 'page');

    const toggle = container.querySelector<HTMLButtonElement>('button[aria-expanded]');
    await expect.element(toggle).toHaveAttribute('aria-expanded', 'true');
    toggle?.click();
    await expect.element(toggle).toHaveAttribute('aria-expanded', 'false');
  });
});
