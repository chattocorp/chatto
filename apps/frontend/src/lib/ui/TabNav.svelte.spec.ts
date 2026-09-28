import { describe, expect, it } from 'vitest';
import { page } from 'vitest/browser';
import { render } from 'vitest-browser-svelte';
import TabNav from './TabNav.svelte';

const items = [
  { href: '/bots/1', label: 'Overview', icon: 'icon-[uil--robot]', current: false },
  {
    href: '/bots/1/permissions',
    label: 'Permissions',
    icon: 'icon-[uil--shield-check]',
    current: true
  }
];

describe('TabNav', () => {
  it('renders a named navigation landmark with one link per section', async () => {
    render(TabNav, { props: { label: 'Bot sections', items } });

    const nav = page.getByRole('navigation', { name: 'Bot sections' });
    await expect.element(nav).toBeVisible();
    await expect
      .element(nav.getByRole('link', { name: 'Overview' }))
      .toHaveAttribute('href', '/bots/1');
    await expect
      .element(nav.getByRole('link', { name: 'Permissions' }))
      .toHaveAttribute('href', '/bots/1/permissions');
  });

  it('marks only the current section as the current page', async () => {
    render(TabNav, { props: { label: 'Bot sections', items } });

    await expect
      .element(page.getByRole('link', { name: 'Permissions' }))
      .toHaveAttribute('aria-current', 'page');
    await expect
      .element(page.getByRole('link', { name: 'Overview' }))
      .not.toHaveAttribute('aria-current');
  });

  it('renders nothing when only one section is available', async () => {
    const { container } = render(TabNav, {
      props: { label: 'Bot sections', items: items.slice(0, 1) }
    });

    expect(container.querySelector('nav')).toBeNull();
  });
});
