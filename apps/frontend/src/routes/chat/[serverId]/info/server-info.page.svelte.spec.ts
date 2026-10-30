import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { ListFeaturesResponse } from '@chatto/api-types/chatto/discovery/v1/server_pb';
import { ServerInfoState } from '@chatto/client/server/state';
import { loadLocaleMessages } from '$lib/i18n/messages';
import { setReactiveLocale } from '$lib/i18n/state.svelte';
import { queryClient } from '$lib/query/client';
import { createTestServerScope } from '$lib/test-utils/serverScope.svelte';
import ServerInfoPage from './+page.svelte';

vi.mock(
  '$lib/state/server/scope.svelte',
  async () => (await import('$lib/test-utils/serverScope.svelte')).serverScopeModule
);

describe('public feature catalog on Server information', () => {
  const requests: { locale?: string; introducedAfterVersion?: string }[] = [];
  let info: ServerInfoState;
  beforeEach(async () => {
    queryClient.clear();
    requests.length = 0;
    await loadLocaleMessages('en-GB');
    setReactiveLocale('en-GB');
    info = new ServerInfoState();
    info.name = 'Remote';
    info.version = '0.6.0';
    info.loading = false;
    info.lastDiscoveredAt = Date.now();
    createTestServerScope({ store: { serverInfo: info } });
    // Public discovery owns its HTTP transport. Exercise the real generated
    // client and JSON codec while retaining fetch for unrelated browser work.
    const originalFetch = globalThis.fetch;
    vi.spyOn(globalThis, 'fetch').mockImplementation(async (input, init) => {
      const request = new Request(input, init);
      if (!request.url.endsWith('ServerDiscoveryService/ListFeatures'))
        return originalFetch(input, init);
      const filters = await request.json();
      requests.push(filters);
      return new Response(
        new ListFeaturesResponse({
          features: [
            {
              id: 'future-feature',
              name: filters.locale === 'de-DE' ? 'Neue Funktion' : 'Future feature',
              description: 'An unfamiliar feature.',
              introducedVersion: '9.0.0'
            }
          ]
        }).toJsonString(),
        { headers: { 'Content-Type': 'application/json' } }
      );
    });
  });
  afterEach(() => {
    vi.restoreAllMocks();
    queryClient.clear();
  });

  it('loads on entry, reuses the cache, and changes keys with locale and server version', async () => {
    expect(requests).toHaveLength(0);
    const first = render(ServerInfoPage);
    await expect.element(first.getByRole('heading', { name: 'Future feature' })).toBeVisible();
    expect(requests).toEqual([{ locale: 'en-GB', introducedAfterVersion: expect.any(String) }]);
    await first.unmount();
    const second = render(ServerInfoPage);
    await expect.element(second.getByRole('heading', { name: 'Future feature' })).toBeVisible();
    expect(requests).toHaveLength(1);
    setReactiveLocale('de-DE');
    await expect.element(second.getByRole('heading', { name: 'Neue Funktion' })).toBeVisible();
    expect(requests).toHaveLength(2);
    info.version = '0.7.0';
    await vi.waitFor(() => expect(requests).toHaveLength(3));
  });

  it('does not request the catalog for the origin server', async () => {
    const origin = new ServerInfoState('origin', undefined, () => null, true);
    origin.loading = false;
    origin.lastDiscoveredAt = Date.now();
    createTestServerScope({ store: { serverInfo: origin } });
    const screen = render(ServerInfoPage);
    await expect.element(screen.getByText('No known compatibility issues')).toBeVisible();
    expect(requests).toHaveLength(0);
  });
});
