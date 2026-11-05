// @vitest-environment node
import { afterEach, describe, expect, it, vi } from 'vitest';
import { passwordLogin, PasswordLoginError } from './passwordLogin.js';

const response = {
  token: 'synthetic-access',
  refreshToken: 'synthetic-refresh',
  expiresIn: 900,
  refreshTokenExpiresIn: 3600
};
afterEach(() => vi.unstubAllGlobals());

describe('native password login', () => {
  it('uses the direct session contract without cookies or credential-bearing redirects', async () => {
    const fetcher = vi.fn().mockResolvedValue(Response.json(response));
    vi.stubGlobal('fetch', fetcher);
    const session = await passwordLogin('https://example.test', {
      login: ' synthetic ',
      password: 'synthetic-password'
    });
    expect(session).toEqual({ ...response, oauthClientId: null });
    const [url, options] = fetcher.mock.calls[0]!;
    expect(String(url)).toBe('https://example.test/auth/login');
    expect(options).toMatchObject({
      method: 'POST',
      redirect: 'error',
      credentials: 'omit',
      body: JSON.stringify({ login: 'synthetic', password: 'synthetic-password' })
    });
  });
  it.each([
    'http://example.test',
    'https://user:secret@example.test',
    'https://example.test/path',
    'https://example.test/?token=secret',
    'https://example.test/#secret',
    'not-a-url'
  ])('rejects an invalid origin before a request: %s', async (url) => {
    const fetcher = vi.fn();
    vi.stubGlobal('fetch', fetcher);
    await expect(
      passwordLogin(url, { login: 'synthetic', password: 'synthetic-password' })
    ).rejects.not.toThrow('secret');
    expect(fetcher).not.toHaveBeenCalled();
  });
  it.each(['http://localhost:4000', 'http://127.0.0.1:4000', 'http://[::1]:4000'])(
    'permits loopback development: %s',
    async (url) => {
      vi.stubGlobal('fetch', vi.fn().mockResolvedValue(Response.json(response)));
      expect(
        (await passwordLogin(url, { login: 'synthetic', password: 'synthetic-password' })).token
      ).toBe(response.token);
    }
  );
  it.each([
    { login: '', password: 'synthetic-password' },
    { login: 'synthetic', password: '' }
  ])('validates credentials before sending', async (input) => {
    const fetcher = vi.fn();
    vi.stubGlobal('fetch', fetcher);
    await expect(passwordLogin('https://example.test', input)).rejects.toThrow(
      'Enter a username and password'
    );
    expect(fetcher).not.toHaveBeenCalled();
  });
  it.each([401, 403, 500])('keeps response bodies private for HTTP %s', async (status) => {
    vi.stubGlobal(
      'fetch',
      vi.fn().mockResolvedValue(new Response('private-user-and-token', { status }))
    );
    await expect(
      passwordLogin('https://example.test', { login: 'synthetic', password: 'synthetic-password' })
    ).rejects.toMatchObject({ name: 'PasswordLoginError', status });
  });
  it.each(['not-json', '{}', 'x'.repeat(65537)])(
    'rejects malformed, missing, and oversized credentials privately',
    async (body) => {
      vi.stubGlobal('fetch', vi.fn().mockResolvedValue(new Response(body)));
      await expect(
        passwordLogin('https://example.test', {
          login: 'synthetic',
          password: 'synthetic-password'
        })
      ).rejects.toBeInstanceOf(PasswordLoginError);
    }
  );
  it('does not expose transport errors or submitted credentials', async () => {
    vi.stubGlobal('fetch', vi.fn().mockRejectedValue(new Error('private-origin-and-token')));
    await expect(
      passwordLogin('https://example.test', { login: 'synthetic', password: 'synthetic-password' })
    ).rejects.toThrow('Could not sign in to this server');
  });
  it('keeps caller cancellation attached to the whole response read', async () => {
    const controller = new AbortController();
    const fetcher = vi.fn().mockImplementation((_url, options) => {
      expect(options.signal.aborted).toBe(true);
      return Promise.reject(new Error('aborted'));
    });
    vi.stubGlobal('fetch', fetcher);
    controller.abort();
    await expect(
      passwordLogin(
        'https://example.test',
        { login: 'synthetic', password: 'synthetic-password' },
        { signal: controller.signal }
      )
    ).rejects.toBeInstanceOf(PasswordLoginError);
  });
  it('rejects a successful response without a body', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(new Response(null)));
    await expect(
      passwordLogin('https://example.test', { login: 'synthetic', password: 'synthetic-password' })
    ).rejects.toBeInstanceOf(PasswordLoginError);
  });
});
