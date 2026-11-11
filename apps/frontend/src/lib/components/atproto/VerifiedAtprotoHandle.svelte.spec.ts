import { describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import VerifiedAtprotoHandle from './VerifiedAtprotoHandle.svelte';

const mocks = vi.hoisted(() => ({ verify: vi.fn() }));
vi.mock('$lib/atproto/session.svelte', () => ({ atprotoLinkVerifier: { verify: mocks.verify } }));

const did = 'did:plc:alice';
const account = { serverOrigin: 'https://chat.example', userId: 'U1' };

describe('VerifiedAtprotoHandle', () => {
  it('shows a verified handle', async () => {
    mocks.verify.mockResolvedValue({ status: 'verified', did, handle: 'alice.example' });
    const screen = render(VerifiedAtprotoHandle, { did, account });
    const link = screen.getByTestId('verified-atproto-handle');
    await expect.element(link).toHaveTextContent('@alice.example');
    await expect.element(link).toHaveAttribute('href', 'https://bsky.app/profile/alice.example');
    expect(mocks.verify).toHaveBeenCalledWith(did, account);
  });

  it('shows nothing for an unverified claim', async () => {
    mocks.verify.mockResolvedValue({ status: 'unverified', did, reason: 'no_link' });
    const screen = render(VerifiedAtprotoHandle, { did, account });
    await Promise.resolve();
    await expect.element(screen.getByTestId('verified-atproto-handle')).not.toBeInTheDocument();
  });
});
