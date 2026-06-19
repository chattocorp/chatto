import {
  Ack,
  ClientFrame,
  ClientHello,
  CancelRequest,
  Request,
  Response,
  ServerFrame,
  ServerHello,
  StreamEvent,
  WireError
} from '$lib/pb/chatto/wire/v1/protocol_pb';
import {
  AddReactionRequest,
  AddReactionResponse,
  AdminClearUsernameCooldownRequest,
  AdminClearUsernameCooldownResponse,
  AdminUpdateUserRequest,
  AdminUpdateUserResponse,
  GetAdminSecurityConfigRequest,
  GetAdminSecurityConfigResponse,
  GetAdminSystemInfoRequest,
  GetAdminSystemInfoResponse,
  GetAdminEventLogEntryRequest,
  GetAdminEventLogEntryResponse,
  ArchiveAdminRoomRequest,
  ArchiveAdminRoomResponse,
  BanRoomMemberRequest,
  BanRoomMemberResponse,
  CreateAdminRoomGroupRequest,
  CreateAdminRoomGroupResponse,
  CreateAdminRoleRequest,
  CreateAdminRoleResponse,
  DeleteMyAccountRequest,
  DeleteMyAccountResponse,
  DeleteAdminRoomGroupRequest,
  DeleteAdminRoomGroupResponse,
  DeleteAdminRoleRequest,
  DeleteAdminRoleResponse,
  GetAuthenticatedServerSettingsRequest,
  GetAuthenticatedServerSettingsResponse,
  GetAccountDeletionStatusRequest,
  GetAccountDeletionStatusResponse,
  GetAdminMemberRequest,
  GetAdminMemberResponse,
  GetAdminRoomLayoutRequest,
  GetAdminRoomLayoutResponse,
  GetAdminRoleCapabilitiesRequest,
  GetAdminRoleCapabilitiesResponse,
  GetAdminRoleRequest,
  GetAdminRoleResponse,
  GetCallParticipantsRequest,
  GetCallParticipantsResponse,
  GetCurrentUserRequest,
  GetCurrentUserResponse,
  CreateRoomRequest,
  CreateRoomResponse,
  DeleteAttachmentRequest,
  DeleteAttachmentResponse,
  DeleteLinkPreviewRequest,
  DeleteLinkPreviewResponse,
  DeleteMessageRequest,
  DeleteMessageResponse,
  FollowThreadRequest,
  FollowThreadResponse,
  GetLinkPreviewRequest,
  GetLinkPreviewResponse,
  HasNotificationsRequest,
  HasNotificationsResponse,
  GetProfileSettingsRequest,
  GetProfileSettingsResponse,
  GetRoomMembersRequest,
  GetRoomMembersResponse,
  GetRoomDirectoryRequest,
  GetRoomDirectoryResponse,
  GetRoomRequest,
  GetRoomResponse,
  GetRoomEventRequest,
  GetRoomEventResponse,
  GetRolePermissionMatrixRequest,
  GetRolePermissionMatrixResponse,
  GetRolePermissionTierMatrixRequest,
  GetRolePermissionTierMatrixResponse,
  GetRoomTimelineRequest,
  GetRoomTimelineAfterRequest,
  GetRoomTimelineAfterResponse,
  GetRoomTimelineAroundRequest,
  GetRoomTimelineAroundResponse,
  GetRoomTimelineResponse,
  GetServerSettingsRequest,
  GetServerSettingsResponse,
  GetThreadEventsAroundRequest,
  GetThreadEventsAroundResponse,
  GetThreadEventsRequest,
  GetThreadEventsResponse,
  GetUserPermissionMatrixRequest,
  GetUserPermissionMatrixResponse,
  GetUserSettingsRequest,
  GetUserSettingsResponse,
  GetViewerRequest,
  GetViewerResponse,
  GetVoiceCallTokenRequest,
  GetVoiceCallTokenResponse,
  JoinGroupRequest,
  JoinGroupResponse,
  JoinRoomRequest,
  JoinRoomResponse,
  JoinVoiceCallRequest,
  JoinVoiceCallResponse,
  LeaveRoomRequest,
  LeaveRoomResponse,
  LeaveVoiceCallRequest,
  LeaveVoiceCallResponse,
  ListNotificationsRequest,
  ListNotificationsResponse,
  ListActiveCallsRequest,
  ListActiveCallsResponse,
  ListAdminEventLogRequest,
  ListAdminEventLogResponse,
  ListAdminMembersRequest,
  ListAdminMembersResponse,
  ListMyFollowedThreadsRequest,
  ListMyFollowedThreadsResponse,
  ListMyRoomsRequest,
  ListMyRoomsResponse,
  ListRoomBansRequest,
  ListRoomBansResponse,
  PostMessageRequest,
  PostMessageResponse,
  MoveAdminRoomToGroupRequest,
  MoveAdminRoomToGroupResponse,
  RemoveReactionRequest,
  RemoveReactionResponse,
  RequestAccountDeletionRequest,
  RequestAccountDeletionResponse,
  RevokeMemberRoleRequest,
  RevokeMemberRoleResponse,
  ReorderAdminRoomGroupsRequest,
  ReorderAdminRoomGroupsResponse,
  ReorderAdminRoomsInGroupRequest,
  ReorderAdminRoomsInGroupResponse,
  SearchMembersRequest,
  SearchMembersResponse,
  SendTypingIndicatorRequest,
  SendTypingIndicatorResponse,
  SetNotificationLevelResponse,
  SetPermissionStateResponse,
  SetRoomNotificationLevelRequest,
  SetRolePermissionStateRequest,
  SetServerNotificationLevelRequest,
  SetUserPermissionStateRequest,
  AssignMemberRoleRequest,
  AssignMemberRoleResponse,
  StartDMRequest,
  StartDMResponse,
  SubscribeToPushRequest,
  SubscribeToPushResponse,
  MarkThreadAsReadRequest,
  MarkThreadAsReadResponse,
  MarkRoomAsReadRequest,
  MarkRoomAsReadResponse,
  UnfollowThreadRequest,
  UnfollowThreadResponse,
  UnarchiveAdminRoomRequest,
  UnarchiveAdminRoomResponse,
  UnbanRoomMemberRequest,
  UnbanRoomMemberResponse,
  DismissAllNotificationsRequest,
  DismissAllNotificationsResponse,
  DismissNotificationRequest,
  DismissNotificationResponse,
  UnsubscribeFromPushRequest,
  UnsubscribeFromPushResponse,
  UpdateAdminRoomRequest,
  UpdateAdminRoomResponse,
  UpdateAdminRoomGroupRequest,
  UpdateAdminRoomGroupResponse,
  UpdateAdminRoleRequest,
  UpdateAdminRoleResponse,
  UpdateMessageRequest,
  UpdateMessageResponse,
  UpdateMyPresenceRequest,
  UpdateMyPresenceResponse,
  UpdateProfileRequest,
  UpdateProfileResponse,
  UpdateBlockedUsernamesRequest,
  UpdateBlockedUsernamesResponse,
  UpdateServerSettingsRequest,
  UpdateServerSettingsResponse,
  UpdateUserSettingsRequest,
  UpdateUserSettingsResponse
} from '$lib/pb/chatto/api/v1/chat_pb';

const WEBSOCKET_OPEN = 1;
const DEFAULT_WIRE_URL = '/api/wire';
const WIRE_PROTOCOL_VERSION = 'chatto-wire-v1';

export const wireMethods = {
  getViewer: '/chatto.api.v1.ChattoApiService/GetViewer',
  getCurrentUser: '/chatto.api.v1.ChattoApiService/GetCurrentUser',
  getAuthenticatedServerSettings: '/chatto.api.v1.ChattoApiService/GetAuthenticatedServerSettings',
  getAccountDeletionStatus: '/chatto.api.v1.ChattoApiService/GetAccountDeletionStatus',
  requestAccountDeletion: '/chatto.api.v1.ChattoApiService/RequestAccountDeletion',
  deleteMyAccount: '/chatto.api.v1.ChattoApiService/DeleteMyAccount',
  getServerSettings: '/chatto.api.v1.ChattoApiService/GetServerSettings',
  updateServerSettings: '/chatto.api.v1.ChattoApiService/UpdateServerSettings',
  getAdminSecurityConfig: '/chatto.api.v1.ChattoApiService/GetAdminSecurityConfig',
  updateBlockedUsernames: '/chatto.api.v1.ChattoApiService/UpdateBlockedUsernames',
  getAdminSystemInfo: '/chatto.api.v1.ChattoApiService/GetAdminSystemInfo',
  listAdminEventLog: '/chatto.api.v1.ChattoApiService/ListAdminEventLog',
  getAdminEventLogEntry: '/chatto.api.v1.ChattoApiService/GetAdminEventLogEntry',
  listAdminMembers: '/chatto.api.v1.ChattoApiService/ListAdminMembers',
  getAdminMember: '/chatto.api.v1.ChattoApiService/GetAdminMember',
  adminUpdateUser: '/chatto.api.v1.ChattoApiService/AdminUpdateUser',
  adminClearUsernameCooldown: '/chatto.api.v1.ChattoApiService/AdminClearUsernameCooldown',
  assignMemberRole: '/chatto.api.v1.ChattoApiService/AssignMemberRole',
  revokeMemberRole: '/chatto.api.v1.ChattoApiService/RevokeMemberRole',
  getAdminRoleCapabilities: '/chatto.api.v1.ChattoApiService/GetAdminRoleCapabilities',
  getAdminRole: '/chatto.api.v1.ChattoApiService/GetAdminRole',
  createAdminRole: '/chatto.api.v1.ChattoApiService/CreateAdminRole',
  updateAdminRole: '/chatto.api.v1.ChattoApiService/UpdateAdminRole',
  deleteAdminRole: '/chatto.api.v1.ChattoApiService/DeleteAdminRole',
  getRolePermissionTierMatrix: '/chatto.api.v1.ChattoApiService/GetRolePermissionTierMatrix',
  getRolePermissionMatrix: '/chatto.api.v1.ChattoApiService/GetRolePermissionMatrix',
  getUserPermissionMatrix: '/chatto.api.v1.ChattoApiService/GetUserPermissionMatrix',
  setRolePermissionState: '/chatto.api.v1.ChattoApiService/SetRolePermissionState',
  setUserPermissionState: '/chatto.api.v1.ChattoApiService/SetUserPermissionState',
  getProfileSettings: '/chatto.api.v1.ChattoApiService/GetProfileSettings',
  updateProfile: '/chatto.api.v1.ChattoApiService/UpdateProfile',
  getUserSettings: '/chatto.api.v1.ChattoApiService/GetUserSettings',
  updateUserSettings: '/chatto.api.v1.ChattoApiService/UpdateUserSettings',
  setServerNotificationLevel: '/chatto.api.v1.ChattoApiService/SetServerNotificationLevel',
  setRoomNotificationLevel: '/chatto.api.v1.ChattoApiService/SetRoomNotificationLevel',
  subscribeToPush: '/chatto.api.v1.ChattoApiService/SubscribeToPush',
  unsubscribeFromPush: '/chatto.api.v1.ChattoApiService/UnsubscribeFromPush',
  listNotifications: '/chatto.api.v1.ChattoApiService/ListNotifications',
  hasNotifications: '/chatto.api.v1.ChattoApiService/HasNotifications',
  dismissNotification: '/chatto.api.v1.ChattoApiService/DismissNotification',
  dismissAllNotifications: '/chatto.api.v1.ChattoApiService/DismissAllNotifications',
  listMyRooms: '/chatto.api.v1.ChattoApiService/ListMyRooms',
  getRoom: '/chatto.api.v1.ChattoApiService/GetRoom',
  getRoomMembers: '/chatto.api.v1.ChattoApiService/GetRoomMembers',
  getRoomDirectory: '/chatto.api.v1.ChattoApiService/GetRoomDirectory',
  searchMembers: '/chatto.api.v1.ChattoApiService/SearchMembers',
  startDM: '/chatto.api.v1.ChattoApiService/StartDM',
  createRoom: '/chatto.api.v1.ChattoApiService/CreateRoom',
  getAdminRoomLayout: '/chatto.api.v1.ChattoApiService/GetAdminRoomLayout',
  createAdminRoomGroup: '/chatto.api.v1.ChattoApiService/CreateAdminRoomGroup',
  updateAdminRoomGroup: '/chatto.api.v1.ChattoApiService/UpdateAdminRoomGroup',
  deleteAdminRoomGroup: '/chatto.api.v1.ChattoApiService/DeleteAdminRoomGroup',
  reorderAdminRoomGroups: '/chatto.api.v1.ChattoApiService/ReorderAdminRoomGroups',
  moveAdminRoomToGroup: '/chatto.api.v1.ChattoApiService/MoveAdminRoomToGroup',
  reorderAdminRoomsInGroup: '/chatto.api.v1.ChattoApiService/ReorderAdminRoomsInGroup',
  updateAdminRoom: '/chatto.api.v1.ChattoApiService/UpdateAdminRoom',
  archiveAdminRoom: '/chatto.api.v1.ChattoApiService/ArchiveAdminRoom',
  unarchiveAdminRoom: '/chatto.api.v1.ChattoApiService/UnarchiveAdminRoom',
  joinRoom: '/chatto.api.v1.ChattoApiService/JoinRoom',
  leaveRoom: '/chatto.api.v1.ChattoApiService/LeaveRoom',
  joinGroup: '/chatto.api.v1.ChattoApiService/JoinGroup',
  banRoomMember: '/chatto.api.v1.ChattoApiService/BanRoomMember',
  listRoomBans: '/chatto.api.v1.ChattoApiService/ListRoomBans',
  unbanRoomMember: '/chatto.api.v1.ChattoApiService/UnbanRoomMember',
  getRoomEvent: '/chatto.api.v1.ChattoApiService/GetRoomEvent',
  getRoomTimeline: '/chatto.api.v1.ChattoApiService/GetRoomTimeline',
  getRoomTimelineAfter: '/chatto.api.v1.ChattoApiService/GetRoomTimelineAfter',
  getRoomTimelineAround: '/chatto.api.v1.ChattoApiService/GetRoomTimelineAround',
  getThreadEvents: '/chatto.api.v1.ChattoApiService/GetThreadEvents',
  getThreadEventsAround: '/chatto.api.v1.ChattoApiService/GetThreadEventsAround',
  listMyFollowedThreads: '/chatto.api.v1.ChattoApiService/ListMyFollowedThreads',
  getLinkPreview: '/chatto.api.v1.ChattoApiService/GetLinkPreview',
  postMessage: '/chatto.api.v1.ChattoApiService/PostMessage',
  updateMessage: '/chatto.api.v1.ChattoApiService/UpdateMessage',
  deleteMessage: '/chatto.api.v1.ChattoApiService/DeleteMessage',
  deleteAttachment: '/chatto.api.v1.ChattoApiService/DeleteAttachment',
  deleteLinkPreview: '/chatto.api.v1.ChattoApiService/DeleteLinkPreview',
  addReaction: '/chatto.api.v1.ChattoApiService/AddReaction',
  removeReaction: '/chatto.api.v1.ChattoApiService/RemoveReaction',
  followThread: '/chatto.api.v1.ChattoApiService/FollowThread',
  unfollowThread: '/chatto.api.v1.ChattoApiService/UnfollowThread',
  markRoomAsRead: '/chatto.api.v1.ChattoApiService/MarkRoomAsRead',
  markThreadAsRead: '/chatto.api.v1.ChattoApiService/MarkThreadAsRead',
  sendTypingIndicator: '/chatto.api.v1.ChattoApiService/SendTypingIndicator',
  updateMyPresence: '/chatto.api.v1.ChattoApiService/UpdateMyPresence',
  listActiveCalls: '/chatto.api.v1.ChattoApiService/ListActiveCalls',
  getCallParticipants: '/chatto.api.v1.ChattoApiService/GetCallParticipants',
  joinVoiceCall: '/chatto.api.v1.ChattoApiService/JoinVoiceCall',
  leaveVoiceCall: '/chatto.api.v1.ChattoApiService/LeaveVoiceCall',
  getVoiceCallToken: '/chatto.api.v1.ChattoApiService/GetVoiceCallToken'
} as const;

export type WireMethod = (typeof wireMethods)[keyof typeof wireMethods];

export type WireConnectionStatus = 'idle' | 'connecting' | 'connected' | 'closed';

export interface WireSocket {
  binaryType: BinaryType;
  readyState: number;
  send(data: Uint8Array): void;
  close(code?: number, reason?: string): void;
  addEventListener(type: 'open', listener: (event: Event) => void): void;
  addEventListener(type: 'message', listener: (event: MessageEvent) => void): void;
  addEventListener(type: 'close', listener: (event: CloseEvent) => void): void;
  addEventListener(type: 'error', listener: (event: Event) => void): void;
  removeEventListener(type: 'open', listener: (event: Event) => void): void;
  removeEventListener(type: 'message', listener: (event: MessageEvent) => void): void;
  removeEventListener(type: 'close', listener: (event: CloseEvent) => void): void;
  removeEventListener(type: 'error', listener: (event: Event) => void): void;
}

export type WireSocketFactory = (url: string) => WireSocket;

export interface WireClientConfig {
  url?: string;
  token?: string | null;
  socketFactory?: WireSocketFactory;
}

export interface WireConnectOptions {
  resumeAfter?: string;
  acceptedFeatures?: string[];
}

export interface WireRequestOptions {
  requestId?: string;
}

interface BinaryMessage {
  toBinary(): Uint8Array<ArrayBufferLike>;
}

interface BinaryMessageType<T> {
  fromBinary(bytes: Uint8Array): T;
}

interface PendingRequest<T> {
  responseType: BinaryMessageType<T>;
  resolve(value: T): void;
  reject(error: unknown): void;
}

type StreamEventListener = (event: StreamEvent) => void;
type WireErrorListener = (error: WireProtocolError) => void;
type WireCloseListener = () => void;

export class WireProtocolError extends Error {
  readonly wireError?: WireError;

  constructor(message: string, wireError?: WireError) {
    super(message);
    this.name = 'WireProtocolError';
    this.wireError = wireError;
  }
}

export function httpToWireWsUrl(url: string): string {
  if (url.startsWith('ws://') || url.startsWith('wss://')) return url;
  if (url.startsWith('http://') || url.startsWith('https://')) {
    return url.replace(/^http/, 'ws');
  }
  if (url.startsWith('/') && typeof window !== 'undefined') {
    const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
    return `${protocol}//${window.location.host}${url}`;
  }
  return url;
}

export class WireClient {
  status: WireConnectionStatus = 'idle';
  lastDeliveryCursor = '';

  #url: string;
  #token: string | null;
  #socketFactory: WireSocketFactory;
  #socket: WireSocket | null = null;
  #connectPromise: Promise<ServerHello> | null = null;
  #helloResolve: ((hello: ServerHello) => void) | null = null;
  #helloReject: ((error: unknown) => void) | null = null;
  #requestSeq = 0;
  #frameSeq = 0;
  #pendingRequests = new Map<string, PendingRequest<unknown>>();
  #eventListeners = new Set<StreamEventListener>();
  #errorListeners = new Set<WireErrorListener>();
  #closeListeners = new Set<WireCloseListener>();

  constructor(config: WireClientConfig = {}) {
    this.#url = httpToWireWsUrl(config.url ?? DEFAULT_WIRE_URL);
    this.#token = config.token ?? null;
    this.#socketFactory = config.socketFactory ?? ((url) => new WebSocket(url));
  }

  connect(options: WireConnectOptions = {}): Promise<ServerHello> {
    if (this.#connectPromise) return this.#connectPromise;

    this.status = 'connecting';
    const socket = this.#socketFactory(this.#url);
    socket.binaryType = 'arraybuffer';
    this.#socket = socket;

    socket.addEventListener('open', this.#handleOpen(options));
    socket.addEventListener('message', this.#handleMessage);
    socket.addEventListener('close', this.#handleClose);
    socket.addEventListener('error', this.#handleSocketError);

    this.#connectPromise = new Promise<ServerHello>((resolve, reject) => {
      this.#helloResolve = resolve;
      this.#helloReject = reject;
    });
    return this.#connectPromise;
  }

  dispose(): void {
    const error = new WireProtocolError('wire connection disposed');
    this.#helloReject?.(error);
    this.#helloResolve = null;
    this.#helloReject = null;
    this.#rejectAll(error);
    this.#socket?.close();
    this.#detachSocket();
    this.status = 'closed';
  }

  onEvent(listener: StreamEventListener): () => void {
    this.#eventListeners.add(listener);
    return () => this.#eventListeners.delete(listener);
  }

  onError(listener: WireErrorListener): () => void {
    this.#errorListeners.add(listener);
    return () => this.#errorListeners.delete(listener);
  }

  onClose(listener: WireCloseListener): () => void {
    this.#closeListeners.add(listener);
    return () => this.#closeListeners.delete(listener);
  }

  async request<TResponse>(
    method: WireMethod | string,
    body: BinaryMessage,
    responseType: BinaryMessageType<TResponse>,
    options: WireRequestOptions = {}
  ): Promise<TResponse> {
    await this.connect();
    const requestId = options.requestId ?? this.#nextRequestId();
    if (this.#pendingRequests.has(requestId)) {
      throw new WireProtocolError(`wire request ${requestId} is already in flight`);
    }

    const promise = new Promise<TResponse>((resolve, reject) => {
      this.#pendingRequests.set(requestId, {
        responseType,
        resolve: resolve as (value: unknown) => void,
        reject
      });
    });

    const frame = new ClientFrame({
      frameId: this.#nextFrameId(),
      kind: {
        case: 'request',
        value: new Request({
          requestId,
          method,
          body: protobufBytes(body)
        })
      }
    });

    try {
      this.#send(frame);
    } catch (error: unknown) {
      this.#pendingRequests.delete(requestId);
      throw error;
    }

    return promise;
  }

  cancel(requestId: string): void {
    this.#send(
      new ClientFrame({
        frameId: this.#nextFrameId(),
        kind: {
          case: 'cancel',
          value: new CancelRequest({ requestId })
        }
      })
    );
  }

  ack(event: StreamEvent): void {
    this.#send(
      new ClientFrame({
        frameId: this.#nextFrameId(),
        kind: {
          case: 'ack',
          value: new Ack({
            eventId: event.eventId,
            deliveryCursor: event.deliveryCursor
          })
        }
      })
    );
  }

  getViewer(): Promise<GetViewerResponse> {
    return this.request(wireMethods.getViewer, new GetViewerRequest(), GetViewerResponse);
  }

  getCurrentUser(request = new GetCurrentUserRequest()): Promise<GetCurrentUserResponse> {
    return this.request(wireMethods.getCurrentUser, request, GetCurrentUserResponse);
  }

  getAuthenticatedServerSettings(
    request = new GetAuthenticatedServerSettingsRequest()
  ): Promise<GetAuthenticatedServerSettingsResponse> {
    return this.request(
      wireMethods.getAuthenticatedServerSettings,
      request,
      GetAuthenticatedServerSettingsResponse
    );
  }

  getAccountDeletionStatus(
    request = new GetAccountDeletionStatusRequest()
  ): Promise<GetAccountDeletionStatusResponse> {
    return this.request(
      wireMethods.getAccountDeletionStatus,
      request,
      GetAccountDeletionStatusResponse
    );
  }

  requestAccountDeletion(
    request = new RequestAccountDeletionRequest()
  ): Promise<RequestAccountDeletionResponse> {
    return this.request(
      wireMethods.requestAccountDeletion,
      request,
      RequestAccountDeletionResponse
    );
  }

  deleteMyAccount(request: DeleteMyAccountRequest): Promise<DeleteMyAccountResponse> {
    return this.request(wireMethods.deleteMyAccount, request, DeleteMyAccountResponse);
  }

  getServerSettings(request = new GetServerSettingsRequest()): Promise<GetServerSettingsResponse> {
    return this.request(wireMethods.getServerSettings, request, GetServerSettingsResponse);
  }

  updateServerSettings(
    request: UpdateServerSettingsRequest
  ): Promise<UpdateServerSettingsResponse> {
    return this.request(wireMethods.updateServerSettings, request, UpdateServerSettingsResponse);
  }

  getAdminSecurityConfig(
    request = new GetAdminSecurityConfigRequest()
  ): Promise<GetAdminSecurityConfigResponse> {
    return this.request(
      wireMethods.getAdminSecurityConfig,
      request,
      GetAdminSecurityConfigResponse
    );
  }

  updateBlockedUsernames(
    request: UpdateBlockedUsernamesRequest
  ): Promise<UpdateBlockedUsernamesResponse> {
    return this.request(
      wireMethods.updateBlockedUsernames,
      request,
      UpdateBlockedUsernamesResponse
    );
  }

  getAdminSystemInfo(
    request = new GetAdminSystemInfoRequest()
  ): Promise<GetAdminSystemInfoResponse> {
    return this.request(wireMethods.getAdminSystemInfo, request, GetAdminSystemInfoResponse);
  }

  listAdminEventLog(
    request = new ListAdminEventLogRequest()
  ): Promise<ListAdminEventLogResponse> {
    return this.request(wireMethods.listAdminEventLog, request, ListAdminEventLogResponse);
  }

  getAdminEventLogEntry(
    request: GetAdminEventLogEntryRequest
  ): Promise<GetAdminEventLogEntryResponse> {
    return this.request(
      wireMethods.getAdminEventLogEntry,
      request,
      GetAdminEventLogEntryResponse
    );
  }

  listAdminMembers(
    request = new ListAdminMembersRequest()
  ): Promise<ListAdminMembersResponse> {
    return this.request(wireMethods.listAdminMembers, request, ListAdminMembersResponse);
  }

  getAdminMember(request: GetAdminMemberRequest): Promise<GetAdminMemberResponse> {
    return this.request(wireMethods.getAdminMember, request, GetAdminMemberResponse);
  }

  adminUpdateUser(request: AdminUpdateUserRequest): Promise<AdminUpdateUserResponse> {
    return this.request(wireMethods.adminUpdateUser, request, AdminUpdateUserResponse);
  }

  adminClearUsernameCooldown(
    request: AdminClearUsernameCooldownRequest
  ): Promise<AdminClearUsernameCooldownResponse> {
    return this.request(
      wireMethods.adminClearUsernameCooldown,
      request,
      AdminClearUsernameCooldownResponse
    );
  }

  assignMemberRole(request: AssignMemberRoleRequest): Promise<AssignMemberRoleResponse> {
    return this.request(wireMethods.assignMemberRole, request, AssignMemberRoleResponse);
  }

  revokeMemberRole(request: RevokeMemberRoleRequest): Promise<RevokeMemberRoleResponse> {
    return this.request(wireMethods.revokeMemberRole, request, RevokeMemberRoleResponse);
  }

  getAdminRoleCapabilities(
    request = new GetAdminRoleCapabilitiesRequest()
  ): Promise<GetAdminRoleCapabilitiesResponse> {
    return this.request(
      wireMethods.getAdminRoleCapabilities,
      request,
      GetAdminRoleCapabilitiesResponse
    );
  }

  getAdminRole(request: GetAdminRoleRequest): Promise<GetAdminRoleResponse> {
    return this.request(wireMethods.getAdminRole, request, GetAdminRoleResponse);
  }

  createAdminRole(request: CreateAdminRoleRequest): Promise<CreateAdminRoleResponse> {
    return this.request(wireMethods.createAdminRole, request, CreateAdminRoleResponse);
  }

  updateAdminRole(request: UpdateAdminRoleRequest): Promise<UpdateAdminRoleResponse> {
    return this.request(wireMethods.updateAdminRole, request, UpdateAdminRoleResponse);
  }

  deleteAdminRole(request: DeleteAdminRoleRequest): Promise<DeleteAdminRoleResponse> {
    return this.request(wireMethods.deleteAdminRole, request, DeleteAdminRoleResponse);
  }

  getRolePermissionTierMatrix(
    request = new GetRolePermissionTierMatrixRequest()
  ): Promise<GetRolePermissionTierMatrixResponse> {
    return this.request(
      wireMethods.getRolePermissionTierMatrix,
      request,
      GetRolePermissionTierMatrixResponse
    );
  }

  getRolePermissionMatrix(
    request: GetRolePermissionMatrixRequest
  ): Promise<GetRolePermissionMatrixResponse> {
    return this.request(
      wireMethods.getRolePermissionMatrix,
      request,
      GetRolePermissionMatrixResponse
    );
  }

  getUserPermissionMatrix(
    request: GetUserPermissionMatrixRequest
  ): Promise<GetUserPermissionMatrixResponse> {
    return this.request(
      wireMethods.getUserPermissionMatrix,
      request,
      GetUserPermissionMatrixResponse
    );
  }

  setRolePermissionState(
    request: SetRolePermissionStateRequest
  ): Promise<SetPermissionStateResponse> {
    return this.request(
      wireMethods.setRolePermissionState,
      request,
      SetPermissionStateResponse
    );
  }

  setUserPermissionState(
    request: SetUserPermissionStateRequest
  ): Promise<SetPermissionStateResponse> {
    return this.request(
      wireMethods.setUserPermissionState,
      request,
      SetPermissionStateResponse
    );
  }

  getProfileSettings(
    request = new GetProfileSettingsRequest()
  ): Promise<GetProfileSettingsResponse> {
    return this.request(wireMethods.getProfileSettings, request, GetProfileSettingsResponse);
  }

  updateProfile(request: UpdateProfileRequest): Promise<UpdateProfileResponse> {
    return this.request(wireMethods.updateProfile, request, UpdateProfileResponse);
  }

  getUserSettings(request = new GetUserSettingsRequest()): Promise<GetUserSettingsResponse> {
    return this.request(wireMethods.getUserSettings, request, GetUserSettingsResponse);
  }

  updateUserSettings(request: UpdateUserSettingsRequest): Promise<UpdateUserSettingsResponse> {
    return this.request(wireMethods.updateUserSettings, request, UpdateUserSettingsResponse);
  }

  setServerNotificationLevel(
    request: SetServerNotificationLevelRequest
  ): Promise<SetNotificationLevelResponse> {
    return this.request(
      wireMethods.setServerNotificationLevel,
      request,
      SetNotificationLevelResponse
    );
  }

  setRoomNotificationLevel(
    request: SetRoomNotificationLevelRequest
  ): Promise<SetNotificationLevelResponse> {
    return this.request(
      wireMethods.setRoomNotificationLevel,
      request,
      SetNotificationLevelResponse
    );
  }

  subscribeToPush(request: SubscribeToPushRequest): Promise<SubscribeToPushResponse> {
    return this.request(wireMethods.subscribeToPush, request, SubscribeToPushResponse);
  }

  unsubscribeFromPush(request: UnsubscribeFromPushRequest): Promise<UnsubscribeFromPushResponse> {
    return this.request(wireMethods.unsubscribeFromPush, request, UnsubscribeFromPushResponse);
  }

  listNotifications(request = new ListNotificationsRequest()): Promise<ListNotificationsResponse> {
    return this.request(wireMethods.listNotifications, request, ListNotificationsResponse);
  }

  hasNotifications(request = new HasNotificationsRequest()): Promise<HasNotificationsResponse> {
    return this.request(wireMethods.hasNotifications, request, HasNotificationsResponse);
  }

  dismissNotification(request: DismissNotificationRequest): Promise<DismissNotificationResponse> {
    return this.request(wireMethods.dismissNotification, request, DismissNotificationResponse);
  }

  dismissAllNotifications(
    request = new DismissAllNotificationsRequest()
  ): Promise<DismissAllNotificationsResponse> {
    return this.request(
      wireMethods.dismissAllNotifications,
      request,
      DismissAllNotificationsResponse
    );
  }

  listMyRooms(request = new ListMyRoomsRequest()): Promise<ListMyRoomsResponse> {
    return this.request(wireMethods.listMyRooms, request, ListMyRoomsResponse);
  }

  getRoom(request: GetRoomRequest): Promise<GetRoomResponse> {
    return this.request(wireMethods.getRoom, request, GetRoomResponse);
  }

  getRoomMembers(request: GetRoomMembersRequest): Promise<GetRoomMembersResponse> {
    return this.request(wireMethods.getRoomMembers, request, GetRoomMembersResponse);
  }

  getRoomDirectory(request = new GetRoomDirectoryRequest()): Promise<GetRoomDirectoryResponse> {
    return this.request(wireMethods.getRoomDirectory, request, GetRoomDirectoryResponse);
  }

  searchMembers(request = new SearchMembersRequest()): Promise<SearchMembersResponse> {
    return this.request(wireMethods.searchMembers, request, SearchMembersResponse);
  }

  startDM(request: StartDMRequest): Promise<StartDMResponse> {
    return this.request(wireMethods.startDM, request, StartDMResponse);
  }

  createRoom(request: CreateRoomRequest): Promise<CreateRoomResponse> {
    return this.request(wireMethods.createRoom, request, CreateRoomResponse);
  }

  getAdminRoomLayout(
    request = new GetAdminRoomLayoutRequest()
  ): Promise<GetAdminRoomLayoutResponse> {
    return this.request(wireMethods.getAdminRoomLayout, request, GetAdminRoomLayoutResponse);
  }

  createAdminRoomGroup(
    request: CreateAdminRoomGroupRequest
  ): Promise<CreateAdminRoomGroupResponse> {
    return this.request(wireMethods.createAdminRoomGroup, request, CreateAdminRoomGroupResponse);
  }

  updateAdminRoomGroup(
    request: UpdateAdminRoomGroupRequest
  ): Promise<UpdateAdminRoomGroupResponse> {
    return this.request(wireMethods.updateAdminRoomGroup, request, UpdateAdminRoomGroupResponse);
  }

  deleteAdminRoomGroup(
    request: DeleteAdminRoomGroupRequest
  ): Promise<DeleteAdminRoomGroupResponse> {
    return this.request(wireMethods.deleteAdminRoomGroup, request, DeleteAdminRoomGroupResponse);
  }

  reorderAdminRoomGroups(
    request: ReorderAdminRoomGroupsRequest
  ): Promise<ReorderAdminRoomGroupsResponse> {
    return this.request(
      wireMethods.reorderAdminRoomGroups,
      request,
      ReorderAdminRoomGroupsResponse
    );
  }

  moveAdminRoomToGroup(
    request: MoveAdminRoomToGroupRequest
  ): Promise<MoveAdminRoomToGroupResponse> {
    return this.request(wireMethods.moveAdminRoomToGroup, request, MoveAdminRoomToGroupResponse);
  }

  reorderAdminRoomsInGroup(
    request: ReorderAdminRoomsInGroupRequest
  ): Promise<ReorderAdminRoomsInGroupResponse> {
    return this.request(
      wireMethods.reorderAdminRoomsInGroup,
      request,
      ReorderAdminRoomsInGroupResponse
    );
  }

  updateAdminRoom(request: UpdateAdminRoomRequest): Promise<UpdateAdminRoomResponse> {
    return this.request(wireMethods.updateAdminRoom, request, UpdateAdminRoomResponse);
  }

  archiveAdminRoom(request: ArchiveAdminRoomRequest): Promise<ArchiveAdminRoomResponse> {
    return this.request(wireMethods.archiveAdminRoom, request, ArchiveAdminRoomResponse);
  }

  unarchiveAdminRoom(request: UnarchiveAdminRoomRequest): Promise<UnarchiveAdminRoomResponse> {
    return this.request(wireMethods.unarchiveAdminRoom, request, UnarchiveAdminRoomResponse);
  }

  joinRoom(request: JoinRoomRequest): Promise<JoinRoomResponse> {
    return this.request(wireMethods.joinRoom, request, JoinRoomResponse);
  }

  leaveRoom(request: LeaveRoomRequest): Promise<LeaveRoomResponse> {
    return this.request(wireMethods.leaveRoom, request, LeaveRoomResponse);
  }

  joinGroup(request: JoinGroupRequest): Promise<JoinGroupResponse> {
    return this.request(wireMethods.joinGroup, request, JoinGroupResponse);
  }

  banRoomMember(request: BanRoomMemberRequest): Promise<BanRoomMemberResponse> {
    return this.request(wireMethods.banRoomMember, request, BanRoomMemberResponse);
  }

  listRoomBans(request = new ListRoomBansRequest()): Promise<ListRoomBansResponse> {
    return this.request(wireMethods.listRoomBans, request, ListRoomBansResponse);
  }

  unbanRoomMember(request: UnbanRoomMemberRequest): Promise<UnbanRoomMemberResponse> {
    return this.request(wireMethods.unbanRoomMember, request, UnbanRoomMemberResponse);
  }

  getRoomEvent(request: GetRoomEventRequest): Promise<GetRoomEventResponse> {
    return this.request(wireMethods.getRoomEvent, request, GetRoomEventResponse);
  }

  getRoomTimeline(request: GetRoomTimelineRequest): Promise<GetRoomTimelineResponse> {
    return this.request(wireMethods.getRoomTimeline, request, GetRoomTimelineResponse);
  }

  getRoomTimelineAfter(
    request: GetRoomTimelineAfterRequest
  ): Promise<GetRoomTimelineAfterResponse> {
    return this.request(wireMethods.getRoomTimelineAfter, request, GetRoomTimelineAfterResponse);
  }

  getRoomTimelineAround(
    request: GetRoomTimelineAroundRequest
  ): Promise<GetRoomTimelineAroundResponse> {
    return this.request(wireMethods.getRoomTimelineAround, request, GetRoomTimelineAroundResponse);
  }

  getThreadEvents(request: GetThreadEventsRequest): Promise<GetThreadEventsResponse> {
    return this.request(wireMethods.getThreadEvents, request, GetThreadEventsResponse);
  }

  getThreadEventsAround(
    request: GetThreadEventsAroundRequest
  ): Promise<GetThreadEventsAroundResponse> {
    return this.request(wireMethods.getThreadEventsAround, request, GetThreadEventsAroundResponse);
  }

  listMyFollowedThreads(
    request = new ListMyFollowedThreadsRequest()
  ): Promise<ListMyFollowedThreadsResponse> {
    return this.request(wireMethods.listMyFollowedThreads, request, ListMyFollowedThreadsResponse);
  }

  getLinkPreview(request: GetLinkPreviewRequest): Promise<GetLinkPreviewResponse> {
    return this.request(wireMethods.getLinkPreview, request, GetLinkPreviewResponse);
  }

  postMessage(request: PostMessageRequest): Promise<PostMessageResponse> {
    return this.request(wireMethods.postMessage, request, PostMessageResponse);
  }

  updateMessage(request: UpdateMessageRequest): Promise<UpdateMessageResponse> {
    return this.request(wireMethods.updateMessage, request, UpdateMessageResponse);
  }

  deleteMessage(request: DeleteMessageRequest): Promise<DeleteMessageResponse> {
    return this.request(wireMethods.deleteMessage, request, DeleteMessageResponse);
  }

  deleteAttachment(request: DeleteAttachmentRequest): Promise<DeleteAttachmentResponse> {
    return this.request(wireMethods.deleteAttachment, request, DeleteAttachmentResponse);
  }

  deleteLinkPreview(request: DeleteLinkPreviewRequest): Promise<DeleteLinkPreviewResponse> {
    return this.request(wireMethods.deleteLinkPreview, request, DeleteLinkPreviewResponse);
  }

  addReaction(request: AddReactionRequest): Promise<AddReactionResponse> {
    return this.request(wireMethods.addReaction, request, AddReactionResponse);
  }

  removeReaction(request: RemoveReactionRequest): Promise<RemoveReactionResponse> {
    return this.request(wireMethods.removeReaction, request, RemoveReactionResponse);
  }

  followThread(request: FollowThreadRequest): Promise<FollowThreadResponse> {
    return this.request(wireMethods.followThread, request, FollowThreadResponse);
  }

  unfollowThread(request: UnfollowThreadRequest): Promise<UnfollowThreadResponse> {
    return this.request(wireMethods.unfollowThread, request, UnfollowThreadResponse);
  }

  markRoomAsRead(request: MarkRoomAsReadRequest): Promise<MarkRoomAsReadResponse> {
    return this.request(wireMethods.markRoomAsRead, request, MarkRoomAsReadResponse);
  }

  markThreadAsRead(request: MarkThreadAsReadRequest): Promise<MarkThreadAsReadResponse> {
    return this.request(wireMethods.markThreadAsRead, request, MarkThreadAsReadResponse);
  }

  sendTypingIndicator(request: SendTypingIndicatorRequest): Promise<SendTypingIndicatorResponse> {
    return this.request(wireMethods.sendTypingIndicator, request, SendTypingIndicatorResponse);
  }

  updateMyPresence(request: UpdateMyPresenceRequest): Promise<UpdateMyPresenceResponse> {
    return this.request(wireMethods.updateMyPresence, request, UpdateMyPresenceResponse);
  }

  listActiveCalls(request = new ListActiveCallsRequest()): Promise<ListActiveCallsResponse> {
    return this.request(wireMethods.listActiveCalls, request, ListActiveCallsResponse);
  }

  getCallParticipants(request: GetCallParticipantsRequest): Promise<GetCallParticipantsResponse> {
    return this.request(wireMethods.getCallParticipants, request, GetCallParticipantsResponse);
  }

  joinVoiceCall(request: JoinVoiceCallRequest): Promise<JoinVoiceCallResponse> {
    return this.request(wireMethods.joinVoiceCall, request, JoinVoiceCallResponse);
  }

  leaveVoiceCall(request: LeaveVoiceCallRequest): Promise<LeaveVoiceCallResponse> {
    return this.request(wireMethods.leaveVoiceCall, request, LeaveVoiceCallResponse);
  }

  getVoiceCallToken(request: GetVoiceCallTokenRequest): Promise<GetVoiceCallTokenResponse> {
    return this.request(wireMethods.getVoiceCallToken, request, GetVoiceCallTokenResponse);
  }

  #handleOpen = (options: WireConnectOptions) => (): void => {
    this.#send(
      new ClientFrame({
        frameId: this.#nextFrameId(),
        kind: {
          case: 'hello',
          value: new ClientHello({
            protocolVersion: WIRE_PROTOCOL_VERSION,
            resumeAfter: options.resumeAfter ?? '',
            acceptedFeatures: options.acceptedFeatures ?? [],
            bearerToken: this.#token ?? ''
          })
        }
      })
    );
  };

  #handleMessage = (event: MessageEvent): void => {
    void this.#decodeFrame(event.data)
      .then((frame) => this.#handleServerFrame(frame))
      .catch((error: unknown) => {
        this.#emitError(new WireProtocolError(errorMessage(error)));
      });
  };

  #handleServerFrame(frame: ServerFrame): void {
    switch (frame.kind.case) {
      case 'hello':
        this.status = 'connected';
        this.#helloResolve?.(frame.kind.value);
        this.#helloResolve = null;
        this.#helloReject = null;
        return;
      case 'response':
        this.#handleResponse(frame.kind.value);
        return;
      case 'event':
        this.#handleStreamEvent(frame.kind.value);
        return;
      case 'error':
        this.#handleWireError(frame.kind.value);
        return;
      default:
        this.#emitError(new WireProtocolError('wire server frame kind is missing'));
    }
  }

  #handleResponse(response: Response): void {
    const pending = this.#pendingRequests.get(response.requestId);
    if (!pending) return;

    this.#pendingRequests.delete(response.requestId);
    try {
      pending.resolve(pending.responseType.fromBinary(response.body));
    } catch (error: unknown) {
      pending.reject(new WireProtocolError(errorMessage(error)));
    }
  }

  #handleStreamEvent(event: StreamEvent): void {
    if (event.deliveryCursor) {
      this.lastDeliveryCursor = event.deliveryCursor;
    }
    for (const listener of this.#eventListeners) listener(event);
  }

  #handleWireError(error: WireError): void {
    const protocolError = new WireProtocolError(error.message, error);
    if (error.requestId) {
      const pending = this.#pendingRequests.get(error.requestId);
      if (pending) {
        this.#pendingRequests.delete(error.requestId);
        pending.reject(protocolError);
        return;
      }
    }
    this.#helloReject?.(protocolError);
    this.#emitError(protocolError);
  }

  #handleClose = (): void => {
    this.status = 'closed';
    this.#helloReject?.(new WireProtocolError('wire connection closed before hello'));
    this.#helloResolve = null;
    this.#helloReject = null;
    this.#rejectAll(new WireProtocolError('wire connection closed'));
    this.#detachSocket();
    this.#emitClose();
  };

  #handleSocketError = (): void => {
    const error = new WireProtocolError('wire connection failed');
    this.#helloReject?.(error);
    this.#emitError(error);
  };

  async #decodeFrame(data: unknown): Promise<ServerFrame> {
    return ServerFrame.fromBinary(await binaryData(data));
  }

  #send(frame: ClientFrame): void {
    if (!this.#socket || this.#socket.readyState !== WEBSOCKET_OPEN) {
      throw new WireProtocolError('wire socket is not open');
    }
    this.#socket.send(frame.toBinary());
  }

  #rejectAll(error: WireProtocolError): void {
    for (const pending of this.#pendingRequests.values()) pending.reject(error);
    this.#pendingRequests.clear();
  }

  #emitError(error: WireProtocolError): void {
    for (const listener of this.#errorListeners) listener(error);
  }

  #emitClose(): void {
    for (const listener of this.#closeListeners) listener();
  }

  #detachSocket(): void {
    this.#socket?.removeEventListener('message', this.#handleMessage);
    this.#socket?.removeEventListener('close', this.#handleClose);
    this.#socket?.removeEventListener('error', this.#handleSocketError);
    this.#socket = null;
    this.#connectPromise = null;
  }

  #nextRequestId(): string {
    this.#requestSeq += 1;
    return `wire-request-${this.#requestSeq}`;
  }

  #nextFrameId(): string {
    this.#frameSeq += 1;
    return `wire-frame-${this.#frameSeq}`;
  }
}

async function binaryData(data: unknown): Promise<Uint8Array> {
  if (data instanceof Uint8Array) return data;
  if (data instanceof ArrayBuffer) return new Uint8Array(data);
  if (ArrayBuffer.isView(data)) {
    return new Uint8Array(data.buffer, data.byteOffset, data.byteLength);
  }
  if (typeof Blob !== 'undefined' && data instanceof Blob) {
    return new Uint8Array(await data.arrayBuffer());
  }
  throw new WireProtocolError('wire frame data must be binary');
}

function errorMessage(error: unknown): string {
  return error instanceof Error ? error.message : String(error);
}

function protobufBytes(message: BinaryMessage): Uint8Array<ArrayBuffer> {
  return new Uint8Array(message.toBinary());
}
