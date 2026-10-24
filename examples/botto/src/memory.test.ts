/** Exercise document provisioning and shared locking through Pi's MCP client. */
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

test('concurrent turns and restarts reuse documents; cancelled waiters do not hold the memory lock', async () => {
  vi.spyOn(console, 'info').mockImplementation(() => {});
  const root = resolve('../../.context');
  await mkdir(root, { recursive: true });
  const directory = await mkdtemp(join(root, 'botto-memory-'));
  for (const file of ['means.schema.json', 'MEANS.md']) {
    await copyFile(new URL(`../memory/${file}`, import.meta.url), join(directory, file));
  }
  const collections = new Map<
    string,
    Array<{ id: string; frontmatter: { data: Record<string, unknown> } }>
  >();
  fixture.transport.mockImplementation(({ args }: { args: string[] }) => {
    const path = args.at(-1)!;
    const documents = collections.get(path) ?? [];
    collections.set(path, documents);
    return mcpFixture([], async (name, args) => {
      if (name === 'search') {
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
      const document = {
        id: String(documents.length + 1),
        frontmatter: args.frontmatter as { data: Record<string, unknown> }
      };
      documents.push(document);
      return { content: [], structuredContent: document };
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
      open(threadId, ['user', 'other'], signal)
    ]);
    expect(first.documents.global).toBe(second.documents.global);
    expect(first.documents.users.user).toBe(second.documents.users.user);
    expect([...collections.values()][0]).toHaveLength(3);
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
    expect(await readFile(join(directory, 'MEANS.md'), 'utf8')).toBe(
      await readFile(new URL('../memory/MEANS.md', import.meta.url), 'utf8')
    );
    await rm(join(directory, 'means.schema.json'));
    await expect(open(threadId, ['user'], signal)).rejects.toThrow('Cannot read means.schema.json');
    expect(fixture.transport).toHaveBeenCalledTimes(4);
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
