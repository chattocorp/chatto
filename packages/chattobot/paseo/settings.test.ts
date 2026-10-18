import { afterEach, expect, test, vi } from 'vitest';
import { paseoSettings } from './settings.ts';

afterEach(() => vi.unstubAllEnvs());
function settings(url = '', path = '', password = '') {
  vi.stubEnv('CHATTO_PASEO_AGENT_MODEL', '');
  vi.stubEnv('CHATTO_PASEO_AGENT_MODE', '');
  vi.stubEnv('CHATTO_PASEO_AGENT_THINKING', '');
  vi.stubEnv('CHATTO_PASEO_URL', url);
  vi.stubEnv('CHATTO_PASEO_REPOSITORY_PATH', path);
  vi.stubEnv('CHATTO_PASEO_PASSWORD', password);
  return paseoSettings;
}

test('Paseo is disabled without configuration', () => {
  expect(settings()()).toBeUndefined();
});

test('implementation uses a host-selected model, permission mode, and thinking level', () => {
  const read = settings('ws://localhost:6767/ws', '/remote/chatto');
  vi.stubEnv('CHATTO_PASEO_AGENT_MODEL', 'test/coder');
  vi.stubEnv('CHATTO_PASEO_AGENT_MODE', 'auto-review');
  vi.stubEnv('CHATTO_PASEO_AGENT_THINKING', 'high');
  expect(read()?.agent).toEqual({
    model: 'test/coder',
    modeId: 'auto-review',
    thinkingOptionId: 'high'
  });
});

test('omitted thinking and permission settings leave provider defaults intact', () => {
  const read = settings('ws://localhost:6767/ws', '/remote/chatto');
  vi.stubEnv('CHATTO_PASEO_AGENT_MODEL', 'test/coder');
  expect(read()?.agent).toEqual({ model: 'test/coder' });
});

test('thinking configuration requires an implementation model', () => {
  const read = settings('ws://localhost:6767/ws', '/remote/chatto');
  vi.stubEnv('CHATTO_PASEO_AGENT_THINKING', 'high');
  expect(read).toThrow('CHATTO_PASEO_AGENT_MODEL');
});

test.each(['', 'coder', 'provider/'])(
  'rejects a permission mode without a valid model: %s',
  (model) => {
    const read = settings('ws://localhost:6767/ws', '/remote/chatto');
    vi.stubEnv('CHATTO_PASEO_AGENT_MODEL', model);
    vi.stubEnv('CHATTO_PASEO_AGENT_MODE', 'review');
    expect(read).toThrow('CHATTO_PASEO_AGENT_MODEL');
  }
);
test('accepts a remote absolute path without inspecting the local filesystem', () => {
  expect(settings('wss://paseo.test/ws', '/remote/chatto', 'secret')()).toEqual({
    url: 'wss://paseo.test/ws',
    repositoryPath: '/remote/chatto',
    password: 'secret'
  });
});
test.each([
  ['wss://paseo.test/ws', ''],
  ['', '/remote/chatto'],
  ['https://paseo.test/ws', '/remote/chatto'],
  ['ws://user:secret@paseo.test/ws', '/remote/chatto'],
  ['ws://paseo.test/ws?token=secret', '/remote/chatto'],
  ['ws://paseo.test/other', '/remote/chatto'],
  ['ws://paseo.test/ws', 'relative/path']
])('rejects invalid configuration without disclosing values', (url, path) => {
  const read = settings(url, path);
  expect(read).toThrow('CHATTO_PASEO_');
  try {
    read();
  } catch (error) {
    expect(String(error)).not.toContain('secret');
  }
});
