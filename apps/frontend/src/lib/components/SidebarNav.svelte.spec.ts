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
            label: 'User Preferences',
            persistKey: 'test:sidebar-nav:user-preferences',
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

    expect(container.querySelectorAll('[data-testid="room-group-section"]')).toHaveLength(2);
    expect(container.querySelector('nav a[href="/manage/general"]')).not.toBeNull();
    await expect.element(getByText('User Preferences')).toBeVisible();
    await expect
      .element(container.querySelector<HTMLElement>('a[href="/settings/preferences"]'))
      .toHaveAttribute('aria-current', 'page');

    const toggle = container.querySelector<HTMLButtonElement>('button[aria-expanded]');
    await expect.element(toggle).toHaveAttribute('aria-expanded', 'true');
    toggle?.click();
    await expect.element(toggle).toHaveAttribute('aria-expanded', 'false');
  });
});
