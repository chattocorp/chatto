import { afterEach, expect, test, vi } from 'vitest';
import { ConfigurationError, thinkingSetting } from './settings.ts';

afterEach(() => vi.unstubAllEnvs());

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
