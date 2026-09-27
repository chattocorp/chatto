import { beforeEach, describe, expect, it, vi } from 'vitest';
import { TimeFormat } from '@chatto/api-types/api/v1/viewer_pb';
import { createAccountAPI } from '$lib/api-client/account';

const mocks = vi.hoisted(() => ({
  createClient: vi.fn(),
  createConnectTransport: vi.fn(),
  changePassword: vi.fn(),
  updateSettings: vi.fn(),
  requestAccountDeletion: vi.fn(),
  deleteMyAccount: vi.fn()
}));

vi.mock('@connectrpc/connect', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@connectrpc/connect')>();
  return {
    ...actual,
    createClient: mocks.createClient
  };
});

vi.mock('@connectrpc/connect-web', () => ({
  createConnectTransport: mocks.createConnectTransport
}));

describe('createAccountAPI', () => {
  beforeEach(() => {
    mocks.createClient.mockReset();
    mocks.createConnectTransport.mockReset();
    mocks.changePassword.mockReset();
    mocks.updateSettings.mockReset();
    mocks.requestAccountDeletion.mockReset();
    mocks.deleteMyAccount.mockReset();
    mocks.createConnectTransport.mockReturnValue({ kind: 'transport' });
    mocks.createClient.mockReturnValue({
      changePassword: mocks.changePassword,
      updateSettings: mocks.updateSettings,
      requestAccountDeletion: mocks.requestAccountDeletion,
      deleteMyAccount: mocks.deleteMyAccount
    });
  });

  it('updates settings and maps time format enums', async () => {
    mocks.updateSettings.mockResolvedValue({
      settings: {
        timezone: 'Europe/Berlin',
        timeFormat: TimeFormat.TIME_FORMAT_24_HOUR,
        shareTimezone: true
      }
    });

    const api = createAccountAPI({
      baseUrl: '/api/connect',
      bearerToken: null
    });

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

    expect(mocks.updateSettings).toHaveBeenCalledWith({
      timezone: 'Europe/Berlin',
      timeFormat: TimeFormat.TIME_FORMAT_24_HOUR,
      shareTimezone: true,
      updateMask: { paths: ['timezone', 'time_format', 'share_timezone'] }
    });
  });

  it('sets a password', async () => {
    mocks.changePassword.mockResolvedValue({});

    const api = createAccountAPI({
      baseUrl: '/api/connect',
      bearerToken: 'token'
    });

    await expect(
      api.changePassword({ password: 'newpassword456', currentPassword: 'oldpassword123' })
    ).resolves.toBeUndefined();

    expect(mocks.changePassword).toHaveBeenCalledWith({
      password: 'newpassword456',
      currentPassword: 'oldpassword123'
    });
  });

  it('sends empty timezone when clearing settings', async () => {
    mocks.updateSettings.mockResolvedValue({
      settings: {
        timeFormat: TimeFormat.TIME_FORMAT_AUTO
      }
    });

    const api = createAccountAPI({
      baseUrl: '/api/connect',
      bearerToken: null
    });

    await expect(api.updateSettings({ timezone: null })).resolves.toEqual({
      timezone: null,
      timeFormat: TimeFormat.TIME_FORMAT_AUTO,
      shareTimezone: undefined
    });

    expect(mocks.updateSettings).toHaveBeenCalledWith({
      timezone: '',
      timeFormat: undefined,
      shareTimezone: undefined,
      updateMask: { paths: ['timezone'] }
    });
  });

  it('requests and confirms account deletion', async () => {
    mocks.requestAccountDeletion.mockResolvedValue({ confirmationToken: 'AD-token' });
    mocks.deleteMyAccount.mockResolvedValue({});

    const api = createAccountAPI({
      baseUrl: '/api/connect',
      bearerToken: null
    });

    await expect(api.requestAccountDeletion()).resolves.toBe('AD-token');
    await expect(api.deleteMyAccount('AD-token')).resolves.toBe(true);

    expect(mocks.requestAccountDeletion).toHaveBeenCalledWith({});
    expect(mocks.deleteMyAccount).toHaveBeenCalledWith({ confirmationToken: 'AD-token' });
  });
});
