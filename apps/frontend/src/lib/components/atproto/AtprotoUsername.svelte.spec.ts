import { describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import AtprotoUsername from './AtprotoUsername.svelte';

const mocks = vi.hoisted(() => ({ verify: vi.fn() }));
vi.mock('$lib/atproto/session.svelte', () => ({ atprotoLinkVerifier: { verify: mocks.verify } }));

const did = 'did:plc:alice';
const account = { serverOrigin: 'https://chat.example', userId: 'U1' };

describe('AtprotoUsername', () => {
  it('marks the @ of a username that is the verified handle', async () => {
    mocks.verify.mockResolvedValue({ status: 'verified', did, handle: 'Alice.Example' });
    const screen = render(AtprotoUsername, { login: 'alice.example', did, account });
    const mark = screen.getByTestId('verified-atproto-username');
    await expect.element(mark).toHaveTextContent('@');
    await expect.element(mark).toHaveAttribute('aria-label', 'Verified internet handle');
    await expect.element(screen.getByText('alice.example')).toBeVisible();
  });

  it('keeps a plain @ when the verified handle is another name', async () => {
    mocks.verify.mockResolvedValue({ status: 'verified', did, handle: 'alice.example' });
    const screen = render(AtprotoUsername, { login: 'alice', did, account });
    await expect.element(screen.getByText('@alice')).toBeVisible();
    await expect.element(screen.getByTestId('verified-atproto-username')).not.toBeInTheDocument();
  });

  it('does not verify without a DID', async () => {
    mocks.verify.mockClear();
    const screen = render(AtprotoUsername, { login: 'bob', did: null, account });
    await expect.element(screen.getByText('@bob')).toBeVisible();
    expect(mocks.verify).not.toHaveBeenCalled();
  });
});
