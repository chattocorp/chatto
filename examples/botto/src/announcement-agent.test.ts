/** Verify operator prompts and MCP grants reach an isolated announcement session. */
import { mkdtempSync, writeFileSync, rmSync } from 'node:fs';
import { join } from 'node:path';
import { tmpdir } from 'node:os';
import { expect, test, vi } from 'vitest';
import type { TaskContext } from 'runling';
import { parseSettings } from './settings.ts';
import { mcpServers } from './mcp.ts';
import { announcementMcpSettings, describePush } from './announcements.ts';

vi.mock('./announcement-response.ts', () => ({
  announcementResponse: () => ({
    extension: vi.fn(),
    result: () => ({ summary: 'Short update', details: 'Configured detail' })
  })
}));

const session = vi.hoisted(() => ({
  run: vi.fn(async () => ({ summary: '### Update\n\nConfigured draft' })),
  dispose: vi.fn()
}));
vi.mock('runling', async (importOriginal) => ({
  ...(await importOriginal<typeof import('runling')>()),
  usePrivateTempDirectory: vi.fn(async () => '/isolated-announcement'),
  createAgentSession: vi.fn(async () => ({
    run: session.run,
    [Symbol.asyncDispose]: session.dispose
  }))
}));
const config = `
[chatto]
url="https://chat.example.test"
api_key="synthetic"
mcp=true
[web]
enabled=true
[agent]
model="test/model"
[mcp.servers.github]
command="github-mcp-server"
tools=["get_commit", "get_file_contents"]
admin_tools=["issue_write"]
[mcp.servers.memory]
command="means"
tools=["read", "create"]
[announcements.github]
enabled=true
repository="owner/repo"
room_id="room"
webhook_secret="synthetic-secret-with-at-least-32-characters"
`;

test('announcements opt in to connections and never inherit built-in or admin tools', () => {
  const settings = parseSettings(config + 'mcp_servers=["github"]', '/example/botto.toml', {});
  const selected = announcementMcpSettings(settings);
  const servers = mcpServers(selected, true);
  expect(servers.map((s) => s.name)).toEqual(['github']);
  expect(servers[0]?.config.toolExposure).toEqual({
    get_commit: 'direct',
    get_file_contents: 'direct'
  });
  expect(settings.extraMcpServers[0]?.adminTools).toEqual(['issue_write']);
  expect(
    mcpServers(announcementMcpSettings(parseSettings(config, '/example/botto.toml', {})))
  ).toEqual([]);
  expect(() => parseSettings(config + 'mcp_servers=["typo"]', '/example/botto.toml', {})).toThrow(
    'must name configured MCP servers'
  );
});

test('loads a prompt relative to TOML, sends the immutable push boundary, and disposes the session', async () => {
  const directory = mkdtempSync(join(tmpdir(), 'botto-announcement-prompt-'));
  try {
    writeFileSync(
      join(directory, 'announcement.md'),
      'Inspect the patch. Literal example: ${UNCHANGED}.'
    );
    const settings = parseSettings(
      config + 'prompt_file="announcement.md"\nmcp_servers=["github"]',
      join(directory, 'botto.toml'),
      {}
    );
    const push = {
      before: 'a'.repeat(40),
      after: 'b'.repeat(40),
      commits: [{ id: 'b'.repeat(40), message: 'Untrusted event text' }],
      truncated: true
    };
    const signal = new AbortController().signal;
    const result = await describePush({} as TaskContext, settings, push, signal);
    const { createAgentSession } = await import('runling');
    expect(createAgentSession).toHaveBeenLastCalledWith(
      expect.objectContaining({
        systemPrompt: expect.stringContaining('Inspect the patch. Literal example: ${UNCHANGED}.'),
        tools: 'extensions',
        extensions: [expect.any(Function), expect.any(Function)],
        resources: expect.objectContaining({
          skills: false,
          contextFiles: false,
          extensions: false
        })
      })
    );
    expect(session.run).toHaveBeenLastCalledWith(
      {},
      JSON.stringify({ repository: 'owner/repo', branch: 'main', ...push }),
      { signal }
    );
    expect(result).toEqual({ summary: 'Short update', details: 'Configured detail' });
    expect(session.dispose).toHaveBeenCalled();
    expect(() =>
      parseSettings(config + 'prompt_file="missing.md"', join(directory, 'botto.toml'), {})
    ).toThrow('Invalid announcements.github');
  } finally {
    rmSync(directory, { recursive: true, force: true });
  }
});
