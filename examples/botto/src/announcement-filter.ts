/** Check immutable GitHub changes before spending model tokens. Never treat failed reads as skips. */
import { createSign } from 'node:crypto';
import { readFile } from 'node:fs/promises';
import type { AnnouncementSettings } from './announcement-settings.ts';
import type { PushSummary } from './announcement-store.ts';

type JsonObject = Record<string, unknown>;
function object(value: unknown): JsonObject {
  if (!value || typeof value !== 'object' || Array.isArray(value))
    throw new Error('Invalid GitHub change response.');
  return value as JsonObject;
}

/** Filter credentials are explicit and independent of agent tool grants. */
export interface ChangeFilter {
  pattern: string;
  appId: string;
  installationId: string;
  privateKeyPath: string;
}

/** Mint a repository-scoped, read-only installation token; do not retain or log it. */
async function installationToken(
  config: AnnouncementSettings,
  signal: AbortSignal,
  request: typeof fetch
): Promise<string> {
  const auth = config.changeFilter!;
  const now = Math.floor(Date.now() / 1000);
  const encode = (value: unknown) => Buffer.from(JSON.stringify(value)).toString('base64url');
  const claims =
    encode({ alg: 'RS256', typ: 'JWT' }) +
    '.' +
    encode({ iat: now - 60, exp: now + 540, iss: auth.appId });
  const key = await readFile(auth.privateKeyPath, { signal });
  const signature = createSign('RSA-SHA256').update(claims).sign(key, 'base64url');
  const response = await request(
    `https://api.github.com/app/installations/${auth.installationId}/access_tokens`,
    {
      method: 'POST',
      signal,
      redirect: 'error',
      headers: {
        Authorization: `Bearer ${claims}.${signature}`,
        Accept: 'application/vnd.github+json',
        'Content-Type': 'application/json'
      },
      body: JSON.stringify({
        repositories: [config.repository.split('/')[1]],
        permissions: { contents: 'read' }
      })
    }
  );
  if (!response.ok) throw new Error('GitHub change authentication failed.');
  const data = object(await response.json());
  if (typeof data.token !== 'string' || !data.token)
    throw new Error('Invalid GitHub change authentication.');
  return data.token;
}

/** True means a matching file exists at the new head and differs from the old head. */
export async function hasReportableChanges(
  config: AnnouncementSettings,
  push: PushSummary,
  signal: AbortSignal,
  request: typeof fetch = fetch
): Promise<boolean> {
  if (!config.changeFilter) return true;
  signal.throwIfAborted();
  const token = await installationToken(config, signal, request);
  const matches = new RegExp(config.changeFilter.pattern);
  const base = `https://api.github.com/repos/${config.repository}`;
  const get = async (path: string) => {
    const response = await request(base + path, {
      signal,
      redirect: 'error',
      headers: { Authorization: `Bearer ${token}`, Accept: 'application/vnd.github+json' }
    });
    if (!response.ok) throw new Error('GitHub change lookup failed.');
    return object(await response.json());
  };
  if (push.before !== '0'.repeat(40)) {
    const comparison = await get(`/compare/${push.before}...${push.after}?per_page=1`);
    const mergeBase = object(comparison.merge_base_commit);
    // Compare uses the merge base. Divergent pushes need a direct tree comparison.
    if (mergeBase.sha === push.before) {
      if (!Array.isArray(comparison.files)) throw new Error('Missing GitHub changed files.');
      for (const raw of comparison.files) {
        const file = object(raw);
        if (typeof file.filename !== 'string' || typeof file.status !== 'string')
          throw new Error('Invalid GitHub changed file.');
        if (
          ['added', 'modified', 'renamed', 'changed', 'copied'].includes(file.status) &&
          matches.test(file.filename)
        )
          return true;
      }
      // GitHub caps comparison files at 300, even when commit pages are requested.
      if (comparison.files.length < 300) return false;
    }
  }
  const tree = async (commit: string): Promise<Map<string, string>> => {
    if (commit === '0'.repeat(40)) return new Map();
    const commitData = await get(`/git/commits/${commit}`);
    const treeId = object(commitData.tree).sha;
    if (typeof treeId !== 'string' || !/^[a-f0-9]{40}$/.test(treeId))
      throw new Error('Invalid GitHub tree identity.');
    const data = await get(`/git/trees/${treeId}?recursive=1`);
    if (data.truncated !== false || !Array.isArray(data.tree))
      throw new Error('Incomplete GitHub change tree; announcement remains queued.');
    const files = new Map<string, string>();
    for (const raw of data.tree) {
      const entry = object(raw);
      if (
        typeof entry.path !== 'string' ||
        typeof entry.sha !== 'string' ||
        typeof entry.type !== 'string'
      )
        throw new Error('Invalid GitHub tree entry.');
      if (entry.type === 'blob' && matches.test(entry.path)) files.set(entry.path, entry.sha);
    }
    return files;
  };
  const before = await tree(push.before);
  const after = await tree(push.after);
  return [...after].some(([path, sha]) => before.get(path) !== sha);
}
