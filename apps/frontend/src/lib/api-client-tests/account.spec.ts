import { beforeEach, describe, expect, it, vi } from 'vitest';
import { MyAccountService } from '@chatto/api-types/api/v1/account_connect';
import { TimeFormat } from '@chatto/api-types/api/v1/viewer_pb';
import { createAccountAPI } from '$lib/api-client/account';
import { fakeServer, mockService, receivedRequest } from '$lib/test-utils';

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
});
