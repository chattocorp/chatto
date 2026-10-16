import { beforeEach, describe, expect, it } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { loadLocaleMessages } from '$lib/i18n/messages';
import { setReactiveLocale } from '$lib/i18n/state.svelte';
import { limitedServerFeatures } from '@chatto/client/server/compatibility';
import FeatureCompatibilityNotice from './FeatureCompatibilityNotice.svelte';

describe('FeatureCompatibilityNotice', () => {
  beforeEach(async () => {
    await loadLocaleMessages('en-GB');
    setReactiveLocale('en-GB');
  });

  it('links to the affected section without repeating the details', async () => {
    const screen = render(FeatureCompatibilityNotice, {
      features: limitedServerFeatures('0.5.0-beta.17'),
      serverSegment: 'chat.example.test'
    });
    await expect
      .element(screen.getByRole('link', { name: 'View compatibility' }))
      .toHaveAttribute('href', '/chat/chat.example.test/info#independentSlowMode');
    expect(screen.container.textContent).not.toContain('upgrade');
  });

  it.each(['0.5.0-beta.18', 'dev'])('does not invent limitations for %s', (version) => {
    const { container } = render(FeatureCompatibilityNotice, {
      features: limitedServerFeatures(version),
      serverSegment: 'chat.example.test'
    });
    expect(container.querySelector('[data-testid="feature-compatibility-notice"]')).toBeNull();
  });
});
