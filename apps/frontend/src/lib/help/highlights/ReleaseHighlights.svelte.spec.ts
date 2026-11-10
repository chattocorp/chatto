import { beforeEach, describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import type { ReleaseHighlightState } from '@chatto/client/api/account';
import { MyAccountService } from '@chatto/api-types/api/v1/account_connect';
import { TimeFormat } from '@chatto/api-types/api/v1/viewer_pb';
import { ConnectError, Code } from '@connectrpc/connect';
import { mockService, receivedRequest } from '$lib/test-utils';
import { createTestServerScope } from '$lib/test-utils/serverScope.svelte';
import { loadLocaleMessages } from '$lib/i18n/messages';
import { setReactiveLocale } from '$lib/i18n/state.svelte';
import ReleaseHighlights from './ReleaseHighlights.svelte';
import { removeRegisteredServerQueries } from '$lib/query/cacheRegistry';

vi.mock(
  '$lib/state/server/scope.svelte',
  async () => (await import('$lib/test-utils/serverScope.svelte')).serverScopeModule
);

vi.mock('$lib/render/pageTitle', () => ({ formatPageTitle: () => 'Chatto' }));
const account = mockService(MyAccountService);
const saved = { dismissedIds: [] as string[], usersSinceVersion: '0.5.0' };

beforeEach(async () => {
  vi.clearAllMocks();
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
});
