import { afterEach, describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';

const state = vi.hoisted(() => ({ url: new URL('https://chat.example/sso/handle') }));
vi.mock('$app/state', () => ({
  page: {
    get url() {
      return state.url;
    }
  }
}));
vi.mock('$lib/render/pageTitle', () => ({ formatPageTitle: () => 'Chatto' }));

import HandlePage from './+page.svelte';

const HANDLE_KEY = 'chatto:atproto-handle';

function open(query: Record<string, string>) {
  state.url = new URL(`${window.location.origin}/sso/handle?${new URLSearchParams(query)}`);
  return render(HandlePage);
}

describe('AT Protocol handle page', () => {
  afterEach(() => localStorage.removeItem(HANDLE_KEY));

  it('suggests the remembered handle and lets the user forget it', async () => {
    localStorage.setItem(HANDLE_KEY, 'alice.example');
    const screen = open({ next: '/auth/providers/bsky?redirect=%2Fchat' });
    await expect
      .element(screen.getByRole('heading', { name: 'Sign in with your Atmosphere account' }))
      .toBeVisible();
    const label = screen.getByText('Handle', { exact: true });
    await expect.element(label).not.toHaveClass('sr-only');
    const field = screen.getByLabelText('Handle', { exact: true });
    await expect.element(field).toHaveValue('alice.example');
    await screen.getByRole('button', { name: 'Use another handle' }).click();
    await expect.element(field).toHaveValue('');
    expect(localStorage.getItem(HANDLE_KEY)).toBeNull();
  });

  it('shows an unknown handle again with an explanation', async () => {
    const screen = open({
      next: '/auth/providers/bsky',
      error: 'atproto_identity_not_found',
      handle: 'nobody.example'
    });
    await expect
      .element(screen.getByText('That handle could not be found.', { exact: false }))
      .toBeVisible();
    await expect
      .element(screen.getByLabelText('Handle', { exact: true }))
      .toHaveValue('nobody.example');
  });

  it('refuses a continuation outside provider sign-in', async () => {
    const screen = open({ next: 'https://evil.example/auth/providers/bsky' });
    await expect
      .element(screen.getByText('This sign-in link is invalid', { exact: false }))
      .toBeVisible();
    expect(screen.getByLabelText('Handle', { exact: true }).query()).toBeNull();
  });
});
