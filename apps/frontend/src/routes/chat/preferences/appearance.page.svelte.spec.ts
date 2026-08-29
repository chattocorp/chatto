import { beforeEach, describe, expect, it } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { flushSync } from 'svelte';
import { userPreferences } from '$lib/state/userPreferences.svelte';
import AppearancePage from './+page.svelte';

async function settle() {
  await Promise.resolve();
  await Promise.resolve();
  flushSync();
}

describe('App Preferences appearance page', () => {
  beforeEach(() => {
    localStorage.clear();
    userPreferences.displayTheme = 'system';
    userPreferences.threadPanePresentation = 'overlay';
  });

  it('identifies its app-wide scope without server-specific preferences', async () => {
    const { container } = render(AppearancePage);
    await settle();

    expect(container.querySelectorAll('.panel-shell')).toHaveLength(2);
    expect(container.textContent).toContain('Appearance');
    expect(container.textContent).toContain(
      'Choices for this app that apply across all your registered servers'
    );
    expect(container.textContent).not.toContain('Timezone');
    expect(container.textContent).toContain('Thread pane');
  });

  it('persists the theme choice immediately for this app', async () => {
    const { getByRole } = render(AppearancePage);
    await settle();

    await getByRole('radio', { name: /^Dark/ }).click();
    await settle();

    expect(userPreferences.displayTheme).toBe('dark');
    expect(JSON.parse(localStorage.getItem('chatto:preferences') ?? '{}')).toMatchObject({
      displayTheme: 'dark'
    });
  });

  it('persists the thread pane presentation immediately for this app', async () => {
    const { getByRole } = render(AppearancePage);
    await settle();

    expect(userPreferences.threadPanePresentation).toBe('overlay');
    await getByRole('radio', { name: /^Side by side/ }).click();
    await settle();

    expect(userPreferences.threadPanePresentation).toBe('split');
    expect(JSON.parse(localStorage.getItem('chatto:preferences') ?? '{}')).toMatchObject({
      threadPanePresentation: 'split'
    });
  });
});
