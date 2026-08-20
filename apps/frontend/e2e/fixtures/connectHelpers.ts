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

export type E2ENotificationPreferenceCategory =
  | 'DIRECT_MESSAGE'
  | 'DIRECT_MENTION'
  | 'REPLY'
  | 'ROLE_MENTION'
  | 'HERE'
  | 'ALL'
  | 'FOLLOWED_THREAD'
  | 'FOLLOWED_ROOM'
  | 'REACTION';

export type E2ENotificationMode = 'UNSPECIFIED' | 'OFF' | 'SILENT' | 'ALERT';

export interface E2ENotificationPolicyPreference {
  category: E2ENotificationPreferenceCategory;
  override: E2ENotificationMode;
  effective: E2ENotificationMode;
}

interface NotificationPolicyResponse {
  preferences?: Array<{
    category?: unknown;
    override?: unknown;
    effective?: unknown;
  }>;
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
  event?: { id?: string };
}

interface ViewerResponse {
  viewerState?: { hasUnreadRooms?: boolean };
}

interface GetUserResponse {
  user?: { profile?: { user?: { id?: string } } };
}

const notificationKindByNumber: Record<number, E2ENotificationPreferenceCategory> = {
  1: 'DIRECT_MESSAGE',
  2: 'DIRECT_MENTION',
  3: 'REPLY',
  4: 'ROLE_MENTION',
  5: 'HERE',
  6: 'ALL',
  7: 'FOLLOWED_THREAD',
  8: 'FOLLOWED_ROOM',
  9: 'REACTION'
};

const notificationModeByNumber: Record<number, E2ENotificationMode> = {
  0: 'UNSPECIFIED',
  1: 'OFF',
  2: 'SILENT',
  3: 'ALERT'
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
): Promise<E2ENotificationPolicyPreference[]> {
  const data = await connectPost<NotificationPolicyResponse>(
    page,
    'chatto.api.v1.NotificationService/GetNotificationPolicy',
    roomId ? { roomId } : {}
  );
  return normalizeNotificationPolicy(data);
}

export async function setNotificationPolicyPreference(
  page: Page,
  category: E2ENotificationPreferenceCategory,
  mode: E2ENotificationMode,
  roomId?: string
): Promise<E2ENotificationPolicyPreference[]> {
  const data = await connectPost<NotificationPolicyResponse>(
    page,
    'chatto.api.v1.NotificationService/SetNotificationPolicyPreference',
    {
      ...(roomId ? { roomId } : {}),
      category: `NOTIFICATION_PREFERENCE_CATEGORY_${category}`,
      ...(mode === 'UNSPECIFIED'
        ? {}
        : { override: `NOTIFICATION_DELIVERY_MODE_${mode}` })
    }
  );
  return normalizeNotificationPolicy(data);
}

function normalizeNotificationPolicy(
  data: NotificationPolicyResponse
): E2ENotificationPolicyPreference[] {
  return (data.preferences ?? []).map((preference) => ({
    category: normalizeNotificationPreferenceCategory(preference.category),
    override: normalizeNotificationMode(preference.override),
    effective: normalizeNotificationMode(preference.effective)
  }));
}

function normalizeNotificationPreferenceCategory(value: unknown): E2ENotificationPreferenceCategory {
  if (typeof value === 'number' && Number.isInteger(value)) {
    const kind = notificationKindByNumber[value];
    if (kind) return kind;
  }

  if (typeof value === 'string') {
    const compact = value.replace(/^NOTIFICATION_PREFERENCE_CATEGORY_/, '') as E2ENotificationPreferenceCategory;
    if (Object.values(notificationKindByNumber).includes(compact)) return compact;
  }

  throw new Error(`Unexpected notification policy category: ${String(value)}`);
}

function normalizeNotificationMode(value: unknown): E2ENotificationMode {
  if (value === undefined || value === null) return 'UNSPECIFIED';
  if (typeof value === 'number' && Number.isInteger(value)) {
    const mode = notificationModeByNumber[value];
    if (mode) return mode;
  }

  if (typeof value === 'string') {
    const compact = value.replace(
      /^NOTIFICATION_DELIVERY_MODE_/,
      ''
    ) as E2ENotificationMode;
    if (Object.values(notificationModeByNumber).includes(compact)) return compact;
  }

  throw new Error(`Unexpected notification delivery mode: ${String(value)}`);
}
