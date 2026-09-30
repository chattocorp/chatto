/** GitHub App installation tokens. The host holds the App's private key and mints a short-lived
 * token for each gh call, limited to the configured repository and to the permissions that the
 * call needs. The private key never leaves this module. */
import { createPrivateKey, sign, type KeyObject } from 'node:crypto';
import { readFileSync } from 'node:fs';
import { resolve } from 'node:path';
import { ConfigurationError, setting } from '../settings.ts';

/** Repository permissions that ChattoBot requests. The App installation must grant each of them. */
export type GitHubPermissions = Partial<
  Record<
    'issues' | 'actions' | 'pull_requests' | 'checks' | 'statuses' | 'metadata',
    'read' | 'write'
  >
>;

/** Read access for every command that is not an approved write. A command that the host
 * classifies wrongly as a read then fails at GitHub instead of changing anything. */
export const READ_PERMISSIONS: GitHubPermissions = {
  issues: 'read',
  actions: 'read',
  pull_requests: 'read',
  checks: 'read',
  statuses: 'read',
  metadata: 'read'
};

/** Host-owned GitHub settings. Chat messages cannot select these values. */
export interface GitHubSettings {
  /** The App's client ID (or numeric App ID), used as the JWT issuer. Not secret. */
  clientId: string;
  privateKey: KeyObject;
  /** The only repository that tokens can access, as `owner/repo`. */
  repository: string;
}

/** Read the optional GitHub App settings. `CHATTO_GITHUB_REPOSITORY` defaults to the
 * implementation repository. Throws a ConfigurationError that names settings, never values. */
export function githubSettings(fallbackRepository?: string): GitHubSettings | undefined {
  const clientId = setting('CHATTO_GITHUB_APP_CLIENT_ID');
  const keyFile = setting('CHATTO_GITHUB_APP_PRIVATE_KEY_FILE');
  if (!clientId && !keyFile) return;
  if (!clientId || !keyFile)
    throw new ConfigurationError(
      'Set both CHATTO_GITHUB_APP_CLIENT_ID and CHATTO_GITHUB_APP_PRIVATE_KEY_FILE, or neither'
    );
  if (!/^(?:Iv[0-9A-Za-z.]{6,64}|\d{1,12})$/.test(clientId))
    throw new ConfigurationError(
      'CHATTO_GITHUB_APP_CLIENT_ID must be a GitHub App client ID or App ID'
    );
  let privateKey: KeyObject;
  try {
    privateKey = createPrivateKey(readFileSync(resolve(keyFile)));
  } catch {
    throw new ConfigurationError(
      'CHATTO_GITHUB_APP_PRIVATE_KEY_FILE must name a readable GitHub App private key (.pem)'
    );
  }
  const repository = setting('CHATTO_GITHUB_REPOSITORY') ?? fallbackRepository;
  if (!repository || !/^[A-Za-z0-9_.-]+\/[A-Za-z0-9_.-]+$/.test(repository))
    throw new ConfigurationError(
      'CHATTO_GITHUB_REPOSITORY must be owner/repo when CHATTO_IMPLEMENTATION_REPOSITORY is unset'
    );
  return { clientId, privateKey, repository };
}

/** A GitHub App request failed. The message names the step and HTTP status, never a response body. */
export class GitHubAppError extends Error {
  override name = 'GitHubAppError';
}

/** Mint installation tokens for one repository. Without `permissions`, a token has all the
 * permissions of the App installation. */
export type TokenSource = (
  permissions: GitHubPermissions | undefined,
  signal: AbortSignal
) => Promise<string>;

const API = 'https://api.github.com';
/** Reuse a token until it has less than this time left. Tokens are valid for one hour. */
const REFRESH_MARGIN_MS = 5 * 60_000;

/** Create a token source with a per-permission-set cache. `request` is injectable for tests. */
export function createTokenSource(
  settings: Pick<GitHubSettings, 'clientId' | 'privateKey' | 'repository'>,
  request: typeof fetch = fetch
): TokenSource {
  const [owner, name] = settings.repository.split('/') as [string, string];
  let installation: Promise<number> | undefined;
  const cache = new Map<string, Promise<{ token: string; expiresAt: number }>>();

  /** A JWT that authenticates as the App itself, valid for at most ten minutes. */
  const appJwt = () => {
    const now = Math.floor(Date.now() / 1000);
    const encode = (value: object) => Buffer.from(JSON.stringify(value)).toString('base64url');
    // Backdate `iat` for clock drift, as GitHub recommends.
    const unsigned = `${encode({ alg: 'RS256', typ: 'JWT' })}.${encode({ iat: now - 60, exp: now + 540, iss: settings.clientId })}`;
    return `${unsigned}.${sign('RSA-SHA256', Buffer.from(unsigned), settings.privateKey).toString('base64url')}`;
  };

  const call = async (step: string, path: string, signal: AbortSignal, body?: object) => {
    const response = await request(`${API}${path}`, {
      method: body ? 'POST' : 'GET',
      headers: {
        Accept: 'application/vnd.github+json',
        Authorization: `Bearer ${appJwt()}`,
        'User-Agent': 'ChattoBot',
        'X-GitHub-Api-Version': '2022-11-28',
        ...(body ? { 'Content-Type': 'application/json' } : {})
      },
      ...(body ? { body: JSON.stringify(body) } : {}),
      signal: AbortSignal.any([signal, AbortSignal.timeout(15_000)])
    });
    if (response.ok) return (await response.json()) as Record<string, unknown>;
    await response.body?.cancel().catch(() => {});
    if (response.status === 404 && step === 'installation')
      throw new GitHubAppError('The GitHub App is not installed on the configured repository.');
    if (response.status === 422)
      throw new GitHubAppError(
        'The GitHub App installation does not grant the permissions that this command needs.'
      );
    throw new GitHubAppError(`The GitHub App ${step} request failed (HTTP ${response.status}).`);
  };

  const installationId = (signal: AbortSignal) => {
    if (installation) return installation;
    const lookup = call('installation', `/repos/${owner}/${name}/installation`, signal).then(
      (value) => {
        if (typeof value.id !== 'number')
          throw new GitHubAppError('GitHub returned no installation ID.');
        return value.id;
      }
    );
    installation = lookup;
    // A failed lookup must not stay cached; the next call tries again.
    lookup.catch(() => {
      if (installation === lookup) installation = undefined;
    });
    return lookup;
  };

  return async (permissions, signal) => {
    const key = permissions ? JSON.stringify(Object.entries(permissions).sort()) : 'installation';
    const cached = cache.get(key);
    if (cached) {
      const value = await cached.catch(() => undefined);
      if (value && value.expiresAt - Date.now() > REFRESH_MARGIN_MS) return value.token;
    }
    const minted = installationId(signal).then(async (id) => {
      const value = await call('token', `/app/installations/${id}/access_tokens`, signal, {
        repositories: [name],
        ...(permissions ? { permissions } : {})
      });
      if (typeof value.token !== 'string' || typeof value.expires_at !== 'string')
        throw new GitHubAppError('GitHub returned no installation token.');
      return { token: value.token, expiresAt: Date.parse(value.expires_at) };
    });
    cache.set(key, minted);
    minted.catch(() => {
      if (cache.get(key) === minted) cache.delete(key);
    });
    return (await minted).token;
  };
}
