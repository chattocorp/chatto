/** Validate TOML defaults, overrides, paths, and errors without exposing secrets. */
import { readFileSync } from 'node:fs';
import { resolve } from 'node:path';
import { expect, test } from 'vitest';
import { parseSettings, readSettings } from './settings.ts';

const source =
  '[chatto]\nurl = "https://chat.example.com"\napi_key = "${BOTTO_CHATTO_API_KEY}"\n[agent]\nmodel = "openrouter/anthropic/claude-sonnet-4-6"';
const env = { BOTTO_CHATTO_API_KEY: 'secret' };
const parse = (text = source) => parseSettings(text, '/config/botto.toml', env);

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
