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
  /** Optional expression applied to the complete immutable file content. */
  contentPattern?: string;
  /** Accept only paths absent at the previous push head. */
  addedOnly?: boolean;
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

/** Matched file evidence fetched at the immutable new head, never from the current branch. */
export interface ChangedFile {
  path: string;
  content: string;
}

/** Undefined means filtering is disabled; an empty list is a durable skip. Read failures must retry. */
export async function loadChangedFiles(
  config: AnnouncementSettings,
  push: PushSummary,
  signal: AbortSignal,
  request: typeof fetch = fetch
): Promise<ChangedFile[] | undefined> {
  if (!config.changeFilter) return;
  signal.throwIfAborted();
  const token = await installationToken(config, signal, request);
  const matches = new RegExp(config.changeFilter.pattern);
  const contentMatches = config.changeFilter.contentPattern
    ? new RegExp(config.changeFilter.contentPattern)
    : undefined;
  const addedOnly = config.changeFilter.addedOnly === true;
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
  let paths: string[] | undefined;
  if (push.before !== '0'.repeat(40)) {
    const comparison = await get(`/compare/${push.before}...${push.after}?per_page=1`);
    const mergeBase = object(comparison.merge_base_commit);
    // Compare uses the merge base. Divergent pushes need a direct tree comparison.
    if (mergeBase.sha === push.before) {
      if (!Array.isArray(comparison.files)) throw new Error('Missing GitHub changed files.');
      const candidates: string[] = [];
      for (const raw of comparison.files) {
        const file = object(raw);
        if (typeof file.filename !== 'string' || typeof file.status !== 'string')
          throw new Error('Invalid GitHub changed file.');
        if (
          (addedOnly
            ? file.status === 'added'
            : ['added', 'modified', 'renamed', 'changed', 'copied'].includes(file.status)) &&
          matches.test(file.filename)
        )
          candidates.push(file.filename);
      }
      // GitHub caps comparison files at 300, even when commit pages are requested.
      if (comparison.files.length < 300) paths = candidates;
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
  if (paths === undefined) {
    const before = await tree(push.before);
    const after = await tree(push.after);
    paths = [...after]
      .filter(([path, sha]) => (addedOnly ? !before.has(path) : before.get(path) !== sha))
      .map(([path]) => path);
  }
  // Do not silently omit or truncate notes: incomplete evidence stays queued for review/retry.
  if (paths.length > 100) throw new Error('Too many announcement source files.');
  const files: ChangedFile[] = [];
  let totalBytes = 0;
  for (const path of [...new Set(paths)].sort()) {
    const encodedPath = path.split('/').map(encodeURIComponent).join('/');
    const file = await get(`/contents/${encodedPath}?ref=${push.after}`);
    if (
      file.type !== 'file' ||
      file.encoding !== 'base64' ||
      typeof file.content !== 'string' ||
      typeof file.size !== 'number' ||
      !Number.isSafeInteger(file.size) ||
      file.size < 0 ||
      file.size > 65536 ||
      file.content.length > 100000
    )
      throw new Error('Invalid or oversized announcement source file.');
    const bytes = Buffer.from(file.content, 'base64');
    totalBytes += bytes.length;
    if (bytes.length !== file.size || totalBytes > 262144)
      throw new Error('Incomplete or oversized announcement source files.');
    const content = new TextDecoder('utf-8', { fatal: true }).decode(bytes);
    if (!contentMatches || contentMatches.test(content)) files.push({ path, content });
  }
  return files;
}
