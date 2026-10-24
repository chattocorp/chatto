/** Exercise memory discovery and shared locking through Pi's MCP client. */
import { copyFile, mkdir, mkdtemp, readFile, rm } from 'node:fs/promises';
import { join, resolve } from 'node:path';
import { expect, test, vi } from 'vitest';
import { createMemory } from './memory.ts';
import { mcpFixture } from './mcp.fixture.ts';
import { parseSettings } from './settings.ts';

const fixture = vi.hoisted(() => ({ transport: vi.fn() }));
vi.mock('@earendil-works/pi-mcp', async (importOriginal) => ({
  ...(await importOriginal<typeof import('@earendil-works/pi-mcp')>()),
  StdioTransport: class {
    constructor(options: unknown) {
      return fixture.transport(options);
    }
  }
}));

test('discovers all memories without creating files; cancelled waiters release the lock', async () => {
  vi.spyOn(console, 'info').mockImplementation(() => {});
  const root = resolve('../../.context');
  await mkdir(root, { recursive: true });
  const directory = await mkdtemp(join(root, 'botto-memory-'));
  await copyFile(new URL('../memory/MEANS.md', import.meta.url), join(directory, 'MEANS.md'));
  const collections = new Map<
    string,
    Array<{ id: string; frontmatter: { data: Record<string, unknown> } }>
  >();
  collections.set(directory, [
    { id: 'global-1', frontmatter: { data: { scope: 'global' } } },
    { id: 'global-2', frontmatter: { data: { scope: 'global' } } },
    { id: 'user-1', frontmatter: { data: { scope: 'user', user_id: 'user' } } },
    { id: 'user-2', frontmatter: { data: { scope: 'user', user_id: 'user' } } },
    { id: 'other-1', frontmatter: { data: { scope: 'user', user_id: 'other' } } }
  ]);
  fixture.transport.mockImplementation(({ args }: { args: string[] }) => {
    const path = args.at(-1)!;
    const documents = collections.get(path) ?? [];
    collections.set(path, documents);
    return mcpFixture([], async (name, args) => {
      if (name === 'search') {
        expect(args.limit).toBe(0);
        const found = documents.filter((doc) =>
          Object.entries(args.filter as Record<string, unknown>).every(
            ([key, value]) => doc.frontmatter.data[key] === value
          )
        );
        await new Promise((resolve) => setTimeout(resolve, 5));
        return {
          content: [],
          structuredContent: { documents: found, diagnostics: [], has_more: false }
        };
      }
      throw new Error(`Unexpected memory operation: ${name}`);
    });
  });
  try {
    const settings = {
      ...parseSettings(
        '[chatto]\nurl="https://example.com"\napi_key="unused"\n[agent]\nmodel="test/test"\n[memory]\nenabled=true',
        'botto.toml'
      ),
      memoryPath: directory
    };
    const open = createMemory(settings);
    const threadId = 'thread';
    const signal = new AbortController().signal;
    const [first, second] = await Promise.all([
      open(threadId, ['user', 'user'], signal),
      open(threadId, ['user', 'other', 'new-user'], signal)
    ]);
    expect(first.documents).toEqual({
      global: ['global-1', 'global-2'],
      users: { user: ['user-1', 'user-2'] }
    });
    expect(second.documents).toEqual({
      global: first.documents.global,
      users: { user: ['user-1', 'user-2'], other: ['other-1'], 'new-user': [] }
    });
    expect([...collections.values()][0]).toHaveLength(5);
    expect(first.directory).toBe(directory);
    expect((await createMemory(settings)(threadId, ['user'], signal)).documents).toEqual(
      first.documents
    );
    const release = await first.acquire(signal);
    try {
      const controller = new AbortController();
      const waiting = open(threadId, ['waiting'], controller.signal);
      controller.abort(new Error('cancelled while queued'));
      await expect(waiting).rejects.toThrow('cancelled while queued');
    } finally {
      release();
    }
    await open('other-thread', ['user'], signal);
    expect(collections.size).toBe(1);
    collections.set(directory, []);
    expect((await open(threadId, ['user'], signal)).documents).toEqual({
      global: [],
      users: { user: [] }
    });
    expect(await readFile(join(directory, 'MEANS.md'), 'utf8')).toBe(
      await readFile(new URL('../memory/MEANS.md', import.meta.url), 'utf8')
    );
    await rm(join(directory, 'MEANS.md'));
    await expect(open(threadId, ['user'], signal)).rejects.toThrow('Cannot read MEANS.md');
    expect(fixture.transport).toHaveBeenCalledTimes(5);
  } finally {
    vi.restoreAllMocks();
    await rm(directory, { recursive: true, force: true });
  }
});

test('memory is independently opt-in and validates TOML booleans', () => {
  const source =
    '[chatto]\nurl="https://example.com"\napi_key="unused"\n[agent]\nmodel="test/test"';
  expect(parseSettings(source, 'botto.toml')).toMatchObject({
    memory: false,
    meansCommand: 'means'
  });
  expect(() => parseSettings(source + '\n[memory]\nenabled="true"', 'botto.toml')).toThrow(
    'boolean'
  );
});
