/** Verify raw GitHub requests before parsing or retaining their contents. */
import { createHmac, timingSafeEqual } from 'node:crypto';
import type { IncomingHttpHeaders, IncomingMessage, ServerResponse } from 'node:http';
import type { AnnouncementSettings } from './announcement-settings.ts';
import type { AnnouncementStore, PushSummary, ReleaseSummary } from './announcement-store.ts';

export class WebhookError extends Error {
  constructor(readonly status: number) {
    super('Webhook request rejected');
  }
}

/** Verify signatures before accepting repository-matched push or release evidence. */
export function parseGitHubWebhook(
  body: Buffer,
  headers: IncomingHttpHeaders,
  settings: AnnouncementSettings
): { event: PushSummary | ReleaseSummary; deliveryId: string } | undefined {
  const signature = headers['x-hub-signature-256'];
  if (typeof signature !== 'string' || !/^sha256=[a-f0-9]{64}$/.test(signature))
    throw new WebhookError(401);
  const expected = createHmac('sha256', settings.secret).update(body).digest();
  if (!timingSafeEqual(expected, Buffer.from(signature.slice(7), 'hex')))
    throw new WebhookError(401);
  if (headers['x-github-event'] === 'ping') return;
  const eventType = headers['x-github-event'];
  if (eventType !== 'push' && !(eventType === 'release' && settings.releases)) return;
  const deliveryId = headers['x-github-delivery'];
  if (typeof deliveryId !== 'string' || !/^[A-Za-z0-9-]{1,100}$/.test(deliveryId))
    throw new WebhookError(400);
  let decoded: unknown;
  try {
    decoded = JSON.parse(body.toString('utf8'));
  } catch {
    throw new WebhookError(400);
  }
  if (!decoded || typeof decoded !== 'object' || Array.isArray(decoded))
    throw new WebhookError(400);
  const value = decoded as Record<string, unknown>;
  const repository = value.repository as { full_name?: unknown } | undefined;
  if (repository?.full_name !== settings.repository) return;
  if (eventType === 'release') {
    if (value.action !== 'published' && value.action !== 'released') return;
    const raw = value.release;
    if (!raw || typeof raw !== 'object' || Array.isArray(raw)) throw new WebhookError(400);
    const release = raw as Record<string, unknown>;
    if (release.draft === true) return;
    if (
      release.draft !== false ||
      typeof release.prerelease !== 'boolean' ||
      typeof release.id !== 'number' ||
      !Number.isSafeInteger(release.id) ||
      release.id <= 0 ||
      typeof release.tag_name !== 'string' ||
      !release.tag_name ||
      release.tag_name.length > 200 ||
      /[\u0000-\u0020\u007f]/.test(release.tag_name) ||
      (release.name !== null && typeof release.name !== 'string') ||
      (release.body !== null && typeof release.body !== 'string') ||
      typeof release.published_at !== 'string' ||
      !Number.isFinite(Date.parse(release.published_at))
    )
      throw new WebhookError(400);
    if (value.action === 'released' && release.prerelease) return;
    const notes = release.body ?? '';
    return {
      deliveryId,
      event: {
        kind: 'release',
        id: release.id,
        tag: release.tag_name,
        name: (release.name ?? '').slice(0, 300),
        body: notes.slice(0, 20000),
        prerelease: release.prerelease,
        publishedAt: release.published_at,
        truncated: notes.length > 20000
      }
    };
  }
  if (value.ref !== `refs/heads/${settings.branch}` || value.deleted === true) return;
  const sha = (s: unknown): s is string => typeof s === 'string' && /^[a-f0-9]{40}$/.test(s);
  if (
    !sha(value.before) ||
    !sha(value.after) ||
    /^0+$/.test(value.after) ||
    !Array.isArray(value.commits)
  )
    throw new WebhookError(400);
  if (value.before === value.after) return;
  const commits: PushSummary['commits'] = [];
  for (const item of value.commits.slice(0, 50) as unknown[]) {
    const commit = item as { id?: unknown; message?: unknown } | null;
    if (!commit || !sha(commit.id) || typeof commit.message !== 'string')
      throw new WebhookError(400);
    commits.push({ id: commit.id, message: commit.message.slice(0, 2000) });
  }
  return {
    deliveryId,
    event: {
      before: value.before,
      after: value.after,
      commits,
      truncated: value.commits.length > 50
    }
  };
}

/** Accept only bounded signed requests and acknowledge only after durable queue insertion. */
export async function receiveWebhook(
  request: IncomingMessage,
  response: ServerResponse,
  settings: AnnouncementSettings,
  store: AnnouncementStore,
  signal: AbortSignal
): Promise<void> {
  try {
    if (request.method !== 'POST' || request.url !== '/github') throw new WebhookError(404);
    if (signal.aborted) throw new WebhookError(503);
    const chunks: Buffer[] = [];
    let size = 0;
    // Keep the request alive long enough to return a useful rejection for oversized bodies.
    for await (const chunk of request.iterator({ destroyOnReturn: false })) {
      const bytes = Buffer.from(chunk);
      size += bytes.length;
      if (size > 1024 * 1024) throw new WebhookError(413);
      chunks.push(bytes);
    }
    signal.throwIfAborted();
    const input = parseGitHubWebhook(Buffer.concat(chunks), request.headers, settings);
    if (input) store.accept(input.event, input.deliveryId);
    response.writeHead(202).end();
  } catch (error) {
    response
      .writeHead(error instanceof WebhookError ? error.status : 503, { Connection: 'close' })
      .end();
  }
}
