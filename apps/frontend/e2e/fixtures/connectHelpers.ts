import { expect, type APIRequestContext, type APIResponse, type Page } from '@playwright/test';

type ConnectRequest = Record<string, unknown>;
type ConnectClient = Page | APIRequestContext;
const DEFAULT_POLL_TIMEOUT = process.env.CI ? 10_000 : 3_000;

export type E2EPermissionDecision =
  | 'PERMISSION_DECISION_ALLOW'
  | 'PERMISSION_DECISION_DENY'
  | 'PERMISSION_DECISION_NONE';

export type E2EPermissionScopeKind =
  | 'PERMISSION_SCOPE_KIND_SERVER'
  | 'PERMISSION_SCOPE_KIND_GROUP'
  | 'PERMISSION_SCOPE_KIND_ROOM';

export interface E2EPermissionDecisionUpdateResponse {
  decision?: {
    permission?: string;
    decision?: E2EPermissionDecision;
    scope?: {
      kind?: E2EPermissionScopeKind;
      id?: string;
    };
  };
}

export interface E2EAdminRole {
  role?: {
    name?: string;
    displayName?: string;
    description?: string;
    isSystem?: boolean;
    position?: number;
    pingable?: boolean;
  };
  permissions?: string[];
  permissionDenials?: string[];
}

export interface E2EServerRole {
  name?: string;
  displayName?: string;
  description?: string;
  isSystem?: boolean;
  position?: number;
  pingable?: boolean;
  permissions: string[];
  permissionDenials: string[];
}

export type E2ENotificationLevel = 'DEFAULT' | 'MUTED' | 'NORMAL' | 'ALL_MESSAGES';

export interface E2ENotificationPreference {
  level: E2ENotificationLevel;
  effectiveLevel: E2ENotificationLevel;
}

interface NotificationPreferenceResponse {
  level?: unknown;
  effectiveLevel?: unknown;
}

interface ListRoomsResponse {
  rooms?: Array<{ room?: { id?: string; name?: string }; viewerState?: { hasUnread?: boolean } }>;
}

interface ListRoomGroupsResponse {
  groups?: Array<{ id?: string; name?: string }>;
}

interface CreateRoomResponse {
  room?: { id?: string; name?: string };
}

interface JoinRoomResponse {
  room?: { id?: string };
}

interface CreateMessageResponse {
  event?: { id?: string };
}

interface ViewerResponse {
  viewerState?: { hasUnreadRooms?: boolean };
}

interface GetUserResponse {
  user?: { id?: string };
}

const notificationLevelToProtoName: Record<E2ENotificationLevel, string> = {
  DEFAULT: 'NOTIFICATION_LEVEL_DEFAULT',
  MUTED: 'NOTIFICATION_LEVEL_MUTED',
  NORMAL: 'NOTIFICATION_LEVEL_NORMAL',
  ALL_MESSAGES: 'NOTIFICATION_LEVEL_ALL_MESSAGES'
};

const notificationLevelByNumber: Record<number, E2ENotificationLevel> = {
  1: 'DEFAULT',
  2: 'MUTED',
  3: 'NORMAL',
  4: 'ALL_MESSAGES'
};

export async function connectPost<T>(
  client: ConnectClient,
  procedure: string,
  data: ConnectRequest = {}
): Promise<T> {
  const response = await connectPostResponse(client, procedure, data);

  if (!response.ok()) {
    throw new Error(`${procedure} failed: ${response.status()} ${await response.text()}`);
  }

  return (await response.json()) as T;
}

export async function connectPostResponse(
  client: ConnectClient,
  procedure: string,
  data: ConnectRequest = {}
): Promise<APIResponse> {
  return requestContext(client).post(`/api/connect/${procedure}`, {
    headers: {
      'Content-Type': 'application/json',
      'Connect-Protocol-Version': '1'
    },
    data
  });
}

function requestContext(client: ConnectClient): APIRequestContext {
  return 'request' in client ? client.request : client;
}

export function unwrapAdminRole(role: E2EAdminRole | undefined): E2EServerRole | undefined {
  if (!role?.role) return undefined;
  return {
    ...role.role,
    permissions: [...(role.permissions ?? [])],
    permissionDenials: [...(role.permissionDenials ?? [])]
  };
}

export function expectPermissionDecisionUpdate(
  data: E2EPermissionDecisionUpdateResponse,
  expected: {
    permission: string;
    decision: E2EPermissionDecision;
    scope?: { kind: E2EPermissionScopeKind; id?: string };
  }
): void {
  expect(data.decision).toEqual(
    expect.objectContaining({
      permission: expected.permission,
      decision: expected.decision,
      ...(expected.scope
        ? {
            scope: expect.objectContaining(expected.scope)
          }
        : {})
    })
  );
}

export async function getRoomIdByNameViaConnect(
  client: ConnectClient,
  roomName: string
): Promise<string> {
  const data = await connectPost<ListRoomsResponse>(
    client,
    'chatto.api.v1.RoomDirectoryService/ListRooms'
  );
  const room = data.rooms?.find((entry) => entry.room?.name === roomName)?.room;
  if (!room?.id) {
    throw new Error(`Room "${roomName}" not found`);
  }
  return room.id;
}

async function getRoomUnreadViaConnect(client: ConnectClient, roomId: string): Promise<boolean> {
  const data = await connectPost<ListRoomsResponse>(
    client,
    'chatto.api.v1.RoomDirectoryService/ListRooms'
  );
  const room = data.rooms?.find((entry) => entry.room?.id === roomId);
  if (!room) {
    throw new Error(`Room "${roomId}" not found`);
  }
  return room.viewerState?.hasUnread ?? false;
}

export async function waitForServerUnreadViaConnect(
  page: Page,
  expected: boolean,
  timeout = DEFAULT_POLL_TIMEOUT
): Promise<void> {
  await expect(async () => {
    const data = await connectPost<ViewerResponse>(
      page,
      'chatto.api.v1.ViewerService/GetViewer'
    );
    expect(data.viewerState?.hasUnreadRooms ?? false).toBe(expected);
  }).toPass({ timeout, intervals: [100, 250, 500, 1000] });
}

export async function waitForRoomUnreadViaConnect(
  page: Page,
  roomId: string,
  expected: boolean,
  timeout = DEFAULT_POLL_TIMEOUT
): Promise<void> {
  await expect(async () => {
    expect(await getRoomUnreadViaConnect(page, roomId)).toBe(expected);
  }).toPass({ timeout, intervals: [100, 250, 500, 1000] });
}

export async function waitForRoomReadViaConnect(
  page: Page,
  roomId: string,
  timeout = DEFAULT_POLL_TIMEOUT
): Promise<void> {
  await waitForRoomUnreadViaConnect(page, roomId, false, timeout);
}

export async function waitForUserDeletedViaConnect(
  page: Page,
  userId: string,
  timeout = DEFAULT_POLL_TIMEOUT
): Promise<void> {
  await expect(async () => {
    const response = await connectPostResponse(page, 'chatto.api.v1.UserDirectoryService/GetUser', {
      userId
    });
    if (response.ok()) {
      const data = (await response.json()) as GetUserResponse;
      expect(data.user).toBeFalsy();
      return;
    }

    const body = await response.text();
    expect(response.status(), body).toBe(404);
  }).toPass({ timeout, intervals: [100, 250, 500, 1000] });
}

export async function getDefaultRoomGroupIdViaConnect(client: ConnectClient): Promise<string> {
  const data = await connectPost<ListRoomGroupsResponse>(
    client,
    'chatto.api.v1.RoomDirectoryService/ListRoomGroups'
  );
  const groupId = data.groups?.[0]?.id;
  if (!groupId) {
    throw new Error(`No room group available for e2e room creation: ${JSON.stringify(data)}`);
  }
  return groupId;
}

export async function createRoomViaConnect(
  client: ConnectClient,
  name: string,
  groupId: string,
  description = ''
): Promise<string> {
  const data = await connectPost<CreateRoomResponse>(
    client,
    'chatto.api.v1.RoomService/CreateRoom',
    {
      name,
      groupId,
      description
    }
  );
  const roomId = data.room?.id;
  if (!roomId) {
    throw new Error('CreateRoom did not return a room id');
  }
  return roomId;
}

export async function joinRoomViaConnect(client: ConnectClient, roomId: string): Promise<string> {
  const data = await connectPost<JoinRoomResponse>(client, 'chatto.api.v1.RoomService/JoinRoom', {
    roomId
  });
  const joinedRoomId = data.room?.id;
  if (joinedRoomId !== roomId) {
    throw new Error(`JoinRoom returned ${joinedRoomId ?? '<none>'}, want ${roomId}`);
  }
  return joinedRoomId;
}

export async function getIdsFromUrlViaConnect(
  page: Page
): Promise<{ spaceId: string; roomId: string }> {
  const match = page.url().match(/\/chat\/-\/([^/]+)/);
  if (!match) throw new Error(`Could not extract roomId from URL: ${page.url()}`);
  return { spaceId: 'server', roomId: match[1] };
}

export async function postMessageViaConnect(
  page: Page,
  roomId: string,
  body: string
): Promise<string> {
  return postMessageWithConnectInput(page, { roomId, body });
}

export async function postMessagesViaConnect(
  page: Page,
  roomId: string,
  messages: string[]
): Promise<void> {
  for (const body of messages) {
    await postMessageViaConnect(page, roomId, body);
  }
}

export async function postReplyViaConnect(
  page: Page,
  roomId: string,
  body: string,
  inReplyTo: string
): Promise<string> {
  return postMessageWithConnectInput(page, { roomId, body, inReplyTo });
}

export async function postThreadReplyViaConnect(
  page: Page,
  roomId: string,
  body: string,
  threadRootEventId: string,
  inReplyTo?: string
): Promise<string> {
  return postMessageWithConnectInput(page, {
    roomId,
    body,
    threadRootEventId,
    ...(inReplyTo ? { inReplyTo } : {})
  });
}

export async function postThreadReplyWithEchoViaConnect(
  page: Page,
  roomId: string,
  body: string,
  threadRootEventId: string,
  inReplyTo: string
): Promise<string> {
  return postMessageWithConnectInput(page, {
    roomId,
    body,
    threadRootEventId,
    inReplyTo,
    alsoSendToChannel: true
  });
}

async function postMessageWithConnectInput(page: Page, input: ConnectRequest): Promise<string> {
  const data = await connectPost<CreateMessageResponse>(
    page,
    'chatto.api.v1.MessageService/CreateMessage',
    input
  );
  const eventId = data.event?.id;
  if (!eventId) {
    throw new Error('CreateMessage did not return an event id');
  }
  return eventId;
}

export async function getServerNotificationPreference(
  page: Page
): Promise<E2ENotificationPreference> {
  const data = await connectPost<NotificationPreferenceResponse>(
    page,
    'chatto.api.v1.NotificationPreferencesService/GetServerNotificationPreference'
  );
  return normalizeNotificationPreference(data);
}

export async function updateServerNotificationPreference(
  page: Page,
  level: E2ENotificationLevel
): Promise<E2ENotificationPreference> {
  const data = await connectPost<NotificationPreferenceResponse>(
    page,
    'chatto.api.v1.NotificationPreferencesService/UpdateServerNotificationPreference',
    { level: notificationLevelToProtoName[level] }
  );
  return normalizeNotificationPreference(data);
}

export async function getRoomNotificationPreference(
  page: Page,
  roomId: string
): Promise<E2ENotificationPreference> {
  const data = await connectPost<NotificationPreferenceResponse>(
    page,
    'chatto.api.v1.NotificationPreferencesService/GetRoomNotificationPreference',
    { roomId }
  );
  return normalizeNotificationPreference(data);
}

export async function updateRoomNotificationPreference(
  page: Page,
  roomId: string,
  level: E2ENotificationLevel
): Promise<E2ENotificationPreference> {
  const data = await connectPost<NotificationPreferenceResponse>(
    page,
    'chatto.api.v1.NotificationPreferencesService/UpdateRoomNotificationPreference',
    { roomId, level: notificationLevelToProtoName[level] }
  );
  return normalizeNotificationPreference(data);
}

function normalizeNotificationPreference(
  data: NotificationPreferenceResponse
): E2ENotificationPreference {
  return {
    level: normalizeNotificationLevel(data.level),
    effectiveLevel: normalizeNotificationLevel(data.effectiveLevel)
  };
}

function normalizeNotificationLevel(value: unknown): E2ENotificationLevel {
  if (typeof value === 'number' && Number.isInteger(value)) {
    const level = notificationLevelByNumber[value];
    if (level) return level;
  }

  if (typeof value === 'string') {
    const compact = value.replace(/^NOTIFICATION_LEVEL_/, '');
    if (isNotificationLevel(compact)) return compact;
  }

  throw new Error(`Unexpected notification level: ${String(value)}`);
}

function isNotificationLevel(value: string): value is E2ENotificationLevel {
  return value === 'DEFAULT' || value === 'MUTED' || value === 'NORMAL' || value === 'ALL_MESSAGES';
}
