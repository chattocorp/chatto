import { describe, it, expect } from 'vitest';
import { print } from 'graphql';
import { GetSpaceNotificationPreferencesDocument } from '$lib/gql/graphql';

describe('GetSpaceNotificationPreferences query', () => {
  // Regression: DM rooms have an empty `name` (clients derive their label from
  // `members`), so without `type: CHANNEL` they'd render as blank `#` rows in
  // the Room Overrides list. Server-side filter via `User.rooms(type: …)`.
  it('asks the server to return only channel rooms, not DMs', () => {
    const printed = print(GetSpaceNotificationPreferencesDocument);
    expect(printed).toMatch(/rooms\s*\(\s*type:\s*CHANNEL\s*\)/);
  });
});
