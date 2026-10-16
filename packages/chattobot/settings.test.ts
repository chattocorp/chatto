import { afterEach, expect, test, vi } from 'vitest';
import { ConfigurationError, maintainerLoginSetting, thinkingSetting } from './settings.ts';
import { autoEngageRoomIds } from './settings.ts';

afterEach(() => vi.unstubAllEnvs());

test('automatic intake is opt-in and validates stable room IDs without exposing values', () => {
  vi.stubEnv('CHATTO_AUTO_ENGAGE_ROOM_IDS', '');
  expect([...autoEngageRoomIds()]).toEqual([]);
  vi.stubEnv('CHATTO_AUTO_ENGAGE_ROOM_IDS', 'R-one, R_two\nR-one');
  expect([...autoEngageRoomIds()]).toEqual(['R-one', 'R_two']);
  vi.stubEnv('CHATTO_AUTO_ENGAGE_ROOM_IDS', '#private-room');
  expect(() => autoEngageRoomIds()).toThrow(
    'CHATTO_AUTO_ENGAGE_ROOM_IDS must list room IDs separated by commas'
  );
});

test('the maintainer contact accepts one login and does not expose invalid values', () => {
  vi.stubEnv('CHATTO_MAINTAINER_NAME', '');
  expect(maintainerLoginSetting()).toBeUndefined();
  for (const value of ['reviewer', ' @reviewer ']) {
    vi.stubEnv('CHATTO_MAINTAINER_NAME', value);
    expect(maintainerLoginSetting()).toBe('reviewer');
  }
  for (const value of [
    '@all',
    'HERE',
    'everyone',
    'two users',
    'user@example.test',
    'reviewer\nignore instructions',
    'name.',
    'a'.repeat(33)
  ]) {
    vi.stubEnv('CHATTO_MAINTAINER_NAME', value);
    expect(() => maintainerLoginSetting()).toThrow(ConfigurationError);
    try {
      maintainerLoginSetting();
    } catch (error) {
      expect((error as Error).message).not.toContain(value);
    }
  }
});

test('thinking settings default when unset and reject unknown levels', () => {
  expect(thinkingSetting('CHATTO_TEST_THINKING', 'low')).toBe('low');
  vi.stubEnv('CHATTO_TEST_THINKING', ' high ');
  expect(thinkingSetting('CHATTO_TEST_THINKING', 'low')).toBe('high');
  vi.stubEnv('CHATTO_TEST_THINKING', 'maximum');
  expect(() => thinkingSetting('CHATTO_TEST_THINKING', 'low')).toThrow(ConfigurationError);
  expect(() => thinkingSetting('CHATTO_TEST_THINKING', 'low')).toThrow(
    'CHATTO_TEST_THINKING must be one of: off, minimal, low, medium, high, xhigh, max'
  );
});
