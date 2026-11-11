import { beforeEach, describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { userEvent } from 'vitest/browser';
import type { PublicAuthProvider } from '@chatto/client/api/server';
import AuthProviderButtons from './AuthProviderButtons.svelte';

const mocks = vi.hoisted(() => ({ atprotoAccount: vi.fn() }));
vi.mock('$lib/atproto/session.svelte', () => ({
  atprotoAccount: mocks.atprotoAccount,
  restoreAtprotoAccount: () => Promise.resolve(null)
}));

const github = {
  id: 'github',
  type: 'github',
  label: 'GitHub',
  loginUrl: '/auth/providers/github'
} as PublicAuthProvider;
const bluesky = {
  id: 'bsky',
  type: 'atproto',
  label: 'Internet Handle',
  loginUrl: '/auth/providers/bsky'
} as PublicAuthProvider;

describe('AuthProviderButtons', () => {
  beforeEach(() => mocks.atprotoAccount.mockReturnValue(null));

  it('passes the typed handle for AT Protocol providers', async () => {
    const onclick = vi.fn((event: Event) => event.preventDefault());
    const screen = render(AuthProviderButtons, { providers: [github, bluesky], onclick });
    await userEvent.fill(screen.getByLabelText('Internet handle'), '@alice.example');
    await screen.getByRole('button', { name: 'Continue with Internet Handle' }).click();
    expect(onclick).toHaveBeenCalledWith(expect.anything(), bluesky, 'alice.example');

    await screen.getByRole('button', { name: 'Continue with GitHub' }).click();
    expect(onclick).toHaveBeenLastCalledWith(expect.anything(), github);
  });

  it('suggests the handle of the device session and allows an empty handle', async () => {
    mocks.atprotoAccount.mockReturnValue({ did: 'did:plc:alice', handle: 'alice.example' });
    const onclick = vi.fn((event: Event) => event.preventDefault());
    const screen = render(AuthProviderButtons, { providers: [bluesky], onclick });
    await expect.element(screen.getByLabelText('Internet handle')).toHaveValue('alice.example');

    await userEvent.clear(screen.getByLabelText('Internet handle'));
    await screen.getByRole('button', { name: 'Continue with Internet Handle' }).click();
    expect(onclick).toHaveBeenCalledWith(expect.anything(), bluesky, undefined);
  });
});
