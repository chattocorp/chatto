import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { page as browserPage } from 'vitest/browser';
import { render } from 'vitest-browser-svelte';
import { authorizationLaunchStorageKey } from '$lib/oauth/authorizationWindow';
import AuthorizePage from './+page.svelte';

const { pageState, replace } = vi.hoisted(() => ({
  pageState: { url: 'https://app.example/servers/authorize#launch-id' },
  replace: vi.fn()
}));

// Page titles are tested separately from this page's partial route fixture.
vi.mock('$lib/render/pageTitle', () => ({ formatPageTitle: () => 'Chatto' }));
vi.mock('$lib/oauth/authorizationWindow', async (importOriginal) => ({
  ...(await importOriginal<typeof import('$lib/oauth/authorizationWindow')>()),
  replaceLaunchPage: replace
}));
vi.mock('$app/state', () => ({
  page: {
    get url() {
      return new URL(pageState.url);
    }
  }
}));

const key = authorizationLaunchStorageKey('launch-id');

describe('authorization launch page', () => {
  beforeEach(() => {
    pageState.url = 'https://app.example/servers/authorize#launch-id';
    replace.mockReset();
  });

  afterEach(() => {
    vi.restoreAllMocks();
    localStorage.removeItem(key);
  });

  it('opens a target that the opener stored before the page loaded', async () => {
    localStorage.setItem(
      key,
      JSON.stringify({
        url: 'https://remote.example/oauth/authorize?state=a',
        createdAt: Date.now()
      })
    );
    render(AuthorizePage);
    await vi.waitFor(() =>
      expect(replace).toHaveBeenCalledWith('https://remote.example/oauth/authorize?state=a')
    );
  });

  it('does not continue in a hidden window', async () => {
    const visibility = vi.spyOn(document, 'visibilityState', 'get').mockReturnValue('hidden');
    localStorage.setItem(
      key,
      JSON.stringify({
        url: 'https://remote.example/oauth/authorize?state=c',
        createdAt: Date.now()
      })
    );
    render(AuthorizePage);
    await new Promise((resolve) => setTimeout(resolve, 300));
    expect(replace).not.toHaveBeenCalled();

    visibility.mockReturnValue('visible');
    await vi.waitFor(() =>
      expect(replace).toHaveBeenCalledWith('https://remote.example/oauth/authorize?state=c')
    );
  });

  it('waits for a target that the opener stores later', async () => {
    render(AuthorizePage);
    await expect.element(browserPage.getByLabelText('Opening sign-in...')).toBeInTheDocument();
    expect(replace).not.toHaveBeenCalled();

    localStorage.setItem(
      key,
      JSON.stringify({
        url: 'https://remote.example/oauth/authorize?state=b',
        createdAt: Date.now()
      })
    );
    await vi.waitFor(() =>
      expect(replace).toHaveBeenCalledWith('https://remote.example/oauth/authorize?state=b')
    );
  });

  it('refuses a target that is not an HTTP or HTTPS URL', async () => {
    localStorage.setItem(
      key,
      JSON.stringify({ url: 'javascript:alert(1)', createdAt: Date.now() })
    );
    render(AuthorizePage);
    await expect
      .element(browserPage.getByText('The sign-in page could not open.', { exact: false }))
      .toBeVisible();
    expect(replace).not.toHaveBeenCalled();
  });

  it('shows an error without a launch ID', async () => {
    pageState.url = 'https://app.example/servers/authorize';
    render(AuthorizePage);
    await expect
      .element(browserPage.getByText('The sign-in page could not open.', { exact: false }))
      .toBeVisible();
  });
});
