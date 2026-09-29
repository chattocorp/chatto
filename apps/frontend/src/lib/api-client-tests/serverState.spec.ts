import { protoInt64 } from '@bufbuild/protobuf';
import { Code } from '@connectrpc/connect';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { AdminServerService } from '@chatto/api-types/admin/v1/server_connect';
import { ServerService } from '@chatto/api-types/api/v1/server_state_connect';
import { ViewerService } from '@chatto/api-types/api/v1/viewer_connect';
import { ServerDiscoveryService } from '@chatto/api-types/chatto/discovery/v1/server_connect';
import {
  deleteServerBanner,
  deleteServerLogo,
  getAuthenticatedServerState,
  getServerSecurityConfig,
  updateBlockedUsernames,
  updateServerConfig,
  uploadServerBanner,
  uploadServerLogo
} from '$lib/api-client/serverState';
import { fakeServer, mockService, receivedRequest } from '$lib/test-utils';

const discovery = mockService(ServerDiscoveryService);
const server = mockService(ServerService);
const viewer = mockService(ViewerService);
const admin = mockService(AdminServerService);

function serverConfig() {
  return fakeServer((router) =>
    router
      .service(ServerDiscoveryService, discovery)
      .service(ServerService, server)
      .service(ViewerService, viewer)
      .service(AdminServerService, admin)
  );
}

describe('getAuthenticatedServerState', () => {
  beforeEach(() => {
    vi.resetAllMocks();
  });

  it('loads authenticated server state and maps optional and int64 fields', async () => {
    discovery.getServer.mockReturnValue({
      profile: {
        name: 'Remote Chatto',
        version: '9.8.7',
        logoUrl: 'https://cdn/logo.webp',
        bannerUrl: 'https://cdn/banner.webp',
        welcomeMessage: 'welcome',
        description: 'description'
      }
    });
    server.getMotd.mockReturnValue({ motd: 'hello' });
    server.getRuntimeConfig.mockReturnValue({
      runtime: {
        pushNotificationsEnabled: true,
        vapidPublicKey: 'vapid',
        livekitUrl: 'wss://livekit',
        videoProcessingEnabled: true,
        maxUploadSize: protoInt64.parse(123),
        maxVideoUploadSize: protoInt64.parse(456),
        messageEditWindowSeconds: 7200
      }
    });
    viewer.getViewer.mockReturnValue({
      viewerPermissions: {
        permissions: [
          { permission: 'server.manage', granted: true },
          { permission: 'room.create', granted: true },
          { permission: 'room.join', granted: true },
          { permission: 'room.list', granted: true },
          { permission: 'room.manage', granted: false },
          { permission: 'room.remove-member', granted: true },
          { permission: 'message.post', granted: true },
          { permission: 'message.post-in-thread', granted: true },
          { permission: 'message.attach', granted: true },
          { permission: 'message.manage', granted: false },
          { permission: 'message.react', granted: true },
          { permission: 'message.echo', granted: true },
          { permission: 'role.manage', granted: true },
          { permission: 'role.assign', granted: true },
          { permission: 'admin.view-users', granted: true },
          { permission: 'admin.view-audit', granted: true },
          { permission: 'user.delete-any', granted: false },
          { permission: 'user.delete-self', granted: true },
          { permission: 'user.manage-permissions', granted: true },
          { permission: 'bot.example.do-thing', granted: true }
        ]
      },
      capabilities: {
        grants: [{ capability: 'admin.view-system', granted: true }]
      },
      viewerState: {
        hasUnreadRooms: true
      }
    });

    const state = await getAuthenticatedServerState(serverConfig());

    expect(receivedRequest(discovery.getServer)).toMatchObject({});
    expect(receivedRequest(server.getMotd)).toMatchObject({});
    expect(receivedRequest(server.getRuntimeConfig)).toMatchObject({});
    expect(receivedRequest(viewer.getViewer)).toMatchObject({});
    expect(state).toEqual({
      name: 'Remote Chatto',
      version: '9.8.7',
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
      viewerPermissions: {
        'admin.view-audit': true,
        'admin.view-users': true,
        'bot.example.do-thing': true,
        'message.attach': true,
        'message.echo': true,
        'message.manage': false,
        'message.post': true,
        'message.post-in-thread': true,
        'message.react': true,
        'role.assign': true,
        'role.manage': true,
        'room.remove-member': true,
        'room.create': true,
        'room.join': true,
        'room.list': true,
        'room.manage': false,
        'server.manage': true,
        'user.delete-any': false,
        'user.delete-self': true,
        'user.manage-permissions': true
      },
      viewerCanManageServer: true,
      viewerCanCreateRooms: true,
      viewerCanJoinRooms: true,
      viewerCanListRooms: true,
      viewerCanManageRooms: false,
      viewerCanBanRoomMembers: true,
      viewerCanPostMessages: true,
      viewerCanPostInThreads: true,
      viewerCanAttachFiles: true,
      viewerCanManageMessages: false,
      viewerCanReactToMessages: true,
      viewerCanEchoMessages: true,
      viewerCanManageRoles: true,
      viewerCanAssignRoles: true,
      viewerCanViewAdminUsers: true,
      viewerCanViewAdminSystem: true,
      viewerCanViewAdminAudit: true,
      viewerCanDeleteAnyUser: false,
      viewerCanDeleteSelf: true,
      viewerCanManageUserPermissions: true,
      viewerHasUnreadRooms: true
    });
  });

  it('maps absent optional fields to null', async () => {
    discovery.getServer.mockReturnValue({
      profile: {}
    });
    server.getMotd.mockReturnValue({});
    server.getRuntimeConfig.mockReturnValue({
      runtime: {
        pushNotificationsEnabled: false,
        videoProcessingEnabled: false,
        maxUploadSize: protoInt64.zero,
        maxVideoUploadSize: protoInt64.zero,
        messageEditWindowSeconds: 10800
      }
    });
    viewer.getViewer.mockReturnValue({});

    const state = await getAuthenticatedServerState(serverConfig());

    expect(receivedRequest(discovery.getServer)).toMatchObject({});
    expect(receivedRequest(server.getMotd)).toMatchObject({});
    expect(receivedRequest(server.getRuntimeConfig)).toMatchObject({});
    expect(receivedRequest(viewer.getViewer)).toMatchObject({});
    expect(state.name).toBe('Chatto');
    expect(state.version).toBe('');
    expect(state.logoUrl).toBeNull();
    expect(state.bannerUrl).toBeNull();
    expect(state.welcomeMessage).toBeNull();
    expect(state.description).toBeNull();
    expect(state.motd).toBeNull();
    expect(state.vapidPublicKey).toBeNull();
    expect(state.livekitUrl).toBeNull();
    expect(state.viewerCanManageServer).toBe(false);
    expect(state.viewerCanCreateRooms).toBe(false);
    expect(state.viewerCanJoinRooms).toBe(false);
    expect(state.viewerCanListRooms).toBe(false);
    expect(state.viewerCanManageRooms).toBe(false);
    expect(state.viewerHasUnreadRooms).toBe(false);
  });

  it('passes cancellation through every authenticated snapshot request', async () => {
    await expect(
      getAuthenticatedServerState(serverConfig(), { signal: AbortSignal.abort() })
    ).rejects.toMatchObject({ code: Code.Canceled });
  });

  it('updates server config and maps the returned profile', async () => {
    admin.updateServerConfig.mockReturnValue({
      publicProfile: {
        name: 'Connect Server',
        description: 'Connect description',
        welcomeMessage: 'Connect welcome',
        logoUrl: 'https://cdn/logo.webp',
        bannerUrl: 'https://cdn/banner.webp'
      },
      config: {
        motd: 'Connect MOTD'
      }
    });

    const profile = await updateServerConfig(serverConfig(), {
      name: 'Connect Server',
      description: 'Connect description',
      motd: 'Connect MOTD',
      welcomeMessage: 'Connect welcome'
    });

    expect(receivedRequest(admin.updateServerConfig)).toMatchObject({
      serverName: 'Connect Server',
      description: 'Connect description',
      motd: 'Connect MOTD',
      welcomeMessage: 'Connect welcome',
      updateMask: { paths: ['server_name', 'description', 'motd', 'welcome_message'] }
    });
    expect(profile).toEqual({
      name: 'Connect Server',
      version: '',
      description: 'Connect description',
      motd: 'Connect MOTD',
      welcomeMessage: 'Connect welcome',
      logoUrl: 'https://cdn/logo.webp',
      bannerUrl: 'https://cdn/banner.webp'
    });
  });

  it('updates server branding through AdminServerService', async () => {
    admin.uploadServerLogo.mockReturnValue({
      publicProfile: {
        name: 'Connect Server',
        logoUrl: 'https://cdn/new-logo.webp'
      }
    });
    admin.deleteServerLogo.mockReturnValue({
      publicProfile: {
        name: 'Connect Server'
      }
    });
    admin.uploadServerBanner.mockReturnValue({
      publicProfile: {
        name: 'Connect Server',
        bannerUrl: 'https://cdn/new-banner.webp'
      }
    });
    admin.deleteServerBanner.mockReturnValue({
      publicProfile: {
        name: 'Connect Server'
      }
    });

    const config = serverConfig();

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

    expect(receivedRequest(admin.uploadServerLogo)).toMatchObject({
      image: {
        image: new Uint8Array([1, 2, 3]),
        filename: 'logo.png',
        contentType: 'image/png'
      }
    });
    expect(admin.deleteServerLogo).toHaveBeenCalledOnce();
    expect(receivedRequest(admin.uploadServerBanner)).toMatchObject({
      image: {
        image: new Uint8Array([4, 5, 6]),
        filename: 'banner.png',
        contentType: 'image/png'
      }
    });
    expect(admin.deleteServerBanner).toHaveBeenCalledOnce();
  });

  it('loads and updates security config through AdminServerService', async () => {
    admin.getServerSecurityConfig.mockReturnValue({
      blockedUsernames: ['root', 'admin']
    });
    admin.updateBlockedUsernames.mockReturnValue({
      blockedUsernames: ['root', 'admin', 'reserved']
    });

    const config = serverConfig();
    await expect(getServerSecurityConfig(config)).resolves.toEqual({
      blockedUsernames: 'root\nadmin'
    });
    await expect(updateBlockedUsernames(config, 'root\nadmin\nreserved')).resolves.toEqual({
      blockedUsernames: 'root\nadmin\nreserved'
    });

    expect(receivedRequest(admin.getServerSecurityConfig)).toMatchObject({});
    expect(receivedRequest(admin.updateBlockedUsernames)).toMatchObject({
      blockedUsernames: ['root', 'admin', 'reserved'],
      updateMask: { paths: ['blocked_usernames'] }
    });
  });
});
