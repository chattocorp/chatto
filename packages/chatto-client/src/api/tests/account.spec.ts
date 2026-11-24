import { Timestamp } from '@bufbuild/protobuf';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { MyAccountService } from '@chatto/api-types/api/v1/account_connect';
import { TimeFormat } from '@chatto/api-types/api/v1/viewer_pb';
import { createAccountAPI } from '../account.js';
import { fakeServer, mockService, receivedRequest } from '../../testing/fakeServer.js';

const mocks = mockService(MyAccountService);

function accountAPI() {
  return createAccountAPI(fakeServer((router) => router.service(MyAccountService, mocks)));
}

describe('createAccountAPI', () => {
  beforeEach(() => {
    vi.resetAllMocks();
  });

  it('updates settings and maps time format enums', async () => {
    mocks.updateSettings.mockReturnValue({
      settings: {
        timezone: 'Europe/Berlin',
        timeFormat: TimeFormat.TIME_FORMAT_24_HOUR,
        shareTimezone: true
      }
    });

    const api = accountAPI();

    await expect(
      api.updateSettings({
        timezone: 'Europe/Berlin',
        timeFormat: TimeFormat.TIME_FORMAT_24_HOUR,
        shareTimezone: true
      })
    ).resolves.toEqual({
      timezone: 'Europe/Berlin',
      timeFormat: TimeFormat.TIME_FORMAT_24_HOUR,
      shareTimezone: true
    });

    expect(receivedRequest(mocks.updateSettings)).toMatchObject({
      timezone: 'Europe/Berlin',
      timeFormat: TimeFormat.TIME_FORMAT_24_HOUR,
      shareTimezone: true,
      updateMask: { paths: ['timezone', 'time_format', 'share_timezone'] }
    });
  });

  it('sets a password', async () => {
    mocks.changePassword.mockReturnValue({});

    const api = accountAPI();

    await expect(
      api.changePassword({ password: 'newpassword456', currentPassword: 'oldpassword123' })
    ).resolves.toBeUndefined();

    expect(receivedRequest(mocks.changePassword)).toMatchObject({
      password: 'newpassword456',
      currentPassword: 'oldpassword123'
    });
  });

  it('sends empty timezone when clearing settings', async () => {
    mocks.updateSettings.mockReturnValue({
      settings: {
        timeFormat: TimeFormat.TIME_FORMAT_AUTO
      }
    });

    const api = accountAPI();

    await expect(api.updateSettings({ timezone: null })).resolves.toEqual({
      timezone: null,
      timeFormat: TimeFormat.TIME_FORMAT_AUTO,
      shareTimezone: undefined
    });

    expect(receivedRequest(mocks.updateSettings)).toMatchObject({
      timezone: '',
      timeFormat: undefined,
      shareTimezone: undefined,
      updateMask: { paths: ['timezone'] }
    });
  });

  it('requests and confirms account deletion', async () => {
    mocks.requestAccountDeletion.mockReturnValue({ confirmationToken: 'AD-token' });
    mocks.deleteMyAccount.mockReturnValue({});

    const api = accountAPI();

    await expect(api.requestAccountDeletion()).resolves.toBe('AD-token');
    await expect(api.deleteMyAccount('AD-token')).resolves.toBe(true);

    expect(mocks.requestAccountDeletion).toHaveBeenCalledOnce();
    expect(receivedRequest(mocks.deleteMyAccount)).toMatchObject({ confirmationToken: 'AD-token' });
  });

  it('manages verified email addresses', async () => {
    const verifiedEmails = [
      { email: 'a@example.test', primary: true, verifiedAt: Timestamp.fromDate(new Date(0)) },
      { email: 'b@example.test', primary: false }
    ];
    const mapped = [
      { email: 'a@example.test', primary: true, verifiedAt: '1970-01-01T00:00:00.000Z' },
      { email: 'b@example.test', primary: false, verifiedAt: null }
    ];
    mocks.listVerifiedEmails.mockReturnValue({ verifiedEmails });
    mocks.requestEmailVerification.mockReturnValue({});
    mocks.confirmEmailVerification.mockReturnValue({ verifiedEmails });
    mocks.setPrimaryEmail.mockReturnValue({ verifiedEmails });
    const api = accountAPI();

    await expect(api.listVerifiedEmails('U1')).resolves.toEqual(mapped);
    await api.requestEmailVerification('U1', 'b@example.test');
    await expect(api.confirmEmailVerification('U1', 'b@example.test', '123456')).resolves.toEqual(
      mapped
    );
    await expect(api.setPrimaryEmail('U1', 'a@example.test')).resolves.toEqual(mapped);
    expect(receivedRequest(mocks.listVerifiedEmails)).toMatchObject({ expectedUserId: 'U1' });
    expect(receivedRequest(mocks.requestEmailVerification)).toMatchObject({
      expectedUserId: 'U1',
      email: 'b@example.test'
    });
    expect(receivedRequest(mocks.confirmEmailVerification)).toMatchObject({
      expectedUserId: 'U1',
      email: 'b@example.test',
      code: '123456'
    });
    expect(receivedRequest(mocks.setPrimaryEmail)).toMatchObject({
      expectedUserId: 'U1',
      email: 'a@example.test'
    });
  });
});
