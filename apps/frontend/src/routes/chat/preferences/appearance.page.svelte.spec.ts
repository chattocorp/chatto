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
    userPreferences.lightSurfaceTone = 'gray';
    userPreferences.darkSurfaceTone = 'neutral';
    userPreferences.surfaceDepth = 50;
    userPreferences.contrastAge = 30;
    userPreferences.threadPanePresentation = 'overlay';
  });

  it('changes depth in 10% steps with the slider and persists the choice', async () => {
    const screen = render(AppearancePage);
    await settle();
    await expect.element(screen.getByText('UI Style', { exact: true })).toBeVisible();
    const slider = screen.getByRole('slider', { name: /^Depth/ });
    await expect.element(slider).toHaveValue('50');
    await expect.element(slider).toHaveAttribute('step', '10');
    await expect.element(slider).toHaveAttribute('aria-valuetext', '50%, Kinda 3D');
    await slider.click();
    await userEvent.keyboard('{ArrowRight}');
    expect(userPreferences.surfaceDepth).toBe(60);
    await expect.element(slider).toHaveAttribute('aria-valuetext', '60%');
    expect(document.documentElement.style.getPropertyValue('--depth-level')).toBe('60');

    const input = slider.element() as HTMLInputElement;
    input.value = '0';
    input.dispatchEvent(new Event('input', { bubbles: true }));
    await settle();
    expect(userPreferences.surfaceDepth).toBe(0);
    await expect.element(slider).toHaveAttribute('aria-valuetext', '0%, Flat');
    expect(JSON.parse(localStorage.getItem('chatto:preferences') ?? '{}')).toMatchObject({
      surfaceDepth: 0
    });
  });

  it('identifies its app-wide scope without server-specific preferences', async () => {
    const { container } = render(AppearancePage);
    await settle();

    expect(container.querySelectorAll('.panel-shell')).toHaveLength(3);
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
    // Colours, depth, and contrast share one Theme customisation panel.
    const customizationPanel = [...screen.container.querySelectorAll('.panel-shell')].find(
      (panel) => panel.querySelector('h2')?.textContent?.includes('Theme customisation')
    );
    for (const control of ['#ui-depth', '#ui-contrast', '[data-tone-theme]']) {
      expect(customizationPanel?.querySelector(control), control).not.toBeNull();
    }
    const slider = screen.getByRole('slider', { name: /^Contrast/ });
    await expect.element(slider).toHaveValue('30');
    await expect.element(slider).toHaveAttribute('aria-valuetext', '50%, current contrast');
    expect(screen.container.textContent).not.toContain('Very Low');
    expect(screen.container.textContent).not.toContain('Very High');
    const input = slider.element() as HTMLInputElement;
    await slider.click();
    await userEvent.keyboard('{ArrowRight}');
    expect(userPreferences.contrastAge).toBe(32);
    await expect.element(slider).toHaveAttribute('aria-valuetext', '60%, stronger contrast');
    expect(document.documentElement.style.getPropertyValue('--contrast-strong-mix')).toBe('20%');
    expect(JSON.parse(localStorage.getItem('chatto:preferences') ?? '{}')).toMatchObject({
      contrastAge: 32
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
    const accents = screen.getByRole('group', { name: 'Accent colour' });
    expect(accents.element().querySelectorAll('input[type="radio"]')).toHaveLength(9);
    await accents.getByRole('radio', { name: 'Violet', exact: true }).click();
    expect(userPreferences.accentColor).toBe('violet');
    expect(document.documentElement.dataset.accent).toBe('violet');
    await userEvent.keyboard('{ArrowRight}');
    expect(userPreferences.accentColor).toBe('grey');
    expect(JSON.parse(localStorage.getItem('chatto:preferences') ?? '{}')).toMatchObject({
      accentColor: 'grey'
    });
    await expect.element(accents.getByRole('radio', { name: 'Grey', exact: true })).toBeChecked();
  });

  it('shows and saves the surface tone of the active theme only', async () => {
    userPreferences.displayTheme = 'light';
    const screen = render(AppearancePage);
    await settle();
    const tones = screen.getByRole('group', { name: 'Background colour' });
    expect(screen.container.querySelectorAll('[data-tone-theme]')).toHaveLength(1);
    expect(tones.element().getAttribute('data-tone-theme')).toBe('light');
    await expect.element(tones.getByRole('radio', { name: 'Grey', exact: true })).toBeChecked();
    await tones.getByRole('radio', { name: 'Forest', exact: true }).click();
    expect(userPreferences.lightSurfaceTone).toBe('forest');

    userPreferences.displayTheme = 'dark';
    await settle();
    const darkTones = screen.getByRole('group', { name: 'Background colour' });
    expect(darkTones.element().getAttribute('data-tone-theme')).toBe('dark');
    await expect
      .element(darkTones.getByRole('radio', { name: 'Neutral', exact: true }))
      .toBeChecked();
    await darkTones.getByRole('radio', { name: 'Olive', exact: true }).click();
    await userEvent.keyboard('{ArrowRight}');

    expect(userPreferences.lightSurfaceTone).toBe('forest');
    expect(userPreferences.darkSurfaceTone).toBe('forest');
    expect(document.documentElement.dataset.lightTone).toBe('forest');
    expect(document.documentElement.dataset.darkTone).toBe('forest');
    expect(JSON.parse(localStorage.getItem('chatto:preferences') ?? '{}')).toMatchObject({
      lightSurfaceTone: 'forest',
      darkSurfaceTone: 'forest'
    });
  });
});
