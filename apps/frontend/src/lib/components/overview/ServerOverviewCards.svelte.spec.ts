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
      serverId: 'origin',
      welcomeMessage: 'Say hi in **introductions**'
    });

    await expect.element(page.getByText('introductions', { exact: true })).toBeVisible();
  });

  it('hides the welcome message on this device', async () => {
    render(ServerOverviewWelcome, { serverId: 'origin', welcomeMessage: 'Hello' });

    await page.getByRole('button', { name: 'Hide welcome message' }).click();

    await expect.element(page.getByText('Hello')).not.toBeInTheDocument();
    expect(isOverviewCardDismissed('origin', 'welcome')).toBe(true);
    expect(isOverviewCardDismissed('other', 'welcome')).toBe(false);
  });

  it('renders nothing without a welcome message', () => {
    const { container } = render(ServerOverviewWelcome, {
      serverId: 'origin',
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
