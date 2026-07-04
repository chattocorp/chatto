import type { Page } from '@playwright/test';
import type { TestInfo } from '@playwright/test';
import { createClient } from '@connectrpc/connect';
import { createConnectTransport } from '@connectrpc/connect-web';
import { readFile } from 'fs/promises';
import { sha256 } from 'js-sha256';
import { AssetUploadService } from '@chatto/api-types/api/v1/asset_uploads_connect';
import { MessageService } from '@chatto/api-types/api/v1/messages_connect';
import { RoomDirectoryService } from '@chatto/api-types/api/v1/room_directory_connect';
import { RoomService } from '@chatto/api-types/api/v1/rooms_connect';
import { AdminServerService } from '@chatto/api-types/admin/v1/server_connect';
import { ServerDiscoveryService } from '@chatto/api-types/chatto/discovery/v1/server_connect';
import { ViewerService } from '@chatto/api-types/api/v1/viewer_connect';
import { startServer, stopServer, type ServerInfo } from './server';

function connectBaseUrl(remoteBaseURL: string): string {
  return new URL('/api/connect', remoteBaseURL).toString();
}

function authHeaders(token: string) {
  return { Authorization: `Bearer ${token}` };
}

function messageClient(remoteBaseURL: string) {
  return createClient(
    MessageService,
    createConnectTransport({
      baseUrl: connectBaseUrl(remoteBaseURL),
      useBinaryFormat: true
    })
  );
}

function assetUploadClient(remoteBaseURL: string) {
  return createClient(
    AssetUploadService,
    createConnectTransport({
      baseUrl: connectBaseUrl(remoteBaseURL),
      useBinaryFormat: true
    })
  );
}

function roomClient(remoteBaseURL: string) {
  return createClient(
    RoomService,
    createConnectTransport({
      baseUrl: connectBaseUrl(remoteBaseURL),
      useBinaryFormat: true
    })
  );
}

function roomDirectoryClient(remoteBaseURL: string) {
  return createClient(
    RoomDirectoryService,
    createConnectTransport({
      baseUrl: connectBaseUrl(remoteBaseURL),
      useBinaryFormat: true
    })
  );
}

function serverDiscoveryClient(remoteBaseURL: string) {
  return createClient(
    ServerDiscoveryService,
    createConnectTransport({
      baseUrl: connectBaseUrl(remoteBaseURL),
      useBinaryFormat: true
    })
  );
}

function adminServerClient(remoteBaseURL: string) {
  return createClient(
    AdminServerService,
    createConnectTransport({
      baseUrl: connectBaseUrl(remoteBaseURL),
      useBinaryFormat: true
    })
  );
}

function viewerClient(remoteBaseURL: string) {
  return createClient(
    ViewerService,
    createConnectTransport({
      baseUrl: connectBaseUrl(remoteBaseURL),
      useBinaryFormat: true
    })
  );
}

function postedEventId(
  response: Awaited<ReturnType<ReturnType<typeof messageClient>['createMessage']>>
) {
  const message = response.message;
  if (!message?.id) {
    throw new Error(`CreateMessage did not return a message: ${JSON.stringify(response.toJson())}`);
  }
  return message.id;
}

/**
 * Starts a second Chatto server for multi-instance tests.
 * Uses parallelIndex + 5 to avoid port collisions with the primary server.
 */
export async function startSecondServer(testInfo: TestInfo): Promise<ServerInfo> {
  // Create a modified testInfo-like object with offset parallelIndex
  // to get a different port range from the primary server
  const modifiedTestInfo = {
    ...testInfo,
    parallelIndex: testInfo.parallelIndex + 5
  } as TestInfo;

  return startServer(modifiedTestInfo);
}

/**
 * Stops a second server and cleans up.
 */
export async function stopSecondServer(server: ServerInfo, testInfo: TestInfo): Promise<void> {
  const modifiedTestInfo = {
    ...testInfo,
    parallelIndex: testInfo.parallelIndex + 5
  } as TestInfo;

  await stopServer(server, modifiedTestInfo);
}

/**
 * Creates a user on a remote server and returns the auth token.
 * This simulates what AddInstanceModal does: register, then login to get a bearer token.
 */
export async function createUserOnRemote(
  remoteBaseURL: string,
  login: string,
  password: string
): Promise<{ token: string; userId: string }> {
  // Create user via the test-only endpoint (build-tagged; not in production
  // binaries). The production create-user mutation was removed for security
  // — see #175 — so e2e tests use this build-gated path instead.
  const createResponse = await fetch(`${remoteBaseURL}/auth/test/create-user`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({
      login,
      displayName: `User ${login}`,
      password
    })
  });

  if (!createResponse.ok) {
    throw new Error(`Failed to create user on remote: ${await createResponse.text()}`);
  }

  const createData = await createResponse.json();
  const userId = createData.id;
  if (!userId) {
    throw new Error(
      `No userId returned from remote test/create-user: ${JSON.stringify(createData)}`
    );
  }

  // Login to get bearer token
  const loginResponse = await fetch(`${remoteBaseURL}/auth/login`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ login, password })
  });

  if (!loginResponse.ok) {
    throw new Error(`Failed to login on remote: ${await loginResponse.text()}`);
  }

  const loginData = await loginResponse.json();
  if (!loginData.token) {
    throw new Error(`No token returned from remote login: ${JSON.stringify(loginData)}`);
  }

  // Join the bootstrap default rooms (announcements + general) on the remote.
  // Most cross-server tests assume `# general` is in scope, so grant those
  // room memberships once as part of creating the remote user.
  await joinDefaultRoomsOnRemote(remoteBaseURL, loginData.token);

  return { token: loginData.token, userId };
}

/**
 * Returns the remote server's legacy scope ID. Multi-instance tests reuse the
 * bootstrap server instead of minting a per-test container. The `_serverName`
 * arg is ignored for backwards compatibility with existing call sites.
 */
export async function getPrimaryServerScopeOnRemote(
  remoteBaseURL: string,
  _token: string,
  _serverName: string
): Promise<string> {
  // Sanity-check that the remote is reachable; the actual ID is the
  // kind discriminator constant (post-ADR-030).
  await serverDiscoveryClient(remoteBaseURL).getServer({});
  return 'server';
}

/**
 * Join the bootstrap default rooms (announcements + general) for a remote user
 * so cross-server tests that land directly in `# general` find a real room
 * membership instead of an empty-sidebar guest view.
 */
export async function joinDefaultRoomsOnRemote(
  remoteBaseURL: string,
  token: string,
  _spaceId?: string
): Promise<void> {
  const roomsData = await roomDirectoryClient(remoteBaseURL).listRooms(
    {},
    { headers: authHeaders(token) }
  );
  const defaults = new Set(['general', 'announcements']);
  const targets = roomsData.rooms.filter((entry) => {
    const name = entry.room?.name;
    return name ? defaults.has(name) : false;
  });
  for (const room of targets) {
    if (room.room?.id) {
      await roomClient(remoteBaseURL).joinRoom(
        { roomId: room.room.id },
        { headers: authHeaders(token) }
      );
    }
  }
}

/**
 * Posts a message in a room on a remote server. Returns the new event ID.
 */
export async function postMessageOnRemote(
  remoteBaseURL: string,
  token: string,
  roomId: string,
  body: string
): Promise<string> {
  const response = await messageClient(remoteBaseURL).createMessage(
    { roomId, body },
    { headers: authHeaders(token) }
  );
  return postedEventId(response);
}

/**
 * Posts a message with one attachment in a room on a remote server. Returns
 * the new event ID and the stable attachment URL emitted by ConnectRPC.
 */
export async function postMessageAttachmentOnRemote(
  remoteBaseURL: string,
  token: string,
  roomId: string,
  body: string,
  filePath: string,
  fileName: string,
  contentType: string
): Promise<{ eventId: string; attachmentUrl: string }> {
  const fileBytes = await readFile(filePath);
  const uploadClient = assetUploadClient(remoteBaseURL);
  const created = await uploadClient.createUpload(
    {
      roomId,
      filename: fileName,
      contentType,
      size: BigInt(fileBytes.byteLength),
      sha256: sha256(fileBytes)
    },
    { headers: authHeaders(token) }
  );
  const upload = created.upload;
  if (!upload?.uploadId) {
    throw new Error(
      `No upload returned from remote CreateUpload: ${JSON.stringify(created.toJson())}`
    );
  }

  let offset = Number(upload.committedOffset);
  const maxChunkSize = Math.max(1, upload.maxChunkSize);
  while (offset < fileBytes.byteLength) {
    const end = Math.min(offset + maxChunkSize, fileBytes.byteLength);
    const chunk = fileBytes.subarray(offset, end);
    const chunkResponse = await uploadClient.uploadChunk(
      {
        uploadId: upload.uploadId,
        offset: BigInt(offset),
        content: chunk,
        chunkSha256: sha256(chunk)
      },
      { headers: authHeaders(token) }
    );
    offset = Number(chunkResponse.upload?.committedOffset ?? BigInt(end));
  }

  const completed = await uploadClient.completeUpload(
    { uploadId: upload.uploadId },
    { headers: authHeaders(token) }
  );
  const assetId = completed.asset?.assetId;
  if (!assetId) {
    throw new Error(
      `No asset returned from remote CompleteUpload: ${JSON.stringify(completed.toJson())}`
    );
  }

  const response = await messageClient(remoteBaseURL).createMessage(
    {
      roomId,
      body,
      attachmentAssetIds: [assetId]
    },
    { headers: authHeaders(token) }
  );

  const message = response.message;
  const eventId = message?.id;
  const attachmentUrl = message?.attachments[0]?.assetUrl?.url;
  if (!eventId || !attachmentUrl) {
    throw new Error(
      `No attachment returned from remote CreateMessage: ${JSON.stringify(response.toJson())}`
    );
  }

  return { eventId, attachmentUrl };
}

/**
 * Posts a thread reply in a room on a remote server. Returns the new event ID.
 */
export async function postThreadReplyOnRemote(
  remoteBaseURL: string,
  token: string,
  roomId: string,
  body: string,
  threadRootEventId: string
): Promise<string> {
  const response = await messageClient(remoteBaseURL).createMessage(
    { roomId, body, threadRootEventId },
    { headers: authHeaders(token) }
  );
  return postedEventId(response);
}

/**
 * Starts a DM conversation on a remote server and posts an initial message.
 * Returns the conversation (room) ID.
 */
export async function startDMOnRemote(
  remoteBaseURL: string,
  senderToken: string,
  receiverUserId: string,
  message: string
): Promise<string> {
  const response = await roomClient(remoteBaseURL).startDM(
    { participantIds: [receiverUserId] },
    { headers: authHeaders(senderToken) }
  );
  const roomId = response.room?.id;
  if (!roomId) throw new Error('Failed to start DM on remote');

  await postMessageOnRemote(remoteBaseURL, senderToken, roomId, message);
  return roomId;
}

/**
 * Sends a typing indicator on a remote server via ConnectRPC.
 */
export async function sendTypingOnRemote(
  remoteBaseURL: string,
  token: string,
  roomId: string
): Promise<void> {
  await roomClient(remoteBaseURL).updateTypingIndicator(
    { roomId },
    {
      headers: authHeaders(token)
    }
  );
}

/**
 * Gets a room by name on a remote server. Returns the room's ID.
 */
export async function getRoomOnRemote(
  remoteBaseURL: string,
  token: string,
  roomName: string
): Promise<string> {
  const data = await roomDirectoryClient(remoteBaseURL).listRooms(
    {},
    { headers: authHeaders(token) }
  );
  const room = data.rooms.find((entry) => entry.room?.name === roomName)?.room;
  if (!room?.id) {
    throw new Error(`Room "${roomName}" not found in instance: ${JSON.stringify(data.toJson())}`);
  }

  return room.id;
}

/**
 * Logs in as the bootstrap admin user (`e2eadmin`) on a remote server and
 * returns a bearer token. Mirrors `loginAsAdmin()` for the origin server.
 */
export async function loginAdminOnRemote(
  remoteBaseURL: string
): Promise<{ token: string; userId: string }> {
  const loginResp = await fetch(`${remoteBaseURL}/auth/login`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ login: 'e2eadmin', password: 'adminpassword123' })
  });
  if (!loginResp.ok) {
    throw new Error(`Failed to login admin on remote: ${await loginResp.text()}`);
  }
  const loginData = await loginResp.json();
  if (!loginData.token) {
    throw new Error(`No token returned from remote admin login: ${JSON.stringify(loginData)}`);
  }

  const viewer = await viewerClient(remoteBaseURL).getViewer(
    {},
    { headers: authHeaders(loginData.token) }
  );
  const userId = viewer.user?.profile?.id;
  if (!userId) {
    throw new Error(
      `No userId returned from remote viewer RPC: ${JSON.stringify(viewer.toJson())}`
    );
  }
  return { token: loginData.token, userId };
}

/**
 * Updates the MOTD on a remote server via the admin ConnectRPC.
 * The token must belong to a user with admin/owner permission.
 */
export async function setMotdOnRemote(
  remoteBaseURL: string,
  token: string,
  motd: string
): Promise<void> {
  const response = await adminServerClient(remoteBaseURL).updateServerConfig(
    { motd },
    { headers: authHeaders(token) }
  );
  if (response.profile?.motd !== motd) {
    throw new Error(`Failed to set MOTD on remote: ${JSON.stringify(response.toJson())}`);
  }
}

/**
 * Drives the real Add-Server dialog → /oauth/authorize → /servers/callback
 * flow to add `remoteServer` as a connected instance, while bypassing the
 * human OAuth login form. The remote's `/oauth/authorize` request is
 * intercepted via Playwright's `page.route`; we POST the PKCE params to the
 * test-only `/auth/test/oauth-authorize` endpoint to mint a real authorization
 * code, then fulfill the navigation with a 302 to the callback URL. From
 * there the origin's callback page runs unchanged: PKCE verifier exchange via
 * `/oauth/token`, real bearer token, real `serverRegistry.addServer()`.
 *
 * The user identified by `userId` must already exist on the remote (use
 * `createUserOnRemote` to create one).
 */
export async function connectRemoteInstance(
  page: Page,
  remoteServer: ServerInfo,
  userId: string
): Promise<void> {
  const remoteBaseURL = remoteServer.baseURL;
  const remoteOrigin = new URL(remoteBaseURL).origin;
  const hostname = new URL(remoteBaseURL).host;

  // Intercept the navigation to the remote's /oauth/authorize and fulfill
  // with a 302 to the callback URL carrying a real authorization code.
  await page.route(`${remoteOrigin}/oauth/authorize*`, async (route) => {
    const requestUrl = new URL(route.request().url());
    const codeChallenge = requestUrl.searchParams.get('code_challenge') ?? '';
    const codeChallengeMethod = requestUrl.searchParams.get('code_challenge_method') ?? '';
    const redirectUri = requestUrl.searchParams.get('redirect_uri') ?? '';
    const state = requestUrl.searchParams.get('state') ?? '';

    const resp = await fetch(`${remoteBaseURL}/auth/test/oauth-authorize`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({
        userId,
        redirectUri,
        codeChallenge,
        codeChallengeMethod,
        state
      })
    });

    if (!resp.ok) {
      throw new Error(`test/oauth-authorize failed (${resp.status}): ${await resp.text()}`);
    }

    const { redirectURL } = (await resp.json()) as { redirectURL: string };
    await route.fulfill({
      status: 302,
      headers: { Location: redirectURL }
    });
  });

  // Drive the real UI: open dialog from sidebar → URL → preview →
  // would-redirect to /oauth/authorize (intercepted) → /servers/callback
  // → token exchange → addServer.
  if (!/\/chat\//.test(page.url())) {
    await page.goto('/chat/-');
  }
  await page.getByTitle('Add Server').click();
  await page.getByLabel('Server URL').fill(hostname);
  await page.getByRole('button', { name: 'Connect' }).click();
  await page.getByRole('button', { name: 'Sign in', exact: true }).click();

  // Callback page redirects into the newly-added remote instance's chat
  // tree on success — `/chat/<hostname>/...` (post-PR(a) there is no
  // `/chat/spaces` landing). The hostname is whatever segment was passed
  // in (typically "127.0.0.1").
  const hostnameOnly = hostname.split(':')[0]!.replace(/\./g, '\\.');
  await page.waitForURL(new RegExp(`/chat/${hostnameOnly}(/|$)`));
}
