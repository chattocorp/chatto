import { Timestamp } from '@bufbuild/protobuf';
import { describe, expect, it, vi } from 'vitest';
import {
  ClientFrame,
  ErrorCode,
  ServerFrame,
  ServerHello,
  StreamEvent,
  WireError,
  Response as WireResponse
} from '$lib/pb/chatto/wire/v1/protocol_pb';
import {
  AddReactionRequest,
  AddReactionResponse,
  AdminAccountInfoView,
  AdminClearUsernameCooldownRequest,
  AdminClearUsernameCooldownResponse,
  AdminConnectionInfoView,
  AdminEventLogEntryView,
  AdminMemberView,
  AdminNatsStatsView,
  AdminProjectionStateView,
  AdminRoleView,
  AdminSecurityConfigView,
  AdminSystemInfoView,
  AdminRoomGroupView,
  AdminRoomInfoView,
  AdminUpdateUserRequest,
  AdminUpdateUserResponse,
  AssignMemberRoleRequest,
  AssignMemberRoleResponse,
  ArchiveAdminRoomRequest,
  ArchiveAdminRoomResponse,
  AuthenticatedServerSettingsView,
  BanRoomMemberRequest,
  BanRoomMemberResponse,
  CreateAdminRoomGroupRequest,
  CreateAdminRoomGroupResponse,
  CreateAdminRoleRequest,
  CreateAdminRoleResponse,
  CreateRoomRequest,
  CreateRoomResponse,
  CurrentUserPresenceStatus,
  CurrentUserView,
  DeleteAdminRoomGroupRequest,
  DeleteAdminRoomGroupResponse,
  DeleteAdminRoleRequest,
  DeleteAdminRoleResponse,
  DeleteMyAccountRequest,
  DeleteMyAccountResponse,
  DismissAllNotificationsRequest,
  DismissAllNotificationsResponse,
  DismissNotificationRequest,
  DismissNotificationResponse,
  GetAccountDeletionStatusRequest,
  GetAccountDeletionStatusResponse,
  GetAdminMemberRequest,
  GetAdminMemberResponse,
  GetAdminRoleCapabilitiesRequest,
  GetAdminRoleCapabilitiesResponse,
  GetAdminRoleRequest,
  GetAdminRoleResponse,
  GetAdminSecurityConfigRequest,
  GetAdminSecurityConfigResponse,
  GetAdminSystemInfoRequest,
  GetAdminSystemInfoResponse,
  GetAdminEventLogEntryRequest,
  GetAdminEventLogEntryResponse,
  GetAdminRoomLayoutRequest,
  GetAdminRoomLayoutResponse,
  GetAuthenticatedServerSettingsRequest,
  GetAuthenticatedServerSettingsResponse,
  GetCurrentUserRequest,
  GetCurrentUserResponse,
  GetProfileSettingsRequest,
  GetProfileSettingsResponse,
  GetRoomDirectoryRequest,
  GetRoomDirectoryResponse,
  GetRolePermissionMatrixRequest,
  GetRolePermissionMatrixResponse,
  GetRolePermissionTierMatrixRequest,
  GetRolePermissionTierMatrixResponse,
  GetServerSettingsRequest,
  GetServerSettingsResponse,
  GetUserPermissionMatrixRequest,
  GetUserPermissionMatrixResponse,
  GetUserSettingsRequest,
  GetUserSettingsResponse,
  GetVoiceCallTokenRequest,
  GetVoiceCallTokenResponse,
  GetViewerRequest,
  GetViewerResponse,
  HasNotificationsRequest,
  HasNotificationsResponse,
  ListAdminEventLogRequest,
  ListAdminEventLogResponse,
  ListAdminMembersRequest,
  ListAdminMembersResponse,
  ListRoomBansRequest,
  ListRoomBansResponse,
  ListMyFollowedThreadsRequest,
  ListMyFollowedThreadsResponse,
  ListNotificationsRequest,
  ListNotificationsResponse,
  MoveAdminRoomToGroupRequest,
  MoveAdminRoomToGroupResponse,
  NotificationItemView,
  NotificationKind,
  ReorderAdminRoomsInGroupRequest,
  ReorderAdminRoomsInGroupResponse,
  RequestAccountDeletionRequest,
  RequestAccountDeletionResponse,
  RevokeMemberRoleRequest,
  RevokeMemberRoleResponse,
  RoomBanView,
  SearchMembersRequest,
  SearchMembersResponse,
  ServerSettingsView,
  StartDMRequest,
  StartDMResponse,
  SubscribeToPushRequest,
  SubscribeToPushResponse,
  ProfileSettingsView,
  PermissionEditState,
  PermissionMatrixDecision,
  PermissionMatrixScopeKind,
  PermissionMatrixCellView,
  PermissionMatrixScopeView,
  RolePermissionMatrixView,
  RolePermissionTierMatrixView,
  SetNotificationLevelResponse,
  SetPermissionStateResponse,
  SetRoomNotificationLevelRequest,
  SetRolePermissionStateRequest,
  SetServerNotificationLevelRequest,
  SetUserPermissionStateRequest,
  TierPermissionsView,
  TierRoleView,
  UpdateMyPresenceRequest,
  UpdateMyPresenceResponse,
  UpdateBlockedUsernamesRequest,
  UpdateBlockedUsernamesResponse,
  UnbanRoomMemberRequest,
  UnbanRoomMemberResponse,
  UnsubscribeFromPushRequest,
  UnsubscribeFromPushResponse,
  UpdateAdminRoleRequest,
  UpdateAdminRoleResponse,
  UpdateProfileRequest,
  UpdateProfileResponse,
  UpdateServerSettingsRequest,
  UpdateServerSettingsResponse,
  UpdateUserSettingsRequest,
  UpdateUserSettingsResponse,
  UserAvatarView,
  VoiceCallTokenView,
  Viewer
} from '$lib/pb/chatto/api/v1/chat_pb';
import { HeartbeatEvent } from '$lib/pb/chatto/core/v1/live_events_pb';
import { User, UserPresenceStatus } from '$lib/pb/chatto/core/v1/models_pb';
import {
  NotificationLevel,
  ServerUserPreferences,
  TimeFormat
} from '$lib/pb/chatto/core/v1/user_preferences_pb';
import { WireClient, type WireSocket, httpToWireWsUrl, wireMethods } from './client';

type FakeListener =
  | ((event: Event) => void)
  | ((event: MessageEvent) => void)
  | ((event: CloseEvent) => void);

class FakeWireSocket implements WireSocket {
  binaryType: BinaryType = 'blob';
  readyState = 0;
  readonly sent: Uint8Array[] = [];
  readonly url: string;
  #listeners = new Map<string, Set<FakeListener>>();

  constructor(url: string) {
    this.url = url;
  }

  send(data: Uint8Array): void {
    this.sent.push(data);
  }

  close(): void {
    this.readyState = 3;
    this.#emit('close', {} as CloseEvent);
  }

  addEventListener(type: 'open', listener: (event: Event) => void): void;
  addEventListener(type: 'message', listener: (event: MessageEvent) => void): void;
  addEventListener(type: 'close', listener: (event: CloseEvent) => void): void;
  addEventListener(type: 'error', listener: (event: Event) => void): void;
  addEventListener(type: string, listener: FakeListener): void {
    const listeners = this.#listeners.get(type) ?? new Set<FakeListener>();
    listeners.add(listener);
    this.#listeners.set(type, listeners);
  }

  removeEventListener(type: 'open', listener: (event: Event) => void): void;
  removeEventListener(type: 'message', listener: (event: MessageEvent) => void): void;
  removeEventListener(type: 'close', listener: (event: CloseEvent) => void): void;
  removeEventListener(type: 'error', listener: (event: Event) => void): void;
  removeEventListener(type: string, listener: FakeListener): void {
    this.#listeners.get(type)?.delete(listener);
  }

  open(): void {
    this.readyState = 1;
    this.#emit('open', {} as Event);
  }

  serverSend(frame: ServerFrame): void {
    this.#emit('message', { data: frame.toBinary() } as MessageEvent);
  }

  lastClientFrame(): ClientFrame {
    const data = this.sent.at(-1);
    if (!data) throw new Error('fake socket did not record a client frame');
    return ClientFrame.fromBinary(data);
  }

  #emit(type: string, event: Event | MessageEvent | CloseEvent): void {
    for (const listener of this.#listeners.get(type) ?? []) {
      (listener as (event: Event | MessageEvent | CloseEvent) => void)(event);
    }
  }
}

function makeClient(config: { token?: string | null } = {}): {
  client: WireClient;
  socket(): FakeWireSocket;
} {
  let socket: FakeWireSocket | null = null;
  const client = new WireClient({
    url: '/api/wire',
    token: config.token ?? null,
    socketFactory: (url) => {
      socket = new FakeWireSocket(url);
      return socket;
    }
  });
  return {
    client,
    socket() {
      if (!socket) throw new Error('wire client has not opened a socket');
      return socket;
    }
  };
}

async function connectClient(harness: ReturnType<typeof makeClient>): Promise<ServerHello> {
  const helloPromise = harness.client.connect();
  const socket = harness.socket();
  socket.open();
  socket.serverSend(
    new ServerFrame({
      frameId: 'hello',
      kind: {
        case: 'hello',
        value: new ServerHello({
          protocolVersion: 'chatto-wire-v1',
          serverVersion: 'test',
          methods: Object.values(wireMethods),
          features: ['binary-protobuf', 'requests', 'my-events']
        })
      }
    })
  );
  return helloPromise;
}

function waitForMessageHandling(): Promise<void> {
  return new Promise((resolve) => setTimeout(resolve, 0));
}

describe('httpToWireWsUrl', () => {
  it('converts HTTP(S) endpoints to WebSocket endpoints', () => {
    expect(httpToWireWsUrl('http://localhost:4000/api/wire')).toBe('ws://localhost:4000/api/wire');
    expect(httpToWireWsUrl('https://chat.example.com/api/wire')).toBe(
      'wss://chat.example.com/api/wire'
    );
    expect(httpToWireWsUrl('wss://chat.example.com/api/wire')).toBe(
      'wss://chat.example.com/api/wire'
    );
  });
});

describe('WireClient', () => {
  it('opens with a protobuf ClientHello carrying resume and bearer auth', async () => {
    const harness = makeClient({ token: 'opaque-token' });
    const { client } = harness;
    const helloPromise = client.connect({
      resumeAfter: 'cursor-123',
      acceptedFeatures: ['example-feature']
    });

    const socket = harness.socket();
    socket.open();
    const sent = socket.lastClientFrame();
    if (sent.kind.case !== 'hello') throw new Error('expected ClientHello frame');

    expect(sent.kind.value.protocolVersion).toBe('chatto-wire-v1');
    expect(sent.kind.value.resumeAfter).toBe('cursor-123');
    expect(sent.kind.value.acceptedFeatures).toEqual(['example-feature']);
    expect(sent.kind.value.bearerToken).toBe('opaque-token');

    socket.serverSend(
      new ServerFrame({
        frameId: sent.frameId,
        kind: {
          case: 'hello',
          value: new ServerHello({
            protocolVersion: 'chatto-wire-v1',
            serverVersion: 'test',
            methods: Object.values(wireMethods),
            features: ['binary-protobuf']
          })
        }
      })
    );

    const hello = await helloPromise;
    expect(hello.methods).toContain(wireMethods.getViewer);
    expect(client.status).toBe('connected');
  });

  it('sends a typed request, handles an interleaved event, and decodes the typed response', async () => {
    const harness = makeClient();
    const { client } = harness;
    await connectClient(harness);
    const socket = harness.socket();

    const events: StreamEvent[] = [];
    client.onEvent((event) => events.push(event));

    const responsePromise = client.getViewer();
    await waitForMessageHandling();
    const requestFrame = socket.lastClientFrame();
    if (requestFrame.kind.case !== 'request') throw new Error('expected Request frame');

    const request = requestFrame.kind.value;
    expect(request.method).toBe(wireMethods.getViewer);
    expect(GetViewerRequest.fromBinary(request.body)).toBeInstanceOf(GetViewerRequest);

    const streamEvent = new StreamEvent({
      eventId: 'evt_1',
      deliveryCursor: 'cursor_1',
      eventType: 'heartbeat',
      payload: {
        case: 'heartbeat',
        value: new HeartbeatEvent()
      }
    });
    socket.serverSend(
      new ServerFrame({
        kind: {
          case: 'event',
          value: streamEvent
        }
      })
    );
    await waitForMessageHandling();

    expect(events).toHaveLength(1);
    expect(events[0].eventId).toBe('evt_1');
    expect(client.lastDeliveryCursor).toBe('cursor_1');

    client.ack(events[0]);
    const ackFrame = socket.lastClientFrame();
    if (ackFrame.kind.case !== 'ack') throw new Error('expected Ack frame');
    expect(ackFrame.kind.value.deliveryCursor).toBe('cursor_1');

    socket.serverSend(
      new ServerFrame({
        frameId: requestFrame.frameId,
        kind: {
          case: 'response',
          value: new WireResponse({
            requestId: request.requestId,
            body: new Uint8Array(
              new GetViewerResponse({
                viewer: new Viewer({
                  user: new User({
                    id: 'user_1',
                    login: 'test-user',
                    displayName: 'Test User'
                  })
                })
              }).toBinary()
            )
          })
        }
      })
    );

    const response = await responsePromise;
    expect(response.viewer?.user?.id).toBe('user_1');
    expect(response.viewer?.user?.displayName).toBe('Test User');
  });

  it('rejects the matching request when the server returns a protobuf WireError', async () => {
    const harness = makeClient();
    const { client } = harness;
    await connectClient(harness);
    const socket = harness.socket();

    const responsePromise = client.request(
      '/chatto.api.v1.ChattoApiService/Nope',
      new GetViewerRequest(),
      GetViewerResponse,
      { requestId: 'req_error' }
    );
    await waitForMessageHandling();
    const requestFrame = socket.lastClientFrame();
    if (requestFrame.kind.case !== 'request') throw new Error('expected Request frame');

    socket.serverSend(
      new ServerFrame({
        frameId: requestFrame.frameId,
        kind: {
          case: 'error',
          value: new WireError({
            requestId: 'req_error',
            code: ErrorCode.UNIMPLEMENTED,
            message: 'unknown method'
          })
        }
      })
    );

    await expect(responsePromise).rejects.toMatchObject({
      name: 'WireProtocolError',
      wireError: expect.objectContaining({ code: ErrorCode.UNIMPLEMENTED })
    });
  });

  it('sends and decodes typed message write requests', async () => {
    const harness = makeClient();
    const { client } = harness;
    await connectClient(harness);
    const socket = harness.socket();

    const responsePromise = client.addReaction(
      new AddReactionRequest({
        roomId: 'room_1',
        messageEventId: 'event_1',
        emoji: 'thumbsup'
      })
    );
    await waitForMessageHandling();

    const requestFrame = socket.lastClientFrame();
    if (requestFrame.kind.case !== 'request') throw new Error('expected Request frame');
    const request = requestFrame.kind.value;
    expect(request.method).toBe(wireMethods.addReaction);
    expect(AddReactionRequest.fromBinary(request.body)).toMatchObject({
      roomId: 'room_1',
      messageEventId: 'event_1',
      emoji: 'thumbsup'
    });

    socket.serverSend(
      new ServerFrame({
        frameId: requestFrame.frameId,
        kind: {
          case: 'response',
          value: new WireResponse({
            requestId: request.requestId,
            body: new Uint8Array(new AddReactionResponse({ changed: true }).toBinary())
          })
        }
      })
    );

    await expect(responsePromise).resolves.toMatchObject({ changed: true });
  });

  it('sends and decodes typed profile settings requests', async () => {
    const harness = makeClient();
    const { client } = harness;
    await connectClient(harness);
    const socket = harness.socket();

    const settingsPromise = client.getProfileSettings();
    await waitForMessageHandling();
    let requestFrame = socket.lastClientFrame();
    if (requestFrame.kind.case !== 'request') throw new Error('expected Request frame');
    let request = requestFrame.kind.value;
    expect(request.method).toBe(wireMethods.getProfileSettings);
    expect(GetProfileSettingsRequest.fromBinary(request.body)).toBeInstanceOf(
      GetProfileSettingsRequest
    );
    socket.serverSend(
      new ServerFrame({
        frameId: requestFrame.frameId,
        kind: {
          case: 'response',
          value: new WireResponse({
            requestId: request.requestId,
            body: new Uint8Array(
              new GetProfileSettingsResponse({
                profile: new ProfileSettingsView({
                  user: new User({
                    id: 'user_1',
                    login: 'profile-user',
                    displayName: 'Profile User'
                  })
                })
              }).toBinary()
            )
          })
        }
      })
    );
    await expect(settingsPromise).resolves.toMatchObject({
      profile: expect.objectContaining({
        user: expect.objectContaining({ id: 'user_1', displayName: 'Profile User' })
      })
    });

    const updatePromise = client.updateProfile(
      new UpdateProfileRequest({ displayName: 'Renamed User' })
    );
    await waitForMessageHandling();
    requestFrame = socket.lastClientFrame();
    if (requestFrame.kind.case !== 'request') throw new Error('expected Request frame');
    request = requestFrame.kind.value;
    expect(request.method).toBe(wireMethods.updateProfile);
    expect(UpdateProfileRequest.fromBinary(request.body)).toMatchObject({
      displayName: 'Renamed User'
    });
    socket.serverSend(
      new ServerFrame({
        frameId: requestFrame.frameId,
        kind: {
          case: 'response',
          value: new WireResponse({
            requestId: request.requestId,
            body: new Uint8Array(
              new UpdateProfileResponse({
                profile: new ProfileSettingsView({
                  user: new User({
                    id: 'user_1',
                    login: 'profile-user',
                    displayName: 'Renamed User'
                  })
                })
              }).toBinary()
            )
          })
        }
      })
    );
    await expect(updatePromise).resolves.toMatchObject({
      profile: expect.objectContaining({
        user: expect.objectContaining({ displayName: 'Renamed User' })
      })
    });
  });

  it('sends and decodes typed current-user and server-settings requests', async () => {
    const harness = makeClient();
    const { client } = harness;
    await connectClient(harness);
    const socket = harness.socket();

    const currentUserPromise = client.getCurrentUser();
    await waitForMessageHandling();
    let requestFrame = socket.lastClientFrame();
    if (requestFrame.kind.case !== 'request') throw new Error('expected Request frame');
    let request = requestFrame.kind.value;
    expect(request.method).toBe(wireMethods.getCurrentUser);
    expect(GetCurrentUserRequest.fromBinary(request.body)).toBeInstanceOf(GetCurrentUserRequest);
    socket.serverSend(
      new ServerFrame({
        frameId: requestFrame.frameId,
        kind: {
          case: 'response',
          value: new WireResponse({
            requestId: request.requestId,
            body: new Uint8Array(
              new GetCurrentUserResponse({
                user: new CurrentUserView({
                  user: new User({
                    id: 'user_1',
                    login: 'profile-user',
                    displayName: 'Profile User'
                  }),
                  avatarUrl: '/assets/avatar.webp',
                  presenceStatus: CurrentUserPresenceStatus.ONLINE,
                  hasVerifiedEmail: true,
                  settings: new ServerUserPreferences({
                    timezone: 'Europe/Berlin',
                    timeFormat: TimeFormat.TIME_FORMAT_24H
                  })
                })
              }).toBinary()
            )
          })
        }
      })
    );
    await expect(currentUserPromise).resolves.toMatchObject({
      user: expect.objectContaining({
        avatarUrl: '/assets/avatar.webp',
        hasVerifiedEmail: true,
        user: expect.objectContaining({ id: 'user_1' })
      })
    });

    const serverSettingsPromise = client.getAuthenticatedServerSettings();
    await waitForMessageHandling();
    requestFrame = socket.lastClientFrame();
    if (requestFrame.kind.case !== 'request') throw new Error('expected Request frame');
    request = requestFrame.kind.value;
    expect(request.method).toBe(wireMethods.getAuthenticatedServerSettings);
    expect(GetAuthenticatedServerSettingsRequest.fromBinary(request.body)).toBeInstanceOf(
      GetAuthenticatedServerSettingsRequest
    );
    socket.serverSend(
      new ServerFrame({
        frameId: requestFrame.frameId,
        kind: {
          case: 'response',
          value: new WireResponse({
            requestId: request.requestId,
            body: new Uint8Array(
              new GetAuthenticatedServerSettingsResponse({
                settings: new AuthenticatedServerSettingsView({
                  pushNotificationsEnabled: true,
                  vapidPublicKey: 'vapid',
                  livekitUrl: 'wss://livekit.example.com',
                  videoProcessingEnabled: true,
                  maxUploadSize: BigInt(100),
                  maxVideoUploadSize: BigInt(200),
                  messageEditWindowSeconds: 7200,
                  motd: 'hello'
                })
              }).toBinary()
            )
          })
        }
      })
    );
    await expect(serverSettingsPromise).resolves.toMatchObject({
      settings: expect.objectContaining({
        pushNotificationsEnabled: true,
        messageEditWindowSeconds: 7200,
        motd: 'hello'
      })
    });
  });

  it('sends and decodes typed account deletion and editable server settings requests', async () => {
    const harness = makeClient();
    const { client } = harness;
    await connectClient(harness);
    const socket = harness.socket();

    const deletionStatusPromise = client.getAccountDeletionStatus();
    await waitForMessageHandling();
    let requestFrame = socket.lastClientFrame();
    if (requestFrame.kind.case !== 'request') throw new Error('expected Request frame');
    let request = requestFrame.kind.value;
    expect(request.method).toBe(wireMethods.getAccountDeletionStatus);
    expect(GetAccountDeletionStatusRequest.fromBinary(request.body)).toBeInstanceOf(
      GetAccountDeletionStatusRequest
    );
    socket.serverSend(
      new ServerFrame({
        frameId: requestFrame.frameId,
        kind: {
          case: 'response',
          value: new WireResponse({
            requestId: request.requestId,
            body: new Uint8Array(
              new GetAccountDeletionStatusResponse({
                viewerCanDeleteAccount: true
              }).toBinary()
            )
          })
        }
      })
    );
    await expect(deletionStatusPromise).resolves.toMatchObject({
      viewerCanDeleteAccount: true
    });

    const tokenPromise = client.requestAccountDeletion();
    await waitForMessageHandling();
    requestFrame = socket.lastClientFrame();
    if (requestFrame.kind.case !== 'request') throw new Error('expected Request frame');
    request = requestFrame.kind.value;
    expect(request.method).toBe(wireMethods.requestAccountDeletion);
    expect(RequestAccountDeletionRequest.fromBinary(request.body)).toBeInstanceOf(
      RequestAccountDeletionRequest
    );
    socket.serverSend(
      new ServerFrame({
        frameId: requestFrame.frameId,
        kind: {
          case: 'response',
          value: new WireResponse({
            requestId: request.requestId,
            body: new Uint8Array(
              new RequestAccountDeletionResponse({
                confirmationToken: 'delete-token'
              }).toBinary()
            )
          })
        }
      })
    );
    await expect(tokenPromise).resolves.toMatchObject({
      confirmationToken: 'delete-token'
    });

    const deletePromise = client.deleteMyAccount(
      new DeleteMyAccountRequest({ confirmationToken: 'delete-token' })
    );
    await waitForMessageHandling();
    requestFrame = socket.lastClientFrame();
    if (requestFrame.kind.case !== 'request') throw new Error('expected Request frame');
    request = requestFrame.kind.value;
    expect(request.method).toBe(wireMethods.deleteMyAccount);
    expect(DeleteMyAccountRequest.fromBinary(request.body)).toMatchObject({
      confirmationToken: 'delete-token'
    });
    socket.serverSend(
      new ServerFrame({
        frameId: requestFrame.frameId,
        kind: {
          case: 'response',
          value: new WireResponse({
            requestId: request.requestId,
            body: new Uint8Array(new DeleteMyAccountResponse({ deleted: true }).toBinary())
          })
        }
      })
    );
    await expect(deletePromise).resolves.toMatchObject({ deleted: true });

    const settingsPromise = client.getServerSettings();
    await waitForMessageHandling();
    requestFrame = socket.lastClientFrame();
    if (requestFrame.kind.case !== 'request') throw new Error('expected Request frame');
    request = requestFrame.kind.value;
    expect(request.method).toBe(wireMethods.getServerSettings);
    expect(GetServerSettingsRequest.fromBinary(request.body)).toBeInstanceOf(
      GetServerSettingsRequest
    );
    socket.serverSend(
      new ServerFrame({
        frameId: requestFrame.frameId,
        kind: {
          case: 'response',
          value: new WireResponse({
            requestId: request.requestId,
            body: new Uint8Array(
              new GetServerSettingsResponse({
                settings: new ServerSettingsView({
                  name: 'Prototype',
                  description: 'description',
                  motd: 'motd',
                  welcomeMessage: 'welcome',
                  viewerCanManageServer: true
                })
              }).toBinary()
            )
          })
        }
      })
    );
    await expect(settingsPromise).resolves.toMatchObject({
      settings: expect.objectContaining({
        name: 'Prototype',
        viewerCanManageServer: true
      })
    });

    const updatePromise = client.updateServerSettings(
      new UpdateServerSettingsRequest({
        serverName: 'Updated',
        description: '',
        motd: 'motd',
        welcomeMessage: 'welcome'
      })
    );
    await waitForMessageHandling();
    requestFrame = socket.lastClientFrame();
    if (requestFrame.kind.case !== 'request') throw new Error('expected Request frame');
    request = requestFrame.kind.value;
    expect(request.method).toBe(wireMethods.updateServerSettings);
    expect(UpdateServerSettingsRequest.fromBinary(request.body)).toMatchObject({
      serverName: 'Updated',
      description: '',
      motd: 'motd',
      welcomeMessage: 'welcome'
    });
    socket.serverSend(
      new ServerFrame({
        frameId: requestFrame.frameId,
        kind: {
          case: 'response',
          value: new WireResponse({
            requestId: request.requestId,
            body: new Uint8Array(
              new UpdateServerSettingsResponse({
                settings: new ServerSettingsView({
                  name: 'Updated',
                  viewerCanManageServer: true
                })
              }).toBinary()
            )
          })
        }
      })
    );
    await expect(updatePromise).resolves.toMatchObject({
      settings: expect.objectContaining({ name: 'Updated' })
    });

    const securityPromise = client.getAdminSecurityConfig();
    await waitForMessageHandling();
    requestFrame = socket.lastClientFrame();
    if (requestFrame.kind.case !== 'request') throw new Error('expected Request frame');
    request = requestFrame.kind.value;
    expect(request.method).toBe(wireMethods.getAdminSecurityConfig);
    expect(GetAdminSecurityConfigRequest.fromBinary(request.body)).toBeInstanceOf(
      GetAdminSecurityConfigRequest
    );
    socket.serverSend(
      new ServerFrame({
        frameId: requestFrame.frameId,
        kind: {
          case: 'response',
          value: new WireResponse({
            requestId: request.requestId,
            body: new Uint8Array(
              new GetAdminSecurityConfigResponse({
                config: new AdminSecurityConfigView({ blockedUsernames: 'root\nadmin' })
              }).toBinary()
            )
          })
        }
      })
    );
    await expect(securityPromise).resolves.toMatchObject({
      config: expect.objectContaining({ blockedUsernames: 'root\nadmin' })
    });

    const blockedUsernames = 'root\nadmin\nreserved';
    const blockedPromise = client.updateBlockedUsernames(
      new UpdateBlockedUsernamesRequest({ blockedUsernames })
    );
    await waitForMessageHandling();
    requestFrame = socket.lastClientFrame();
    if (requestFrame.kind.case !== 'request') throw new Error('expected Request frame');
    request = requestFrame.kind.value;
    expect(request.method).toBe(wireMethods.updateBlockedUsernames);
    expect(UpdateBlockedUsernamesRequest.fromBinary(request.body)).toMatchObject({
      blockedUsernames
    });
    socket.serverSend(
      new ServerFrame({
        frameId: requestFrame.frameId,
        kind: {
          case: 'response',
          value: new WireResponse({
            requestId: request.requestId,
            body: new Uint8Array(
              new UpdateBlockedUsernamesResponse({
                config: new AdminSecurityConfigView({ blockedUsernames })
              }).toBinary()
            )
          })
        }
      })
    );
    await expect(blockedPromise).resolves.toMatchObject({
      config: expect.objectContaining({ blockedUsernames })
    });

    const systemPromise = client.getAdminSystemInfo();
    await waitForMessageHandling();
    requestFrame = socket.lastClientFrame();
    if (requestFrame.kind.case !== 'request') throw new Error('expected Request frame');
    request = requestFrame.kind.value;
    expect(request.method).toBe(wireMethods.getAdminSystemInfo);
    expect(GetAdminSystemInfoRequest.fromBinary(request.body)).toBeInstanceOf(
      GetAdminSystemInfoRequest
    );
    socket.serverSend(
      new ServerFrame({
        frameId: requestFrame.frameId,
        kind: {
          case: 'response',
          value: new WireResponse({
            requestId: request.requestId,
            body: new Uint8Array(
              new GetAdminSystemInfoResponse({
                systemInfo: new AdminSystemInfoView({
                  connection: new AdminConnectionInfoView({
                    connected: true,
                    serverId: 'nats_1',
                    version: '2.11.0'
                  }),
                  account: new AdminAccountInfoView({ streamsUsed: 3 }),
                  nats: new AdminNatsStatsView({ totalMessages: 42n })
                }),
                projections: [
                  new AdminProjectionStateView({
                    name: 'Rooms',
                    subjects: ['evt.room.>'],
                    started: true,
                    lag: 0n,
                    entryCount: 12n
                  })
                ]
              }).toBinary()
            )
          })
        }
      })
    );
    await expect(systemPromise).resolves.toMatchObject({
      systemInfo: expect.objectContaining({
        connection: expect.objectContaining({ connected: true, serverId: 'nats_1' }),
        account: expect.objectContaining({ streamsUsed: 3 })
      }),
      projections: [expect.objectContaining({ name: 'Rooms', started: true })]
    });

    const eventLogPromise = client.listAdminEventLog(
      new ListAdminEventLogRequest({ limit: 50, before: '123' })
    );
    await waitForMessageHandling();
    requestFrame = socket.lastClientFrame();
    if (requestFrame.kind.case !== 'request') throw new Error('expected Request frame');
    request = requestFrame.kind.value;
    expect(request.method).toBe(wireMethods.listAdminEventLog);
    expect(ListAdminEventLogRequest.fromBinary(request.body)).toMatchObject({
      limit: 50,
      before: '123'
    });
    socket.serverSend(
      new ServerFrame({
        frameId: requestFrame.frameId,
        kind: {
          case: 'response',
          value: new WireResponse({
            requestId: request.requestId,
            body: new Uint8Array(
              new ListAdminEventLogResponse({
                entries: [
                  new AdminEventLogEntryView({
                    sequence: '122',
                    subject: 'evt.room.room_1.message_posted',
                    eventType: 'MessagePostedEvent',
                    actorId: 'user_1',
                    createdAt: Timestamp.fromDate(new Date('2026-01-01T00:00:00Z'))
                  })
                ],
                hasOlder: true,
                endCursor: '122',
                totalCount: 900n
              }).toBinary()
            )
          })
        }
      })
    );
    await expect(eventLogPromise).resolves.toMatchObject({
      entries: [expect.objectContaining({ sequence: '122', eventType: 'MessagePostedEvent' })],
      hasOlder: true,
      endCursor: '122'
    });

    const eventLogEntryPromise = client.getAdminEventLogEntry(
      new GetAdminEventLogEntryRequest({ sequence: '122' })
    );
    await waitForMessageHandling();
    requestFrame = socket.lastClientFrame();
    if (requestFrame.kind.case !== 'request') throw new Error('expected Request frame');
    request = requestFrame.kind.value;
    expect(request.method).toBe(wireMethods.getAdminEventLogEntry);
    expect(GetAdminEventLogEntryRequest.fromBinary(request.body)).toMatchObject({
      sequence: '122'
    });
    socket.serverSend(
      new ServerFrame({
        frameId: requestFrame.frameId,
        kind: {
          case: 'response',
          value: new WireResponse({
            requestId: request.requestId,
            body: new Uint8Array(
              new GetAdminEventLogEntryResponse({
                entry: new AdminEventLogEntryView({
                  sequence: '122',
                  eventId: 'evt_1',
                  payloadJson: '{}'
                })
              }).toBinary()
            )
          })
        }
      })
    );
    await expect(eventLogEntryPromise).resolves.toMatchObject({
      entry: expect.objectContaining({ sequence: '122', eventId: 'evt_1' })
    });
  });

  it('sends and decodes typed admin member requests', async () => {
    const harness = makeClient();
    const { client } = harness;
    await connectClient(harness);
    const socket = harness.socket();

    const member = new AdminMemberView({
      user: new User({ id: 'user_1', login: 'alice', displayName: 'Alice' }),
      avatarUrl: '/assets/alice.webp',
      roles: ['moderator'],
      lastLoginChange: Timestamp.fromDate(new Date('2026-01-02T00:00:00Z'))
    });
    const role = new AdminRoleView({
      name: 'moderator',
      displayName: 'Moderator',
      position: 100,
      permissions: ['room.ban-member']
    });

    const listPromise = client.listAdminMembers(
      new ListAdminMembersRequest({ search: 'alice', limit: 20, offset: 0 })
    );
    await waitForMessageHandling();
    let requestFrame = socket.lastClientFrame();
    if (requestFrame.kind.case !== 'request') throw new Error('expected Request frame');
    let request = requestFrame.kind.value;
    expect(request.method).toBe(wireMethods.listAdminMembers);
    expect(ListAdminMembersRequest.fromBinary(request.body)).toMatchObject({
      search: 'alice',
      limit: 20,
      offset: 0
    });
    socket.serverSend(
      new ServerFrame({
        frameId: requestFrame.frameId,
        kind: {
          case: 'response',
          value: new WireResponse({
            requestId: request.requestId,
            body: new Uint8Array(
              new ListAdminMembersResponse({
                members: [member],
                roles: [role],
                totalCount: 1,
                hasMore: false
              }).toBinary()
            )
          })
        }
      })
    );
    await expect(listPromise).resolves.toMatchObject({
      members: [expect.objectContaining({ avatarUrl: '/assets/alice.webp', roles: ['moderator'] })],
      totalCount: 1,
      hasMore: false
    });

    const detailPromise = client.getAdminMember(new GetAdminMemberRequest({ userId: 'user_1' }));
    await waitForMessageHandling();
    requestFrame = socket.lastClientFrame();
    if (requestFrame.kind.case !== 'request') throw new Error('expected Request frame');
    request = requestFrame.kind.value;
    expect(request.method).toBe(wireMethods.getAdminMember);
    expect(GetAdminMemberRequest.fromBinary(request.body)).toMatchObject({ userId: 'user_1' });
    socket.serverSend(
      new ServerFrame({
        frameId: requestFrame.frameId,
        kind: {
          case: 'response',
          value: new WireResponse({
            requestId: request.requestId,
            body: new Uint8Array(
              new GetAdminMemberResponse({
                member,
                roles: [role],
                availablePermissions: ['role.assign'],
                viewerCanAssignRoles: true,
                viewerCanManageRoles: true,
                viewerCanManageUserPermissions: true
              }).toBinary()
            )
          })
        }
      })
    );
    await expect(detailPromise).resolves.toMatchObject({
      member: expect.objectContaining({ roles: ['moderator'] }),
      viewerCanAssignRoles: true
    });

    const updatePromise = client.adminUpdateUser(
      new AdminUpdateUserRequest({ userId: 'user_1', login: 'alice2', displayName: 'Alice Two' })
    );
    await waitForMessageHandling();
    requestFrame = socket.lastClientFrame();
    if (requestFrame.kind.case !== 'request') throw new Error('expected Request frame');
    request = requestFrame.kind.value;
    expect(request.method).toBe(wireMethods.adminUpdateUser);
    expect(AdminUpdateUserRequest.fromBinary(request.body)).toMatchObject({
      userId: 'user_1',
      login: 'alice2',
      displayName: 'Alice Two'
    });
    socket.serverSend(
      new ServerFrame({
        frameId: requestFrame.frameId,
        kind: {
          case: 'response',
          value: new WireResponse({
            requestId: request.requestId,
            body: new Uint8Array(
              new AdminUpdateUserResponse({
                member: new AdminMemberView({
                  user: new User({ id: 'user_1', login: 'alice2', displayName: 'Alice Two' })
                })
              }).toBinary()
            )
          })
        }
      })
    );
    await expect(updatePromise).resolves.toMatchObject({
      member: expect.objectContaining({
        user: expect.objectContaining({ login: 'alice2', displayName: 'Alice Two' })
      })
    });

    const clearPromise = client.adminClearUsernameCooldown(
      new AdminClearUsernameCooldownRequest({ userId: 'user_1' })
    );
    await waitForMessageHandling();
    requestFrame = socket.lastClientFrame();
    if (requestFrame.kind.case !== 'request') throw new Error('expected Request frame');
    request = requestFrame.kind.value;
    expect(request.method).toBe(wireMethods.adminClearUsernameCooldown);
    expect(AdminClearUsernameCooldownRequest.fromBinary(request.body)).toMatchObject({
      userId: 'user_1'
    });
    socket.serverSend(
      new ServerFrame({
        frameId: requestFrame.frameId,
        kind: {
          case: 'response',
          value: new WireResponse({
            requestId: request.requestId,
            body: new Uint8Array(new AdminClearUsernameCooldownResponse({ member }).toBinary())
          })
        }
      })
    );
    await expect(clearPromise).resolves.toMatchObject({
      member: expect.objectContaining({ roles: ['moderator'] })
    });

    const assignPromise = client.assignMemberRole(
      new AssignMemberRoleRequest({ userId: 'user_1', roleName: 'admin' })
    );
    await waitForMessageHandling();
    requestFrame = socket.lastClientFrame();
    if (requestFrame.kind.case !== 'request') throw new Error('expected Request frame');
    request = requestFrame.kind.value;
    expect(request.method).toBe(wireMethods.assignMemberRole);
    expect(AssignMemberRoleRequest.fromBinary(request.body)).toMatchObject({
      userId: 'user_1',
      roleName: 'admin'
    });
    socket.serverSend(
      new ServerFrame({
        frameId: requestFrame.frameId,
        kind: {
          case: 'response',
          value: new WireResponse({
            requestId: request.requestId,
            body: new Uint8Array(
              new AssignMemberRoleResponse({
                member: new AdminMemberView({ user: member.user, roles: ['moderator', 'admin'] })
              }).toBinary()
            )
          })
        }
      })
    );
    await expect(assignPromise).resolves.toMatchObject({
      member: expect.objectContaining({ roles: ['moderator', 'admin'] })
    });

    const revokePromise = client.revokeMemberRole(
      new RevokeMemberRoleRequest({ userId: 'user_1', roleName: 'admin' })
    );
    await waitForMessageHandling();
    requestFrame = socket.lastClientFrame();
    if (requestFrame.kind.case !== 'request') throw new Error('expected Request frame');
    request = requestFrame.kind.value;
    expect(request.method).toBe(wireMethods.revokeMemberRole);
    expect(RevokeMemberRoleRequest.fromBinary(request.body)).toMatchObject({
      userId: 'user_1',
      roleName: 'admin'
    });
    socket.serverSend(
      new ServerFrame({
        frameId: requestFrame.frameId,
        kind: {
          case: 'response',
          value: new WireResponse({
            requestId: request.requestId,
            body: new Uint8Array(new RevokeMemberRoleResponse({ member }).toBinary())
          })
        }
      })
    );
    await expect(revokePromise).resolves.toMatchObject({
      member: expect.objectContaining({ roles: ['moderator'] })
    });
  });

  it('sends and decodes typed admin role requests', async () => {
    const harness = makeClient();
    const { client } = harness;
    await connectClient(harness);
    const socket = harness.socket();

    const role = new AdminRoleView({
      name: 'moderator',
      displayName: 'Moderator',
      description: 'Moderates rooms',
      position: 100,
      permissions: ['room.ban-member'],
      pingable: true
    });

    const capsPromise = client.getAdminRoleCapabilities();
    await waitForMessageHandling();
    let requestFrame = socket.lastClientFrame();
    if (requestFrame.kind.case !== 'request') throw new Error('expected Request frame');
    let request = requestFrame.kind.value;
    expect(request.method).toBe(wireMethods.getAdminRoleCapabilities);
    expect(GetAdminRoleCapabilitiesRequest.fromBinary(request.body)).toBeInstanceOf(
      GetAdminRoleCapabilitiesRequest
    );
    socket.serverSend(
      new ServerFrame({
        frameId: requestFrame.frameId,
        kind: {
          case: 'response',
          value: new WireResponse({
            requestId: request.requestId,
            body: new Uint8Array(
              new GetAdminRoleCapabilitiesResponse({
                viewerCanManageRoles: true,
                viewerCanAssignRoles: true
              }).toBinary()
            )
          })
        }
      })
    );
    await expect(capsPromise).resolves.toMatchObject({
      viewerCanManageRoles: true,
      viewerCanAssignRoles: true
    });

    const getPromise = client.getAdminRole(new GetAdminRoleRequest({ name: 'moderator' }));
    await waitForMessageHandling();
    requestFrame = socket.lastClientFrame();
    if (requestFrame.kind.case !== 'request') throw new Error('expected Request frame');
    request = requestFrame.kind.value;
    expect(request.method).toBe(wireMethods.getAdminRole);
    expect(GetAdminRoleRequest.fromBinary(request.body)).toMatchObject({ name: 'moderator' });
    socket.serverSend(
      new ServerFrame({
        frameId: requestFrame.frameId,
        kind: {
          case: 'response',
          value: new WireResponse({
            requestId: request.requestId,
            body: new Uint8Array(
              new GetAdminRoleResponse({
                role,
                users: [new User({ id: 'user_1', login: 'alice', displayName: 'Alice' })],
                viewerCanManageRoles: true,
                viewerCanAssignRoles: true
              }).toBinary()
            )
          })
        }
      })
    );
    await expect(getPromise).resolves.toMatchObject({
      role: expect.objectContaining({ name: 'moderator', pingable: true }),
      users: [expect.objectContaining({ id: 'user_1' })]
    });

    const createPromise = client.createAdminRole(
      new CreateAdminRoleRequest({
        name: 'helper',
        displayName: 'Helper',
        description: 'Helps out',
        pingable: false
      })
    );
    await waitForMessageHandling();
    requestFrame = socket.lastClientFrame();
    if (requestFrame.kind.case !== 'request') throw new Error('expected Request frame');
    request = requestFrame.kind.value;
    expect(request.method).toBe(wireMethods.createAdminRole);
    expect(CreateAdminRoleRequest.fromBinary(request.body)).toMatchObject({
      name: 'helper',
      displayName: 'Helper',
      description: 'Helps out',
      pingable: false
    });
    socket.serverSend(
      new ServerFrame({
        frameId: requestFrame.frameId,
        kind: {
          case: 'response',
          value: new WireResponse({
            requestId: request.requestId,
            body: new Uint8Array(
              new CreateAdminRoleResponse({
                role: new AdminRoleView({ name: 'helper', displayName: 'Helper' })
              }).toBinary()
            )
          })
        }
      })
    );
    await expect(createPromise).resolves.toMatchObject({
      role: expect.objectContaining({ name: 'helper', displayName: 'Helper' })
    });

    const updatePromise = client.updateAdminRole(
      new UpdateAdminRoleRequest({
        name: 'helper',
        displayName: 'Helper Renamed',
        description: 'Still helps',
        pingable: true
      })
    );
    await waitForMessageHandling();
    requestFrame = socket.lastClientFrame();
    if (requestFrame.kind.case !== 'request') throw new Error('expected Request frame');
    request = requestFrame.kind.value;
    expect(request.method).toBe(wireMethods.updateAdminRole);
    expect(UpdateAdminRoleRequest.fromBinary(request.body)).toMatchObject({
      name: 'helper',
      displayName: 'Helper Renamed',
      description: 'Still helps',
      pingable: true
    });
    socket.serverSend(
      new ServerFrame({
        frameId: requestFrame.frameId,
        kind: {
          case: 'response',
          value: new WireResponse({
            requestId: request.requestId,
            body: new Uint8Array(
              new UpdateAdminRoleResponse({
                role: new AdminRoleView({
                  name: 'helper',
                  displayName: 'Helper Renamed',
                  description: 'Still helps',
                  pingable: true
                })
              }).toBinary()
            )
          })
        }
      })
    );
    await expect(updatePromise).resolves.toMatchObject({
      role: expect.objectContaining({ displayName: 'Helper Renamed', pingable: true })
    });

    const deletePromise = client.deleteAdminRole(new DeleteAdminRoleRequest({ name: 'helper' }));
    await waitForMessageHandling();
    requestFrame = socket.lastClientFrame();
    if (requestFrame.kind.case !== 'request') throw new Error('expected Request frame');
    request = requestFrame.kind.value;
    expect(request.method).toBe(wireMethods.deleteAdminRole);
    expect(DeleteAdminRoleRequest.fromBinary(request.body)).toMatchObject({ name: 'helper' });
    socket.serverSend(
      new ServerFrame({
        frameId: requestFrame.frameId,
        kind: {
          case: 'response',
          value: new WireResponse({
            requestId: request.requestId,
            body: new Uint8Array(new DeleteAdminRoleResponse({ deleted: true }).toBinary())
          })
        }
      })
    );
    await expect(deletePromise).resolves.toMatchObject({ deleted: true });

    const tierPromise = client.getRolePermissionTierMatrix(
      new GetRolePermissionTierMatrixRequest({ groupId: 'group_1' })
    );
    await waitForMessageHandling();
    requestFrame = socket.lastClientFrame();
    if (requestFrame.kind.case !== 'request') throw new Error('expected Request frame');
    request = requestFrame.kind.value;
    expect(request.method).toBe(wireMethods.getRolePermissionTierMatrix);
    expect(GetRolePermissionTierMatrixRequest.fromBinary(request.body)).toMatchObject({
      groupId: 'group_1'
    });
    socket.serverSend(
      new ServerFrame({
        frameId: requestFrame.frameId,
        kind: {
          case: 'response',
          value: new WireResponse({
            requestId: request.requestId,
            body: new Uint8Array(
              new GetRolePermissionTierMatrixResponse({
                matrix: new RolePermissionTierMatrixView({
                  applicablePermissions: ['room.join'],
                  roles: [
                    new TierRoleView({
                      roleName: 'everyone',
                      displayName: 'Everyone',
                      override: new TierPermissionsView({ permissions: ['room.join'] })
                    })
                  ]
                })
              }).toBinary()
            )
          })
        }
      })
    );
    await expect(tierPromise).resolves.toMatchObject({
      matrix: expect.objectContaining({
        applicablePermissions: ['room.join'],
        roles: [expect.objectContaining({ roleName: 'everyone' })]
      })
    });

    const roleMatrix = new RolePermissionMatrixView({
      roleName: 'helper',
      applicablePermissions: ['message.post'],
      scopes: [
        new PermissionMatrixScopeView({
          id: 'server',
          label: 'Server',
          kind: PermissionMatrixScopeKind.SERVER
        })
      ],
      cells: [
        new PermissionMatrixCellView({
          permission: 'message.post',
          scopeId: 'server',
          override: PermissionMatrixDecision.ALLOW,
          effective: PermissionMatrixDecision.ALLOW
        })
      ]
    });
    const roleMatrixPromise = client.getRolePermissionMatrix(
      new GetRolePermissionMatrixRequest({ roleName: 'helper' })
    );
    await waitForMessageHandling();
    requestFrame = socket.lastClientFrame();
    if (requestFrame.kind.case !== 'request') throw new Error('expected Request frame');
    request = requestFrame.kind.value;
    expect(request.method).toBe(wireMethods.getRolePermissionMatrix);
    expect(GetRolePermissionMatrixRequest.fromBinary(request.body)).toMatchObject({
      roleName: 'helper'
    });
    socket.serverSend(
      new ServerFrame({
        frameId: requestFrame.frameId,
        kind: {
          case: 'response',
          value: new WireResponse({
            requestId: request.requestId,
            body: new Uint8Array(
              new GetRolePermissionMatrixResponse({ matrix: roleMatrix }).toBinary()
            )
          })
        }
      })
    );
    await expect(roleMatrixPromise).resolves.toMatchObject({
      matrix: expect.objectContaining({ roleName: 'helper' })
    });

    const userMatrixPromise = client.getUserPermissionMatrix(
      new GetUserPermissionMatrixRequest({ userId: 'user_1' })
    );
    await waitForMessageHandling();
    requestFrame = socket.lastClientFrame();
    if (requestFrame.kind.case !== 'request') throw new Error('expected Request frame');
    request = requestFrame.kind.value;
    expect(request.method).toBe(wireMethods.getUserPermissionMatrix);
    expect(GetUserPermissionMatrixRequest.fromBinary(request.body)).toMatchObject({
      userId: 'user_1'
    });
    socket.serverSend(
      new ServerFrame({
        frameId: requestFrame.frameId,
        kind: {
          case: 'response',
          value: new WireResponse({
            requestId: request.requestId,
            body: new Uint8Array(
              new GetUserPermissionMatrixResponse({
                matrix: {
                  userId: 'user_1',
                  applicablePermissions: ['message.post'],
                  scopes: roleMatrix.scopes,
                  cells: roleMatrix.cells
                }
              }).toBinary()
            )
          })
        }
      })
    );
    await expect(userMatrixPromise).resolves.toMatchObject({
      matrix: expect.objectContaining({ userId: 'user_1' })
    });

    const setRolePromise = client.setRolePermissionState(
      new SetRolePermissionStateRequest({
        roleName: 'helper',
        permission: 'message.post',
        state: PermissionEditState.ALLOW
      })
    );
    await waitForMessageHandling();
    requestFrame = socket.lastClientFrame();
    if (requestFrame.kind.case !== 'request') throw new Error('expected Request frame');
    request = requestFrame.kind.value;
    expect(request.method).toBe(wireMethods.setRolePermissionState);
    expect(SetRolePermissionStateRequest.fromBinary(request.body)).toMatchObject({
      roleName: 'helper',
      permission: 'message.post',
      state: PermissionEditState.ALLOW
    });
    socket.serverSend(
      new ServerFrame({
        frameId: requestFrame.frameId,
        kind: {
          case: 'response',
          value: new WireResponse({
            requestId: request.requestId,
            body: new Uint8Array(new SetPermissionStateResponse({ changed: true }).toBinary())
          })
        }
      })
    );
    await expect(setRolePromise).resolves.toMatchObject({ changed: true });

    const setUserPromise = client.setUserPermissionState(
      new SetUserPermissionStateRequest({
        userId: 'user_1',
        permission: 'message.post',
        state: PermissionEditState.DENY
      })
    );
    await waitForMessageHandling();
    requestFrame = socket.lastClientFrame();
    if (requestFrame.kind.case !== 'request') throw new Error('expected Request frame');
    request = requestFrame.kind.value;
    expect(request.method).toBe(wireMethods.setUserPermissionState);
    expect(SetUserPermissionStateRequest.fromBinary(request.body)).toMatchObject({
      userId: 'user_1',
      permission: 'message.post',
      state: PermissionEditState.DENY
    });
    socket.serverSend(
      new ServerFrame({
        frameId: requestFrame.frameId,
        kind: {
          case: 'response',
          value: new WireResponse({
            requestId: request.requestId,
            body: new Uint8Array(new SetPermissionStateResponse({ changed: true }).toBinary())
          })
        }
      })
    );
    await expect(setUserPromise).resolves.toMatchObject({ changed: true });
  });

  it('sends and decodes typed admin room layout requests', async () => {
    const harness = makeClient();
    const { client } = harness;
    await connectClient(harness);
    const socket = harness.socket();

    const layoutPromise = client.getAdminRoomLayout();
    await waitForMessageHandling();
    let requestFrame = socket.lastClientFrame();
    if (requestFrame.kind.case !== 'request') throw new Error('expected Request frame');
    let request = requestFrame.kind.value;
    expect(request.method).toBe(wireMethods.getAdminRoomLayout);
    expect(GetAdminRoomLayoutRequest.fromBinary(request.body)).toBeInstanceOf(
      GetAdminRoomLayoutRequest
    );
    socket.serverSend(
      new ServerFrame({
        frameId: requestFrame.frameId,
        kind: {
          case: 'response',
          value: new WireResponse({
            requestId: request.requestId,
            body: new Uint8Array(
              new GetAdminRoomLayoutResponse({
                groups: [
                  new AdminRoomGroupView({
                    id: 'g1',
                    name: 'Lobby',
                    rooms: [new AdminRoomInfoView({ id: 'r1', name: 'general' })]
                  })
                ]
              }).toBinary()
            )
          })
        }
      })
    );
    await expect(layoutPromise).resolves.toMatchObject({
      groups: [expect.objectContaining({ id: 'g1', name: 'Lobby' })]
    });

    const createPromise = client.createAdminRoomGroup(
      new CreateAdminRoomGroupRequest({ name: 'Projects' })
    );
    await waitForMessageHandling();
    requestFrame = socket.lastClientFrame();
    if (requestFrame.kind.case !== 'request') throw new Error('expected Request frame');
    request = requestFrame.kind.value;
    expect(request.method).toBe(wireMethods.createAdminRoomGroup);
    expect(CreateAdminRoomGroupRequest.fromBinary(request.body)).toMatchObject({
      name: 'Projects'
    });
    socket.serverSend(
      new ServerFrame({
        frameId: requestFrame.frameId,
        kind: {
          case: 'response',
          value: new WireResponse({
            requestId: request.requestId,
            body: new Uint8Array(
              new CreateAdminRoomGroupResponse({
                group: new AdminRoomGroupView({ id: 'g2', name: 'Projects' })
              }).toBinary()
            )
          })
        }
      })
    );
    await expect(createPromise).resolves.toMatchObject({
      group: expect.objectContaining({ id: 'g2', name: 'Projects' })
    });

    const movePromise = client.moveAdminRoomToGroup(
      new MoveAdminRoomToGroupRequest({ roomId: 'r1', groupId: 'g2' })
    );
    await waitForMessageHandling();
    requestFrame = socket.lastClientFrame();
    if (requestFrame.kind.case !== 'request') throw new Error('expected Request frame');
    request = requestFrame.kind.value;
    expect(request.method).toBe(wireMethods.moveAdminRoomToGroup);
    expect(MoveAdminRoomToGroupRequest.fromBinary(request.body)).toMatchObject({
      roomId: 'r1',
      groupId: 'g2'
    });
    socket.serverSend(
      new ServerFrame({
        frameId: requestFrame.frameId,
        kind: {
          case: 'response',
          value: new WireResponse({
            requestId: request.requestId,
            body: new Uint8Array(
              new MoveAdminRoomToGroupResponse({
                room: new AdminRoomInfoView({ id: 'r1', name: 'general' })
              }).toBinary()
            )
          })
        }
      })
    );
    await expect(movePromise).resolves.toMatchObject({
      room: expect.objectContaining({ id: 'r1' })
    });

    const reorderPromise = client.reorderAdminRoomsInGroup(
      new ReorderAdminRoomsInGroupRequest({ groupId: 'g2', orderedRoomIds: ['r1'] })
    );
    await waitForMessageHandling();
    requestFrame = socket.lastClientFrame();
    if (requestFrame.kind.case !== 'request') throw new Error('expected Request frame');
    request = requestFrame.kind.value;
    expect(request.method).toBe(wireMethods.reorderAdminRoomsInGroup);
    expect(ReorderAdminRoomsInGroupRequest.fromBinary(request.body)).toMatchObject({
      groupId: 'g2',
      orderedRoomIds: ['r1']
    });
    socket.serverSend(
      new ServerFrame({
        frameId: requestFrame.frameId,
        kind: {
          case: 'response',
          value: new WireResponse({
            requestId: request.requestId,
            body: new Uint8Array(
              new ReorderAdminRoomsInGroupResponse({
                group: new AdminRoomGroupView({ id: 'g2', name: 'Projects' })
              }).toBinary()
            )
          })
        }
      })
    );
    await expect(reorderPromise).resolves.toMatchObject({
      group: expect.objectContaining({ id: 'g2' })
    });

    const archivePromise = client.archiveAdminRoom(new ArchiveAdminRoomRequest({ roomId: 'r1' }));
    await waitForMessageHandling();
    requestFrame = socket.lastClientFrame();
    if (requestFrame.kind.case !== 'request') throw new Error('expected Request frame');
    request = requestFrame.kind.value;
    expect(request.method).toBe(wireMethods.archiveAdminRoom);
    expect(ArchiveAdminRoomRequest.fromBinary(request.body)).toMatchObject({ roomId: 'r1' });
    socket.serverSend(
      new ServerFrame({
        frameId: requestFrame.frameId,
        kind: {
          case: 'response',
          value: new WireResponse({
            requestId: request.requestId,
            body: new Uint8Array(
              new ArchiveAdminRoomResponse({
                room: new AdminRoomInfoView({ id: 'r1', name: 'general', archived: true })
              }).toBinary()
            )
          })
        }
      })
    );
    await expect(archivePromise).resolves.toMatchObject({
      room: expect.objectContaining({ archived: true })
    });

    const deletePromise = client.deleteAdminRoomGroup(
      new DeleteAdminRoomGroupRequest({ groupId: 'g2' })
    );
    await waitForMessageHandling();
    requestFrame = socket.lastClientFrame();
    if (requestFrame.kind.case !== 'request') throw new Error('expected Request frame');
    request = requestFrame.kind.value;
    expect(request.method).toBe(wireMethods.deleteAdminRoomGroup);
    expect(DeleteAdminRoomGroupRequest.fromBinary(request.body)).toMatchObject({
      groupId: 'g2'
    });
    socket.serverSend(
      new ServerFrame({
        frameId: requestFrame.frameId,
        kind: {
          case: 'response',
          value: new WireResponse({
            requestId: request.requestId,
            body: new Uint8Array(new DeleteAdminRoomGroupResponse({ deleted: true }).toBinary())
          })
        }
      })
    );
    await expect(deletePromise).resolves.toMatchObject({ deleted: true });
  });

  it('sends and decodes typed user preference requests', async () => {
    const harness = makeClient();
    const { client } = harness;
    await connectClient(harness);
    const socket = harness.socket();

    const settingsPromise = client.getUserSettings();
    await waitForMessageHandling();
    let requestFrame = socket.lastClientFrame();
    if (requestFrame.kind.case !== 'request') throw new Error('expected Request frame');
    let request = requestFrame.kind.value;
    expect(request.method).toBe(wireMethods.getUserSettings);
    expect(GetUserSettingsRequest.fromBinary(request.body)).toBeInstanceOf(GetUserSettingsRequest);
    socket.serverSend(
      new ServerFrame({
        frameId: requestFrame.frameId,
        kind: {
          case: 'response',
          value: new WireResponse({
            requestId: request.requestId,
            body: new Uint8Array(
              new GetUserSettingsResponse({
                settings: new ServerUserPreferences({
                  timezone: 'Europe/Berlin',
                  timeFormat: TimeFormat.TIME_FORMAT_24H
                })
              }).toBinary()
            )
          })
        }
      })
    );
    await expect(settingsPromise).resolves.toMatchObject({
      settings: expect.objectContaining({ timezone: 'Europe/Berlin' })
    });

    const updatePromise = client.updateUserSettings(
      new UpdateUserSettingsRequest({
        timezone: '',
        timeFormat: TimeFormat.TIME_FORMAT_12H
      })
    );
    await waitForMessageHandling();
    requestFrame = socket.lastClientFrame();
    if (requestFrame.kind.case !== 'request') throw new Error('expected Request frame');
    request = requestFrame.kind.value;
    expect(request.method).toBe(wireMethods.updateUserSettings);
    expect(UpdateUserSettingsRequest.fromBinary(request.body)).toMatchObject({
      timezone: '',
      timeFormat: TimeFormat.TIME_FORMAT_12H
    });
    socket.serverSend(
      new ServerFrame({
        frameId: requestFrame.frameId,
        kind: {
          case: 'response',
          value: new WireResponse({
            requestId: request.requestId,
            body: new Uint8Array(
              new UpdateUserSettingsResponse({
                settings: new ServerUserPreferences({
                  timeFormat: TimeFormat.TIME_FORMAT_12H
                })
              }).toBinary()
            )
          })
        }
      })
    );
    await expect(updatePromise).resolves.toMatchObject({
      settings: expect.objectContaining({ timeFormat: TimeFormat.TIME_FORMAT_12H })
    });
  });

  it('sends and decodes typed notification preference requests', async () => {
    const harness = makeClient();
    const { client } = harness;
    await connectClient(harness);
    const socket = harness.socket();

    const serverPromise = client.setServerNotificationLevel(
      new SetServerNotificationLevelRequest({ level: NotificationLevel.MUTED })
    );
    await waitForMessageHandling();
    let requestFrame = socket.lastClientFrame();
    if (requestFrame.kind.case !== 'request') throw new Error('expected Request frame');
    let request = requestFrame.kind.value;
    expect(request.method).toBe(wireMethods.setServerNotificationLevel);
    expect(SetServerNotificationLevelRequest.fromBinary(request.body)).toMatchObject({
      level: NotificationLevel.MUTED
    });
    socket.serverSend(
      new ServerFrame({
        frameId: requestFrame.frameId,
        kind: {
          case: 'response',
          value: new WireResponse({
            requestId: request.requestId,
            body: new Uint8Array(
              new SetNotificationLevelResponse({
                preference: {
                  level: NotificationLevel.MUTED,
                  effectiveLevel: NotificationLevel.MUTED
                }
              }).toBinary()
            )
          })
        }
      })
    );
    await expect(serverPromise).resolves.toMatchObject({
      preference: expect.objectContaining({ level: NotificationLevel.MUTED })
    });

    const roomPromise = client.setRoomNotificationLevel(
      new SetRoomNotificationLevelRequest({
        roomId: 'room_1',
        level: NotificationLevel.ALL_MESSAGES
      })
    );
    await waitForMessageHandling();
    requestFrame = socket.lastClientFrame();
    if (requestFrame.kind.case !== 'request') throw new Error('expected Request frame');
    request = requestFrame.kind.value;
    expect(request.method).toBe(wireMethods.setRoomNotificationLevel);
    expect(SetRoomNotificationLevelRequest.fromBinary(request.body)).toMatchObject({
      roomId: 'room_1',
      level: NotificationLevel.ALL_MESSAGES
    });
    socket.serverSend(
      new ServerFrame({
        frameId: requestFrame.frameId,
        kind: {
          case: 'response',
          value: new WireResponse({
            requestId: request.requestId,
            body: new Uint8Array(
              new SetNotificationLevelResponse({
                preference: {
                  level: NotificationLevel.ALL_MESSAGES,
                  effectiveLevel: NotificationLevel.ALL_MESSAGES
                }
              }).toBinary()
            )
          })
        }
      })
    );
    await expect(roomPromise).resolves.toMatchObject({
      preference: expect.objectContaining({ effectiveLevel: NotificationLevel.ALL_MESSAGES })
    });
  });

  it('sends and decodes typed presence update requests', async () => {
    const harness = makeClient();
    const { client } = harness;
    await connectClient(harness);
    const socket = harness.socket();

    const responsePromise = client.updateMyPresence(
      new UpdateMyPresenceRequest({ status: UserPresenceStatus.AWAY })
    );
    await waitForMessageHandling();
    const requestFrame = socket.lastClientFrame();
    if (requestFrame.kind.case !== 'request') throw new Error('expected Request frame');
    const request = requestFrame.kind.value;
    expect(request.method).toBe(wireMethods.updateMyPresence);
    expect(UpdateMyPresenceRequest.fromBinary(request.body)).toMatchObject({
      status: UserPresenceStatus.AWAY
    });

    socket.serverSend(
      new ServerFrame({
        frameId: requestFrame.frameId,
        kind: {
          case: 'response',
          value: new WireResponse({
            requestId: request.requestId,
            body: new Uint8Array(new UpdateMyPresenceResponse({ updated: true }).toBinary())
          })
        }
      })
    );

    await expect(responsePromise).resolves.toMatchObject({ updated: true });
  });

  it('sends and decodes typed push subscription requests', async () => {
    const harness = makeClient();
    const { client } = harness;
    await connectClient(harness);
    const socket = harness.socket();

    const subscribePromise = client.subscribeToPush(
      new SubscribeToPushRequest({
        endpoint: 'https://push.example.com/subscription',
        p256dh: 'client-public-key',
        auth: 'auth-secret',
        userAgent: 'test-browser'
      })
    );
    await waitForMessageHandling();
    let requestFrame = socket.lastClientFrame();
    if (requestFrame.kind.case !== 'request') throw new Error('expected Request frame');
    let request = requestFrame.kind.value;
    expect(request.method).toBe(wireMethods.subscribeToPush);
    expect(SubscribeToPushRequest.fromBinary(request.body)).toMatchObject({
      endpoint: 'https://push.example.com/subscription',
      p256dh: 'client-public-key',
      auth: 'auth-secret',
      userAgent: 'test-browser'
    });

    socket.serverSend(
      new ServerFrame({
        frameId: requestFrame.frameId,
        kind: {
          case: 'response',
          value: new WireResponse({
            requestId: request.requestId,
            body: new Uint8Array(new SubscribeToPushResponse({ subscribed: true }).toBinary())
          })
        }
      })
    );
    await expect(subscribePromise).resolves.toMatchObject({ subscribed: true });

    const unsubscribePromise = client.unsubscribeFromPush(
      new UnsubscribeFromPushRequest({ endpoint: 'https://push.example.com/subscription' })
    );
    await waitForMessageHandling();
    requestFrame = socket.lastClientFrame();
    if (requestFrame.kind.case !== 'request') throw new Error('expected Request frame');
    request = requestFrame.kind.value;
    expect(request.method).toBe(wireMethods.unsubscribeFromPush);
    expect(UnsubscribeFromPushRequest.fromBinary(request.body)).toMatchObject({
      endpoint: 'https://push.example.com/subscription'
    });

    socket.serverSend(
      new ServerFrame({
        frameId: requestFrame.frameId,
        kind: {
          case: 'response',
          value: new WireResponse({
            requestId: request.requestId,
            body: new Uint8Array(new UnsubscribeFromPushResponse({ unsubscribed: true }).toBinary())
          })
        }
      })
    );
    await expect(unsubscribePromise).resolves.toMatchObject({ unsubscribed: true });
  });

  it('sends and decodes typed notification requests', async () => {
    const harness = makeClient();
    const { client } = harness;
    await connectClient(harness);
    const socket = harness.socket();

    const listPromise = client.listNotifications(new ListNotificationsRequest({ limit: 25 }));
    await waitForMessageHandling();
    let requestFrame = socket.lastClientFrame();
    if (requestFrame.kind.case !== 'request') throw new Error('expected Request frame');
    let request = requestFrame.kind.value;
    expect(request.method).toBe(wireMethods.listNotifications);
    expect(ListNotificationsRequest.fromBinary(request.body)).toMatchObject({ limit: 25 });
    socket.serverSend(
      new ServerFrame({
        frameId: requestFrame.frameId,
        kind: {
          case: 'response',
          value: new WireResponse({
            requestId: request.requestId,
            body: new Uint8Array(
              new ListNotificationsResponse({
                items: [
                  new NotificationItemView({
                    id: 'notif_1',
                    kind: NotificationKind.MENTION,
                    summary: 'Test mentioned you',
                    roomId: 'room_1',
                    roomName: 'general',
                    eventId: 'event_1'
                  })
                ],
                totalCount: 1,
                serverName: 'Test Server'
              }).toBinary()
            )
          })
        }
      })
    );
    await expect(listPromise).resolves.toMatchObject({
      items: [expect.objectContaining({ id: 'notif_1', roomName: 'general' })],
      totalCount: 1,
      serverName: 'Test Server'
    });

    const hasPromise = client.hasNotifications();
    await waitForMessageHandling();
    requestFrame = socket.lastClientFrame();
    if (requestFrame.kind.case !== 'request') throw new Error('expected Request frame');
    request = requestFrame.kind.value;
    expect(request.method).toBe(wireMethods.hasNotifications);
    expect(HasNotificationsRequest.fromBinary(request.body)).toBeInstanceOf(
      HasNotificationsRequest
    );
    socket.serverSend(
      new ServerFrame({
        frameId: requestFrame.frameId,
        kind: {
          case: 'response',
          value: new WireResponse({
            requestId: request.requestId,
            body: new Uint8Array(
              new HasNotificationsResponse({ hasNotifications: true }).toBinary()
            )
          })
        }
      })
    );
    await expect(hasPromise).resolves.toMatchObject({ hasNotifications: true });

    const dismissPromise = client.dismissNotification(
      new DismissNotificationRequest({ notificationId: 'notif_1' })
    );
    await waitForMessageHandling();
    requestFrame = socket.lastClientFrame();
    if (requestFrame.kind.case !== 'request') throw new Error('expected Request frame');
    request = requestFrame.kind.value;
    expect(request.method).toBe(wireMethods.dismissNotification);
    expect(DismissNotificationRequest.fromBinary(request.body)).toMatchObject({
      notificationId: 'notif_1'
    });
    socket.serverSend(
      new ServerFrame({
        frameId: requestFrame.frameId,
        kind: {
          case: 'response',
          value: new WireResponse({
            requestId: request.requestId,
            body: new Uint8Array(new DismissNotificationResponse({ dismissed: true }).toBinary())
          })
        }
      })
    );
    await expect(dismissPromise).resolves.toMatchObject({ dismissed: true });

    const dismissAllPromise = client.dismissAllNotifications();
    await waitForMessageHandling();
    requestFrame = socket.lastClientFrame();
    if (requestFrame.kind.case !== 'request') throw new Error('expected Request frame');
    request = requestFrame.kind.value;
    expect(request.method).toBe(wireMethods.dismissAllNotifications);
    expect(DismissAllNotificationsRequest.fromBinary(request.body)).toBeInstanceOf(
      DismissAllNotificationsRequest
    );
    socket.serverSend(
      new ServerFrame({
        frameId: requestFrame.frameId,
        kind: {
          case: 'response',
          value: new WireResponse({
            requestId: request.requestId,
            body: new Uint8Array(
              new DismissAllNotificationsResponse({ dismissedCount: 3 }).toBinary()
            )
          })
        }
      })
    );
    await expect(dismissAllPromise).resolves.toMatchObject({ dismissedCount: 3 });
  });

  it('sends and decodes typed room moderation requests', async () => {
    const harness = makeClient();
    const { client } = harness;
    await connectClient(harness);
    const socket = harness.socket();

    const banPromise = client.banRoomMember(
      new BanRoomMemberRequest({
        roomId: 'room_1',
        userId: 'user_2',
        reason: 'spam'
      })
    );
    await waitForMessageHandling();

    const requestFrame = socket.lastClientFrame();
    if (requestFrame.kind.case !== 'request') throw new Error('expected Request frame');
    const request = requestFrame.kind.value;
    expect(request.method).toBe(wireMethods.banRoomMember);
    expect(BanRoomMemberRequest.fromBinary(request.body)).toMatchObject({
      roomId: 'room_1',
      userId: 'user_2',
      reason: 'spam'
    });

    socket.serverSend(
      new ServerFrame({
        frameId: requestFrame.frameId,
        kind: {
          case: 'response',
          value: new WireResponse({
            requestId: request.requestId,
            body: new Uint8Array(new BanRoomMemberResponse().toBinary())
          })
        }
      })
    );

    await expect(banPromise).resolves.toBeInstanceOf(BanRoomMemberResponse);

    const listPromise = client.listRoomBans(new ListRoomBansRequest({ roomId: 'room_1' }));
    await waitForMessageHandling();

    const listFrame = socket.lastClientFrame();
    if (listFrame.kind.case !== 'request') throw new Error('expected Request frame');
    const listRequest = listFrame.kind.value;
    expect(listRequest.method).toBe(wireMethods.listRoomBans);
    expect(ListRoomBansRequest.fromBinary(listRequest.body)).toMatchObject({
      roomId: 'room_1'
    });

    socket.serverSend(
      new ServerFrame({
        frameId: listFrame.frameId,
        kind: {
          case: 'response',
          value: new WireResponse({
            requestId: listRequest.requestId,
            body: new Uint8Array(
              new ListRoomBansResponse({
                bans: [
                  new RoomBanView({
                    id: 'ban_1',
                    roomId: 'room_1',
                    room: new AdminRoomInfoView({ id: 'room_1', name: 'general' }),
                    userId: 'user_2',
                    user: new UserAvatarView({
                      user: new User({ id: 'user_2', login: 'target', displayName: 'Target' }),
                      presenceStatus: CurrentUserPresenceStatus.OFFLINE
                    }),
                    reason: 'spam'
                  })
                ]
              }).toBinary()
            )
          })
        }
      })
    );

    await expect(listPromise).resolves.toMatchObject({
      bans: [{ id: 'ban_1', roomId: 'room_1', userId: 'user_2', reason: 'spam' }]
    });

    const unbanPromise = client.unbanRoomMember(
      new UnbanRoomMemberRequest({
        roomId: 'room_1',
        userId: 'user_2',
        reason: 'appeal accepted'
      })
    );
    await waitForMessageHandling();

    const unbanFrame = socket.lastClientFrame();
    if (unbanFrame.kind.case !== 'request') throw new Error('expected Request frame');
    const unbanRequest = unbanFrame.kind.value;
    expect(unbanRequest.method).toBe(wireMethods.unbanRoomMember);
    expect(UnbanRoomMemberRequest.fromBinary(unbanRequest.body)).toMatchObject({
      roomId: 'room_1',
      userId: 'user_2',
      reason: 'appeal accepted'
    });

    socket.serverSend(
      new ServerFrame({
        frameId: unbanFrame.frameId,
        kind: {
          case: 'response',
          value: new WireResponse({
            requestId: unbanRequest.requestId,
            body: new Uint8Array(new UnbanRoomMemberResponse({ unbanned: true }).toBinary())
          })
        }
      })
    );

    await expect(unbanPromise).resolves.toMatchObject({ unbanned: true });
  });

  it('sends and decodes typed voice call token requests', async () => {
    const harness = makeClient();
    const { client } = harness;
    await connectClient(harness);
    const socket = harness.socket();

    const responsePromise = client.getVoiceCallToken(
      new GetVoiceCallTokenRequest({ roomId: 'room_1' })
    );
    await waitForMessageHandling();

    const requestFrame = socket.lastClientFrame();
    if (requestFrame.kind.case !== 'request') throw new Error('expected Request frame');
    const request = requestFrame.kind.value;
    expect(request.method).toBe(wireMethods.getVoiceCallToken);
    expect(GetVoiceCallTokenRequest.fromBinary(request.body)).toMatchObject({
      roomId: 'room_1'
    });

    socket.serverSend(
      new ServerFrame({
        frameId: requestFrame.frameId,
        kind: {
          case: 'response',
          value: new WireResponse({
            requestId: request.requestId,
            body: new Uint8Array(
              new GetVoiceCallTokenResponse({
                token: new VoiceCallTokenView({
                  token: 'livekit-token',
                  e2eeKey: 'shared-key',
                  callId: 'call_1'
                })
              }).toBinary()
            )
          })
        }
      })
    );

    await expect(responsePromise).resolves.toMatchObject({
      token: expect.objectContaining({
        token: 'livekit-token',
        e2eeKey: 'shared-key',
        callId: 'call_1'
      })
    });
  });

  it('sends and decodes typed followed-thread read requests', async () => {
    const harness = makeClient();
    const { client } = harness;
    await connectClient(harness);
    const socket = harness.socket();

    const responsePromise = client.listMyFollowedThreads(
      new ListMyFollowedThreadsRequest({
        limit: 20,
        offset: 5
      })
    );
    await waitForMessageHandling();

    const requestFrame = socket.lastClientFrame();
    if (requestFrame.kind.case !== 'request') throw new Error('expected Request frame');
    const request = requestFrame.kind.value;
    expect(request.method).toBe(wireMethods.listMyFollowedThreads);
    expect(ListMyFollowedThreadsRequest.fromBinary(request.body)).toMatchObject({
      limit: 20,
      offset: 5
    });

    socket.serverSend(
      new ServerFrame({
        frameId: requestFrame.frameId,
        kind: {
          case: 'response',
          value: new WireResponse({
            requestId: request.requestId,
            body: new Uint8Array(
              new ListMyFollowedThreadsResponse({
                totalCount: 1,
                hasMore: false
              }).toBinary()
            )
          })
        }
      })
    );

    await expect(responsePromise).resolves.toMatchObject({ totalCount: 1, hasMore: false });
  });

  it('sends and decodes typed quick-switcher and room-create requests', async () => {
    const harness = makeClient();
    const { client } = harness;
    await connectClient(harness);
    const socket = harness.socket();

    const searchPromise = client.searchMembers(new SearchMembersRequest({ search: 'ali' }));
    await waitForMessageHandling();
    let requestFrame = socket.lastClientFrame();
    if (requestFrame.kind.case !== 'request') throw new Error('expected Request frame');
    let request = requestFrame.kind.value;
    expect(request.method).toBe(wireMethods.searchMembers);
    expect(SearchMembersRequest.fromBinary(request.body)).toMatchObject({ search: 'ali' });
    socket.serverSend(
      new ServerFrame({
        frameId: requestFrame.frameId,
        kind: {
          case: 'response',
          value: new WireResponse({
            requestId: request.requestId,
            body: new Uint8Array(new SearchMembersResponse({ viewerUserId: 'user_1' }).toBinary())
          })
        }
      })
    );
    await expect(searchPromise).resolves.toMatchObject({ viewerUserId: 'user_1' });

    const directoryPromise = client.getRoomDirectory();
    await waitForMessageHandling();
    requestFrame = socket.lastClientFrame();
    if (requestFrame.kind.case !== 'request') throw new Error('expected Request frame');
    request = requestFrame.kind.value;
    expect(request.method).toBe(wireMethods.getRoomDirectory);
    expect(GetRoomDirectoryRequest.fromBinary(request.body)).toBeInstanceOf(
      GetRoomDirectoryRequest
    );
    socket.serverSend(
      new ServerFrame({
        frameId: requestFrame.frameId,
        kind: {
          case: 'response',
          value: new WireResponse({
            requestId: request.requestId,
            body: new Uint8Array(new GetRoomDirectoryResponse().toBinary())
          })
        }
      })
    );
    await expect(directoryPromise).resolves.toBeInstanceOf(GetRoomDirectoryResponse);

    const dmPromise = client.startDM(new StartDMRequest({ participantIds: ['user_2'] }));
    await waitForMessageHandling();
    requestFrame = socket.lastClientFrame();
    if (requestFrame.kind.case !== 'request') throw new Error('expected Request frame');
    request = requestFrame.kind.value;
    expect(request.method).toBe(wireMethods.startDM);
    expect(StartDMRequest.fromBinary(request.body)).toMatchObject({
      participantIds: ['user_2']
    });
    socket.serverSend(
      new ServerFrame({
        frameId: requestFrame.frameId,
        kind: {
          case: 'response',
          value: new WireResponse({
            requestId: request.requestId,
            body: new Uint8Array(new StartDMResponse({ created: true }).toBinary())
          })
        }
      })
    );
    await expect(dmPromise).resolves.toMatchObject({ created: true });

    const createPromise = client.createRoom(
      new CreateRoomRequest({ name: 'general', groupId: 'group_1' })
    );
    await waitForMessageHandling();
    requestFrame = socket.lastClientFrame();
    if (requestFrame.kind.case !== 'request') throw new Error('expected Request frame');
    request = requestFrame.kind.value;
    expect(request.method).toBe(wireMethods.createRoom);
    expect(CreateRoomRequest.fromBinary(request.body)).toMatchObject({
      name: 'general',
      groupId: 'group_1'
    });
    socket.serverSend(
      new ServerFrame({
        frameId: requestFrame.frameId,
        kind: {
          case: 'response',
          value: new WireResponse({
            requestId: request.requestId,
            body: new Uint8Array(new CreateRoomResponse().toBinary())
          })
        }
      })
    );
    await expect(createPromise).resolves.toBeInstanceOf(CreateRoomResponse);
  });

  it('notifies close listeners when the socket closes', async () => {
    const harness = makeClient();
    const { client } = harness;
    await connectClient(harness);
    const socket = harness.socket();
    const onClose = vi.fn();

    client.onClose(onClose);
    socket.close();

    expect(client.status).toBe('closed');
    expect(onClose).toHaveBeenCalledOnce();
  });
});
