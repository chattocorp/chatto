import { describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import AppPreferencesLayout from './+layout.svelte';

vi.mock('$app/state', () => ({
  page: { url: new URL('https://chatto.test/chat/preferences') }
}));

describe('App Preferences layout', () => {
  it('uses the shared sidebar shell with section navigation', async () => {
    const { container, getByRole } = render(AppPreferencesLayout);

    expect(container.querySelector('[data-testid="server-sidebar"]')).not.toBeNull();
    expect(container.querySelector('[data-testid="current-user-identity-card"]')).toBeNull();
    await expect.element(getByRole('heading', { name: 'App Preferences' })).toBeVisible();
    await expect
      .element(getByRole('link', { name: 'Appearance' }))
      .toHaveAttribute('aria-current', 'page');
    await expect.element(getByRole('link', { name: 'Language' })).toBeVisible();
    await expect.element(getByRole('link', { name: 'Composer' })).toBeVisible();
  });
});
