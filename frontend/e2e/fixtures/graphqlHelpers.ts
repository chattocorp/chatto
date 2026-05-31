import { expect, type Page } from '@playwright/test';

// Shorter timeouts locally for faster feedback
const DEFAULT_POLL_TIMEOUT = process.env.CI ? 10_000 : 3_000;

/**
 * Execute a GraphQL query from within the page context.
 * Uses the page's cookies for authentication.
 */
export async function graphqlQuery<T>(
  page: Page,
  query: string,
  variables?: Record<string, unknown>
): Promise<T> {
  return page.evaluate(
    async ({ query, variables }) => {
      const response = await fetch('/api/graphql', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        credentials: 'include',
        body: JSON.stringify({ query, variables })
      });
      const json = await response.json();
      if (json.errors) throw new Error(JSON.stringify(json.errors));
      return json.data;
    },
    { query, variables }
  );
}

/**
 * Wait for the server to have/not have any unread rooms (server-side state).
 * Polls the server until the expected state is reached.
 */
export async function waitForSpaceUnread(
  page: Page,
  expected: boolean,
  timeout = DEFAULT_POLL_TIMEOUT
): Promise<void> {
  await expect(async () => {
    const data = await graphqlQuery<{ server: { viewerHasUnreadRooms: boolean } }>(
      page,
      `query { server { viewerHasUnreadRooms } }`
    );
    expect(data.server.viewerHasUnreadRooms).toBe(expected);
  }).toPass({ timeout, intervals: [100, 250, 500, 1000] });
}

/**
 * Wait for a room to have/not have unread messages (server-side state).
 * Polls the server until the expected state is reached.
 */
export async function waitForRoomUnread(
  page: Page,
  roomId: string,
  expected: boolean,
  timeout = DEFAULT_POLL_TIMEOUT
): Promise<void> {
  await expect(async () => {
    const data = await graphqlQuery<{ room: { hasUnread: boolean } }>(
      page,
      `query($roomId: ID!) { room(roomId: $roomId) { hasUnread } }`,
      { roomId }
    );
    expect(data.room.hasUnread).toBe(expected);
  }).toPass({ timeout, intervals: [100, 250, 500, 1000] });
}

/**
 * Wait for markRoomAsRead to complete by verifying server state.
 * Use this instead of arbitrary timeouts after entering a room.
 */
export async function waitForRoomRead(
  page: Page,
  roomId: string,
  timeout = DEFAULT_POLL_TIMEOUT
): Promise<void> {
  await waitForRoomUnread(page, roomId, false, timeout);
}

/**
 * Wait for a user to be deleted (no longer exists in the system).
 * Polls the server until the user query returns null or throws a "not found" error.
 */
export async function waitForUserDeleted(
  page: Page,
  userId: string,
  timeout = DEFAULT_POLL_TIMEOUT
): Promise<void> {
  await expect(async () => {
    const result = await page.evaluate(
      async ({ query, variables }) => {
        const response = await fetch('/api/graphql', {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          credentials: 'include',
          body: JSON.stringify({ query, variables })
        });
        const json = await response.json();
        // User is deleted if: errors exist (not found) OR user field is null
        if (json.errors || json.data?.user === null) {
          return { deleted: true };
        }
        return { deleted: false, user: json.data?.user };
      },
      {
        query: `query($id: ID!) { user(id: $id) { id } }`,
        variables: { id: userId }
      }
    );
    expect(result.deleted).toBe(true);
  }).toPass({ timeout, intervals: [100, 250, 500, 1000] });
}

/**
 * Wait for the server's member count to reach the expected value.
 * Useful for verifying membership changes after user deletion.
 */
export async function waitForSpaceMemberCount(
  page: Page,
  expectedCount: number,
  timeout = DEFAULT_POLL_TIMEOUT
): Promise<void> {
  await expect(async () => {
    const data = await graphqlQuery<{ server: { memberCount: number } }>(
      page,
      `query { server { memberCount } }`
    );
    expect(data.server.memberCount).toBe(expectedCount);
  }).toPass({ timeout, intervals: [100, 250, 500, 1000] });
}

/**
 * Post a message via the GraphQL API and return the event ID.
 * Uses Playwright's request API (not in-page fetch) for speed.
 */
export async function postMessageViaAPI(
  page: Page,
  roomId: string,
  body: string
): Promise<string> {
  const response = await page.request.post('/api/graphql', {
    headers: { 'Content-Type': 'application/json', 'X-REQUEST-TYPE': 'GraphQL' },
    data: {
      query: `mutation($input: PostMessageInput!) { postMessage(input: $input) { id } }`,
      variables: { input: { roomId, body } }
    }
  });
  const json = await response.json();
  return json.data.postMessage.id;
}

/**
 * Post multiple messages via the GraphQL API (no return values).
 */
export async function postMessagesViaAPI(
  page: Page,
  roomId: string,
  messages: string[]
): Promise<void> {
  for (const body of messages) {
    await postMessageViaAPI(page, roomId, body);
  }
}

/**
 * Post a reply (with inReplyTo attribution) via the GraphQL API and return the event ID.
 */
export async function postReplyViaAPI(
  page: Page,
  roomId: string,
  body: string,
  inReplyTo: string
): Promise<string> {
  const response = await page.request.post('/api/graphql', {
    headers: { 'Content-Type': 'application/json', 'X-REQUEST-TYPE': 'GraphQL' },
    data: {
      query: `mutation($input: PostMessageInput!) { postMessage(input: $input) { id } }`,
      variables: { input: { roomId, body, inReplyTo } }
    }
  });
  const json = await response.json();
  return json.data.postMessage.id;
}

/**
 * Post a thread reply via the GraphQL API and return the event ID.
 */
export async function postThreadReplyViaAPI(
  page: Page,
  roomId: string,
  body: string,
  inThread: string,
  inReplyTo?: string
): Promise<string> {
  const input: Record<string, unknown> = { roomId, body, threadRootEventId: inThread };
  if (inReplyTo) input.inReplyTo = inReplyTo;
  const response = await page.request.post('/api/graphql', {
    headers: { 'Content-Type': 'application/json', 'X-REQUEST-TYPE': 'GraphQL' },
    data: {
      query: `mutation($input: PostMessageInput!) { postMessage(input: $input) { id } }`,
      variables: { input }
    }
  });
  const json = await response.json();
  return json.data.postMessage.id;
}

/**
 * Extract roomId from the current URL (`/chat/-/{roomId}`). Post-ADR-030
 * the spaceId is just the legacy kind discriminator constant —
 * `core.LegacyServerSpaceID` on the backend, `SERVER_SPACE_ID` on the
 * frontend.
 */
export async function getIdsFromUrl(
  page: Page
): Promise<{ spaceId: string; roomId: string }> {
  const match = page.url().match(/\/chat\/-\/([^/]+)/);
  if (!match) throw new Error(`Could not extract roomId from URL: ${page.url()}`);
  const roomId = match[1];
  return { spaceId: 'server', roomId };
}

/**
 * Get the room ID for a room by name on the deployment.
 * Useful when tests need to reference rooms by ID for GraphQL queries.
 */
export async function getRoomIdByName(
  page: Page,
  roomName: string
): Promise<string> {
  const data = await graphqlQuery<{
    viewer: { user: { rooms: Array<{ id: string; name: string }> } };
  }>(
    page,
    `query {
        viewer {
          user {
            rooms {
              id
              name
            }
          }
        }
      }`
  );

  const room = data.viewer.user.rooms.find((r) => r.name === roomName);
  if (!room) {
    throw new Error(`Room "${roomName}" not found`);
  }
  return room.id;
}
