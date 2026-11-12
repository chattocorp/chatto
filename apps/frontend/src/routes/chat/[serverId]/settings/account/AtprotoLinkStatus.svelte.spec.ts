import { beforeEach, describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import AtprotoLinkStatus from './AtprotoLinkStatus.svelte';

const mocks = vi.hoisted(() => ({
  verify: vi.fn(),
  ensureAtprotoSession: vi.fn(),
  ensureAtprotoLinkRecord: vi.fn(),
  atprotoOAuthClient: vi.fn(),
  restoreAtprotoAccount: vi.fn()
}));
vi.mock('$lib/atproto/session.svelte', () => ({
  atprotoLinkVerifier: { verify: mocks.verify, forget: vi.fn() },
  ensureAtprotoSession: mocks.ensureAtprotoSession,
  atprotoOAuthClient: mocks.atprotoOAuthClient,
  restoreAtprotoAccount: mocks.restoreAtprotoAccount
}));
vi.mock('$lib/atproto/signIn', () => ({
  ensureAtprotoLinkRecord: mocks.ensureAtprotoLinkRecord,
  canWriteAtprotoLinkRecord: (account: { serverOrigin: string }) =>
    new URL(account.serverOrigin).origin === window.location.origin
}));

const did = 'did:plc:alice';
const account = { serverOrigin: window.location.origin, userId: 'U1' };

describe('AtprotoLinkStatus', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mocks.ensureAtprotoSession.mockResolvedValue({ did });
    mocks.ensureAtprotoLinkRecord.mockResolvedValue(undefined);
    mocks.atprotoOAuthClient.mockResolvedValue({});
    mocks.restoreAtprotoAccount.mockResolvedValue(null);
  });

  it('links to the app of another server, which alone can write its records', async () => {
    mocks.verify.mockResolvedValue({ status: 'unverified', did, reason: 'no_link' });
    const screen = render(AtprotoLinkStatus, {
      did,
      account: { serverOrigin: 'https://remote.example', userId: 'U1' }
    });
    const link = screen.getByRole('link', { name: 'Verify on remote.example' });
    await expect
      .element(link)
      .toHaveAttribute('href', 'https://remote.example/chat/-/settings/account');
    expect(screen.getByRole('button', { name: 'Verify' }).query()).toBeNull();
    expect(mocks.atprotoOAuthClient).not.toHaveBeenCalled();
  });

  it('loads the OAuth client before the Verify click', async () => {
    mocks.verify.mockResolvedValue({ status: 'unverified', did, reason: 'no_link' });
    const screen = render(AtprotoLinkStatus, { did, account });
    await expect.element(screen.getByRole('button', { name: 'Verify' })).toBeVisible();
    expect(mocks.atprotoOAuthClient).toHaveBeenCalled();
    expect(mocks.ensureAtprotoSession).not.toHaveBeenCalled();
  });

  it('shows the verified handle instead of the linked state', async () => {
    mocks.verify.mockResolvedValue({ status: 'verified', did, handle: 'alice.example' });
    const screen = render(AtprotoLinkStatus, { did, account });
    const handle = screen.getByTestId('verified-atproto-handle');
    await expect.element(handle).toHaveTextContent('@alice.example');
    // The row has its own provider icon; the handle adds no second one.
    expect(handle.element().querySelector('[aria-hidden="true"]')).toBeNull();
    await expect.element(screen.getByText('Linked', { exact: true })).not.toBeInTheDocument();
  });

  it('offers Verify while other apps cannot verify the link', async () => {
    mocks.verify
      .mockResolvedValueOnce({ status: 'unverified', did, reason: 'no_link' })
      .mockResolvedValue({ status: 'verified', did, handle: 'alice.example' });
    const screen = render(AtprotoLinkStatus, { did, account });
    await screen.getByRole('button', { name: 'Verify' }).click();
    await vi.waitFor(() =>
      expect(mocks.ensureAtprotoLinkRecord).toHaveBeenCalledWith(account, did)
    );
    expect(mocks.ensureAtprotoSession).toHaveBeenCalledWith(did);
    await expect.element(screen.getByTestId('verified-atproto-handle')).toBeVisible();
  });
});
