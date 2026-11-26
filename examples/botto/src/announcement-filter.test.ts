/** Verify pre-model eligibility against complete immutable changes and safe authentication. */
import { generateKeyPairSync, verify } from 'node:crypto';
import { expect, test, vi } from 'vitest';
import { loadChangedFiles } from './announcement-filter.ts';
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
const noteContent = '---\ntype: feature\n---\nA useful change.';
const note = {
  type: 'file',
  encoding: 'base64',
  size: Buffer.byteLength(noteContent),
  content: Buffer.from(noteContent).toString('base64')
};
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
    const request = responses(
      { token: 'installation-token' },
      comparison([{ filename, status }]),
      ...(expected ? [note] : [])
    );
    expect(await loadChangedFiles(config, push, signal, request)).toEqual(
      expected ? [{ path: filename, content: noteContent }] : []
    );
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
        },
        ...(changed ? [note] : [])
      );
      expect(await loadChangedFiles(config, push, signal, request)).toEqual(
        changed ? [{ path: '.sweet/note.md', content: noteContent }] : []
      );
      expect(request).toHaveBeenCalledTimes(changed ? 7 : 6);
    }
  }
});

test('branch creation checks the new tree; incomplete data and API failures never mean skip', async () => {
  const created = { ...push, before: '0'.repeat(40) };
  const request = responses(
    { token: 'token' },
    { tree: { sha: 'd'.repeat(40) } },
    { truncated: false, tree: [{ path: '.sweet/new.md', sha: 'new', type: 'blob' }] },
    note
  );
  expect(await loadChangedFiles(config, created, signal, request)).toEqual([
    { path: '.sweet/new.md', content: noteContent }
  ]);
  for (const value of [
    { truncated: true, tree: [] },
    { tree: [] },
    new Response('secret failure', { status: 403 })
  ]) {
    await expect(
      loadChangedFiles(
        config,
        created,
        signal,
        responses({ token: 'token' }, { tree: { sha: 'd'.repeat(40) } }, value)
      )
    ).rejects.toThrow();
  }
  await expect(
    loadChangedFiles(
      config,
      push,
      signal,
      responses(new Response('secret failure', { status: 401 }))
    )
  ).rejects.toThrow('authentication failed');
  await expect(
    loadChangedFiles(
      config,
      push,
      signal,
      responses({ token: 'token' }, comparison([{ filename: '.sweet/note.md' }]))
    )
  ).rejects.toThrow();
  const aborted = new AbortController();
  aborted.abort();
  const unused = responses();
  await expect(loadChangedFiles(config, push, aborted.signal, unused)).rejects.toThrow();
  expect(unused).not.toHaveBeenCalled();
  expect(
    await loadChangedFiles({ ...config, changeFilter: undefined }, push, signal, unused)
  ).toBeUndefined();
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
  expect(
    parseAnnouncements(
      {
        github: {
          ...github,
          required_changed_content: '^---',
          added_files_only: true
        }
      },
      { APP: '123' }
    )?.changeFilter
  ).toMatchObject({
    contentPattern: '^---',
    addedOnly: true
  });
  for (const changes of [
    { required_changed_path: '[' },
    { required_changed_content: '[' },
    { added_files_only: 'yes' },
    { required_changed_path: undefined, added_files_only: true },
    { required_changed_path: undefined, required_changed_content: 'highlight' },
    { github_app_id: 'bad' },
    { github_installation_id: undefined },
    { github_private_key_path: undefined }
  ]) {
    expect(() =>
      parseAnnouncements({ github: { ...github, ...changes } }, { APP: '123' })
    ).toThrow();
  }
});

test('fetches all matched notes at the new head and excludes source diffs and payload URLs', async () => {
  const request = responses(
    { token: 'token' },
    comparison([
      {
        filename: 'authling/.sweet/z note.md',
        status: 'modified',
        raw_url: 'https://untrusted.invalid'
      },
      { filename: '.sweet/a.md', status: 'added', patch: 'implementation details' },
      { filename: '.sweet/deleted.md', status: 'removed' },
      { filename: 'src/app.ts', status: 'modified' }
    ]),
    note,
    { ...note, content: Buffer.from('Second note').toString('base64'), size: 11 }
  );
  expect(await loadChangedFiles(config, push, signal, request)).toEqual([
    { path: '.sweet/a.md', content: noteContent },
    { path: 'authling/.sweet/z note.md', content: 'Second note' }
  ]);
  expect(request.mock.calls.slice(2).map(([url]) => url)).toEqual([
    'https://api.github.com/repos/owner/repo/contents/.sweet/a.md?ref=' + push.after,
    'https://api.github.com/repos/owner/repo/contents/authling/.sweet/z%20note.md?ref=' + push.after
  ]);
});

test('incomplete comparisons collect matching notes beyond the first 300 files', async () => {
  const request = responses(
    { token: 'token' },
    comparison([
      { filename: '.sweet/first.md', status: 'added' },
      ...Array.from({ length: 299 }, (_, i) => ({ filename: 'src/' + i, status: 'modified' }))
    ]),
    { tree: { sha: 'd'.repeat(40) } },
    { truncated: false, tree: [] },
    { tree: { sha: 'e'.repeat(40) } },
    {
      truncated: false,
      tree: [
        { path: '.sweet/first.md', sha: 'one', type: 'blob' },
        { path: '.sweet/last.md', sha: 'two', type: 'blob' }
      ]
    },
    note,
    note
  );
  expect(await loadChangedFiles(config, push, signal, request)).toEqual([
    { path: '.sweet/first.md', content: noteContent },
    { path: '.sweet/last.md', content: noteContent }
  ]);
});

test('missing, oversized, invalid UTF-8 and incomplete note contents fail before model input', async () => {
  for (const file of [
    new Response('not found', { status: 404 }),
    { ...note, type: 'symlink' },
    { ...note, size: 65537 },
    { ...note, size: 1 },
    { ...note, encoding: 'none' },
    { ...note, size: 1, content: '/w==' }
  ]) {
    await expect(
      loadChangedFiles(
        config,
        push,
        signal,
        responses(
          { token: 'token' },
          comparison([{ filename: '.sweet/a.md', status: 'added' }]),
          file
        )
      )
    ).rejects.toThrow();
  }
  const large = 'x'.repeat(65536);
  await expect(
    loadChangedFiles(
      config,
      push,
      signal,
      responses(
        { token: 'token' },
        comparison(
          Array.from({ length: 5 }, (_, i) => ({
            filename: '.sweet/' + i + '.md',
            status: 'added'
          }))
        ),
        ...Array.from({ length: 5 }, () => ({
          ...note,
          size: 65536,
          content: Buffer.from(large).toString('base64')
        }))
      )
    )
  ).rejects.toThrow('oversized');
});

const highlightFilter = {
  ...config,
  changeFilter: {
    ...config.changeFilter,
    addedOnly: true,
    contentPattern: String.raw`^---\r?\n(?:(?!---\r?(?:\n|$))[^\n]*\n)*?type:[ \t]*(?:highlight|"highlight"|\x27highlight\x27)[ \t]*(?:#[^\r\n]*)?\r?\n`
  }
};
const encodedNote = (content: string) => ({
  ...note,
  size: Buffer.byteLength(content),
  content: Buffer.from(content).toString('base64')
});

test('highlight configuration accepts only new highlight notes and filters model evidence', async () => {
  for (const [content, accepted] of [
    ['---\ntype: highlight\n---\nA highlight.', true],
    ['---\r\ntitle: Example\r\ntype: "highlight" # main benefit\r\n---\r\nUseful.', true],
    ["---\ntype: 'highlight'\n---\nUseful.", true],
    [noteContent, false],
    ['---\ntype: fix\n---\ntype: highlight\n', false],
    ['---\ntype: feature\n---\n---\ntype: highlight\n---\n', false],
    ['type: highlight\n', false],
    ['---\ntype: highlights\n---\n', false]
  ] as const) {
    const request = responses(
      { token: 'token' },
      comparison([
        { filename: '.sweet/a.md', status: 'added' },
        { filename: '.sweet/existing.md', status: 'modified' },
        { filename: '.sweet/deleted.md', status: 'removed' }
      ]),
      encodedNote(content)
    );
    expect(await loadChangedFiles(highlightFilter, push, signal, request)).toEqual(
      accepted ? [{ path: '.sweet/a.md', content }] : []
    );
    expect(request).toHaveBeenCalledTimes(3);
  }
});

test('tree fallback excludes changed existing highlights but keeps new highlight paths', async () => {
  const content = '---\ntype: highlight\n---\nNew benefit.';
  const request = responses(
    { token: 'token' },
    { merge_base_commit: { sha: 'c'.repeat(40) } },
    { tree: { sha: 'd'.repeat(40) } },
    { truncated: false, tree: [{ path: '.sweet/old.md', type: 'blob', sha: 'old' }] },
    { tree: { sha: 'e'.repeat(40) } },
    {
      truncated: false,
      tree: [
        { path: '.sweet/old.md', type: 'blob', sha: 'edited' },
        { path: '.sweet/new.md', type: 'blob', sha: 'new' }
      ]
    },
    encodedNote(content)
  );
  expect(await loadChangedFiles(highlightFilter, push, signal, request)).toEqual([
    { path: '.sweet/new.md', content }
  ]);
  expect(request.mock.calls.at(-1)![0]).toContain('/contents/.sweet/new.md?ref=' + push.after);
});

test('mixed pushes supply only highlight notes, not accompanying fix notes', async () => {
  const highlight = '---\ntype: highlight\n---\nUser-facing benefit.';
  const request = responses(
    { token: 'token' },
    comparison([
      { filename: '.sweet/a-fix.md', status: 'added' },
      { filename: '.sweet/b-highlight.md', status: 'added' }
    ]),
    encodedNote(noteContent),
    encodedNote(highlight)
  );
  expect(await loadChangedFiles(highlightFilter, push, signal, request)).toEqual([
    { path: '.sweet/b-highlight.md', content: highlight }
  ]);
});
