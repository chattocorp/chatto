import { describe, expect, it } from 'vitest';
import { MyAccountService } from '@chatto/api-types/api/v1/account_connect';
import { PresenceStatus } from '@chatto/api-types/api/v1/presence_pb';
import { createPresenceAPI } from '../presence.js';
import { fakeServer, mockService, receivedRequest } from '../../testing/fakeServer.js';

describe('createPresenceAPI', () => {
  it('reads, sets, and refreshes the presence preference', async () => {
    const mocks = mockService(MyAccountService);
    const api = createPresenceAPI(fakeServer((router) => router.service(MyAccountService, mocks)));
    const preference = { status: PresenceStatus.AWAY, revision: 'r2' };
    mocks.getPresencePreference.mockReturnValue({ preference });
    mocks.setPresencePreference.mockReturnValue({ preference });
    mocks.refreshPresence.mockReturnValue({});

    await expect(api.getPreference()).resolves.toMatchObject(preference);
    await expect(api.setPreference(PresenceStatus.AWAY, 'r1')).resolves.toMatchObject(preference);
    expect(receivedRequest(mocks.setPresencePreference)).toMatchObject({
      status: PresenceStatus.AWAY,
      expectedRevision: 'r1'
    });
    await expect(api.refreshPresence()).resolves.toBeUndefined();
  });
});
