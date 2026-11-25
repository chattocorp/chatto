/** Verify operator prompts and MCP grants reach an isolated announcement session. */
import { mkdtempSync, writeFileSync, rmSync } from 'node:fs';
import { join } from 'node:path';
import { tmpdir } from 'node:os';
import { expect, test, vi } from 'vitest';
import type { TaskContext } from 'runling';
import { parseSettings } from './settings.ts';
import { mcpServers } from './mcp.ts';
import { announcementMcpSettings, describeAnnouncement } from './announcements.ts';

vi.mock('./announcement-response.ts', () => ({
  announcementResponse: (_signal: AbortSignal, channel = true) => ({
    extension: vi.fn(),
    result: () => (channel ? { summary: 'Short update', details: 'Configured detail' } : null)
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
tools=["info", "search", "read", "create"]
admin_tools=["update"]
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
    const result = await describeAnnouncement({} as TaskContext, settings, push, signal);
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

test('release sessions load their own prompt and receive release notes instead of push evidence', async () => {
  const directory = mkdtempSync(join(tmpdir(), 'botto-release-prompt-'));
  try {
    writeFileSync(join(directory, 'release.md'), 'Use release notes and explain user benefits.');
    writeFileSync(join(directory, 'push.md'), 'Inspect commit patches.');
    const settings = parseSettings(
      config + 'releases=true\nrelease_prompt_file="release.md"\nprompt_file="push.md"',
      join(directory, 'botto.toml'),
      {}
    );
    const event = {
      kind: 'release' as const,
      id: 123,
      tag: 'v1.2.0',
      name: 'Version 1.2.0',
      body: 'Untrusted notes',
      prerelease: false,
      publishedAt: '2026-11-16T10:00:00Z',
      truncated: false
    };
    const signal = new AbortController().signal;
    await describeAnnouncement({} as TaskContext, settings, event, signal);
    const { createAgentSession } = await import('runling');
    const options = vi.mocked(createAgentSession).mock.calls.at(-1)![0]!;
    expect(options.systemPrompt).toContain('Use release notes and explain user benefits.');
    expect(options.systemPrompt).not.toContain('Inspect commit patches.');
    expect(session.run).toHaveBeenLastCalledWith(
      {},
      JSON.stringify({ repository: 'owner/repo', branch: 'main', ...event }),
      { signal }
    );
    expect(settings.announcements?.releases).toBe(true);
    expect(() =>
      parseSettings(config + 'releases="true"', join(directory, 'botto.toml'), {})
    ).toThrow('Invalid announcements.github');
    expect(() =>
      parseSettings(config + 'release_prompt_file="missing.md"', join(directory, 'botto.toml'), {})
    ).toThrow('Invalid announcements.github');
  } finally {
    rmSync(directory, { recursive: true, force: true });
  }
});

test('the change filter runs before model startup, bypasses releases, and propagates lookup failures', async () => {
  const filter = await import('./announcement-filter.ts');
  const check = vi.spyOn(filter, 'loadChangedFiles');
  const { createAgentSession } = await import('runling');
  const settings = parseSettings(config, '/example/botto.toml', {});
  const push = { before: 'a'.repeat(40), after: 'b'.repeat(40), commits: [], truncated: false };
  const signal = new AbortController().signal;
  try {
    vi.mocked(createAgentSession).mockClear();
    check.mockResolvedValue([]);
    expect(await describeAnnouncement({} as TaskContext, settings, push, signal)).toBeNull();
    expect(createAgentSession).not.toHaveBeenCalled();
    check.mockRejectedValue(new Error('Lookup failed'));
    await expect(describeAnnouncement({} as TaskContext, settings, push, signal)).rejects.toThrow(
      'Announcement GitHub lookup failed.'
    );
    expect(createAgentSession).not.toHaveBeenCalled();
    check.mockResolvedValue([{ path: '.sweet/note.md', content: 'Useful release notes' }]);
    await describeAnnouncement({} as TaskContext, settings, push, signal);
    expect(createAgentSession).toHaveBeenCalledTimes(1);
    expect(session.run).toHaveBeenLastCalledWith(
      {},
      JSON.stringify({
        repository: 'owner/repo',
        branch: 'main',
        ...push,
        changedFiles: [{ path: '.sweet/note.md', content: 'Useful release notes' }]
      }),
      { signal }
    );
    check.mockClear();
    await describeAnnouncement(
      {} as TaskContext,
      settings,
      {
        kind: 'release',
        id: 1,
        tag: 'v1',
        name: 'Version 1',
        body: 'Notes',
        prerelease: false,
        publishedAt: '2026-11-16T10:00:00Z',
        truncated: false
      },
      signal
    );
    expect(check).not.toHaveBeenCalled();
    expect(createAgentSession).toHaveBeenCalledTimes(2);
  } finally {
    check.mockRestore();
  }
});

test('opted-in pushes check follow-ups without Sweet notes, but releases get no thread write tool', async () => {
  const filter = await import('./announcement-filter.ts');
  const check = vi.spyOn(filter, 'loadChangedFiles').mockResolvedValue([]);
  const { createAgentSession } = await import('runling');
  const settings = parseSettings(
    config + 'thread_replies=true\nfollowup_memory="memory"\nmcp_servers=["github", "memory"]',
    '/example/botto.toml',
    {}
  );
  const server = { getMessage: vi.fn(), createMessage: vi.fn() };
  const signal = new AbortController().signal;
  const push = { before: 'a'.repeat(40), after: 'b'.repeat(40), commits: [], truncated: false };
  try {
    await expect(describeAnnouncement({} as TaskContext, settings, push, signal)).rejects.toThrow(
      'delivery is unavailable'
    );
    expect(
      await describeAnnouncement({} as TaskContext, settings, push, signal, server)
    ).toBeNull();
    const options = vi.mocked(createAgentSession).mock.calls.at(-1)![0]!;
    expect(options.extensions).toHaveLength(3);
    expect(options.systemPrompt).toContain('call finish_followups');
    expect(session.run).toHaveBeenLastCalledWith(
      {},
      expect.stringContaining('"channelAnnouncement":false'),
      { signal }
    );
    await describeAnnouncement(
      {} as TaskContext,
      settings,
      {
        kind: 'release',
        id: 1,
        tag: 'v1',
        name: 'v1',
        body: '',
        prerelease: false,
        publishedAt: '',
        truncated: false
      },
      signal,
      server
    );
    expect(vi.mocked(createAgentSession).mock.calls.at(-1)![0]!.extensions).toHaveLength(2);
    expect(() => parseSettings(config + 'thread_replies="yes"', '/example/botto.toml', {})).toThrow(
      'Invalid announcements.github'
    );
  } finally {
    check.mockRestore();
  }
});

test('model failures identify setup or turn stages without leaking provider messages', async () => {
  const { createAgentSession } = await import('runling');
  const settings = parseSettings(config, '/example/botto.toml', {});
  const push = { before: 'a'.repeat(40), after: 'b'.repeat(40), commits: [], truncated: false };
  const signal = new AbortController().signal;
  vi.mocked(createAgentSession).mockRejectedValueOnce(new Error('secret setup response'));
  await expect(describeAnnouncement({} as TaskContext, settings, push, signal)).rejects.toThrow(
    'Announcement model setup failed.'
  );
  session.run.mockRejectedValueOnce(new Error('secret model response'));
  await expect(describeAnnouncement({} as TaskContext, settings, push, signal)).rejects.toThrow(
    'Announcement model turn failed.'
  );
  expect(session.dispose).toHaveBeenCalled();
});

test('follow-up announcement agents cannot create or update delivery records', () => {
  const settings = parseSettings(
    config + 'mcp_servers=["memory"]\nthread_replies=true\nfollowup_memory="memory"',
    '/example/botto.toml',
    {}
  );
  const selected = announcementMcpSettings(settings).extraMcpServers[0]!;
  expect(selected.tools).toEqual(['info', 'search', 'read']);
  expect(selected.adminTools).toEqual([]);
  expect(settings.extraMcpServers[1]!.tools).toContain('create');
  expect(settings.extraMcpServers[1]!.adminTools).toContain('update');
});
