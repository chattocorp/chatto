import { beforeEach, describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import AtprotoLinkStatus from './AtprotoLinkStatus.svelte';

const mocks = vi.hoisted(() => ({
  verify: vi.fn(),
  ensureAtprotoSession: vi.fn(),
  ensureAtprotoLinkRecord: vi.fn()
}));
vi.mock('$lib/atproto/session.svelte', () => ({
  atprotoLinkVerifier: { verify: mocks.verify, forget: vi.fn() },
  ensureAtprotoSession: mocks.ensureAtprotoSession
}));
vi.mock('$lib/atproto/signIn', () => ({ ensureAtprotoLinkRecord: mocks.ensureAtprotoLinkRecord }));

const did = 'did:plc:alice';
const account = { serverOrigin: 'https://chat.example', userId: 'U1' };

describe('AtprotoLinkStatus', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mocks.ensureAtprotoSession.mockResolvedValue({ did });
    mocks.ensureAtprotoLinkRecord.mockResolvedValue(undefined);
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
