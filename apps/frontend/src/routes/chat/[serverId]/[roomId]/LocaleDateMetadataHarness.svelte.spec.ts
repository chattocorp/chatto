import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { flushSync } from 'svelte';
import { render } from 'vitest-browser-svelte';
import { q } from '$lib/test-utils';
import { loadLocaleMessages } from '$lib/i18n/messages';
import { setReactiveLocale } from '$lib/i18n/state.svelte';
import LocaleDateMetadataHarness from './LocaleDateMetadataHarness.svelte';

describe('localized date metadata', () => {
  beforeEach(async () => {
    vi.useFakeTimers();
    vi.setSystemTime(new Date('2025-11-28T12:00:00Z'));
    await loadLocaleMessages('en');
    setReactiveLocale('en');
  });

  afterEach(async () => {
    vi.useRealTimers();
    await loadLocaleMessages('en');
    setReactiveLocale('en');
  });

  it('updates precomputed day labels when the active locale changes', async () => {
    const { container } = render(LocaleDateMetadataHarness);
    flushSync();

    const label = q(container, '[data-testid="day-label"]');
    await expect.element(label).toHaveTextContent('Thursday, November 20');

    await loadLocaleMessages('de');
    setReactiveLocale('de');
    flushSync();

    await expect.element(label).toHaveTextContent(/Donnerstag/);
    await expect.element(label).toHaveTextContent(/November/);
  });
});
