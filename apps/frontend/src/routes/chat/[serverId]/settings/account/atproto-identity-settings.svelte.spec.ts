import { beforeEach, describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { userEvent } from 'vitest/browser';
import { createTestServerScope, type TestServerScope } from '$lib/test-utils/serverScope.svelte';
import AtprotoIdentitySettings from './AtprotoIdentitySettings.svelte';

const DID = 'did:plc:alice0000000000000000000';
const account = { serverOrigin: 'https://chat.example', userId: 'U1' };

const mocks = vi.hoisted(() => ({
  session: { did: 'did:plc:alice0000000000000000000', fetchHandler: vi.fn() },
  atprotoAccount: vi.fn(),
  atprotoRepoSession: vi.fn(),
  connectAtprotoAccount: vi.fn(),
  verify: vi.fn(),
  forget: vi.fn(),
  writeAccountLinkRecord: vi.fn(),
  deleteAccountLinkRecord: vi.fn(),
  setAtprotoIdentity: vi.fn(),
  clearAtprotoIdentity: vi.fn()
}));

vi.mock(
  '$lib/state/server/scope.svelte',
  async () => (await import('$lib/test-utils/serverScope.svelte')).serverScopeModule
);
vi.mock('$lib/atproto/session.svelte', () => ({
  atprotoAccount: mocks.atprotoAccount,
  atprotoLinkVerifier: { verify: mocks.verify, forget: mocks.forget },
  atprotoOAuthClient: () => Promise.resolve({}),
  atprotoRepoSession: mocks.atprotoRepoSession,
  connectAtprotoAccount: mocks.connectAtprotoAccount,
  restoreAtprotoAccount: () => Promise.resolve(null)
}));
vi.mock('@chatto/client/atproto/accountLink', () => ({
  writeAccountLinkRecord: mocks.writeAccountLinkRecord,
  deleteAccountLinkRecord: mocks.deleteAccountLinkRecord
}));

let server: TestServerScope;

function renderSettings(atprotoDid: string | null) {
  server = createTestServerScope({
    serverId: 'origin',
    api: {
      setAtprotoIdentity: mocks.setAtprotoIdentity,
      clearAtprotoIdentity: mocks.clearAtprotoIdentity
    },
    connection: { origin: account.serverOrigin },
    viewer: { id: account.userId, login: 'alice', displayName: 'Alice', atprotoDid }
  });
  return render(AtprotoIdentitySettings, { currentUser: server.currentUser });
}

describe('AtprotoIdentitySettings', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mocks.atprotoAccount.mockReturnValue(null);
    mocks.atprotoRepoSession.mockResolvedValue(mocks.session);
    mocks.connectAtprotoAccount.mockResolvedValue({ did: DID, handle: 'alice.example' });
    mocks.writeAccountLinkRecord.mockResolvedValue(undefined);
    mocks.deleteAccountLinkRecord.mockResolvedValue(undefined);
    mocks.setAtprotoIdentity.mockResolvedValue(DID);
    mocks.clearAtprotoIdentity.mockResolvedValue(undefined);
    mocks.verify.mockResolvedValue({ status: 'verified', did: DID, handle: 'alice.example' });
  });

  it('writes the link record before it publishes the DID', async () => {
    const screen = renderSettings(null);
    await userEvent.fill(screen.getByLabelText('Bluesky handle'), 'alice.example');
    await screen.getByRole('button', { name: 'Connect' }).click();

    await expect.poll(() => mocks.setAtprotoIdentity).toHaveBeenCalledWith(DID);
    expect(mocks.connectAtprotoAccount).toHaveBeenCalledWith('alice.example');
    expect(mocks.writeAccountLinkRecord).toHaveBeenCalledWith(mocks.session, account);
    expect(mocks.writeAccountLinkRecord.mock.invocationCallOrder[0]).toBeLessThan(
      mocks.setAtprotoIdentity.mock.invocationCallOrder[0]
    );
    await expect
      .element(screen.getByText('Your profile shows @alice.example as verified.'))
      .toBeVisible();
  });

  it('does not publish the DID when the record cannot be written', async () => {
    mocks.writeAccountLinkRecord.mockRejectedValue(new Error('forbidden'));
    const screen = renderSettings(null);
    await screen.getByRole('button', { name: 'Connect' }).click();

    await expect
      .element(screen.getByText('The Bluesky identity could not be connected.'))
      .toBeVisible();
    expect(mocks.setAtprotoIdentity).not.toHaveBeenCalled();
  });

  it('deletes the record again when the server rejects the DID', async () => {
    mocks.setAtprotoIdentity.mockRejectedValue(new Error('unimplemented'));
    const screen = renderSettings(null);
    await screen.getByRole('button', { name: 'Connect' }).click();

    await expect
      .element(screen.getByText('The Bluesky identity could not be connected.'))
      .toBeVisible();
    expect(mocks.deleteAccountLinkRecord).toHaveBeenCalledWith(mocks.session, account);
  });

  it('shows an unverified claim as a warning', async () => {
    mocks.verify.mockResolvedValue({ status: 'unverified', did: DID, reason: 'no_link' });
    const screen = renderSettings(DID);
    await expect
      .element(
        screen.getByText('Other members cannot verify your Bluesky identity. Connect it again.')
      )
      .toBeVisible();
    expect(mocks.verify).toHaveBeenCalledWith(DID, account);
  });

  it('removes the claim and deletes the link record', async () => {
    const screen = renderSettings(DID);
    await screen.getByRole('button', { name: 'Remove' }).click();

    await expect
      .poll(() => mocks.deleteAccountLinkRecord)
      .toHaveBeenCalledWith(mocks.session, account);
    expect(mocks.atprotoRepoSession).toHaveBeenCalledWith(DID);
    expect(mocks.clearAtprotoIdentity).toHaveBeenCalledOnce();
    expect(server.currentUser.user?.atprotoDid).toBeNull();
  });

  it('rejects a sign-in with a different account during removal', async () => {
    mocks.atprotoRepoSession.mockResolvedValueOnce(null);
    mocks.connectAtprotoAccount.mockResolvedValue({ did: 'did:plc:someoneelse', handle: null });
    const screen = renderSettings(DID);
    await screen.getByRole('button', { name: 'Remove' }).click();

    await expect
      .element(
        screen.getByText(
          'You signed in with a different Bluesky account. Sign in with the connected account.'
        )
      )
      .toBeVisible();
    expect(mocks.clearAtprotoIdentity).not.toHaveBeenCalled();
  });
});
