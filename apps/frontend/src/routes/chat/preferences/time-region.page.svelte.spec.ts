import { beforeEach, describe, expect, it } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { flushSync } from 'svelte';
import { userPreferences } from '$lib/state/userPreferences.svelte';
import TimeRegionPage from './time-region/+page.svelte';

async function settle() {
  await Promise.resolve();
  await Promise.resolve();
  flushSync();
}

describe('App Preferences time & region page', () => {
  beforeEach(() => {
    localStorage.clear();
    userPreferences.timeZone = '';
    userPreferences.clockFormat = 'device';
  });

  it('identifies its app-wide scope without server-specific settings', async () => {
    const { container } = render(TimeRegionPage);
    await settle();

    expect(container.textContent).toContain('Time & region');
    expect(container.textContent).toContain('This choice applies to every Chatto server');
    expect(container.textContent).not.toContain('Save');
  });

  it('persists the clock format choice immediately for this app', async () => {
    const { getByRole } = render(TimeRegionPage);
    await settle();

    await getByRole('radio', { name: /^24-hour/ }).click();
    await settle();

    expect(userPreferences.clockFormat).toBe('24h');
    expect(JSON.parse(localStorage.getItem('chatto:preferences') ?? '{}')).toMatchObject({
      clockFormat: '24h'
    });
  });
});
