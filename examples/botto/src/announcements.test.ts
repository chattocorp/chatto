/** Exercise authenticated input and durable delivery without external writes. */
import { createHmac } from 'node:crypto';
import { mkdtempSync, rmSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { afterEach, expect, test, vi } from 'vitest';
import { parseAnnouncements } from './announcement-settings.ts';
import { AnnouncementStore } from './announcement-store.ts';
import { parsePush } from './announcement-webhook.ts';
import { deliverAnnouncement } from './announcements.ts';

const config = {
  repository: 'chattocorp/chatto',
  branch: 'main',
  roomId: 'room',
  secret: 'a'.repeat(32),
  host: '127.0.0.1',
  port: 5152
};
const push = {
  before: 'a'.repeat(40),
  after: 'b'.repeat(40),
  commits: [{ id: 'b'.repeat(40), message: 'Fix a bug' }],
  truncated: false
};
const payload = { ...push, repository: { full_name: config.repository }, ref: 'refs/heads/main' };
function signed(value: unknown) {
  const body = Buffer.from(JSON.stringify(value));
  return {
    body,
    headers: {
      'x-github-event': 'push',
      'x-github-delivery': 'delivery-1',
      'x-hub-signature-256': `sha256=${createHmac('sha256', config.secret).update(body).digest('hex')}`
    }
  };
}
const directories: string[] = [];
afterEach(() => {
  for (const directory of directories.splice(0))
    rmSync(directory, { recursive: true, force: true });
});
function ledger() {
  const directory = mkdtempSync(join(tmpdir(), 'botto-announcements-'));
  directories.push(directory);
  return {
    directory,
    store: new AnnouncementStore(directory, ['server', 'viewer', 'repo', 'main', 'room'])
  };
}

test('requires the exact signed bytes before parsing malformed JSON', () => {
  const { body, headers } = signed(payload);
  expect(parsePush(body, headers, config)).toEqual({ push, deliveryId: 'delivery-1' });
  expect(() => parsePush(Buffer.from('{'), headers, config)).toThrow(
    expect.objectContaining({ status: 401 })
  );
  expect(() => parsePush(body, {}, config)).toThrow(expect.objectContaining({ status: 401 }));
  expect(() => parsePush(body, headers, { ...config, secret: 'wrong' })).toThrow(
    expect.objectContaining({ status: 401 })
  );
});
test('filters signed pushes by repository and branch and ignores deletions', () => {
  for (const change of [
    { repository: { full_name: 'other/repo' } },
    { ref: 'refs/heads/other' },
    { deleted: true }
  ]) {
    const { body, headers } = signed({ ...payload, ...change });
    expect(parsePush(body, headers, config)).toBeUndefined();
  }
});
test('bounds commit input and excludes author fields', () => {
  const { body, headers } = signed({
    ...payload,
    commits: Array.from({ length: 51 }, () => ({
      ...push.commits[0],
      message: 'x'.repeat(3000),
      author: { email: 'private@example.test' }
    }))
  });
  const result = parsePush(body, headers, config)!;
  expect(result.push.commits).toHaveLength(50);
  expect(result.push.truncated).toBe(true);
  expect(result.push.commits[0]).toEqual({ id: push.after, message: 'x'.repeat(2000) });
});
test('disabled configuration allows missing secrets; enabled configuration validates them', () => {
  const github = {
    enabled: false,
    repository: config.repository,
    room_id: 'room',
    webhook_secret: '${SECRET}'
  };
  expect(parseAnnouncements({ github }, {})).toBeUndefined();
  expect(() => parseAnnouncements({ github: { ...github, enabled: true } }, {})).toThrow(
    'Invalid announcements.github'
  );
  expect(
    parseAnnouncements({ github: { ...github, enabled: true } }, { SECRET: config.secret })
  ).toEqual(config);
});
test('deduplicates signed push facts across delivery headers and restarts', () => {
  const { store, directory } = ledger();
  expect(store.accept(push, 'delivery-1')).toBe(true);
  const restored = new AnnouncementStore(directory, ['server', 'viewer', 'repo', 'main', 'room']);
  expect(restored.accept(push, 'different-header')).toBe(false);
  expect(restored.queued).toEqual(store.queued);
  expect(
    new AnnouncementStore(directory, ['server', 'viewer', 'repo', 'main', 'other-room']).accept(
      push,
      'delivery-1'
    )
  ).toBe(true);
});
test('persists independent root and reply receipts, then suppresses repeated delivery', async () => {
  const { store, directory } = ledger();
  store.accept(push, 'delivery-1');
  const key = store.queued[0]!;
  const compose = vi.fn(async () => ({ summary: 'Summary', details: 'Details' }));
  const post = vi.fn(async (body: string, id: string, threadRootId: string) => {
    const restored = new AnnouncementStore(directory, ['server', 'viewer', 'repo', 'main', 'room']);
    expect(restored.get(key)).toMatchObject(
      threadRootId
        ? {
            status: 'sent',
            messageId: 'root-1',
            detailsStatus: 'sending',
            detailsBody: body,
            detailsIdempotencyKey: id
          }
        : { status: 'sending', body, idempotencyKey: id }
    );
    return { id: threadRootId ? 'reply-1' : 'root-1' };
  });
  await deliverAnnouncement(store, key, compose, post, 'link');
  await deliverAnnouncement(store, key, compose, post, 'link');
  expect(post.mock.calls).toEqual([
    ['Summary\n\nlink', expect.any(String), ''],
    ['Details', expect.any(String), 'root-1']
  ]);
  expect(post.mock.calls[0]![1]).not.toBe(post.mock.calls[1]![1]);
  expect(compose).toHaveBeenCalledOnce();
  expect(store.get(key)).toMatchObject({
    status: 'sent',
    messageId: 'root-1',
    detailsStatus: 'sent',
    detailsMessageId: 'reply-1'
  });
});

test('restart after the root receipt resumes only the queued reply', async () => {
  const { store, directory } = ledger();
  store.accept(push, 'delivery-1');
  const key = store.queued[0]!;
  store.prepare(key, 'Existing root', 'Pending details');
  store.sending(key);
  store.complete(key, 'root-1');
  const restored = new AnnouncementStore(directory, ['server', 'viewer', 'repo', 'main', 'room']);
  expect(restored.queued).toEqual([key]);
  const compose = vi.fn(async () => ({ summary: 'Wrong', details: 'Wrong' }));
  const post = vi.fn(async () => ({ id: 'reply-1' }));
  await deliverAnnouncement(restored, key, compose, post, 'link');
  expect(compose).not.toHaveBeenCalled();
  expect(post).toHaveBeenCalledExactlyOnceWith(
    'Pending details',
    store.get(key)!.detailsIdempotencyKey,
    'root-1'
  );
});

test('an uncertain reply never recreates the root or retries the reply after restart', async () => {
  const { store, directory } = ledger();
  store.accept(push, 'delivery-1');
  const key = store.queued[0]!;
  const compose = async () => ({ summary: 'Summary', details: 'Details' });
  const post = vi.fn(async (_body: string, _id: string, root: string) => {
    if (root) throw new Error('Lost reply response');
    return { id: 'root-1' };
  });
  await expect(deliverAnnouncement(store, key, compose, post, 'link')).rejects.toThrow(
    'Lost reply response'
  );
  const restored = new AnnouncementStore(directory, ['server', 'viewer', 'repo', 'main', 'room']);
  expect(restored.get(key)).toMatchObject({
    status: 'sent',
    messageId: 'root-1',
    detailsStatus: 'sending'
  });
  expect(restored.uncertain).toBe(1);
  expect(restored.queued).toEqual([]);
  await deliverAnnouncement(restored, key, compose, post, 'link');
  expect(post).toHaveBeenCalledTimes(2);
});

test('existing single-message receipts stay complete without creating new details', async () => {
  const { store, directory } = ledger();
  store.accept(push, 'old');
  const key = store.queued[0]!;
  store.prepare(key, 'Legacy announcement');
  store.sending(key);
  store.complete(key, 'old-root');
  const restored = new AnnouncementStore(directory, ['server', 'viewer', 'repo', 'main', 'room']);
  const post = vi.fn();
  const compose = vi.fn();
  await deliverAnnouncement(restored, key, compose, post, 'link');
  expect(restored.queued).toEqual([]);
  expect(compose).not.toHaveBeenCalled();
  expect(post).not.toHaveBeenCalled();
});
test('model failures stay queued but uncertain writes never retry after restart', async () => {
  const { store, directory } = ledger();
  store.accept(push, 'delivery-1');
  const key = store.queued[0]!;
  const post = vi.fn(async () => {
    throw new Error('lost response');
  });
  await expect(
    deliverAnnouncement(
      store,
      key,
      async () => {
        throw new Error('model failed');
      },
      post,
      'link'
    )
  ).rejects.toThrow('model failed');
  expect(store.queued).toEqual([key]);
  expect(post).not.toHaveBeenCalled();
  await expect(
    deliverAnnouncement(
      store,
      key,
      async () => ({ summary: 'Summary', details: 'Details' }),
      post,
      'link'
    )
  ).rejects.toThrow('lost response');
  const restored = new AnnouncementStore(directory, ['server', 'viewer', 'repo', 'main', 'room']);
  expect(restored.uncertain).toBe(1);
  expect(restored.queued).toEqual([]);
  await deliverAnnouncement(
    restored,
    key,
    async () => ({ summary: 'Summary', details: 'Details' }),
    post,
    'link'
  );
  expect(post).toHaveBeenCalledTimes(1);
});

test('HTTP receiver persists valid requests, rejects invalid signatures and oversized bodies', async () => {
  const { createServer } = await import('node:http');
  const { receiveWebhook } = await import('./announcement-webhook.ts');
  const { store } = ledger();
  const controller = new AbortController();
  const server = createServer((req, res) => {
    void receiveWebhook(req, res, config, store, controller.signal);
  });
  try {
    await new Promise<void>((resolve, reject) => {
      server.once('error', reject);
      server.listen(0, '127.0.0.1', resolve);
    });
    const address = server.address() as import('node:net').AddressInfo;
    const url = `http://127.0.0.1:${address.port}/github`;
    const { body, headers } = signed(payload);
    expect((await fetch(url, { method: 'POST', body, headers })).status).toBe(202);
    expect(store.queued).toHaveLength(1);
    expect(
      (
        await fetch(url, {
          method: 'POST',
          body,
          headers: { ...headers, 'x-github-delivery': 'replayed' }
        })
      ).status
    ).toBe(202);
    expect(store.queued).toHaveLength(1);
    expect((await fetch(url, { method: 'POST', body })).status).toBe(401);
    expect((await fetch(url, { method: 'POST', body: Buffer.alloc(1024 * 1024 + 1) })).status).toBe(
      413
    );
    expect((await fetch(url, { method: 'GET' })).status).toBe(404);
    controller.abort();
    expect((await fetch(url, { method: 'POST', body, headers })).status).toBe(503);
  } finally {
    server.closeAllConnections();
    await new Promise<void>((resolve) => server.close(() => resolve()));
  }
});
