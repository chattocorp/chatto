/** Verify pre-model eligibility against complete immutable changes and safe authentication. */
import { generateKeyPairSync, verify } from 'node:crypto';
import { expect, test, vi } from 'vitest';
import { hasReportableChanges } from './announcement-filter.ts';
import { parseAnnouncements } from './announcement-settings.ts';

const keys = generateKeyPairSync('rsa', { modulusLength: 2048 });
vi.mock('node:fs/promises', () => ({
  readFile: vi.fn(async () => keys.privateKey.export({ type: 'pkcs8', format: 'pem' }))
}));

const config = {
  repository: 'owner/repo',
  branch: 'main',
  roomId: 'room',
  secret: 's'.repeat(32),
  host: '127.0.0.1',
  port: 5152,
  changeFilter: {
    pattern: '(^|/)\\.sweet/[^/]+\\.md$',
    appId: '123',
    installationId: '456',
    privateKeyPath: '/synthetic.pem'
  }
};
const push = { before: 'a'.repeat(40), after: 'b'.repeat(40), commits: [], truncated: false };
const signal = new AbortController().signal;
function responses(...values: unknown[]) {
  return vi.fn<typeof fetch>(async () => {
    const value = values.shift();
    if (value === undefined) throw new Error('Unexpected request');
    return value instanceof Response ? value : Response.json(value);
  });
}
const comparison = (files: unknown[]) => ({ merge_base_commit: { sha: push.before }, files });

test('checks added, modified and renamed note paths; excludes removals and unrelated files', async () => {
  for (const [filename, status, expected] of [
    ['.sweet/new.md', 'added', true],
    ['packages/runling/.sweet/fix.md', 'modified', true],
    ['authling/.sweet/name.md', 'renamed', true],
    ['.sweet/old.md', 'removed', false],
    ['.sweet.yml', 'modified', false],
    ['docs/not.sweet/note.md', 'added', false],
    ['.sweet/metadata.json', 'added', false]
  ] as const) {
    const request = responses({ token: 'installation-token' }, comparison([{ filename, status }]));
    expect(await hasReportableChanges(config, push, signal, request)).toBe(expected);
    expect(request.mock.calls[1]![0]).toContain('/compare/' + push.before + '...' + push.after);
    const auth = request.mock.calls[0]![1]!;
    expect(JSON.parse(auth.body as string)).toEqual({
      repositories: ['repo'],
      permissions: { contents: 'read' }
    });
    const jwt = (auth.headers as Record<string, string>).Authorization!.slice(7).split('.');
    expect(
      verify(
        'RSA-SHA256',
        Buffer.from(jwt[0] + '.' + jwt[1]),
        keys.publicKey,
        Buffer.from(jwt[2]!, 'base64url')
      )
    ).toBe(true);
    expect(request.mock.calls[1]![1]).toMatchObject({
      redirect: 'error',
      headers: { Authorization: 'Bearer installation-token' }
    });
  }
});

test('uses exact trees when compare files hit the limit or the push diverges', async () => {
  for (const compare of [
    comparison(
      Array.from({ length: 300 }, (_, i) => ({ filename: 'src/' + i, status: 'modified' }))
    ),
    {
      merge_base_commit: { sha: 'c'.repeat(40) },
      files: [{ filename: '.sweet/false-positive.md', status: 'added' }]
    }
  ]) {
    for (const changed of [true, false]) {
      const request = responses(
        { token: 'token' },
        compare,
        { tree: { sha: 'd'.repeat(40) } },
        { truncated: false, tree: [{ path: '.sweet/note.md', sha: 'old', type: 'blob' }] },
        { tree: { sha: 'e'.repeat(40) } },
        {
          truncated: false,
          tree: [{ path: '.sweet/note.md', sha: changed ? 'new' : 'old', type: 'blob' }]
        }
      );
      expect(await hasReportableChanges(config, push, signal, request)).toBe(changed);
      expect(request).toHaveBeenCalledTimes(6);
    }
  }
});

test('branch creation checks the new tree; incomplete data and API failures never mean skip', async () => {
  const created = { ...push, before: '0'.repeat(40) };
  const request = responses(
    { token: 'token' },
    { tree: { sha: 'd'.repeat(40) } },
    { truncated: false, tree: [{ path: '.sweet/new.md', sha: 'new', type: 'blob' }] }
  );
  expect(await hasReportableChanges(config, created, signal, request)).toBe(true);
  for (const value of [
    { truncated: true, tree: [] },
    { tree: [] },
    new Response('secret failure', { status: 403 })
  ]) {
    await expect(
      hasReportableChanges(
        config,
        created,
        signal,
        responses({ token: 'token' }, { tree: { sha: 'd'.repeat(40) } }, value)
      )
    ).rejects.toThrow();
  }
  await expect(
    hasReportableChanges(
      config,
      push,
      signal,
      responses(new Response('secret failure', { status: 401 }))
    )
  ).rejects.toThrow('authentication failed');
  await expect(
    hasReportableChanges(
      config,
      push,
      signal,
      responses({ token: 'token' }, comparison([{ filename: '.sweet/note.md' }]))
    )
  ).rejects.toThrow();
  const aborted = new AbortController();
  aborted.abort();
  const unused = responses();
  await expect(hasReportableChanges(config, push, aborted.signal, unused)).rejects.toThrow();
  expect(unused).not.toHaveBeenCalled();
  expect(
    await hasReportableChanges({ ...config, changeFilter: undefined }, push, signal, unused)
  ).toBe(true);
});

test('validates filter settings and resolves existing credential references without reading keys', () => {
  const github = {
    enabled: true,
    repository: 'owner/repo',
    room_id: 'room',
    webhook_secret: 's'.repeat(32),
    required_changed_path: '(^|/)\\.sweet/[^/]+\\.md$',
    github_app_id: '${APP}',
    github_installation_id: '456',
    github_private_key_path: 'key.pem'
  };
  expect(parseAnnouncements({ github }, { APP: '123' }, '/config')?.changeFilter).toEqual({
    pattern: github.required_changed_path,
    appId: '123',
    installationId: '456',
    privateKeyPath: '/config/key.pem'
  });
  for (const changes of [
    { required_changed_path: '[' },
    { github_app_id: 'bad' },
    { github_installation_id: undefined },
    { github_private_key_path: undefined }
  ]) {
    expect(() =>
      parseAnnouncements({ github: { ...github, ...changes } }, { APP: '123' })
    ).toThrow();
  }
});
