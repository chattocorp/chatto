import { expect, type APIRequestContext, type APIResponse, type Page } from '@playwright/test';

type ConnectRequest = Record<string, unknown>;
type ConnectClient = Page | APIRequestContext;
const DEFAULT_POLL_TIMEOUT = process.env.CI ? 10_000 : 3_000;

export type E2EPermissionDecision =
  'PERMISSION_DECISION_ALLOW' | 'PERMISSION_DECISION_DENY' | 'PERMISSION_DECISION_NONE';

export type E2EPermissionScopeKind =
  'PERMISSION_SCOPE_KIND_SERVER' | 'PERMISSION_SCOPE_KIND_GROUP' | 'PERMISSION_SCOPE_KIND_ROOM';

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

export type E2ENotificationMode =
  'UNSPECIFIED' | 'OFF' | 'UNREAD_BADGE' | 'IN_APP_NOTIFICATION' | 'PUSH_NOTIFICATION';

type E2ENotificationPolicyShape<Value> = {
  directMessages: Value;
  roomMessages: Value;
  directMentions: Value;
  replies: Value;
  roleMentions: Value;
  hereMentions: Value;
  allMentions: Value;
  followedThreads: Value;
  followedRooms: Value;
  reactions: Value;
};

export interface E2ENotificationPolicy {
  overrides: E2ENotificationPolicyShape<E2ENotificationMode | null>;
  effective: E2ENotificationPolicyShape<E2ENotificationMode>;
}

export type E2ENotificationPolicyScope =
  { server: Record<string, never> } | { roomGroupId: string } | { roomId: string };

interface NotificationPolicyResponse {
  policy?: {
    overrides?: Partial<E2ENotificationPolicyShape<unknown>>;
    effective?: Partial<E2ENotificationPolicyShape<unknown>>;
  };
}

interface ScopedNotificationPolicyResponse {
  policy?: {
    policy?: NotificationPolicyResponse['policy'];
  };
}

interface ListRoomsResponse {
  rooms?: Array<{
    room?: { id?: string; name?: string };
    viewerState?: { hasUnread?: boolean };
  }>;
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
  message?: { id?: string };
}

interface GetMessageResponse {
  message?: { id?: string };
}

interface ViewerResponse {
  viewerState?: { hasUnreadRooms?: boolean };
}

interface GetUserResponse {
  user?: { profile?: { user?: { id?: string } } };
}

const notificationModeByNumber: Record<number, E2ENotificationMode> = {
  0: 'UNSPECIFIED',
  1: 'OFF',
  2: 'IN_APP_NOTIFICATION',
  3: 'PUSH_NOTIFICATION',
  4: 'UNREAD_BADGE'
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
    const data = await connectPost<ViewerResponse>(page, 'chatto.api.v1.ViewerService/GetViewer');
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
    const response = await connectPostResponse(page, 'chatto.api.v1.UserService/GetUser', {
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

/** Wait until one message is observable through the viewer's room timeline projection. */
export async function waitForMessageViaConnect(
  page: Page,
  roomId: string,
  eventId: string,
  timeout = DEFAULT_POLL_TIMEOUT
): Promise<void> {
  await expect(async () => {
    const data = await connectPost<GetMessageResponse>(
      page,
      'chatto.api.v1.MessageService/GetMessage',
      { roomId, eventId }
    );
    expect(data.message?.id).toBe(eventId);
  }).toPass({ timeout, intervals: [100, 250, 500, 1000] });
}

/** Establish the Message Read Cursor through the room's current root event. */
export async function markRoomAsReadViaConnect(page: Page, roomId: string): Promise<void> {
  await connectPost(page, 'chatto.api.v1.RoomService/MarkRoomAsRead', { roomId });
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
  const eventId = data.message?.id;
  if (!eventId) {
    throw new Error('CreateMessage did not return a message id');
  }
  return eventId;
}

export async function getNotificationPolicy(
  page: Page,
  roomId?: string
): Promise<E2ENotificationPolicy> {
  const data = await connectPost<NotificationPolicyResponse>(
    page,
    'chatto.api.v1.NotificationService/GetNotificationPolicy',
    roomId ? { roomId } : {}
  );
  return normalizeNotificationPolicy(data);
}

export async function updateNotificationPolicy(
  page: Page,
  patch: Partial<E2ENotificationPolicyShape<E2ENotificationMode | null>>,
  roomId?: string
): Promise<E2ENotificationPolicy> {
  const fields = Object.keys(patch) as Array<keyof typeof patch>;
  const overrides = Object.fromEntries(
    fields.flatMap((field) => {
      const mode = patch[field];
      return mode === null || mode === undefined
        ? []
        : [[field, `NOTIFICATION_DELIVERY_MODE_${mode}`]];
    })
  );
  const data = await connectPost<NotificationPolicyResponse>(
    page,
    'chatto.api.v1.NotificationService/UpdateNotificationPolicy',
    {
      ...(roomId ? { roomId } : {}),
      overrides,
      updateMask: fields.join(',')
    }
  );
  return normalizeNotificationPolicy(data);
}

export async function getScopedNotificationPolicy(
  page: Page,
  scope: E2ENotificationPolicyScope
): Promise<E2ENotificationPolicy> {
  const data = await connectPost<ScopedNotificationPolicyResponse>(
    page,
    'chatto.api.v1.NotificationPolicyService/GetNotificationPolicy',
    { scope }
  );
  return normalizeNotificationPolicy({ policy: data.policy?.policy });
}

export async function updateScopedNotificationPolicy(
  page: Page,
  scope: E2ENotificationPolicyScope,
  patch: Partial<E2ENotificationPolicyShape<E2ENotificationMode | null>>
): Promise<E2ENotificationPolicy> {
  const fields = Object.keys(patch) as Array<keyof typeof patch>;
  const overrides = Object.fromEntries(
    fields.flatMap((field) => {
      const mode = patch[field];
      return mode === null || mode === undefined
        ? []
        : [[field, `NOTIFICATION_DELIVERY_MODE_${mode}`]];
    })
  );
  const data = await connectPost<ScopedNotificationPolicyResponse>(
    page,
    'chatto.api.v1.NotificationPolicyService/UpdateNotificationPolicy',
    { scope, overrides, updateMask: fields.join(',') }
  );
  return normalizeNotificationPolicy({ policy: data.policy?.policy });
}

function normalizeNotificationPolicy(data: NotificationPolicyResponse): E2ENotificationPolicy {
  const overrides = data.policy?.overrides;
  const effective = data.policy?.effective;
  return {
    overrides: {
      directMessages: normalizeNotificationOverride(overrides?.directMessages),
      roomMessages: normalizeNotificationOverride(overrides?.roomMessages),
      directMentions: normalizeNotificationOverride(overrides?.directMentions),
      replies: normalizeNotificationOverride(overrides?.replies),
      roleMentions: normalizeNotificationOverride(overrides?.roleMentions),
      hereMentions: normalizeNotificationOverride(overrides?.hereMentions),
      allMentions: normalizeNotificationOverride(overrides?.allMentions),
      followedThreads: normalizeNotificationOverride(overrides?.followedThreads),
      followedRooms: normalizeNotificationOverride(overrides?.followedRooms),
      reactions: normalizeNotificationOverride(overrides?.reactions)
    },
    effective: {
      directMessages: normalizeNotificationMode(effective?.directMessages),
      roomMessages: normalizeNotificationMode(effective?.roomMessages),
      directMentions: normalizeNotificationMode(effective?.directMentions),
      replies: normalizeNotificationMode(effective?.replies),
      roleMentions: normalizeNotificationMode(effective?.roleMentions),
      hereMentions: normalizeNotificationMode(effective?.hereMentions),
      allMentions: normalizeNotificationMode(effective?.allMentions),
      followedThreads: normalizeNotificationMode(effective?.followedThreads),
      followedRooms: normalizeNotificationMode(effective?.followedRooms),
      reactions: normalizeNotificationMode(effective?.reactions)
    }
  };
}

function normalizeNotificationOverride(value: unknown): E2ENotificationMode | null {
  return value === undefined || value === null ? null : normalizeNotificationMode(value);
}

function normalizeNotificationMode(value: unknown): E2ENotificationMode {
  if (value === undefined || value === null) return 'UNSPECIFIED';
  if (typeof value === 'number' && Number.isInteger(value)) {
    const mode = notificationModeByNumber[value];
    if (mode) return mode;
  }

  if (typeof value === 'string') {
    const raw = value.replace(/^NOTIFICATION_DELIVERY_MODE_/, '');
    const compact = (
      raw === 'SILENT' ? 'IN_APP_NOTIFICATION' : raw === 'ALERT' ? 'PUSH_NOTIFICATION' : raw
    ) as E2ENotificationMode;
    if (Object.values(notificationModeByNumber).includes(compact)) return compact;
  }

  throw new Error(`Unexpected notification delivery mode: ${String(value)}`);
}
