import '../../../app.css';
import { afterEach, describe, expect, it } from 'vitest';
import { page } from 'vitest/browser';
import { render } from 'vitest-browser-svelte';
import { isOverviewCardDismissed } from '$lib/storage/overviewDismissals';
import ServerOverviewWelcome from './ServerOverviewWelcome.svelte';
import ServerOverviewSetup from './ServerOverviewSetup.svelte';

afterEach(() => localStorage.clear());

describe('ServerOverviewWelcome', () => {
  it('renders the welcome message as Markdown', async () => {
    render(ServerOverviewWelcome, {
      welcomeMessage: 'Say hi in **introductions**'
    });

    await expect.element(page.getByText('introductions', { exact: true })).toBeVisible();
  });

  it('keeps a long welcome message in a limited scroll area', async () => {
    render(ServerOverviewWelcome, { welcomeMessage: 'Line\n\n'.repeat(60) + 'Last line' });

    const scroll = page.getByTestId('server-overview-welcome-scroll');
    await expect.element(scroll).toBeInTheDocument();
    // ScrollFader puts the size class on its viewport, which may carry the test ID itself.
    const root = scroll.element() as HTMLElement;
    const viewport =
      root.closest('.max-h-64') ??
      root.querySelector('.max-h-64') ??
      document.querySelector('.max-h-64');
    if (!(viewport instanceof HTMLElement)) throw new Error('no scroll viewport');
    await expect.poll(() => viewport.scrollHeight > viewport.clientHeight).toBe(true);
    expect(page.getByRole('button', { name: /hide/i }).elements()).toHaveLength(0);
  });

  it('renders nothing without a welcome message', () => {
    const { container } = render(ServerOverviewWelcome, {
      welcomeMessage: '  '
    });
    expect(container.querySelector('[data-testid="server-overview-welcome"]')).toBeNull();
  });
});

describe('ServerOverviewSetup', () => {
  it('lists the missing profile fields and links to the General settings', async () => {
    render(ServerOverviewSetup, {
      serverId: 'origin',
      serverSegment: '-',
      missing: ['logo', 'welcome_message']
    });

    await expect.element(page.getByText('Still missing: logo and welcome message.')).toBeVisible();
    await expect
      .element(page.getByRole('link', { name: 'Edit server profile' }))
      .toHaveAttribute('href', expect.stringContaining('/chat/-/manage/server/general'));
  });

  it('hides the reminder on this device', async () => {
    render(ServerOverviewSetup, { serverId: 'origin', serverSegment: '-', missing: ['logo'] });

    await page.getByRole('button', { name: 'Hide setup reminder' }).click();

    await expect.element(page.getByTestId('server-overview-setup')).not.toBeInTheDocument();
    expect(isOverviewCardDismissed('origin', 'setup')).toBe(true);
  });

  it('renders nothing for a complete profile', () => {
    const { container } = render(ServerOverviewSetup, {
      serverId: 'origin',
      serverSegment: '-',
      missing: []
    });
    expect(container.querySelector('[data-testid="server-overview-setup"]')).toBeNull();
  });
});
