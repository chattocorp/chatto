import { beforeEach, describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import type { ReleaseHighlightState } from '@chatto/client/api/account';
import { MyAccountService } from '@chatto/api-types/api/v1/account_connect';
import { TimeFormat } from '@chatto/api-types/api/v1/viewer_pb';
import { ListFeaturesResponse } from '@chatto/api-types/chatto/discovery/v1/server_pb';
import { ConnectError, Code } from '@connectrpc/connect';
import { mockService, receivedRequest } from '$lib/test-utils';
import { createTestServerScope } from '$lib/test-utils/serverScope.svelte';
import { loadLocaleMessages } from '$lib/i18n/messages';
import { setReactiveLocale } from '$lib/i18n/state.svelte';
import { sidebarNav } from '$lib/state/globals.svelte';
import ReleaseHighlights from './ReleaseHighlights.svelte';
import { removeRegisteredServerQueries } from '$lib/query/cacheRegistry';

vi.mock(
  '$lib/state/server/scope.svelte',
  async () => (await import('$lib/test-utils/serverScope.svelte')).serverScopeModule
);

vi.mock('$lib/render/pageTitle', () => ({ formatPageTitle: () => 'Chatto' }));
const { helpPage } = vi.hoisted(() => ({
  helpPage: {
    url: 'https://app.example/help/whats-new?server=server-1.example',
    serverSegment: undefined as string | undefined
  }
}));
vi.mock('$app/state', () => ({
  page: {
    get url() {
      return new URL(helpPage.url);
    },
    get params() {
      return { serverId: helpPage.serverSegment };
    }
  }
}));
vi.mock('$lib/client', async () => {
  const { clientMockDefaults } = await import('$lib/test-utils/clientMock');
  return {
    ...clientMockDefaults,
    serverRegistry: {
      ...clientMockDefaults.serverRegistry,
      isOriginServer: () => false,
      getServer: (id: string) => ({ id, url: `https://${id}.example` })
    }
  };
});
const account = mockService(MyAccountService);
const saved = { dismissedIds: [] as string[], usersSinceVersion: '0.5.0' };

beforeEach(async () => {
  vi.clearAllMocks();
  sidebarNav.setMobile(false);
  helpPage.url = 'https://app.example/help/whats-new?server=server-1.example';
  helpPage.serverSegment = undefined;
  await loadLocaleMessages('en-GB');
  setReactiveLocale('en-GB');
  account.dismissReleaseHighlights.mockImplementation(async (request) => ({
    settings: {
      timeFormat: TimeFormat.TIME_FORMAT_AUTO,
      releaseHighlights: { ...saved, dismissedIds: request.ids }
    }
  }));
  account.initializeReleaseHighlights.mockResolvedValue({
    settings: { timeFormat: TimeFormat.TIME_FORMAT_AUTO, releaseHighlights: saved }
  });
});

function show(releaseHighlights: ReleaseHighlightState | null = saved) {
  const fixture = createTestServerScope({
    viewer: {
      settings: {
        timeFormat: TimeFormat.TIME_FORMAT_AUTO,
        releaseHighlights: releaseHighlights ?? undefined
      }
    },
    serverInfo: { version: '0.5.0', isOriginServer: true, name: 'Example' },
    routes: (router) => router.service(MyAccountService, account)
  });
  const screen = render(ReleaseHighlights);
  return { fixture, screen };
}

describe('release highlights', () => {
  it('does not dismiss on opening; explicit dismissal keeps the article in history', async () => {
    const { screen, fixture } = show();
    await expect.element(screen.getByText('Help is now in Chatto')).toBeVisible();
    expect(account.dismissReleaseHighlights).not.toHaveBeenCalled();
    await screen.getByRole('button', { name: 'Dismiss', exact: true }).click();
    await expect.element(screen.getByText('You are up to date.')).toBeVisible();
    expect(receivedRequest(account.dismissReleaseHighlights)?.ids).toEqual(['users:help-guides']);
    expect(fixture.currentUser.user?.settings?.releaseHighlights?.dismissedIds).toEqual([
      'users:help-guides'
    ]);
    await expect.element(screen.getByText('Help is now in Chatto')).toBeVisible();
  });
  it('keeps an unsuccessful dismissal unread and shows a retryable error', async () => {
    account.dismissReleaseHighlights.mockRejectedValue(
      new ConnectError('Unavailable', Code.Unavailable)
    );
    const { screen, fixture } = show();
    await screen.getByRole('button', { name: 'Dismiss all', exact: true }).click();
    await expect
      .element(screen.getByRole('button', { name: 'Dismiss all', exact: true }))
      .toBeEnabled();
    expect(fixture.currentUser.user?.settings?.releaseHighlights?.dismissedIds).toEqual([]);
  });
  it('keeps the archive readable when the server cannot save dismissals', async () => {
    const { screen } = show(null);
    await expect
      .element(screen.getByText('This server does not support saving dismissals across devices.'))
      .toBeVisible();
    await expect
      .element(screen.getByRole('button', { name: 'Dismiss', exact: true }))
      .not.toBeInTheDocument();
  });
  it('requires successful baseline persistence before dismissal and offers retry', async () => {
    account.initializeReleaseHighlights.mockRejectedValue(
      new ConnectError('Unavailable', Code.Unavailable)
    );
    const { screen } = show({ dismissedIds: [] });
    await expect
      .element(screen.getByRole('button', { name: 'Try Again', exact: true }))
      .toBeVisible();
    await expect
      .element(screen.getByRole('button', { name: 'Dismiss', exact: true }))
      .not.toBeInTheDocument();
    expect(account.dismissReleaseHighlights).not.toHaveBeenCalled();
    account.initializeReleaseHighlights.mockResolvedValue({
      settings: { timeFormat: TimeFormat.TIME_FORMAT_AUTO, releaseHighlights: saved }
    });
    await screen.getByRole('button', { name: 'Try Again', exact: true }).click();
    await expect
      .element(screen.getByRole('button', { name: 'Dismiss', exact: true }))
      .toBeVisible();
  });

  it('does not apply a dismissal response after the owning scope retires', async () => {
    let release!: () => void;
    const gate = new Promise<void>((resolve) => {
      release = resolve;
    });
    account.dismissReleaseHighlights.mockImplementation(async (request) => {
      await gate;
      return {
        settings: {
          timeFormat: TimeFormat.TIME_FORMAT_AUTO,
          releaseHighlights: { ...saved, dismissedIds: request.ids }
        }
      };
    });
    const { screen, fixture } = show();
    await screen.getByRole('button', { name: 'Dismiss', exact: true }).click();
    await vi.waitFor(() => expect(account.dismissReleaseHighlights).toHaveBeenCalledOnce());
    fixture.current = false;
    release();
    await account.dismissReleaseHighlights.mock.results[0].value;
    expect(fixture.currentUser.user?.settings?.releaseHighlights?.dismissedIds).toEqual([]);
  });
  it('allows another dismissal after a private-data refresh fences a pending response', async () => {
    let release!: () => void;
    const gate = new Promise<void>((resolve) => {
      release = resolve;
    });
    account.dismissReleaseHighlights.mockImplementation(async (request) => {
      await gate;
      return {
        settings: {
          timeFormat: TimeFormat.TIME_FORMAT_AUTO,
          releaseHighlights: { ...saved, dismissedIds: request.ids }
        }
      };
    });
    const { screen, fixture } = show();
    await screen.getByRole('button', { name: 'Dismiss', exact: true }).click();
    await vi.waitFor(() => expect(account.dismissReleaseHighlights).toHaveBeenCalledOnce());
    removeRegisteredServerQueries(fixture.scope.serverId);
    release();
    await expect
      .element(screen.getByRole('button', { name: 'Dismiss', exact: true }))
      .toBeEnabled();
    expect(fixture.currentUser.user?.settings?.releaseHighlights?.dismissedIds).toEqual([]);
  });
  it('restores a deep link at the article heading after the MDX loads', async () => {
    const previousHash = window.location.hash;
    const scroll = vi.spyOn(Element.prototype, 'scrollIntoView');
    try {
      window.location.hash = '#highlight-help-guides';
      const { screen } = show();
      await expect.element(screen.getByRole('link', { name: 'Read the guide' })).toBeVisible();
      expect(
        scroll.mock.contexts.some(
          (element) => element instanceof HTMLElement && element.id === 'highlight-help-guides'
        )
      ).toBe(true);
    } finally {
      scroll.mockRestore();
      window.location.hash = previousHash;
    }
  });
  it('keeps the selected account on mobile back navigation and the styled guide link', async () => {
    sidebarNav.setMobile(true);
    const { screen } = show();
    await expect
      .element(screen.getByRole('link', { name: 'Help', exact: true }))
      .toHaveAttribute('href', '/help?server=server-1.example');
    const guide = screen.getByRole('link', { name: 'Read the guide', exact: true });
    await expect
      .element(guide)
      .toHaveAttribute('href', '/help/conversations?server=server-1.example');
    await expect.element(guide).toHaveClass('link');
  });

  it('shows translated newer-server features as plain text through real discovery requests', async () => {
    await loadLocaleMessages('de-DE');
    setReactiveLocale('de-DE');
    const feature = {
      id: 'unknown-future-feature',
      name: 'Neue Funktion <b>Vorschau</b>',
      description: '<img src="bad" onerror="alert(1)"> bleibt Text.',
      introducedVersion: '0.6.0'
    };
    // Public discovery owns its HTTP transport. Stub its HTTP response, keeping
    // the generated Connect client, locale/version query, and JSON decoding real.
    const fetch = vi.spyOn(globalThis, 'fetch').mockResolvedValue(
      new Response(new ListFeaturesResponse({ features: [feature] }).toJsonString(), {
        headers: { 'Content-Type': 'application/json' }
      })
    );
    try {
      createTestServerScope({
        viewer: { settings: { timeFormat: TimeFormat.TIME_FORMAT_AUTO, releaseHighlights: saved } },
        serverInfo: {
          version: '0.6.0',
          isOriginServer: false,
          lastDiscoveredAt: Date.now(),
          error: null,
          name: 'Remote'
        }
      });
      const screen = render(ReleaseHighlights);
      await expect
        .element(screen.getByRole('heading', { name: 'Mit einer neueren App verfügbar' }))
        .toBeVisible();
      await expect.element(screen.getByText(feature.name, { exact: true })).toBeVisible();
      await expect.element(screen.getByText(feature.description, { exact: true })).toBeVisible();
      expect(screen.container.querySelector('img')).toBeNull();
      const [url, options] = fetch.mock.calls[0];
      expect(String(url)).toContain('chatto.discovery.v1.ServerDiscoveryService/ListFeatures');
      expect(JSON.parse(new URL(String(url)).searchParams.get('message')!)).toEqual({
        locale: 'de-DE',
        introducedAfterVersion: '0.5.0'
      });
      expect(options).toMatchObject({
        method: 'GET',
        credentials: 'omit',
        referrerPolicy: 'no-referrer'
      });
    } finally {
      fetch.mockRestore();
    }
  });
  it('does not request a feature catalog for the origin server', async () => {
    const fetch = vi.spyOn(globalThis, 'fetch');
    try {
      const { screen } = show();
      await expect.element(screen.getByRole('link', { name: 'Read the guide' })).toBeVisible();
      expect(fetch).not.toHaveBeenCalled();
    } finally {
      fetch.mockRestore();
    }
  });

  it('carries the administrative route server into the bot guide', async () => {
    helpPage.url = 'https://app.example/chat/remote.example/manage/server/whats-new';
    helpPage.serverSegment = 'remote.example';
    createTestServerScope({
      viewer: {
        settings: {
          timeFormat: TimeFormat.TIME_FORMAT_AUTO,
          releaseHighlights: { dismissedIds: [], adminsSinceVersion: '0.5.0' }
        }
      },
      serverInfo: { version: '0.5.0', isOriginServer: true, name: 'Remote' }
    });
    const screen = render(ReleaseHighlights, { surface: 'admins' });
    await expect
      .element(screen.getByRole('link', { name: 'Read the guide' }))
      .toHaveAttribute('href', '/help/bots?server=remote.example');
  });
});
