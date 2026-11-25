/** Exercise authenticated input and durable delivery without external writes. */
import { createHmac } from 'node:crypto';
import { mkdtempSync, rmSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { afterEach, expect, test, vi } from 'vitest';
import { parseAnnouncements } from './announcement-settings.ts';
import { AnnouncementStore } from './announcement-store.ts';
import { parseGitHubWebhook } from './announcement-webhook.ts';
import { announcementLink, deliverAnnouncement } from './announcements.ts';

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
  expect(parseGitHubWebhook(body, headers, config)).toEqual({
    event: push,
    deliveryId: 'delivery-1'
  });
  expect(() => parseGitHubWebhook(Buffer.from('{'), headers, config)).toThrow(
    expect.objectContaining({ status: 401 })
  );
  expect(() => parseGitHubWebhook(body, {}, config)).toThrow(
    expect.objectContaining({ status: 401 })
  );
  expect(() => parseGitHubWebhook(body, headers, { ...config, secret: 'wrong' })).toThrow(
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
    expect(parseGitHubWebhook(body, headers, config)).toBeUndefined();
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
  const result = parseGitHubWebhook(body, headers, config)!;
  if ('kind' in result.event) throw new Error('Expected a push');
  expect(result.event.commits).toHaveLength(50);
  expect(result.event.truncated).toBe(true);
  expect(result.event.commits[0]).toEqual({ id: push.after, message: 'x'.repeat(2000) });
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
    ['Summary', expect.any(String), ''],
    ['Details\n\nlink', expect.any(String), 'root-1']
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
    'Announcement details delivery failed.'
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
  ).rejects.toThrow('Announcement root delivery failed.');
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
    void receiveWebhook(req, res, { ...config, releases: true }, store, controller.signal);
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
    const release = released();
    expect((await fetch(url, { method: 'POST', ...release })).status).toBe(202);
    expect(store.queued).toHaveLength(2);
    expect(store.get(store.queued[1]!)?.release).toMatchObject({ id: 123, kind: 'release' });
    expect((await fetch(url, { method: 'POST', ...release })).status).toBe(202);
    expect(store.queued).toHaveLength(2);
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

const releasePayload = {
  action: 'published',
  repository: { full_name: config.repository },
  release: {
    id: 123,
    tag_name: 'authling/v1.2.0',
    name: 'Authling 1.2.0',
    body: 'New sign-in options.',
    draft: false,
    prerelease: false,
    published_at: '2026-11-16T10:00:00Z',
    target_commitish: 'release-branch',
    html_url: 'https://untrusted.invalid',
    author: { email: 'private@example.test' }
  }
};
function released(value: unknown = releasePayload) {
  const { body, headers } = signed(value);
  return { body, headers: { ...headers, 'x-github-event': 'release' } };
}
function parseRelease(value: unknown = releasePayload) {
  const { body, headers } = released(value);
  return parseGitHubWebhook(body, headers, { ...config, releases: true });
}

test('release events require opt-in, valid signatures and matching repositories', () => {
  const { body, headers } = released();
  expect(parseGitHubWebhook(body, headers, config)).toBeUndefined();
  expect(() =>
    parseGitHubWebhook(Buffer.from('{'), headers, { ...config, releases: true })
  ).toThrow(expect.objectContaining({ status: 401 }));
  expect(
    parseRelease({ ...releasePayload, repository: { full_name: 'other/repo' } })
  ).toBeUndefined();
  for (const action of ['created', 'edited', 'deleted', 'unpublished', 'prereleased']) {
    expect(parseRelease({ ...releasePayload, action })).toBeUndefined();
  }
  expect(
    parseRelease({ ...releasePayload, release: { ...releasePayload.release, draft: true } })
  ).toBeUndefined();
});

test('accepts published prereleases and stable releases regardless of push branch', () => {
  for (const prerelease of [true, false]) {
    const event = parseRelease({
      ...releasePayload,
      release: { ...releasePayload.release, prerelease }
    })!.event;
    expect(event).toEqual({
      kind: 'release',
      id: 123,
      tag: 'authling/v1.2.0',
      name: 'Authling 1.2.0',
      body: 'New sign-in options.',
      prerelease,
      publishedAt: '2026-11-16T10:00:00Z',
      truncated: false
    });
    expect(announcementLink(config.repository, event)).toBe(
      '[Release notes](https://github.com/chattocorp/chatto/releases/tag/authling%2Fv1.2.0)'
    );
  }
  expect(parseRelease({ ...releasePayload, action: 'released' })).toBeDefined();
  expect(
    parseRelease({
      ...releasePayload,
      action: 'released',
      release: { ...releasePayload.release, prerelease: true }
    })
  ).toBeUndefined();
  const malformed = [
    { id: 0 },
    { id: 1.5 },
    { tag_name: '' },
    { tag_name: 'tag\nname' },
    { body: [] },
    { published_at: 'invalid' },
    { draft: undefined },
    { prerelease: 'false' }
  ];
  for (const change of malformed) {
    expect(() =>
      parseRelease({ ...releasePayload, release: { ...releasePayload.release, ...change } })
    ).toThrow(expect.objectContaining({ status: 400 }));
  }
  expect(
    parseRelease({
      ...releasePayload,
      release: { ...releasePayload.release, body: null, name: null }
    })!.event
  ).toMatchObject({ body: '', name: '' });
  expect(
    parseRelease({
      ...releasePayload,
      release: { ...releasePayload.release, body: 'a'.repeat(20001) }
    })!.event
  ).toMatchObject({ body: 'a'.repeat(20000), truncated: true });
});

test('release receipts survive restart, deduplicate publication stages, and deliver one root and reply', async () => {
  const { directory, store } = ledger();
  const pre = parseRelease({
    ...releasePayload,
    release: { ...releasePayload.release, prerelease: true }
  })!.event;
  const stable = parseRelease()!.event;
  expect(store.accept(pre, 'pre')).toBe(true);
  expect(store.accept(pre, 'replay')).toBe(false);
  expect(store.accept(stable, 'stable')).toBe(true);
  expect(store.accept(push, 'push')).toBe(true);
  const restored = new AnnouncementStore(directory, ['server', 'viewer', 'repo', 'main', 'room']);
  expect(
    restored.accept(parseRelease({ ...releasePayload, action: 'released' })!.event, 'released')
  ).toBe(false);
  expect(
    restored.accept(
      parseRelease({
        ...releasePayload,
        release: { ...releasePayload.release, body: 'Edited notes' }
      })!.event,
      'edit'
    )
  ).toBe(false);
  const key = restored.queued[1]!;
  const compose = vi.fn(async () => ({
    summary: '**🚀 Released:** Authling 1.2.0 adds sign-in options.',
    details: '**Highlights**\n- More sign-in options.'
  }));
  const post = vi.fn(async () => ({ id: 'root' }));
  await deliverAnnouncement(
    restored,
    key,
    compose,
    post,
    announcementLink(config.repository, stable)
  );
  expect(compose).toHaveBeenCalledWith(stable);
  expect(post.mock.calls).toHaveLength(2);
  expect(post).toHaveBeenNthCalledWith(
    1,
    '**🚀 Released:** Authling 1.2.0 adds sign-in options. [Release notes](https://github.com/chattocorp/chatto/releases/tag/authling%2Fv1.2.0)',
    expect.any(String),
    ''
  );
  expect(post).toHaveBeenNthCalledWith(
    2,
    '**Highlights**\n- More sign-in options.',
    expect.any(String),
    'root'
  );
  await deliverAnnouncement(restored, key, compose, post, 'unused');
  expect(post).toHaveBeenCalledTimes(2);
});

test('skipped announcements post nothing and remain terminal across restart and redelivery', async () => {
  const { directory, store } = ledger();
  store.accept(push, 'original');
  const key = store.queued[0]!;
  const compose = vi.fn(async () => null);
  const post = vi.fn(async () => ({ id: 'must-not-post' }));
  await deliverAnnouncement(store, key, compose, post, 'link');
  expect(store.get(key)?.status).toBe('skipped');
  expect(store.queued).toEqual([]);
  expect(store.uncertain).toBe(0);
  const restored = new AnnouncementStore(directory, ['server', 'viewer', 'repo', 'main', 'room']);
  expect(restored.accept(push, 'redelivery')).toBe(false);
  expect(restored.queued).toEqual([]);
  await deliverAnnouncement(restored, key, compose, post, 'link');
  expect(compose).toHaveBeenCalledTimes(1);
  expect(post).not.toHaveBeenCalled();
  expect(() => restored.prepare(key, 'Must not post')).toThrow();
});

test('prepared announcements cannot change to a skip on retry', () => {
  const { store } = ledger();
  store.accept(push, 'original');
  const key = store.queued[0]!;
  store.prepare(key, 'Saved summary', 'Saved reply');
  expect(() => store.skip(key)).toThrow('after preparation');
});

test('a confirmed announcement admits untagged follow-ups without a transcript, including after restart', async () => {
  const { rememberParticipation, hasParticipation } = await import('./participation.ts');
  const { conversationIdentity } = await import('./threads.ts');
  const { incomingMessage, hasTranscript } = await import('./messages.ts');
  const { RealtimeEvent } = await import('@chatto/client');
  const { RoomKind } = await import('@chatto/api-types/api/v1/rooms_pb');
  const { directory, store } = ledger();
  const location = { roomId: 'room', threadRootId: 'announcement-root' };
  const identity = conversationIdentity('https://chat.example.test', 'bot', location);
  store.accept(push, 'delivery');
  const key = store.queued[0]!;
  const post = vi.fn(async (_body: string, _key: string, root: string) => {
    if (root) {
      expect(await hasParticipation(directory, identity)).toBe(true);
      throw new Error('Uncertain details delivery');
    }
    return { id: location.threadRootId };
  });
  await expect(
    deliverAnnouncement(
      store,
      key,
      async () => ({ summary: 'Summary', details: 'Details' }),
      post,
      'link',
      (root) => {
        expect(store.get(key)?.status).toBe('sent');
        rememberParticipation(
          directory,
          conversationIdentity('https://chat.example.test', 'bot', {
            ...location,
            threadRootId: root
          })
        );
      }
    )
  ).rejects.toThrow('Announcement details delivery failed.');
  expect(await hasTranscript(directory, identity)).toBe(false);
  const restored = new AnnouncementStore(directory, ['server', 'viewer', 'repo', 'main', 'room']);
  expect(restored.sentRoots).toEqual([location.threadRootId]);
  const isActive = (thread: typeof location) =>
    hasParticipation(directory, conversationIdentity('https://chat.example.test', 'bot', thread));
  const event = new RealtimeEvent({
    id: 'follow-up',
    actorId: 'human',
    event: {
      case: 'messagePosted',
      value: {
        roomId: 'room',
        threadRootEventId: location.threadRootId,
        roomKind: RoomKind.CHANNEL,
        bodyPlaintext: 'Can you explain this change?'
      }
    }
  });
  const server = {
    accountId: 'bot',
    addressedMessage: vi.fn(async () => undefined),
    getMessage: vi.fn()
  };
  expect(
    await incomingMessage(server, event, isActive, new AbortController().signal)
  ).toMatchObject({ body: 'Can you explain this change?', reasons: [] });
  event.actorId = 'bot';
  expect(
    await incomingMessage(server, event, isActive, new AbortController().signal)
  ).toBeUndefined();
  expect(
    await hasParticipation(
      directory,
      conversationIdentity('https://other.example.test', 'bot', location)
    )
  ).toBe(false);
  expect(
    await hasParticipation(
      directory,
      conversationIdentity('https://chat.example.test', 'other-bot', location)
    )
  ).toBe(false);
  expect(await isActive({ ...location, roomId: 'other-room' })).toBe(false);
  expect(await isActive({ ...location, threadRootId: 'other-thread' })).toBe(false);
});

test('startup can register legacy roots and repair a gap after saving the receipt without reposting', async () => {
  const { rememberParticipation, hasParticipation } = await import('./participation.ts');
  const { directory, store } = ledger();
  store.accept(push, 'delivery');
  const key = store.queued[0]!;
  store.prepare(key, 'Legacy single-message announcement');
  store.sending(key);
  store.complete(key, 'old-root');
  const identity = ['server', 'bot', 'room', 'old-root'];
  expect(await hasParticipation(directory, identity)).toBe(false);
  const restored = new AnnouncementStore(directory, ['server', 'viewer', 'repo', 'main', 'room']);
  for (const root of restored.sentRoots)
    rememberParticipation(directory, ['server', 'bot', 'room', root]);
  for (const root of restored.sentRoots)
    rememberParticipation(directory, ['server', 'bot', 'room', root]);
  expect(await hasParticipation(directory, identity)).toBe(true);
  expect(restored.queued).toEqual([]);
  const post = vi.fn(async () => ({ id: 'unexpected' }));
  await deliverAnnouncement(
    restored,
    key,
    async () => ({ summary: 'Unused', details: 'Unused' }),
    post,
    'link'
  );
  expect(post).not.toHaveBeenCalled();
});

test('unconfirmed and skipped announcement roots do not register participation', async () => {
  const { store } = ledger();
  store.accept(push, 'delivery');
  const key = store.queued[0]!;
  const register = vi.fn();
  await deliverAnnouncement(store, key, async () => null, vi.fn(), 'link', register);
  expect(register).not.toHaveBeenCalled();
  expect(store.sentRoots).toEqual([]);
  store.accept({ ...push, after: 'c'.repeat(40) }, 'second');
  const other = store.queued[0]!;
  await expect(
    deliverAnnouncement(
      store,
      other,
      async () => ({ summary: 'Summary', details: 'Details' }),
      async () => {
        throw new Error('Uncertain root');
      },
      'link',
      register
    )
  ).rejects.toThrow('Announcement root delivery failed.');
  expect(register).not.toHaveBeenCalled();
  expect(store.sentRoots).toEqual([]);
});

test('release destinations survive restart while pushes and legacy replies stay in their room', async () => {
  const { directory, store } = ledger();
  const release = {
    kind: 'release' as const,
    id: 900,
    tag: 'v1',
    name: 'v1',
    body: '',
    prerelease: false,
    publishedAt: '',
    truncated: false
  };
  store.accept(push, 'push');
  store.accept(release, 'release');
  const [pushKey, releaseKey] = store.queued as [string, string];
  expect(store.destination(pushKey, 'development', 'announcements')).toBe('development');
  expect(store.destination(releaseKey, 'development', 'announcements')).toBe('announcements');
  store.prepare(releaseKey, 'Release', 'Details');
  store.sending(releaseKey);
  store.complete(releaseKey, 'release-root');
  const restored = new AnnouncementStore(directory, ['server', 'viewer', 'repo', 'main', 'room']);
  expect(restored.destination(releaseKey, 'development', 'different-room')).toBe('announcements');
  expect(restored.sentThreads('development')).toEqual([
    { roomId: 'announcements', threadRootId: 'release-root' }
  ]);
  const post = vi.fn(async () => ({ id: 'details' }));
  await deliverAnnouncement(restored, releaseKey, vi.fn(), post, 'link');
  expect(post).toHaveBeenCalledTimes(1);
  expect(post.mock.calls[0]).toEqual(['Details', expect.any(String), 'release-root']);

  restored.accept({ ...release, id: 901 }, 'old-release');
  const oldKey = restored.queued.find((key) => restored.get(key)?.release?.id === 901)!;
  restored.prepare(oldKey, 'Previously prepared', 'Pending details');
  expect(restored.destination(oldKey, 'development', 'announcements')).toBe('development');
  restored.accept({ ...release, id: 902 }, 'default-release');
  const defaultKey = restored.queued.find((key) => restored.get(key)?.release?.id === 902)!;
  expect(restored.destination(defaultKey, 'development')).toBe('development');
});

test('parses an optional release room without changing the push room', () => {
  const github = {
    enabled: true,
    repository: 'owner/repo',
    room_id: 'development',
    release_room_id: 'announcements',
    webhook_secret: 's'.repeat(32)
  };
  expect(parseAnnouncements({ github }, {})).toMatchObject({
    roomId: 'development',
    releaseRoomId: 'announcements'
  });
  expect(() => parseAnnouncements({ github: { ...github, release_room_id: '' } }, {})).toThrow();
});
