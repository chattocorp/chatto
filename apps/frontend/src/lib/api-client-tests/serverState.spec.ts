import { protoInt64 } from '@bufbuild/protobuf';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import {
  deleteServerBanner,
  deleteServerLogo,
  getAuthenticatedServerState,
  getServerSecurityConfig,
  updateBlockedUsernames,
  updateServerConfig,
  uploadServerBanner,
  uploadServerLogo
} from '@chatto/api-client/serverState';

const mocks = vi.hoisted(() => ({
  createClient: vi.fn(),
  createConnectTransport: vi.fn(),
  getServerState: vi.fn(),
  updateServerConfig: vi.fn(),
  uploadServerLogo: vi.fn(),
  deleteServerLogo: vi.fn(),
  uploadServerBanner: vi.fn(),
  deleteServerBanner: vi.fn(),
  getServerSecurityConfig: vi.fn(),
  updateBlockedUsernames: vi.fn()
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

describe('getAuthenticatedServerState', () => {
  beforeEach(() => {
    mocks.createClient.mockReset();
    mocks.createConnectTransport.mockReset();
    mocks.getServerState.mockReset();
    mocks.updateServerConfig.mockReset();
    mocks.uploadServerLogo.mockReset();
    mocks.deleteServerLogo.mockReset();
    mocks.uploadServerBanner.mockReset();
    mocks.deleteServerBanner.mockReset();
    mocks.getServerSecurityConfig.mockReset();
    mocks.updateBlockedUsernames.mockReset();
    mocks.createConnectTransport.mockReturnValue({ kind: 'transport' });
    mocks.createClient.mockReturnValue({
      getServerState: mocks.getServerState,
      updateServerConfig: mocks.updateServerConfig,
      uploadServerLogo: mocks.uploadServerLogo,
      deleteServerLogo: mocks.deleteServerLogo,
      uploadServerBanner: mocks.uploadServerBanner,
      deleteServerBanner: mocks.deleteServerBanner,
      getServerSecurityConfig: mocks.getServerSecurityConfig,
      updateBlockedUsernames: mocks.updateBlockedUsernames
    });
  });

  it('loads authenticated server state and maps optional and int64 fields', async () => {
    mocks.getServerState.mockResolvedValue({
      profile: {
        name: 'Remote Chatto',
        logoUrl: 'https://cdn/logo.webp',
        bannerUrl: 'https://cdn/banner.webp',
        welcomeMessage: 'welcome',
        description: 'description',
        motd: 'hello'
      },
      pushNotificationsEnabled: true,
      vapidPublicKey: 'vapid',
      livekitUrl: 'wss://livekit',
      videoProcessingEnabled: true,
      maxUploadSize: protoInt64.parse(123),
      maxVideoUploadSize: protoInt64.parse(456),
      messageEditWindowSeconds: 7200,
      viewerCapabilities: {
        hasAnyAdminPermission: true,
        canManageServer: true,
        canCreateRoom: true,
        canManageRooms: false,
        hasUnreadRooms: true
      }
    });

    const state = await getAuthenticatedServerState({
      baseUrl: 'https://chat.example.test/api/connect',
      bearerToken: 'token'
    });

    expect(mocks.createConnectTransport).toHaveBeenCalledWith({
      baseUrl: 'https://chat.example.test/api/connect',
      useBinaryFormat: true
    });
    expect(mocks.getServerState).toHaveBeenCalledWith(
      {},
      { headers: { Authorization: 'Bearer token' } }
    );
    expect(state).toEqual({
      name: 'Remote Chatto',
      logoUrl: 'https://cdn/logo.webp',
      bannerUrl: 'https://cdn/banner.webp',
      welcomeMessage: 'welcome',
      description: 'description',
      motd: 'hello',
      pushNotificationsEnabled: true,
      vapidPublicKey: 'vapid',
      livekitUrl: 'wss://livekit',
      videoProcessingEnabled: true,
      maxUploadSize: 123,
      maxVideoUploadSize: 456,
      messageEditWindowSeconds: 7200,
      viewerHasAnyAdminPermission: true,
      viewerCanManageServer: true,
      viewerCanCreateRoom: true,
      viewerCanManageRooms: false,
      viewerHasUnreadRooms: true
    });
  });

  it('maps absent optional fields to null and omits auth headers without a token', async () => {
    mocks.getServerState.mockResolvedValue({
      profile: {},
      pushNotificationsEnabled: false,
      videoProcessingEnabled: false,
      maxUploadSize: protoInt64.zero,
      maxVideoUploadSize: protoInt64.zero,
      messageEditWindowSeconds: 10800
    });

    const state = await getAuthenticatedServerState({
      baseUrl: '/api/connect',
      bearerToken: null
    });

    expect(mocks.getServerState).toHaveBeenCalledWith({}, { headers: undefined });
    expect(state.name).toBe('Chatto');
    expect(state.logoUrl).toBeNull();
    expect(state.bannerUrl).toBeNull();
    expect(state.welcomeMessage).toBeNull();
    expect(state.description).toBeNull();
    expect(state.motd).toBeNull();
    expect(state.vapidPublicKey).toBeNull();
    expect(state.livekitUrl).toBeNull();
    expect(state.viewerHasAnyAdminPermission).toBe(false);
    expect(state.viewerCanManageServer).toBe(false);
    expect(state.viewerCanCreateRoom).toBe(false);
    expect(state.viewerCanManageRooms).toBe(false);
    expect(state.viewerHasUnreadRooms).toBe(false);
  });

  it('updates server config with bearer auth and maps the returned profile', async () => {
    mocks.updateServerConfig.mockResolvedValue({
      profile: {
        name: 'Connect Server',
        description: 'Connect description',
        motd: 'Connect MOTD',
        welcomeMessage: 'Connect welcome',
        logoUrl: 'https://cdn/logo.webp',
        bannerUrl: 'https://cdn/banner.webp'
      }
    });

    const profile = await updateServerConfig(
      {
        baseUrl: 'https://chat.example.test/api/connect',
        bearerToken: 'token'
      },
      {
        name: 'Connect Server',
        description: 'Connect description',
        motd: 'Connect MOTD',
        welcomeMessage: 'Connect welcome'
      }
    );

    expect(mocks.createConnectTransport).toHaveBeenCalledWith({
      baseUrl: 'https://chat.example.test/api/connect',
      useBinaryFormat: true
    });
    expect(mocks.updateServerConfig).toHaveBeenCalledWith(
      {
        serverName: 'Connect Server',
        description: 'Connect description',
        motd: 'Connect MOTD',
        welcomeMessage: 'Connect welcome'
      },
      { headers: { Authorization: 'Bearer token' } }
    );
    expect(profile).toEqual({
      name: 'Connect Server',
      description: 'Connect description',
      motd: 'Connect MOTD',
      welcomeMessage: 'Connect welcome',
      logoUrl: 'https://cdn/logo.webp',
      bannerUrl: 'https://cdn/banner.webp'
    });
  });

  it('updates server branding through AdminServerService', async () => {
    mocks.uploadServerLogo.mockResolvedValue({
      profile: {
        name: 'Connect Server',
        logoUrl: 'https://cdn/new-logo.webp'
      }
    });
    mocks.deleteServerLogo.mockResolvedValue({
      profile: {
        name: 'Connect Server'
      }
    });
    mocks.uploadServerBanner.mockResolvedValue({
      profile: {
        name: 'Connect Server',
        bannerUrl: 'https://cdn/new-banner.webp'
      }
    });
    mocks.deleteServerBanner.mockResolvedValue({
      profile: {
        name: 'Connect Server'
      }
    });

    const config = {
      baseUrl: 'https://chat.example.test/api/connect',
      bearerToken: 'token'
    };

    await expect(
      uploadServerLogo(
        config,
        new File([new Uint8Array([1, 2, 3])], 'logo.png', { type: 'image/png' })
      )
    ).resolves.toMatchObject({ logoUrl: 'https://cdn/new-logo.webp' });
    await expect(deleteServerLogo(config)).resolves.toMatchObject({ logoUrl: null });
    await expect(
      uploadServerBanner(
        config,
        new File([new Uint8Array([4, 5, 6])], 'banner.png', { type: 'image/png' })
      )
    ).resolves.toMatchObject({ bannerUrl: 'https://cdn/new-banner.webp' });
    await expect(deleteServerBanner(config)).resolves.toMatchObject({ bannerUrl: null });

    expect(mocks.uploadServerLogo).toHaveBeenCalledWith(
      {
        image: new Uint8Array([1, 2, 3]),
        filename: 'logo.png',
        contentType: 'image/png'
      },
      { headers: { Authorization: 'Bearer token' } }
    );
    expect(mocks.deleteServerLogo).toHaveBeenCalledWith(
      {},
      { headers: { Authorization: 'Bearer token' } }
    );
    expect(mocks.uploadServerBanner).toHaveBeenCalledWith(
      {
        image: new Uint8Array([4, 5, 6]),
        filename: 'banner.png',
        contentType: 'image/png'
      },
      { headers: { Authorization: 'Bearer token' } }
    );
    expect(mocks.deleteServerBanner).toHaveBeenCalledWith(
      {},
      { headers: { Authorization: 'Bearer token' } }
    );
  });

  it('loads and updates security config through AdminServerService', async () => {
    mocks.getServerSecurityConfig.mockResolvedValue({
      blockedUsernames: 'root\nadmin'
    });
    mocks.updateBlockedUsernames.mockResolvedValue({
      blockedUsernames: 'root\nadmin\nreserved'
    });

    const config = {
      baseUrl: 'https://chat.example.test/api/connect',
      bearerToken: 'token'
    };

    await expect(getServerSecurityConfig(config)).resolves.toEqual({
      blockedUsernames: 'root\nadmin'
    });
    await expect(updateBlockedUsernames(config, 'root\nadmin\nreserved')).resolves.toEqual({
      blockedUsernames: 'root\nadmin\nreserved'
    });

    expect(mocks.getServerSecurityConfig).toHaveBeenCalledWith(
      {},
      { headers: { Authorization: 'Bearer token' } }
    );
    expect(mocks.updateBlockedUsernames).toHaveBeenCalledWith(
      { blockedUsernames: 'root\nadmin\nreserved' },
      { headers: { Authorization: 'Bearer token' } }
    );
  });
});
