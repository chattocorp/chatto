import { Code, ConnectError } from '@connectrpc/connect';
import { describe, expect, it } from 'vitest';
import { classifyManagementLoadError } from './managementLoadError';

describe('classifyManagementLoadError', () => {
  it.each([Code.PermissionDenied, Code.NotFound])(
    'treats code %s as an unavailable protected resource',
    (code) => {
      expect(classifyManagementLoadError(new ConnectError('unavailable', code))).toEqual({
        kind: 'access-denied'
      });
    }
  );

  it('preserves transient failure details for a retry state', () => {
    expect(
      classifyManagementLoadError(new ConnectError('temporarily unavailable', Code.Unavailable))
    ).toEqual({
      kind: 'failure',
      message: '[unavailable] temporarily unavailable'
    });
  });
});
