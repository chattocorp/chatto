import { beforeEach, describe, expect, it } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { flushSync } from 'svelte';
import { userEvent } from 'vitest/browser';
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
    userPreferences.accentColor = 'cyan';
    userPreferences.surfaceDepth = '3d';
    userPreferences.threadPanePresentation = 'overlay';
  });

  it('changes depth through the standard radio group and persists the choice', async () => {
    const screen = render(AppearancePage);
    await settle();
    await expect.element(screen.getByRole('radio', { name: '3D', exact: true })).toHaveAttribute('aria-checked', 'true');
    await screen.getByRole('radio', { name: 'Flat', exact: true }).click();
    expect(userPreferences.surfaceDepth).toBe('flat');
    expect(document.documentElement.dataset.depth).toBe('flat');
    await screen.getByRole('radio', { name: 'Very 3D', exact: true }).click();
    expect(userPreferences.surfaceDepth).toBe('very-3d');
    expect(JSON.parse(localStorage.getItem('chatto:preferences') ?? '{}')).toMatchObject({
      surfaceDepth: 'very-3d'
    });
  });

  it('identifies its app-wide scope without server-specific preferences', async () => {
    const { container } = render(AppearancePage);
    await settle();

    expect(container.querySelectorAll('.panel-shell')).toHaveLength(4);
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

  it('selects and saves an accent with native radio keyboard controls', async () => {
    const screen = render(AppearancePage);
    await settle();
    expect(screen.container.querySelectorAll('input[type="radio"]')).toHaveLength(9);
    await screen.getByRole('radio', { name: 'Violet', exact: true }).click();
    expect(userPreferences.accentColor).toBe('violet');
    expect(document.documentElement.dataset.accent).toBe('violet');
    await userEvent.keyboard('{ArrowRight}');
    expect(userPreferences.accentColor).toBe('grey');
    expect(JSON.parse(localStorage.getItem('chatto:preferences') ?? '{}')).toMatchObject({
      accentColor: 'grey'
    });
    await expect.element(screen.getByRole('radio', { name: 'Grey', exact: true })).toBeChecked();
  });
});
