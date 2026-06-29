import { beforeEach, describe, expect, it, vi } from 'vitest';
import { TimeFormat as APITimeFormat } from '@chatto/api-types/api/v1/viewer_pb';
import { TimeFormat } from '$lib/render/types';
import { createAccountAPI } from './account';

const mocks = vi.hoisted(() => ({
  createClient: vi.fn(),
  createConnectTransport: vi.fn(),
  updateProfile: vi.fn(),
  uploadAvatar: vi.fn(),
  deleteAvatar: vi.fn(),
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
    mocks.uploadAvatar.mockReset();
    mocks.deleteAvatar.mockReset();
    mocks.updateSettings.mockReset();
    mocks.requestAccountDeletion.mockReset();
    mocks.deleteMyAccount.mockReset();
    mocks.createConnectTransport.mockReturnValue({ kind: 'transport' });
    mocks.createClient.mockReturnValue({
      updateProfile: mocks.updateProfile,
      uploadAvatar: mocks.uploadAvatar,
      deleteAvatar: mocks.deleteAvatar,
      updateSettings: mocks.updateSettings,
      requestAccountDeletion: mocks.requestAccountDeletion,
      deleteMyAccount: mocks.deleteMyAccount
    });
  });

  it('updates profile and avatar with bearer auth', async () => {
    mocks.updateProfile.mockResolvedValue({
      user: {
        id: 'U1',
        login: 'alice2',
        displayName: 'Alice Two',
        avatarUrl: 'https://cdn/avatar.webp'
      }
    });
    mocks.deleteAvatar.mockResolvedValue({
      user: {
        id: 'U1',
        login: 'alice2',
        displayName: 'Alice Two'
      }
    });
    mocks.uploadAvatar.mockResolvedValue({
      user: {
        id: 'U1',
        login: 'alice2',
        displayName: 'Alice Two',
        avatarUrl: 'https://cdn/new-avatar.webp'
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
        avatarUrl: 'https://cdn/avatar.webp'
      }
    );
    await expect(api.deleteAvatar()).resolves.toEqual({
      id: 'U1',
      login: 'alice2',
      displayName: 'Alice Two',
      avatarUrl: null
    });
    await expect(
      api.uploadAvatar(new File([new Uint8Array([1, 2, 3])], 'avatar.png', { type: 'image/png' }))
    ).resolves.toEqual({
      id: 'U1',
      login: 'alice2',
      displayName: 'Alice Two',
      avatarUrl: 'https://cdn/new-avatar.webp'
    });

    expect(mocks.createConnectTransport).toHaveBeenCalledWith({
      baseUrl: 'https://origin.test/api/connect',
      useBinaryFormat: true
    });
    expect(mocks.updateProfile).toHaveBeenCalledWith(
      { displayName: 'Alice Two', login: 'alice2' },
      { headers: { Authorization: 'Bearer token' } }
    );
    expect(mocks.deleteAvatar).toHaveBeenCalledWith(
      {},
      { headers: { Authorization: 'Bearer token' } }
    );
    expect(mocks.uploadAvatar).toHaveBeenCalledWith(
      {
        image: new Uint8Array([1, 2, 3]),
        filename: 'avatar.png',
        contentType: 'image/png'
      },
      { headers: { Authorization: 'Bearer token' } }
    );
  });

  it('updates settings and maps time format enums', async () => {
    mocks.updateSettings.mockResolvedValue({
      settings: {
        timezone: 'Europe/Berlin',
        timeFormat: APITimeFormat.TIME_FORMAT_24_HOUR
      }
    });

    const api = createAccountAPI({
      baseUrl: '/api/connect',
      bearerToken: null
    });

    await expect(
      api.updateSettings({
        timezone: 'Europe/Berlin',
        timeFormat: TimeFormat.TwentyFourHour
      })
    ).resolves.toEqual({
      timezone: 'Europe/Berlin',
      timeFormat: TimeFormat.TwentyFourHour
    });

    expect(mocks.updateSettings).toHaveBeenCalledWith(
      {
        timezone: 'Europe/Berlin',
        timeFormat: APITimeFormat.TIME_FORMAT_24_HOUR
      },
      { headers: undefined }
    );
  });

  it('sends empty timezone when clearing settings', async () => {
    mocks.updateSettings.mockResolvedValue({
      settings: {
        timeFormat: APITimeFormat.TIME_FORMAT_AUTO
      }
    });

    const api = createAccountAPI({
      baseUrl: '/api/connect',
      bearerToken: null
    });

    await expect(api.updateSettings({ timezone: null })).resolves.toEqual({
      timezone: null,
      timeFormat: TimeFormat.Auto
    });

    expect(mocks.updateSettings).toHaveBeenCalledWith(
      {
        timezone: '',
        timeFormat: undefined
      },
      { headers: undefined }
    );
  });

  it('requests and confirms account deletion', async () => {
    mocks.requestAccountDeletion.mockResolvedValue({ confirmationToken: 'AD-token' });
    mocks.deleteMyAccount.mockResolvedValue({ deleted: true });

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
