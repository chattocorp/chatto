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
    userPreferences.contrastAge = 30;
    userPreferences.threadPanePresentation = 'overlay';
  });

  it('changes depth through the standard radio group and persists the choice', async () => {
    const screen = render(AppearancePage);
    await settle();
    await expect.element(screen.getByText('UI Style', { exact: true })).toBeVisible();
    await expect.element(screen.getByRole('radio', { name: 'Kinda 3D', exact: true })).toHaveAttribute('aria-checked', 'true');
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

    expect(container.querySelectorAll('.panel-shell')).toHaveLength(5);
    expect(container.textContent).toContain('Appearance');
    expect(container.textContent).toContain(
      'Choices for this app that apply across all your registered servers'
    );
    expect(container.textContent).not.toContain('Timezone');
    expect(container.textContent).toContain('Thread pane');
  });

  it('applies and saves contrast while the slider moves with the keyboard', async () => {
    const screen = render(AppearancePage);
    await settle();
    const stylePanel = [...screen.container.querySelectorAll('.panel-shell')].find((panel) =>
      panel.querySelector('h2')?.textContent?.includes('UI Style')
    );
    expect(stylePanel?.querySelector('#ui-contrast')).not.toBeNull();
    const slider = screen.getByRole('slider', { name: /^Contrast/ });
    await expect.element(slider).toHaveValue('30');
    await expect.element(slider).toHaveAttribute('aria-valuetext', '50%, current contrast');
    expect(screen.container.textContent).not.toContain('Very Low');
    expect(screen.container.textContent).not.toContain('Very High');
    const input = slider.element() as HTMLInputElement;
    await slider.click();
    await userEvent.keyboard('{ArrowRight}');
    expect(userPreferences.contrastAge).toBe(30.5);
    await expect.element(slider).toHaveAttribute('aria-valuetext', '53%, stronger contrast');
    expect(document.documentElement.style.getPropertyValue('--contrast-strong-mix')).toBe('5%');
    expect(JSON.parse(localStorage.getItem('chatto:preferences') ?? '{}')).toMatchObject({
      contrastAge: 30.5
    });

    input.value = '40';
    input.dispatchEvent(new Event('input', { bubbles: true }));
    await settle();
    expect(userPreferences.contrastAge).toBe(40);
    await expect.element(slider).toHaveAttribute('aria-valuetext', '100%, stronger contrast');
    await expect.element(slider).not.toHaveAttribute('aria-describedby');

    input.value = '20';
    input.dispatchEvent(new Event('input', { bubbles: true }));
    await settle();
    await expect.element(slider).toHaveAttribute('aria-valuetext', '0%, softer contrast');
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
