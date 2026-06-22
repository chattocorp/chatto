import { Code, ConnectError } from '@connectrpc/connect';
import { describe, expect, it } from 'vitest';
import { shouldFallbackToGraphQL } from './notificationPreferences';

describe('notificationPreferences API', () => {
  it('falls back only for unavailable ConnectRPC paths', () => {
    expect(shouldFallbackToGraphQL(new ConnectError('unimplemented', Code.Unimplemented))).toBe(
      true
    );
    expect(shouldFallbackToGraphQL(new ConnectError('unavailable', Code.Unavailable))).toBe(true);
    expect(shouldFallbackToGraphQL(new ConnectError('permission denied', Code.PermissionDenied))).toBe(
      false
    );
    expect(shouldFallbackToGraphQL(new Error('network blocked'))).toBe(true);
  });
});
