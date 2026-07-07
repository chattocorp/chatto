import { describe, expect, it } from 'vitest';
import { isSafeInternalPath } from './safeInternalPath';

describe('isSafeInternalPath', () => {
  it.each(['/chat', '/chat?tab=rooms', '/oauth/authorize?client_id=test', '/'])(
    'accepts %s',
    (value) => {
      expect(isSafeInternalPath(value)).toBe(true);
    }
  );

  it.each([
    '',
    null,
    undefined,
    'chat',
    '//attacker.example/path',
    '/\\attacker.example/path',
    'https://attacker.example/path',
    'http://attacker.example/path',
    'javascript:alert(1)'
  ])('rejects %s', (value) => {
    expect(isSafeInternalPath(value)).toBe(false);
  });
});
