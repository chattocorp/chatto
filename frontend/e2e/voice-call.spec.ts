/**
 * E2E tests for voice/video call integration.
 *
 * These tests configure the test server with LiveKit credentials via the
 * serverOptions fixture. Token generation is pure JWT signing (no real
 * LiveKit server needed). Actual WebRTC connections cannot be tested in
 * CI — these tests verify the signaling layer, UI visibility, and
 * GraphQL queries.
 *
 * Camera/video tests require an actual LiveKit connection (participant mode)
 * which is not available in CI. Camera toggle, video thumbnails, and device
 * menu camera section can only be tested manually with `mise dev`.
 */

import { test, expect } from './setup';
import { createAndLoginTestUser, joinSpace } from './fixtures/testUser';
import { graphqlQuery, getRoomIdByName } from './fixtures/graphqlHelpers';
import { DMPage } from './pages/DMPage';
import { TIMEOUTS } from './constants';

// Configure the test server with LiveKit credentials via the server fixture.
// This scopes the env vars to the spawned server process without polluting
// the worker's process.env (which would leak to other test files).
test.use({
	serverOptions: {
		env: {
			CHATTO_LIVEKIT_ENABLED: 'true',
			CHATTO_LIVEKIT_URL: 'ws://localhost:7880',
			CHATTO_LIVEKIT_API_KEY: 'devkey',
			CHATTO_LIVEKIT_API_SECRET: 'secret'
		}
	}
});

test.describe('Voice calls', () => {
	test('call button appears in room header', async ({ page, chatPage }) => {
		await createAndLoginTestUser(page);
		await chatPage.goto();
		await chatPage.createSpace();
		await chatPage.enterRoom('general');

		const callButton = page.getByTitle('Join voice call');
		await expect(callButton).toBeVisible({ timeout: TIMEOUTS.UI_STANDARD });
	});

	test('call button appears in DM rooms', async ({ page, browser, serverURL }) => {
		// Create two users
		await createAndLoginTestUser(page);

		const context2 = await browser!.newContext({ baseURL: serverURL });
		const page2 = await context2.newPage();

		try {
			const userB = await createAndLoginTestUser(page2);

			// User A starts a DM with User B
			const dmPage = new DMPage(page);
			await dmPage.startConversation(userB.login);

			// Call button should be visible in the DM room header
			const callButton = page.getByTitle('Join voice call');
			await expect(callButton).toBeVisible({ timeout: TIMEOUTS.UI_STANDARD });
		} finally {
			await context2.close();
		}
	});

	test('voiceCallToken query returns a valid JWT', async ({ page, chatPage }) => {
		await createAndLoginTestUser(page);
		await chatPage.goto();
		await chatPage.createSpace();
		const spaceId = await chatPage.getSpaceId();
		await chatPage.enterRoom('general');
		const roomId = await getRoomIdByName(page, 'general');

		const data = await graphqlQuery<{ voiceCallToken: { token: string } | null }>(
			page,
			`query($roomId: ID!) {
				voiceCallToken(roomId: $roomId) { token }
			}`,
			{ roomId }
		);

		expect(data.voiceCallToken).not.toBeNull();
		expect(data.voiceCallToken!.token).toBeTruthy();
		// JWT tokens have 3 base64url parts separated by dots
		expect(data.voiceCallToken!.token.split('.')).toHaveLength(3);
	});

	test('voiceCallToken requires room membership', async ({ page, chatPage, browser, serverURL }) => {
		// User A creates a space and room
		await createAndLoginTestUser(page);
		await chatPage.goto();
		await chatPage.createSpace();
		const spaceId = await chatPage.getSpaceId();
		await chatPage.enterRoom('general');
		const roomId = await getRoomIdByName(page, 'general');

		// User B — auto-joins the bootstrap space at signup (issue #330), so we
		// must explicitly leave the room to verify non-member rejection.
		const context2 = await browser!.newContext({ baseURL: serverURL });
		const page2 = await context2.newPage();

		try {
			await createAndLoginTestUser(page2);

			// Navigate page2 so relative URL works in page.evaluate
			await page2.goto('/chat');
			await page2.waitForURL((url) => url.pathname.startsWith('/chat'));

			// Leave the room so user B is genuinely not a member when the test
			// fires the voiceCallToken query below.
			await page2.request.post('/api/graphql', {
				headers: { 'Content-Type': 'application/json', 'X-REQUEST-TYPE': 'GraphQL' },
				data: {
					query: `mutation($input: LeaveRoomInput!) { leaveRoom(input: $input) }`,
					variables: { input: { roomId } }
				}
			});

			// User B tries to get a voice call token — should fail
			const result = await page2.evaluate(
				async ({ roomId }) => {
					const response = await fetch('/api/graphql', {
						method: 'POST',
						headers: { 'Content-Type': 'application/json' },
						credentials: 'include',
						body: JSON.stringify({
							query: `query($roomId: ID!) {
								voiceCallToken(roomId: $roomId) { token }
							}`,
							variables: { roomId }
						})
					});
					return response.json();
				},
				{ roomId }
			);

			// Should have errors (not a room member)
			expect(result.errors).toBeTruthy();
			expect(result.errors.length).toBeGreaterThan(0);
		} finally {
			await context2.close();
		}
	});

	test('activeCallRoomIds returns empty when no calls active', async ({ page, chatPage }) => {
		await createAndLoginTestUser(page);
		await chatPage.goto();
		await chatPage.createSpace();
		const spaceId = await chatPage.getSpaceId();

		const data = await graphqlQuery<{ activeCallRoomIds: string[] }>(
			page,
			`query { activeCallRoomIds }`,
			{}
		);

		expect(data.activeCallRoomIds).toEqual([]);
	});

	test('call icon appears and disappears via real-time events', async ({
		page,
		chatPage,
		browser,
		serverURL
	}) => {
		// User A creates a space and enters a room
		await createAndLoginTestUser(page);
		await chatPage.goto();
		await chatPage.createSpace();
		const spaceId = await chatPage.getSpaceId();
		await chatPage.enterRoom('general');
		const roomId = await getRoomIdByName(page, 'general');

		// The call icon should NOT be visible initially
		const callIcon = chatPage.roomList.locator('.uil--phone');
		await expect(callIcon).not.toBeVisible();

		// User B joins the same space and room
		const context2 = await browser!.newContext({ baseURL: serverURL });
		const page2 = await context2.newPage();

		try {
			const userB = await createAndLoginTestUser(page2);
			await joinSpace(page2, spaceId);

			// User B joins the room via API (use page2.request to avoid page navigation requirement)
			await page2.request.post('/api/graphql', {
				headers: { 'Content-Type': 'application/json', 'X-REQUEST-TYPE': 'GraphQL' },
				data: {
					query: `mutation($input: JoinRoomInput!) { joinRoom(input: $input) }`,
					variables: { input: { roomId } }
				}
			});

			// Simulate User B joining a voice call via test webhook endpoint
			// (bypasses LiveKit HMAC — calls core.HandleCallParticipantJoined directly)
			await page.request.post('/webhooks/test/call-join', {
				data: {
					spaceId,
					roomId,
					userId: userB.id,
					displayName: userB.displayName,
					login: userB.login
				}
			});

			// User A should see the call icon appear
			await expect(callIcon).toBeVisible({ timeout: TIMEOUTS.REALTIME_EVENT });

			// Simulate User B leaving the voice call
			await page.request.post('/webhooks/test/call-leave', {
				data: {
					spaceId,
					roomId,
					userId: userB.id
				}
			});

			// User A should see the call icon disappear
			// (handleLeave re-queries activeCallRoomIds, which returns [] since KV is now empty)
			await expect(callIcon).not.toBeVisible({ timeout: TIMEOUTS.REALTIME_EVENT });
		} finally {
			await context2.close();
		}
	});

	test('callParticipants returns participants after webhook join', async ({
		page,
		chatPage,
		browser,
		serverURL
	}) => {
		await createAndLoginTestUser(page);
		await chatPage.goto();
		await chatPage.createSpace();
		const spaceId = await chatPage.getSpaceId();
		await chatPage.enterRoom('general');
		const roomId = await getRoomIdByName(page, 'general');

		// Initially empty
		const before = await graphqlQuery<{
			callParticipants: { userId: string; displayName: string; login: string }[];
		}>(
			page,
			`query($roomId: ID!) {
				callParticipants(roomId: $roomId) { userId displayName login }
			}`,
			{ roomId }
		);
		expect(before.callParticipants).toEqual([]);

		// Create User B and have them join
		const context2 = await browser!.newContext({ baseURL: serverURL });
		const page2 = await context2.newPage();

		try {
			const userB = await createAndLoginTestUser(page2);
			await joinSpace(page2, spaceId);
			await page2.request.post('/api/graphql', {
				headers: { 'Content-Type': 'application/json', 'X-REQUEST-TYPE': 'GraphQL' },
				data: {
					query: `mutation($input: JoinRoomInput!) { joinRoom(input: $input) }`,
					variables: { input: { roomId } }
				}
			});

			// Simulate join via webhook test endpoint
			await page.request.post('/webhooks/test/call-join', {
				data: {
					spaceId,
					roomId,
					userId: userB.id,
					displayName: userB.displayName,
					login: userB.login
				}
			});

			// Query participants — should now include User B
			const after = await graphqlQuery<{
				callParticipants: { userId: string; displayName: string; login: string }[];
			}>(
				page,
				`query($roomId: ID!) {
					callParticipants(roomId: $roomId) { userId displayName login }
				}`,
				{ roomId }
			);
			expect(after.callParticipants).toHaveLength(1);
			expect(after.callParticipants[0].userId).toBe(userB.id);
			expect(after.callParticipants[0].login).toBe(userB.login);

			// Simulate leave
			await page.request.post('/webhooks/test/call-leave', {
				data: { spaceId, roomId, userId: userB.id }
			});

			// Query again — should be empty
			const afterLeave = await graphqlQuery<{
				callParticipants: { userId: string; displayName: string; login: string }[];
			}>(
				page,
				`query($roomId: ID!) {
					callParticipants(roomId: $roomId) { userId displayName login }
				}`,
				{ roomId }
			);
			expect(afterLeave.callParticipants).toEqual([]);
		} finally {
			await context2.close();
		}
	});

	test('observer panel appears when another user joins a call', async ({
		page,
		chatPage,
		browser,
		serverURL
	}) => {
		// User A creates a space and enters a room
		await createAndLoginTestUser(page);
		await chatPage.goto();
		await chatPage.createSpace();
		const spaceId = await chatPage.getSpaceId();
		await chatPage.enterRoom('general');
		const roomId = await getRoomIdByName(page, 'general');

		// Observer panel should NOT be visible initially
		await expect(page.getByTestId('call-observer-panel')).not.toBeVisible();

		// User B joins the same space and room
		const context2 = await browser!.newContext({ baseURL: serverURL });
		const page2 = await context2.newPage();

		try {
			const userB = await createAndLoginTestUser(page2);
			await joinSpace(page2, spaceId);
			await page2.request.post('/api/graphql', {
				headers: { 'Content-Type': 'application/json', 'X-REQUEST-TYPE': 'GraphQL' },
				data: {
					query: `mutation($input: JoinRoomInput!) { joinRoom(input: $input) }`,
					variables: { input: { roomId } }
				}
			});

			// Simulate User B joining a voice call via test webhook endpoint
			await page.request.post('/webhooks/test/call-join', {
				data: {
					spaceId,
					roomId,
					userId: userB.id,
					displayName: userB.displayName,
					login: userB.login
				}
			});

			// User A should see the observer panel with "Call active" and a Join button
			const observerPanel = page.getByTestId('call-observer-panel');
			await expect(observerPanel).toBeVisible({ timeout: TIMEOUTS.REALTIME_EVENT });
			await expect(page.getByTestId('call-join-button')).toBeVisible();

			// User A should see User B's display name in the participant list
			await expect(observerPanel.getByTitle(userB.displayName)).toBeVisible();

			// Simulate User B leaving — observer panel should disappear
			await page.request.post('/webhooks/test/call-leave', {
				data: { spaceId, roomId, userId: userB.id }
			});

			await expect(observerPanel).not.toBeVisible({ timeout: TIMEOUTS.REALTIME_EVENT });
			await expect(page.getByTestId('call-join-button')).not.toBeVisible();
		} finally {
			await context2.close();
		}
	});

	test('observer panel updates when participants join and leave', async ({
		page,
		chatPage,
		browser,
		serverURL
	}) => {
		// User A creates a space and enters a room
		await createAndLoginTestUser(page);
		await chatPage.goto();
		await chatPage.createSpace();
		const spaceId = await chatPage.getSpaceId();
		await chatPage.enterRoom('general');
		const roomId = await getRoomIdByName(page, 'general');

		// Create User B and User C, both join space + room
		const context2 = await browser!.newContext({ baseURL: serverURL });
		const page2 = await context2.newPage();
		const context3 = await browser!.newContext({ baseURL: serverURL });
		const page3 = await context3.newPage();

		try {
			const userB = await createAndLoginTestUser(page2);
			await joinSpace(page2, spaceId);
			await page2.request.post('/api/graphql', {
				headers: { 'Content-Type': 'application/json', 'X-REQUEST-TYPE': 'GraphQL' },
				data: {
					query: `mutation($input: JoinRoomInput!) { joinRoom(input: $input) }`,
					variables: { input: { roomId } }
				}
			});

			const userC = await createAndLoginTestUser(page3);
			await joinSpace(page3, spaceId);
			await page3.request.post('/api/graphql', {
				headers: { 'Content-Type': 'application/json', 'X-REQUEST-TYPE': 'GraphQL' },
				data: {
					query: `mutation($input: JoinRoomInput!) { joinRoom(input: $input) }`,
					variables: { input: { roomId } }
				}
			});

			const observerPanel = page.getByTestId('call-observer-panel');

			// User B joins the call
			await page.request.post('/webhooks/test/call-join', {
				data: {
					spaceId,
					roomId,
					userId: userB.id,
					displayName: userB.displayName,
					login: userB.login
				}
			});

			await expect(observerPanel).toBeVisible({ timeout: TIMEOUTS.REALTIME_EVENT });
			await expect(observerPanel.getByTitle(userB.displayName)).toBeVisible();

			// User C joins the call
			await page.request.post('/webhooks/test/call-join', {
				data: {
					spaceId,
					roomId,
					userId: userC.id,
					displayName: userC.displayName,
					login: userC.login
				}
			});

			// Both participants should be visible
			await expect(observerPanel.getByTitle(userC.displayName)).toBeVisible({
				timeout: TIMEOUTS.REALTIME_EVENT
			});
			await expect(observerPanel.getByTitle(userB.displayName)).toBeVisible();

			// User B leaves — User C should still be visible, panel still showing
			await page.request.post('/webhooks/test/call-leave', {
				data: { spaceId, roomId, userId: userB.id }
			});

			await expect(observerPanel.getByTitle(userB.displayName)).not.toBeVisible({
				timeout: TIMEOUTS.REALTIME_EVENT
			});
			await expect(observerPanel.getByTitle(userC.displayName)).toBeVisible();
			await expect(observerPanel).toBeVisible();

			// User C leaves — panel should disappear
			await page.request.post('/webhooks/test/call-leave', {
				data: { spaceId, roomId, userId: userC.id }
			});

			await expect(observerPanel).not.toBeVisible({ timeout: TIMEOUTS.REALTIME_EVENT });
		} finally {
			await context2.close();
			await context3.close();
		}
	});

	test('participant avatars appear in room list sidebar during active call', async ({
		page,
		chatPage,
		browser,
		serverURL
	}) => {
		// User A creates a space and enters a room
		await createAndLoginTestUser(page);
		await chatPage.goto();
		await chatPage.createSpace();
		const spaceId = await chatPage.getSpaceId();
		await chatPage.enterRoom('general');
		const roomId = await getRoomIdByName(page, 'general');

		// No call badge in sidebar initially
		const roomList = chatPage.roomList;
		await expect(roomList.locator('.meta-badge')).not.toBeVisible();

		// User B joins the same space and room
		const context2 = await browser!.newContext({ baseURL: serverURL });
		const page2 = await context2.newPage();

		try {
			const userB = await createAndLoginTestUser(page2);
			await joinSpace(page2, spaceId);
			await page2.request.post('/api/graphql', {
				headers: { 'Content-Type': 'application/json', 'X-REQUEST-TYPE': 'GraphQL' },
				data: {
					query: `mutation($input: JoinRoomInput!) { joinRoom(input: $input) }`,
					variables: { input: { roomId } }
				}
			});

			// Simulate User B joining a voice call
			await page.request.post('/webhooks/test/call-join', {
				data: {
					spaceId,
					roomId,
					userId: userB.id,
					displayName: userB.displayName,
					login: userB.login
				}
			});

			// Call badge with phone icon should appear in sidebar
			const callBadge = roomList.locator('.meta-badge');
			await expect(callBadge).toBeVisible({ timeout: TIMEOUTS.REALTIME_EVENT });
			await expect(callBadge.locator('.uil--phone')).toBeVisible();

			// Simulate User B leaving — badge should disappear
			await page.request.post('/webhooks/test/call-leave', {
				data: { spaceId, roomId, userId: userB.id }
			});

			await expect(callBadge).not.toBeVisible({ timeout: TIMEOUTS.REALTIME_EVENT });
		} finally {
			await context2.close();
		}
	});

	test('livekitUrl is exposed in instance info', async ({ page, chatPage }) => {
		await createAndLoginTestUser(page);
		await chatPage.goto();

		const data = await graphqlQuery<{ instance: { livekitUrl: string | null } }>(
			page,
			`query { instance { livekitUrl } }`
		);

		expect(data.instance.livekitUrl).toBe('ws://localhost:7880');
	});
});
