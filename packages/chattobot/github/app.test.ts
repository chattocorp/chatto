import { generateKeyPairSync, verify } from 'node:crypto';
import { expect, test, vi } from 'vitest';
import { createTokenSource, GitHubAppError } from './app.ts';

const { privateKey, publicKey } = generateKeyPairSync('rsa', { modulusLength: 2048 });
const settings = { clientId: 'Iv23liExample1', privateKey, repository: 'chattocorp/chatto' };

function github(
  options: { installation?: number; tokenStatus?: number; expiresInMs?: number } = {}
) {
  const requests: { url: string; init: RequestInit }[] = [];
  let minted = 0;
  const request = vi.fn(async (url: string | URL | Request, init?: RequestInit) => {
    requests.push({ url: String(url), init: init ?? {} });
    if (String(url).endsWith('/installation'))
      return options.installation === 404
        ? new Response('{}', { status: 404 })
        : Response.json({ id: options.installation ?? 77 });
    if (options.tokenStatus) return new Response('{}', { status: options.tokenStatus });
    minted++;
    return Response.json({
      token: `ghs_token${minted}`,
      expires_at: new Date(Date.now() + (options.expiresInMs ?? 3_600_000)).toISOString()
    });
  });
  return { request: request as unknown as typeof fetch, requests };
}

test('mints a repository-scoped token for the requested permissions with an App JWT', async () => {
  const { request, requests } = github();
  const tokens = createTokenSource(settings, request);
  expect(await tokens({ issues: 'read' }, AbortSignal.timeout(1000))).toBe('ghs_token1');
  expect(requests.map((entry) => entry.url)).toEqual([
    'https://api.github.com/repos/chattocorp/chatto/installation',
    'https://api.github.com/app/installations/77/access_tokens'
  ]);
  expect(JSON.parse(String(requests[1]!.init.body))).toEqual({
    repositories: ['chatto'],
    permissions: { issues: 'read' }
  });
  const jwt = (requests[0]!.init.headers as Record<string, string>).Authorization!.replace(
    'Bearer ',
    ''
  );
  const [header, payload, signature] = jwt.split('.') as [string, string, string];
  expect(
    verify(
      'RSA-SHA256',
      Buffer.from(`${header}.${payload}`),
      publicKey,
      Buffer.from(signature, 'base64url')
    )
  ).toBe(true);
  const claims = JSON.parse(Buffer.from(payload, 'base64url').toString());
  expect(claims.iss).toBe('Iv23liExample1');
  expect(claims.exp - claims.iat).toBeLessThanOrEqual(600);
});

test('caches tokens per permission set until they near expiry', async () => {
  const { request, requests } = github();
  const tokens = createTokenSource(settings, request);
  const signal = AbortSignal.timeout(1000);
  expect(await tokens({ issues: 'read', actions: 'read' }, signal)).toBe('ghs_token1');
  expect(await tokens({ actions: 'read', issues: 'read' }, signal)).toBe('ghs_token1');
  expect(await tokens({ issues: 'write' }, signal)).toBe('ghs_token2');
  // One installation lookup, two tokens.
  expect(requests).toHaveLength(3);

  const expiring = github({ expiresInMs: 60_000 });
  const refreshing = createTokenSource(settings, expiring.request);
  await refreshing({ issues: 'read' }, signal);
  expect(await refreshing({ issues: 'read' }, signal)).toBe('ghs_token2');
});

test('reports installation and permission failures without response bodies', async () => {
  const signal = AbortSignal.timeout(1000);
  await expect(
    createTokenSource(settings, github({ installation: 404 }).request)({}, signal)
  ).rejects.toThrow('The GitHub App is not installed on the configured repository.');
  const denied = createTokenSource(settings, github({ tokenStatus: 422 }).request);
  await expect(denied({ actions: 'write' }, signal)).rejects.toBeInstanceOf(GitHubAppError);
  await expect(denied({ actions: 'write' }, signal)).rejects.toThrow(
    'does not grant the permissions'
  );
});

test('a token without requested permissions gets the installation grant', async () => {
  const { request, requests } = github();
  await createTokenSource(settings, request)(undefined, AbortSignal.timeout(1000));
  expect(JSON.parse(String(requests[1]!.init.body))).toEqual({ repositories: ['chatto'] });
});
