import { describe, expect, it } from 'vitest';
import { atprotoHandleContinuationUrl } from './handleContinuation';

const origin = 'https://chat.example';

describe('atprotoHandleContinuationUrl', () => {
  it('continues the provider start with the handle', () => {
    expect(
      atprotoHandleContinuationUrl(
        '/auth/providers/bsky?redirect=%2Fchat&intent=link',
        ' @alice.example ',
        origin
      )
    ).toBe('/auth/providers/bsky?redirect=%2Fchat&intent=link&login_hint=alice.example');
  });

  it.each([
    null,
    '',
    '/chat',
    'https://evil.example/auth/providers/bsky',
    '//evil.example/auth/providers/bsky',
    '/auth/providers/../../chat'
  ])('rejects %s', (next) => {
    expect(atprotoHandleContinuationUrl(next, 'alice.example', origin)).toBeNull();
  });
});
