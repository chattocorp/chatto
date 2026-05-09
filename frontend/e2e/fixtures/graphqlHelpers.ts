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
 * Wait for a space to have/not have unread rooms (server-side state).
 * Polls the server until the expected state is reached.
 */
export async function waitForSpaceUnread(
  page: Page,
  _spaceId: string,
  expected: boolean,
  timeout = DEFAULT_POLL_TIMEOUT
): Promise<void> {
  await expect(async () => {
    const data = await graphqlQuery<{ instance: { viewerHasUnreadRooms: boolean } }>(
      page,
      `query { instance { viewerHasUnreadRooms } }`
    );
    expect(data.instance.viewerHasUnreadRooms).toBe(expected);
  }).toPass({ timeout, intervals: [100, 250, 500, 1000] });
}

/**
 * Wait for a room to have/not have unread messages (server-side state).
 * Polls the server until the expected state is reached.
 */
export async function waitForRoomUnread(
  page: Page,
  spaceId: string,
  roomId: string,
  expected: boolean,
  timeout = DEFAULT_POLL_TIMEOUT
): Promise<void> {
  await expect(async () => {
    const data = await graphqlQuery<{ room: { hasUnread: boolean } }>(
      page,
      `query($spaceId: ID!, $roomId: ID!) { room(spaceId: $spaceId, roomId: $roomId) { hasUnread } }`,
      { spaceId, roomId }
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
  spaceId: string,
  roomId: string,
  timeout = DEFAULT_POLL_TIMEOUT
): Promise<void> {
  await waitForRoomUnread(page, spaceId, roomId, false, timeout);
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
 * Wait for a space's member count to reach the expected value.
 * Useful for verifying membership changes after user deletion.
 */
export async function waitForSpaceMemberCount(
  page: Page,
  _spaceId: string,
  expectedCount: number,
  timeout = DEFAULT_POLL_TIMEOUT
): Promise<void> {
  await expect(async () => {
    const data = await graphqlQuery<{ instance: { memberCount: number } }>(
      page,
      `query { instance { memberCount } }`
    );
    expect(data.instance.memberCount).toBe(expectedCount);
  }).toPass({ timeout, intervals: [100, 250, 500, 1000] });
}

/**
 * Post a message via the GraphQL API and return the event ID.
 * Uses Playwright's request API (not in-page fetch) for speed.
 */
export async function postMessageViaAPI(
  page: Page,
  spaceId: string,
  roomId: string,
  body: string
): Promise<string> {
  const response = await page.request.post('/api/graphql', {
    headers: { 'Content-Type': 'application/json', 'X-REQUEST-TYPE': 'GraphQL' },
    data: {
      query: `mutation($input: PostMessageInput!) { postMessage(input: $input) { id } }`,
      variables: { input: { spaceId, roomId, body } }
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
  spaceId: string,
  roomId: string,
  messages: string[]
): Promise<void> {
  for (const body of messages) {
    await postMessageViaAPI(page, spaceId, roomId, body);
  }
}

/**
 * Post a reply (with inReplyTo attribution) via the GraphQL API and return the event ID.
 */
export async function postReplyViaAPI(
  page: Page,
  spaceId: string,
  roomId: string,
  body: string,
  inReplyTo: string
): Promise<string> {
  const response = await page.request.post('/api/graphql', {
    headers: { 'Content-Type': 'application/json', 'X-REQUEST-TYPE': 'GraphQL' },
    data: {
      query: `mutation($input: PostMessageInput!) { postMessage(input: $input) { id } }`,
      variables: { input: { spaceId, roomId, body, inReplyTo } }
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
  spaceId: string,
  roomId: string,
  body: string,
  inThread: string,
  inReplyTo?: string
): Promise<string> {
  const input: Record<string, unknown> = { spaceId, roomId, body, inThread };
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
 * Extract roomId from the current URL (`/chat/-/{roomId}`) and resolve
 * spaceId via the GraphQL `Instance.primarySpaceId` field. After ADR-027 the
 * URL no longer carries spaceId — it has to come from server state.
 */
export async function getIdsFromUrl(
  page: Page
): Promise<{ spaceId: string; roomId: string }> {
  const match = page.url().match(/\/chat\/-\/([^/]+)/);
  if (!match) throw new Error(`Could not extract roomId from URL: ${page.url()}`);
  const roomId = match[1];
  const data = await page.evaluate(async () => {
    const r = await fetch('/api/graphql', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      credentials: 'include',
      body: JSON.stringify({ query: `query { instance { primarySpaceId } }` })
    });
    return r.json();
  });
  return { spaceId: data.data.instance.primarySpaceId, roomId };
}

/**
 * Get the room ID for a room by name in a space.
 * Useful when tests need to reference rooms by ID for GraphQL queries.
 */
export async function getRoomIdByName(
  page: Page,
  _spaceId: string,
  roomName: string
): Promise<string> {
  const data = await graphqlQuery<{
    me: { rooms: Array<{ id: string; name: string }> };
  }>(
    page,
    `query {
			me {
				rooms {
					id
					name
				}
			}
		}`
  );

  const room = data.me.rooms.find((r) => r.name === roomName);
  if (!room) {
    throw new Error(`Room "${roomName}" not found in space ${spaceId}`);
  }
  return room.id;
}
