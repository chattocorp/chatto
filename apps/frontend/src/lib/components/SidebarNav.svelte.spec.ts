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
  });

  it('renders collapsible navigation groups and marks the active item', async () => {
    const { container, getByText } = render(SidebarNav, {
      props: {
        title: 'Settings',
        subtitle: 'Chatto Test',
        backHref: '/chat/test',
        groups: [
          {
            label: 'Server Preferences',
            items: [
              { href: '/settings', label: 'Profile', icon: 'icon-profile' },
              { href: '/settings/preferences', label: 'Time & region', icon: 'icon-time' }
            ]
          },
          {
            label: 'Server Settings',
            items: [{ href: '/manage/general', label: 'General', icon: 'icon-general' }]
          }
        ]
      }
    });

    expect(container.querySelectorAll('details')).toHaveLength(2);
    await expect.element(getByText('Server Preferences')).toBeVisible();
    await expect
      .element(container.querySelector<HTMLElement>('a[href="/settings/preferences"]'))
      .toHaveAttribute('aria-current', 'page');

    (container.querySelector('summary') as HTMLElement).click();
    expect((container.querySelector('details') as HTMLDetailsElement).open).toBe(false);
  });
});
