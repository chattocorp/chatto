/** Exercise memory ownership with real MCP framing and an in-memory Means fixture. */
import { mkdir, mkdtemp, rm } from 'node:fs/promises';
import { join, resolve } from 'node:path';
import { afterEach, expect, test, vi } from 'vitest';
import { Server } from '@modelcontextprotocol/sdk/server/index.js';
import { InMemoryTransport } from '@modelcontextprotocol/sdk/inMemory.js';
import { CallToolRequestSchema, ListToolsRequestSchema } from '@modelcontextprotocol/sdk/types.js';
import { createMemory } from './memory.ts';
import { parseSettings } from './settings.ts';

const fixture = vi.hoisted(() => ({ transport: vi.fn() }));
vi.mock('@modelcontextprotocol/sdk/client/stdio.js', () => ({
  StdioClientTransport: class {
    constructor(options: unknown) {
      return fixture.transport(options);
    }
  }
}));
const directories: string[] = [];
afterEach(async () => {
  await Promise.all(
    directories.splice(0).map((path) => rm(path, { recursive: true, force: true }))
  );
});

test('concurrent turns and a new memory owner reuse global and per-user documents', async () => {
  const root = resolve('../../.context');
  await mkdir(root, { recursive: true });
  const directory = await mkdtemp(join(root, 'botto-memory-'));
  directories.push(directory);
  const collections = new Map<
    string,
    Array<{ id: string; frontmatter: { data: Record<string, unknown> } }>
  >();
  fixture.transport.mockImplementation(({ args }: { args: string[] }) => {
    const path = args.at(-1)!;
    const documents = collections.get(path) ?? [];
    collections.set(path, documents);
    const [client, transport] = InMemoryTransport.createLinkedPair();
    const server = new Server(
      { name: 'means-fixture', version: '1' },
      { capabilities: { tools: {} } }
    );
    server.setRequestHandler(ListToolsRequestSchema, async () => ({
      tools: ['search', 'create', 'read', 'update', 'info'].map((name) => ({
        name,
        inputSchema: { type: 'object' as const }
      }))
    }));
    server.setRequestHandler(CallToolRequestSchema, async ({ params }) => {
      if (params.name === 'search') {
        const filter = params.arguments?.filter as Record<string, unknown>;
        const found = documents.filter((doc) =>
          Object.entries(filter).every(([key, value]) => doc.frontmatter.data[key] === value)
        );
        // Without host serialization, concurrent empty searches can both create a document.
        await new Promise((resolve) => setTimeout(resolve, 5));
        return {
          content: [],
          structuredContent: { documents: found, diagnostics: [], has_more: false }
        };
      }
      if (params.name === 'create') {
        const doc = {
          id: String(documents.length + 1),
          frontmatter: params.arguments?.frontmatter as { data: Record<string, unknown> }
        };
        documents.push(doc);
        return { content: [], structuredContent: doc };
      }
      return { content: [], structuredContent: { body: 'remembered fact' } };
    });
    void server.connect(transport);
    return client;
  });
  const settings = {
    ...parseSettings(
      '[chatto]\nurl="https://example.com"\napi_key="unused"\n[agent]\nmodel="test/test"\n[memory]\nenabled=true',
      'botto.toml'
    ),
    directory
  };
  const open = createMemory(settings);
  const identity = ['https://example.com', 'bot', 'room', 'thread'];
  const signal = new AbortController().signal;
  const [first, second] = await Promise.all([
    open(identity, ['user', 'user'], signal),
    open(identity, ['user', 'other'], signal)
  ]);
  try {
    expect(first.documents.global).toBe(second.documents.global);
    expect(first.documents.users.user).toBe(second.documents.users.user);
    expect([...collections.values()][0]).toHaveLength(3);
    expect(first.tools.map((tool) => tool.name)).toContain('memory_update');
    const reopened = await createMemory(settings)(identity, ['user'], signal);
    try {
      expect(reopened.documents).toEqual(first.documents);
    } finally {
      await reopened.close();
    }
    const otherBot = await open(['https://example.com', 'another-bot'], ['user'], signal);
    try {
      expect(collections.size).toBe(2);
    } finally {
      await otherBot.close();
    }
  } finally {
    await first.close();
    await second.close();
  }
});

test('memory is independently opt-in and validates its configuration', () => {
  const source =
    '[chatto]\nurl="https://example.com"\napi_key="unused"\n[agent]\nmodel="test/test"';
  expect(parseSettings(source, 'botto.toml')).toMatchObject({
    memory: false,
    meansCommand: 'means'
  });
  expect(
    parseSettings(source + '\n[memory]\nenabled=true\ncommand="/opt/bin/means"', 'botto.toml')
  ).toMatchObject({ memory: true, mcp: false, meansCommand: '/opt/bin/means' });
  expect(() => parseSettings(source + '\n[memory]\nenabled="true"', 'botto.toml')).toThrow(
    'boolean'
  );
});
