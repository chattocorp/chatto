import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import {
  AtprotoSignInError,
  linkAtprotoIdentity,
  signInToOriginWithAtproto,
  signInToRemoteWithAtproto
} from './signIn';

const DID = 'did:plc:alice0000000000000000000';
const session = { did: DID, fetchHandler: vi.fn() };

const mocks = vi.hoisted(() => ({
  ensureAtprotoSession: vi.fn(),
  atprotoServiceToken: vi.fn(),
  atprotoRepoSession: vi.fn(),
  verify: vi.fn(),
  writeAccountLinkRecord: vi.fn()
}));
vi.mock('./session.svelte', () => ({
  ensureAtprotoSession: mocks.ensureAtprotoSession,
  atprotoServiceToken: mocks.atprotoServiceToken,
  atprotoRepoSession: mocks.atprotoRepoSession,
  atprotoLinkVerifier: { verify: mocks.verify, forget: vi.fn() }
}));
vi.mock('@chatto/client/atproto/accountLink', async (importOriginal) => ({
  ...(await importOriginal<typeof import('@chatto/client/atproto/accountLink')>()),
  writeAccountLinkRecord: mocks.writeAccountLinkRecord,
  deleteAccountLinkRecord: vi.fn()
}));

function respond(status: number, body: unknown) {
  const fetchMock = vi.fn(async () => new Response(JSON.stringify(body), { status }));
  vi.stubGlobal('fetch', fetchMock);
  return fetchMock;
}

function requestOf(fetchMock: ReturnType<typeof respond>) {
  const [url, init] = fetchMock.mock.calls[0] as unknown as [URL, RequestInit];
  return { url: url.toString(), init, body: JSON.parse(init.body as string) };
}

describe('AT Protocol sign-in', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mocks.ensureAtprotoSession.mockResolvedValue(session);
    mocks.atprotoServiceToken.mockResolvedValue('service-token');
    mocks.atprotoRepoSession.mockResolvedValue(session);
    mocks.verify.mockResolvedValue({ status: 'unverified', did: DID, reason: 'no_link' });
    mocks.writeAccountLinkRecord.mockResolvedValue(undefined);
    vi.stubGlobal('window', { location: { origin: 'https://chat.example' } });
  });
  afterEach(() => vi.unstubAllGlobals());

  it('signs in to the origin with a cookie session and writes the link record', async () => {
    const fetchMock = respond(200, { status: 'signed_in', user: { id: 'U1' } });
    await expect(signInToOriginWithAtproto('alice.example')).resolves.toEqual({
      kind: 'signed-in',
      userId: 'U1'
    });
    const origin = 'https://chat.example';
    // The audience comes from this window's own origin, not from the server.
    expect(mocks.atprotoServiceToken).toHaveBeenCalledWith(session, 'did:web:chat.example#chatto');
    const request = requestOf(fetchMock);
    expect(request.url).toBe(`${origin}/auth/browser/atproto`);
    expect(request.init.credentials).toBe('include');
    expect(request.body).toEqual({ token: 'service-token' });
    expect(mocks.writeAccountLinkRecord).toHaveBeenCalledWith(session, {
      serverOrigin: origin,
      userId: 'U1'
    });
  });

  it('returns a new-account confirmation without writing a record', async () => {
    respond(200, { status: 'confirm_account', token: 'pending' });
    await expect(signInToOriginWithAtproto('')).resolves.toEqual({
      kind: 'confirm-account',
      token: 'pending'
    });
    expect(mocks.writeAccountLinkRecord).not.toHaveBeenCalled();
  });

  it('reports server error codes', async () => {
    respond(403, { error: 'external_identity_unlinked' });
    await expect(signInToOriginWithAtproto('')).rejects.toMatchObject({
      code: 'external_identity_unlinked'
    });
  });

  it('does not write a record when the link is already verified', async () => {
    respond(200, { status: 'signed_in', user: { id: 'U1' } });
    mocks.verify.mockResolvedValue({ status: 'verified', did: DID, handle: 'alice.example' });
    await signInToOriginWithAtproto('');
    expect(mocks.writeAccountLinkRecord).not.toHaveBeenCalled();
  });

  it('signs in to another server with a bearer session', async () => {
    const fetchMock = respond(200, {
      status: 'signed_in',
      user: { id: 'R1' },
      token: 'access',
      refreshToken: 'refresh',
      expiresIn: 900,
      refreshTokenExpiresIn: 7776000
    });
    const credentials = await signInToRemoteWithAtproto('https://remote.example/chat');
    expect(credentials?.token).toBe('access');
    expect(mocks.atprotoServiceToken).toHaveBeenCalledWith(
      session,
      'did:web:remote.example#chatto'
    );
    const request = requestOf(fetchMock);
    expect(request.url).toBe('https://remote.example/auth/atproto');
    expect(request.init.credentials).toBe('omit');
    expect(mocks.writeAccountLinkRecord).toHaveBeenCalledWith(session, {
      serverOrigin: 'https://remote.example',
      userId: 'R1'
    });
  });

  it('reports a remote server without an account for the identity', async () => {
    respond(403, { error: 'external_identity_unlinked' });
    await expect(signInToRemoteWithAtproto('https://remote.example')).resolves.toBeNull();
  });

  it('refuses servers that cannot accept AT Protocol sign-in', async () => {
    respond(200, {});
    await expect(signInToRemoteWithAtproto('https://remote.example:8443')).rejects.toBeInstanceOf(
      AtprotoSignInError
    );
    expect(fetch).not.toHaveBeenCalled();
  });

  it('links the identity with a link token and writes the record', async () => {
    const fetchMock = respond(200, { status: 'linked', user: { id: 'U2' } });
    await linkAtprotoIdentity(session as never, 'https://remote.example', 'link-token');
    expect(requestOf(fetchMock).body).toEqual({ token: 'service-token', link_token: 'link-token' });
    expect(mocks.writeAccountLinkRecord).toHaveBeenCalledWith(session, {
      serverOrigin: 'https://remote.example',
      userId: 'U2'
    });
  });
});
