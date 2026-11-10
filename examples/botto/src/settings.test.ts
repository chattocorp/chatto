/** Validate TOML defaults, overrides, paths, and errors without exposing secrets. */
import { readFileSync } from 'node:fs';
import { resolve } from 'node:path';
import { expect, test } from 'vitest';
import { parseSettings, readSettings } from './settings.ts';

const source =
  '[chatto]\nurl = "https://chat.example.com"\napi_key = "${BOTTO_CHATTO_API_KEY}"\n[agent]\nmodel = "openrouter/anthropic/claude-sonnet-4-6"';
const env = { BOTTO_CHATTO_API_KEY: 'secret' };
const parse = (text = source) => parseSettings(text, '/config/botto.toml', env);

test('extra MCP tools default to no grants and preserve explicit access lists', () => {
  const base = `${source}\n[mcp.servers.github]\ncommand="server"`;
  expect(parse(base).extraMcpServers[0]).toMatchObject({ tools: [], adminTools: [] });
  expect(
    parse(`${base}\ntools=["issue_read"]\nadmin_tools=["issue_write"]`).extraMcpServers[0]
  ).toMatchObject({ tools: ['issue_read'], adminTools: ['issue_write'] });
});

test.each([
  'tools=["*"]',
  'tools="read"',
  'tools=["read","read"]',
  'tools=["read"]\nadmin_tools=["read"]',
  'tools=["do-work"]\nadmin_tools=["do_work"]',
  'admin_tools=[1]'
])('rejects ambiguous MCP access: %s', (access) => {
  expect(() => parse(`${source}\n[mcp.servers.github]\ncommand="server"\n${access}`)).toThrow(
    'Invalid mcp.servers'
  );
});

test.each(['[participation]\nmodel="test/model"', '[subagents.worker]\ndescription="Work"'])(
  'removed conversation settings require explicit migration: %s',
  (table) => {
    expect(() => parse(`${source}\n${table}`)).toThrow('Remove [participation] and [subagents.*]');
  }
);

test('configures remote and local MCP servers with literal environment credentials', () => {
  expect(parse().extraMcpServers).toEqual([]);
  const settings = parseSettings(
    `${source}
[mcp.servers.remote]
url="https://example.com/mcp"
[mcp.servers.remote.headers]
Authorization="Bearer \${REMOTE_AUTH}"
[mcp.servers.local]
command="./server"
args=["--stdio", ""]
[mcp.servers.local.env]
TOKEN="\${REMOTE_AUTH}"
`,
    '/config/botto.toml',
    { ...env, REMOTE_AUTH: '!literal${TOKEN}' }
  );
  expect(settings.extraMcpServers.map((entry) => entry.config)).toEqual([
    {
      url: 'https://example.com/mcp',
      headers: { Authorization: 'Bearer !literal$${TOKEN}' },
      exposure: 'direct'
    },
    {
      command: './server',
      args: ['--stdio', ''],
      cwd: '/config',
      env: { TOKEN: '$!literal$${TOKEN}' },
      exposure: 'direct'
    }
  ]);
});

test.each([
  '[mcp]\nunknown=true',
  '[mcp.servers.memory]\ncommand="server"',
  '[mcp.servers."bad-name"]\ncommand="server"',
  '[mcp.servers.custom]\nurl="http://example.com/mcp"',
  '[mcp.servers.custom]\nurl="https://user:private-secret@example.com/mcp"',
  '[mcp.servers.custom]\nurl="https://example.com/mcp"\ncommand="server"',
  '[mcp.servers.custom]\ncommand="server"\nargs=[1]',
  '[mcp.servers.custom]\ncommand="server"\nheaders={Authorization="${MISSING}"}',
  '[mcp.servers.custom]\nurl="https://example.com/mcp"\nheaders={Authorization="${MISSING}"}',
  '[mcp.servers.custom]\nurl="https://example.com/mcp"\nheaders={Authorization="${BAD_HEADER}"}',
  '[mcp.servers.custom]\ncommand=""'
])('rejects invalid MCP settings without exposing secrets: %s', (configuration) => {
  const load = () =>
    parseSettings(`${source}\n${configuration}`, '/config/botto.toml', {
      ...env,
      BAD_HEADER: 'private-secret\r\nInjected: value'
    });
  expect(load).toThrow('Invalid mcp.servers');
  expect(load).not.toThrow('private-secret');
});

test.each(['http://127.0.0.1:8080/mcp', 'http://localhost:8080/mcp', 'http://[::1]:8080/mcp'])(
  'allows loopback MCP: %s',
  (url) => {
    expect(parse(`${source}\n[mcp.servers.local]\nurl="${url}"`).extraMcpServers).toHaveLength(1);
  }
);

test('MCP strings share single-pass interpolation and accept literal values', () => {
  const config =
    source +
    '\n' +
    `
[mcp.servers.remote]
url="https://\${HOST}/mcp"
[mcp.servers.remote.headers]
Authorization="Bearer \${TOKEN}"
"X-Mode"="test"
[mcp.servers.local]
command="\${COMMAND}"
args=["--path", "\${PATH_VALUE}", "$\${UNEXPANDED}", ""]
[mcp.servers.local.env]
MODE="production"
LITERAL="!literal $\${UNEXPANDED}"
`;
  const parsed = parseSettings(config, '/config/botto.toml', {
    ...env,
    HOST: 'example.com',
    TOKEN: '$OTHER',
    COMMAND: './server',
    PATH_VALUE: 'path with spaces'
  });
  expect(parsed.extraMcpServers.map((entry) => entry.config)).toEqual([
    {
      url: 'https://example.com/mcp',
      headers: { Authorization: 'Bearer $$OTHER', 'X-Mode': 'test' },
      exposure: 'direct'
    },
    {
      command: './server',
      args: ['--path', 'path with spaces', '${UNEXPANDED}', ''],
      cwd: '/config',
      env: { MODE: 'production', LITERAL: '$!literal $${UNEXPANDED}' },
      exposure: 'direct'
    }
  ]);
});

test.each(['env_from_env', 'headers_from_env'])('rejects obsolete MCP %s tables', (key) => {
  expect(() =>
    parse(`${source}\n[mcp.servers.local]\ncommand="server"\n${key}={TOKEN="NAME"}`)
  ).toThrow('Invalid mcp.servers');
});

test('reasoning is optional, supports Pi levels, and rejects unknown values without exposing them', () => {
  expect(parse().reasoningLevel).toBeUndefined();
  for (const level of ['off', 'minimal', 'low', 'medium', 'high', 'xhigh', 'max'])
    expect(parse(`${source}\nreasoning_level="${level}"`).reasoningLevel).toBe(level);
  for (const value of ['"secret-invalid"', 'false', '4', '""']) {
    expect(() => parse(`${source}\nreasoning_level=${value}`)).toThrow('agent.reasoning_level');
    expect(() => parse(`${source}\nreasoning_level=${value}`)).not.toThrow('secret-invalid');
  }
});

test('title model defaults to the main model and accepts independent model and reasoning settings', () => {
  expect(parse()).toMatchObject({
    titleModel: 'openrouter/anthropic/claude-sonnet-4-6',
    titleReasoningLevel: 'off'
  });
  expect(parse(`${source}\n[titles]\nmodel="test/small"\nreasoning_level="minimal"`)).toMatchObject(
    { titleModel: 'test/small', titleReasoningLevel: 'minimal' }
  );
  expect(() => parse(`${source}\n[titles]\nmodel="invalid"`)).toThrow('titles.model');
  expect(() => parse(`${source}\n[titles]\nreasoning_level="invalid"`)).toThrow(
    'titles.reasoning_level'
  );
});

test('admin IDs default to none and must be explicit nonempty strings', () => {
  expect(parse().adminUserIds).toEqual([]);
  const withAdmins = (value: string) =>
    source.replace('[chatto]', `[chatto]\nadmin_user_ids=${value}`);
  expect(parse(withAdmins('["admin", "admin", "second"]')).adminUserIds).toEqual([
    'admin',
    'second'
  ]);
  for (const invalid of ['"admin"', '[1]', '[""]', '[" "]'])
    expect(() => parse(withAdmins(invalid))).toThrow('chatto.admin_user_ids');
});

test('presence and addressing default to online and direct addressing', () => {
  const withChatto = (line: string) => source.replace('[chatto]', `[chatto]\n${line}`);
  expect(parse()).toMatchObject({
    reportPresence: true,
    respondTo: ['direct_message', 'mention', 'reply']
  });
  expect(parse(withChatto('report_presence = false')).reportPresence).toBe(false);
  expect(
    parse(withChatto('respond_to = ["mention", "here_mention", "mention"]')).respondTo
  ).toEqual(['mention', 'here_mention']);
  expect(() => parse(withChatto('report_presence = "yes"'))).toThrow('chatto.report_presence');
  for (const invalid of ['[]', '["everyone"]', '"mention"'])
    expect(() => parse(withChatto(`respond_to = ${invalid}`))).toThrow('chatto.respond_to');
});

test('uses defaults and preserves slashes in the model ID', () => {
  expect(parse()).toMatchObject({
    name: 'Botto',
    provider: 'openrouter',
    modelId: 'anthropic/claude-sonnet-4-6',
    debounceMs: 2000,
    passiveDebounceMs: 5000,
    maxThreads: 8,
    maxPendingMessages: 100,
    turnTimeoutMs: 900000,
    contextMessages: 30,
    directory: '/config/.botto',
    skillsDirectory: '/config/.agents/skills',
    memoryPath: '/config/memory',
    acknowledgementEmoji: 'eyes'
  });
});

test('acknowledgement emoji can be changed or disabled', () => {
  expect(parse(`${source}\n[runtime]\nacknowledgement_emoji="wave"`).acknowledgementEmoji).toBe(
    'wave'
  );
  expect(parse(`${source}\n[runtime]\nacknowledgement_emoji=""`).acknowledgementEmoji).toBe('');
  expect(() => parse(`${source}\n[runtime]\nacknowledgement_emoji=false`)).toThrow(
    'runtime.acknowledgement_emoji'
  );
});

test('resolves memory paths relative to TOML and accepts environment variables and absolute paths', () => {
  expect(parse(`${source}\n[memory]\npath="notes"`).memoryPath).toBe('/config/notes');
  expect(parse(`${source}\n[memory]\npath="/private/notes"`).memoryPath).toBe('/private/notes');
  expect(
    parseSettings(source + '\n[memory]\npath="${MEMORY_PATH}"', '/config/botto.toml', {
      ...env,
      MEMORY_PATH: 'notes'
    }).memoryPath
  ).toBe('/config/notes');
  expect(() => parse(`${source}\n[memory]\npath=""`)).toThrow(
    'memory.path must be a non-empty string'
  );
});

test('web access is opt-in and its Exa key is optional', () => {
  expect(parse()).toMatchObject({ web: false, exaApiKey: undefined });
  expect(parse(`${source}\n[web]\nenabled=true`)).toMatchObject({
    web: true,
    exaApiKey: undefined
  });
  expect(
    parseSettings(`${source}\n[web]\nenabled=true\nexa_api_key="\${EXA_API_KEY}"`, 'botto.toml', {
      ...env,
      EXA_API_KEY: 'exa-secret'
    }).exaApiKey
  ).toBe('exa-secret');
  expect(() => parse(`${source}\n[web]\nenabled="yes"`)).toThrow('web.enabled must be a boolean');
  expect(() => parse(`${source}\n[web]\nexa_api_key=""`)).toThrow(
    'web.exa_api_key must be a non-empty string'
  );
});

test('loads persona, multiline prompt, and runtime overrides', () => {
  expect(
    parse(
      `${source}\nname="Helper"\nsystem_prompt="""First line\nSecond line"""\n[runtime]\ndebounce_ms=2000\nmax_threads=2\nmax_pending_messages=5\nturn_timeout_ms=5000\ncontext_messages=10\ndata_directory="state"`
    )
  ).toMatchObject({
    name: 'Helper',
    systemPrompt: 'First line\nSecond line',
    directory: '/config/state',
    debounceMs: 2000,
    maxThreads: 2,
    maxPendingMessages: 5,
    turnTimeoutMs: 5000,
    contextMessages: 10
  });
});

test.each([
  'debounce_ms=-1',
  'debounce_ms=1999',
  'debounce_ms=1.5',
  'debounce_ms="5000"',
  'debounce_ms=60001',
  'passive_debounce_ms=-1',
  'passive_debounce_ms=1999',
  'passive_debounce_ms=600001',
  'max_threads=0',
  'turn_timeout_ms=0',
  'context_messages=51'
])('rejects invalid runtime value %s', (setting) => {
  expect(() => parse(`${source}\n[runtime]\n${setting}`)).toThrow('must be an integer');
});

test('rejects unknown settings and secrets in TOML without printing their names or values', () => {
  expect(() => parse(`${source}\nprivate_secret="do-not-print"`)).toThrow(
    'Unsupported setting in agent.'
  );
  expect(() => parse('private-secret = "unterminated')).toThrow(
    'Invalid TOML configuration. Check TOML syntax.'
  );
  expect(() => parse('chatto = "private-secret"')).toThrow('chatto must be a TOML table');
});

test('validates required fields and keeps credentials out of errors', () => {
  expect(() =>
    parse(source.replace('https://chat.example.com', 'https://user:secret@example.com'))
  ).toThrow('chatto.url must be an HTTP or HTTPS URL without credentials');
  expect(() => parse(source.replace('openrouter/anthropic/claude-sonnet-4-6', 'invalid'))).toThrow(
    'provider/model'
  );
  expect(() => parseSettings(source, 'botto.toml', {})).toThrow(
    new Error('chatto.api_key references missing environment variable BOTTO_CHATTO_API_KEY')
  );
  expect(() => readSettings('/missing/botto.toml', env)).toThrow('Cannot read Botto configuration');
});

test('the shipped sample is valid and resolves data relative to its location', () => {
  const path = resolve('botto.example.toml');
  const settings = parseSettings(readFileSync(path, 'utf8'), path, env);
  expect(settings.directory).toBe(resolve('.botto'));
  expect(settings.passiveDebounceMs).toBe(5000);
  expect(settings.systemPrompt).toContain('Be friendly, clear, and concise.');
  expect(readSettings(path, env)).toEqual(settings);
});

test('configures the passive quiet period independently of addressed messages', () => {
  expect(parse(`${source}\n[runtime]\ndebounce_ms=2500\npassive_debounce_ms=90000`)).toMatchObject({
    debounceMs: 2500,
    passiveDebounceMs: 90_000
  });
});

test('loads the API key only from its configured TOML value', () => {
  const custom = source.replace('${BOTTO_CHATTO_API_KEY}', '${CUSTOM_CHATTO_KEY}');
  expect(
    parseSettings(custom, 'botto.toml', { ...env, CUSTOM_CHATTO_KEY: 'selected-secret' }).apiKey
  ).toBe('selected-secret');
  expect(
    parseSettings(source.replace('${BOTTO_CHATTO_API_KEY}', 'literal-key'), 'botto.toml', {}).apiKey
  ).toBe('literal-key');
  const omitted = source.replace('api_key = "${BOTTO_CHATTO_API_KEY}"\n', '');
  expect(() => parseSettings(omitted, 'botto.toml', env)).toThrow(
    new Error('chatto.api_key must be a non-empty string')
  );
  expect(() => parseSettings(source, 'botto.toml', { BOTTO_CHATTO_API_KEY: '' })).toThrow(
    new Error('chatto.api_key must be a non-empty string')
  );
});

test('expands references in parsed string settings and resolves expanded paths', () => {
  const settings = parseSettings(
    '[chatto]\nurl="${SERVER}"\napi_key="${BOTTO_CHATTO_API_KEY}"\nallowed_user_id="${USER_ID}"\n[agent]\nname="Hello ${NAME}"\nmodel="${PROVIDER}/${MODEL}"\nsystem_prompt="""Project: ${PROJECT}\n${PROMPT}"""\n[runtime]\ndata_directory="${DATA}/sessions"',
    '/config/botto.toml',
    {
      ...env,
      SERVER: 'https://chat.example.com',
      USER_ID: 'user-1',
      NAME: 'Botto',
      PROVIDER: 'openrouter',
      MODEL: 'vendor/model',
      PROJECT: 'Chatto',
      PROMPT: 'Use "quotes".\nKeep \\slashes and ${LITERAL}.',
      DATA: 'private'
    }
  );
  expect(settings).toMatchObject({
    serverUrl: 'https://chat.example.com',
    allowedUserId: 'user-1',
    name: 'Hello Botto',
    provider: 'openrouter',
    modelId: 'vendor/model',
    directory: '/config/private/sessions',
    systemPrompt: 'Project: Chatto\nUse "quotes".\nKeep \\slashes and ${LITERAL}.'
  });
});

test('supports literal dollars and does not recursively expand environment values', () => {
  const settings = parseSettings(
    `${source}\nsystem_prompt='$$ $${'${MISSING}'} ${'${VALUE}'}'`,
    '/config/botto.toml',
    {
      ...env,
      VALUE: '${MISSING} $$'
    }
  );
  expect(settings.systemPrompt).toBe('$ ${MISSING} ${MISSING} $$');
});

test('reports missing variables without exposing nearby secret text', () => {
  expect(() => parse(`${source}\nname="private-value ${'${MISSING}'}"`)).toThrow(
    new Error('agent.name references missing environment variable MISSING')
  );
  expect(() =>
    parseSettings(`${source}\nname="${'${EMPTY}'}"`, 'botto.toml', { ...env, EMPTY: '' })
  ).toThrow(new Error('agent.name must be a non-empty string'));
});

test('validates expanded values and keeps numeric settings typed', () => {
  expect(() =>
    parseSettings(source.replace('https://chat.example.com', '${SERVER}'), 'botto.toml', {
      ...env,
      SERVER: 'https://user:secret@example.com'
    })
  ).toThrow(new Error('chatto.url must be an HTTP or HTTPS URL without credentials'));
  expect(() =>
    parseSettings(`${source}\n[runtime]\ndebounce_ms="${'${DELAY}'}"`, 'botto.toml', {
      ...env,
      DELAY: '5000'
    })
  ).toThrow('runtime.debounce_ms must be an integer');
});
