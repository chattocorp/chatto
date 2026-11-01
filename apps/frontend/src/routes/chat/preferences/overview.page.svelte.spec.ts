import { beforeEach, describe, expect, it } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { desktopUpdates } from '$lib/desktop/updates.svelte';
import Overview from './+page.svelte';

describe('signed-out settings overview', () => {
  beforeEach(() => {
    desktopUpdates.status = null;
  });

  it('shows app preference cards without a server scope', async () => {
    const view = render(Overview);

    await expect.element(view.getByRole('heading', { name: 'Overview' })).toBeVisible();
    expect(view.container.querySelectorAll('h2')).toHaveLength(1);
    expect(view.container.querySelectorAll('nav a')).toHaveLength(3);
    for (const [name, path] of [
      ['Appearance', 'appearance'],
      ['Language', 'language'],
      ['Composer', 'composer']
    ]) {
      const link = view.getByRole('link', { name, exact: true });
      await expect.element(link).toHaveAttribute('href', `/chat/preferences/${path}`);
      const descriptionId = link.element().getAttribute('aria-describedby')!;
      expect(document.getElementById(descriptionId)?.textContent?.trim()).toBeTruthy();
    }
    await expect
      .element(view.getByRole('link', { name: 'Account', exact: true }))
      .not.toBeInTheDocument();
    await expect
      .element(view.getByRole('link', { name: 'Profile', exact: true }))
      .not.toBeInTheDocument();
    await expect
      .element(view.getByRole('link', { name: 'Voice', exact: true }))
      .not.toBeInTheDocument();
  });

  it('includes Updates when the desktop capability is available', async () => {
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
    const view = render(Overview);
    await expect
      .element(view.getByRole('link', { name: 'Updates' }))
      .toHaveAttribute('href', '/chat/preferences/desktop');
  });
});
