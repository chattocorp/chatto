import { describe, expect, it, vi } from 'vitest';

vi.mock('$app/paths', () => ({
  resolve: (path: string, params: Record<string, string>) =>
    Object.entries(params).reduce(
      (resolved, [name, value]) => resolved.replace(`[${name}]`, value),
      path
    )
}));

import { legacyServerAdminDestination } from './legacyServerAdminRedirect';

describe('legacyServerAdminDestination', () => {
  it.each([
    ['', '/chat/acme/manage/server/general'],
    ['general', '/chat/acme/manage/server/general'],
    ['members/user-1', '/chat/acme/manage/server/members/user-1'],
    ['permissions/new', '/chat/acme/manage/server/permissions/new'],
    ['event-log/42', '/chat/acme/manage/server/event-log/42'],
    ['rooms', '/chat/acme/manage/rooms'],
    ['rooms/room/room-1', '/chat/acme/manage/rooms/room-1'],
    ['rooms/group/group-1', '/chat/acme/manage/room-groups/group-1']
  ])('maps %s', (legacyPath, expected) => {
    expect(legacyServerAdminDestination('acme', legacyPath)).toBe(expected);
  });

  it('preserves the query string', () => {
    expect(legacyServerAdminDestination('acme', 'members', '?offset=20')).toBe(
      '/chat/acme/manage/server/members?offset=20'
    );
  });

  it.each(['rooms/room', 'rooms/group', 'rooms/unknown/id', 'unknown'])(
    'rejects malformed or unknown path %s',
    (legacyPath) => {
      expect(legacyServerAdminDestination('acme', legacyPath)).toBeNull();
    }
  );
});
