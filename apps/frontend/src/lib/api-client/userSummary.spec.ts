import { describe, expect, it } from 'vitest';
import type { User as APIUser } from '@chatto/api-types/api/v1/users_pb';
import { PresenceStatus } from '@chatto/api-types/api/v1/presence_pb';
import {
  mapOptionalUserSummary,
  mapUserPresenceView,
  mapUserSummary
} from './userSummary';

function apiUser(overrides: Partial<APIUser> = {}): APIUser {
  return {
    id: 'U1',
    login: 'alice',
    displayName: 'Alice',
    deleted: false,
    isBot: false,
    avatarUrl: '',
    presenceStatus: PresenceStatus.ONLINE,
    customStatus: undefined,
    ...overrides
  } as APIUser;
}

// Stand-in for a generated protobuf Timestamp in fixture data.
function protoTimestamp(date: Date): unknown {
  return { toDate: () => date };
}

describe('mapUserSummary', () => {
  it('normalizes unset and empty avatar URLs to null', () => {
    expect(mapUserSummary(apiUser({ avatarUrl: '' })).avatarUrl).toBeNull();
    expect(mapUserSummary(apiUser({ avatarUrl: undefined })).avatarUrl).toBeNull();
    expect(mapUserSummary(apiUser({ avatarUrl: '/assets/u1' })).avatarUrl).toBe('/assets/u1');
  });
});

describe('mapOptionalUserSummary', () => {
  it('returns null when the response omits the user', () => {
    expect(mapOptionalUserSummary(undefined)).toBeNull();
    expect(mapOptionalUserSummary(apiUser())?.id).toBe('U1');
  });
});

describe('mapUserPresenceView', () => {
  it('falls back to offline presence and null custom status', () => {
    const view = mapUserPresenceView(undefined);
    expect(view.presenceStatus).toBe(PresenceStatus.OFFLINE);
    expect(view.customStatus).toBeNull();
  });

  it('converts custom status expiry timestamps to ISO strings', () => {
    const expiresAt = new Date('2026-04-29T12:00:00Z');
    const view = mapUserPresenceView({
      ...apiUser(),
      customStatus: {
        emoji: 'coffee',
        text: 'brewing',
        expiresAt: protoTimestamp(expiresAt)
      }
    } as APIUser);
    expect(view.customStatus).toEqual({
      emoji: 'coffee',
      text: 'brewing',
      expiresAt: '2026-04-29T12:00:00.000Z'
    });
  });
});
