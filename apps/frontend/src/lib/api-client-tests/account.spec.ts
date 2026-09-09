import { beforeEach, describe, expect, it, vi } from 'vitest';
import { TimeFormat } from '@chatto/api-types/api/v1/viewer_pb';
import { createAccountAPI } from '$lib/api-client/account';

const mocks = vi.hoisted(() => ({
  createClient: vi.fn(),
  createConnectTransport: vi.fn(),
  updateProfile: vi.fn(),
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
    mocks.updateProfile.mockReset();
    mocks.changePassword.mockReset();
    mocks.updateSettings.mockReset();
    mocks.requestAccountDeletion.mockReset();
    mocks.deleteMyAccount.mockReset();
    mocks.createConnectTransport.mockReturnValue({ kind: 'transport' });
    mocks.createClient.mockReturnValue({
      updateProfile: mocks.updateProfile,
      changePassword: mocks.changePassword,
      updateSettings: mocks.updateSettings,
      requestAccountDeletion: mocks.requestAccountDeletion,
      deleteMyAccount: mocks.deleteMyAccount
    });
  });

  it('updates a profile with bearer auth', async () => {
    mocks.updateProfile.mockResolvedValue({
      user: {
        id: 'U1',
        login: 'alice2',
        displayName: 'Alice Two',
        avatarUrl: 'https://cdn/avatar.webp'
      }
    });
    const api = createAccountAPI({
      baseUrl: 'https://origin.test/api/connect',
      bearerToken: 'token'
    });

    await expect(api.updateProfile({ displayName: 'Alice Two', login: 'alice2' })).resolves.toEqual(
      {
        id: 'U1',
        login: 'alice2',
        displayName: 'Alice Two',
        avatarUrl: 'https://cdn/avatar.webp',
        bio: null
      }
    );
    expect(mocks.createConnectTransport).toHaveBeenCalledWith({
      baseUrl: 'https://origin.test/api/connect',
      useBinaryFormat: true
    });
    expect(mocks.updateProfile).toHaveBeenCalledWith(
      {
        displayName: 'Alice Two',
        login: 'alice2',
        updateMask: { paths: ['display_name', 'login'] }
      },
      { headers: { Authorization: 'Bearer token' } }
    );
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

    expect(mocks.updateSettings).toHaveBeenCalledWith(
      {
        timezone: 'Europe/Berlin',
        timeFormat: TimeFormat.TIME_FORMAT_24_HOUR,
        shareTimezone: true,
        updateMask: { paths: ['timezone', 'time_format', 'share_timezone'] }
      },
      { headers: undefined }
    );
  });

  it('sets a password with bearer auth', async () => {
    mocks.changePassword.mockResolvedValue({});

    const api = createAccountAPI({
      baseUrl: '/api/connect',
      bearerToken: 'token'
    });

    await expect(
      api.changePassword({ password: 'newpassword456', currentPassword: 'oldpassword123' })
    ).resolves.toBeUndefined();

    expect(mocks.changePassword).toHaveBeenCalledWith(
      { password: 'newpassword456', currentPassword: 'oldpassword123' },
      { headers: { Authorization: 'Bearer token' } }
    );
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

    expect(mocks.updateSettings).toHaveBeenCalledWith(
      {
        timezone: '',
        timeFormat: undefined,
        shareTimezone: undefined,
        updateMask: { paths: ['timezone'] }
      },
      { headers: undefined }
    );
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

    expect(mocks.requestAccountDeletion).toHaveBeenCalledWith({}, { headers: undefined });
    expect(mocks.deleteMyAccount).toHaveBeenCalledWith(
      { confirmationToken: 'AD-token' },
      { headers: undefined }
    );
  });
});
