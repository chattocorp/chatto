import { describe, expect, it } from 'vitest';
import { safeSettingsReturnTo } from './returnTo';

describe('safeSettingsReturnTo', () => {
  const currentURL = new URL('https://chat.example/settings');

  it('preserves same-origin paths, queries, and fragments', () => {
    expect(safeSettingsReturnTo('/chat/server?tab=files#latest', currentURL, '/chat')).toBe(
      '/chat/server?tab=files#latest'
    );
  });

  it.each(['/\\evil.example', '//evil.example', '\\/\\evil.example'])(
    'rejects external redirect spelling %s',
    (candidate) => {
      expect(safeSettingsReturnTo(candidate, currentURL, '/chat')).toBe('/chat');
    }
  );

  it('rejects an encoded backslash after query decoding', () => {
    const candidate = new URL(
      'https://chat.example/settings?returnTo=/%5Cevil.example'
    ).searchParams.get('returnTo');
    expect(safeSettingsReturnTo(candidate, currentURL, '/chat')).toBe('/chat');
  });
});
