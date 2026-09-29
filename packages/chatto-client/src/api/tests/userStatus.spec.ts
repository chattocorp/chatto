import { Timestamp } from '@bufbuild/protobuf';
import { describe, expect, it } from 'vitest';
import { MyAccountService } from '@chatto/api-types/api/v1/account_connect';
import { deleteCustomStatus, setCustomStatus } from '../userStatus.js';
import { fakeServer, mockService, receivedRequest } from '../../testing/fakeServer.js';

function statusAPI() {
  const mocks = mockService(MyAccountService);
  const config = {
    ...fakeServer((router) => router.service(MyAccountService, mocks)),
    serverId: 'S1'
  };
  return { mocks, config };
}

describe('custom user status', () => {
  it('sets a status with an expiry and maps the answer', async () => {
    const { mocks, config } = statusAPI();
    const expiresAt = '2026-09-30T08:00:00.000Z';
    mocks.setCustomStatus.mockReturnValue({
      status: { emoji: '🌴', text: 'Away', expiresAt: Timestamp.fromDate(new Date(expiresAt)) }
    });
    await expect(
      setCustomStatus(config, { emoji: '🌴', text: 'Away', expiresAt })
    ).resolves.toEqual({ emoji: '🌴', text: 'Away', expiresAt });
    expect(receivedRequest(mocks.setCustomStatus)?.expiresAt?.toDate().toISOString()).toBe(
      expiresAt
    );
  });

  it('sets a status without an expiry', async () => {
    const { mocks, config } = statusAPI();
    mocks.setCustomStatus.mockReturnValue({ status: { emoji: '📚', text: 'Reading' } });
    await expect(setCustomStatus(config, { emoji: '📚', text: 'Reading' })).resolves.toEqual({
      emoji: '📚',
      text: 'Reading',
      expiresAt: null
    });
    expect(receivedRequest(mocks.setCustomStatus)?.expiresAt).toBeUndefined();
  });

  it('deletes the status', async () => {
    const { mocks, config } = statusAPI();
    mocks.deleteCustomStatus.mockReturnValue({});
    await expect(deleteCustomStatus(config)).resolves.toBeNull();
  });
});
