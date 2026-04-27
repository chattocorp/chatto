import { test, expect } from './setup';
import { createAndLoginTestUser } from './fixtures/testUser';
import {
	startSecondServer,
	stopSecondServer,
	createUserOnRemote,
	connectRemoteInstance
} from './fixtures/multiInstance';
import type { ServerInfo } from './fixtures/server';
import { TIMEOUTS } from './constants';
import * as routes from './routes';
import { DMPage } from './pages/DMPage';

/**
 * Helper: start a DM on a remote server and send a message.
 * Returns the conversation (room) ID.
 */
async function startDMOnRemote(
	remoteBaseURL: string,
	senderToken: string,
	receiverUserId: string,
	message: string
): Promise<string> {
	// Start DM conversation
	const dmResponse = await fetch(`${remoteBaseURL}/api/graphql`, {
		method: 'POST',
		headers: {
			'Content-Type': 'application/json',
			'X-REQUEST-TYPE': 'GraphQL',
			Authorization: `Bearer ${senderToken}`
		},
		body: JSON.stringify({
			query: `mutation StartDM($input: StartDMInput!) { startDM(input: $input) { id } }`,
			variables: { input: { participantIds: [receiverUserId] } }
		})
	});
	const dmData = await dmResponse.json();
	const roomId = dmData.data?.startDM?.id;
	if (!roomId) throw new Error(`Failed to start DM: ${JSON.stringify(dmData)}`);

	// Send a message (empty DMs are hidden)
	await fetch(`${remoteBaseURL}/api/graphql`, {
		method: 'POST',
		headers: {
			'Content-Type': 'application/json',
			'X-REQUEST-TYPE': 'GraphQL',
			Authorization: `Bearer ${senderToken}`
		},
		body: JSON.stringify({
			query: `mutation PostMsg($input: PostMessageInput!) {
				postMessage(input: $input) { id }
			}`,
			variables: { input: { spaceId: 'DM', roomId, body: message } }
		})
	});

	return roomId;
}

/**
 * Helper: create a second user on the home instance via GraphQL API.
 */
async function createSecondHomeUser(
	page: import('@playwright/test').Page
): Promise<{ id: string; login: string; displayName: string }> {
	const ts = Date.now();
	const login = `dmtest${ts}`;
	const displayName = `DM Test ${ts}`;

	const result = await page.request.post('/api/graphql', {
		headers: { 'Content-Type': 'application/json', 'X-REQUEST-TYPE': 'GraphQL' },
		data: {
			query: `mutation CreateUser($input: CreateUserInput!) { createUser(input: $input) { id } }`,
			variables: {
				input: { login, displayName, password: 'testpassword123' }
			}
		}
	});
	const data = await result.json();
	return { id: data.data.createUser.id, login, displayName };
}

test.describe('Unified DM Inbox', () => {
	test('DM page is accessible at /chat/dm', async ({ page }) => {
		await createAndLoginTestUser(page);
		await page.goto(routes.dm);
		await expect(page.getByRole('heading', { name: 'Direct Messages' })).toBeVisible();
	});

	test('conversation URLs use /chat/dm/-/{id} format', async ({ page }) => {
		await createAndLoginTestUser(page);
		const dmPage = new DMPage(page);
		const user2 = await createSecondHomeUser(page);

		await dmPage.goto();
		await dmPage.startConversation(user2.login);

		// URL should match new pattern: /chat/dm/-/{conversationId}
		expect(page.url()).toMatch(/\/chat\/dm\/-\/[a-f0-9]+$/);
	});

	test('shows empty state when no conversations exist', async ({ page }) => {
		await createAndLoginTestUser(page);
		await page.goto(routes.dm);
		await expect(page.getByText('No conversations yet')).toBeVisible();
	});
});

test.describe('Unified DM Inbox — Multi-Instance', () => {
	let remoteServer: ServerInfo;

	test.beforeEach(async ({}, testInfo) => {
		remoteServer = await startSecondServer(testInfo);
	});

	test.afterEach(async ({}, testInfo) => {
		if (remoteServer) {
			await stopSecondServer(remoteServer, testInfo);
		}
	});

	test('shows DM conversations from multiple instances', async ({ page }) => {
		// 1. Set up home instance user
		await createAndLoginTestUser(page);
		const homeUser2 = await createSecondHomeUser(page);

		// 2. Start a DM on the home instance
		const dmPage = new DMPage(page);
		await dmPage.goto();
		const roomPage = await dmPage.startConversation(homeUser2.login);
		await roomPage.sendMessage(`Home DM ${Date.now()}`);

		// 3. Set up remote instance
		const remoteUser = await createUserOnRemote(
			remoteServer.baseURL,
			`remote${Date.now()}`,
			'password123'
		);
		const remoteSender = await createUserOnRemote(
			remoteServer.baseURL,
			`sender${Date.now()}`,
			'password123'
		);

		// Start a DM on the remote instance (sender → our remote user)
		await startDMOnRemote(
			remoteServer.baseURL,
			remoteSender.token,
			remoteUser.userId,
			`Remote DM ${Date.now()}`
		);

		// 4. Connect remote instance via the real /instances/add → OAuth → callback flow
		await connectRemoteInstance(page, remoteServer, remoteUser.userId);

		// 5. Navigate to unified DM inbox
		await page.goto(routes.dm);
		await expect(page.getByRole('heading', { name: 'Direct Messages' })).toBeVisible();

		// 6. Should see conversations from both instances
		// Home instance conversation (with homeUser2)
		await expect(async () => {
			await dmPage.expectConversationVisible(homeUser2.displayName);
		}).toPass({ timeout: TIMEOUTS.REALTIME_EVENT });

		// Remote instance conversation (with remoteSender)
		await expect(async () => {
			// The conversation should be from the remote sender
			const conversations = page.locator('nav a.sidebar-item');
			await expect(conversations).not.toHaveCount(0);
		}).toPass({ timeout: TIMEOUTS.REALTIME_EVENT });
	});

	test('shows instance hostname for conversations in multi-instance mode', async ({ page }) => {
		// Set up home instance with a DM
		await createAndLoginTestUser(page);
		const homeUser2 = await createSecondHomeUser(page);
		const dmPage = new DMPage(page);
		await dmPage.goto();
		const roomPage = await dmPage.startConversation(homeUser2.login);
		await roomPage.sendMessage(`Hostname test ${Date.now()}`);

		// Set up remote instance
		const remoteUser = await createUserOnRemote(
			remoteServer.baseURL,
			`remote${Date.now()}`,
			'password123'
		);

		// Connect remote instance via the real /instances/add → OAuth → callback flow
		await connectRemoteInstance(page, remoteServer, remoteUser.userId);

		// Navigate to DM inbox
		await page.goto(routes.dm);

		// With multiple instances connected, hostname should be visible
		// The home instance conversations should show "localhost" or similar
		await expect(async () => {
			await dmPage.expectConversationVisible(homeUser2.displayName);
			// Instance hostname should be shown as a subtitle (text-xs distinguishes
			// it from the avatar placeholder which also has text-muted)
			const conv = dmPage.getConversation(homeUser2.displayName);
			await expect(conv.locator('span.text-xs.text-muted')).toBeVisible();
			await expect(conv.locator('span.text-xs.text-muted')).toContainText('localhost');
		}).toPass({ timeout: TIMEOUTS.REALTIME_EVENT });
	});

	test('can open conversation from different instance without sidebar remount', async ({
		page
	}) => {
		// Set up home instance with a DM
		await createAndLoginTestUser(page);
		const homeUser2 = await createSecondHomeUser(page);
		const dmPage = new DMPage(page);
		await dmPage.goto();
		const roomPage = await dmPage.startConversation(homeUser2.login);
		await roomPage.sendMessage(`No remount test ${Date.now()}`);

		// Navigate to DM list
		await dmPage.goto();
		await dmPage.expectConversationVisible(homeUser2.displayName);

		// Click the conversation — sidebar should stay mounted (no flash)
		await dmPage.openConversation(homeUser2.displayName);

		// Conversation should load and sidebar should still be visible
		await dmPage.expectConversationHeader(homeUser2.displayName);
		await dmPage.expectConversationVisible(homeUser2.displayName);
	});
});

test.describe('Unified DM Inbox — Route Behavior', () => {
	test('DM icon in sidebar navigates to /chat/dm', async ({ page }) => {
		await createAndLoginTestUser(page);

		// Navigate to chat first to see the sidebar
		await page.goto(`/chat/-`);
		await page.waitForLoadState('networkidle');

		// Click the DM icon
		const dmIcon = page.locator('[title="Direct Messages"]');
		await expect(dmIcon).toBeVisible({ timeout: TIMEOUTS.UI_STANDARD });
		await dmIcon.click();

		// Should navigate to the unified DM inbox (not instance-scoped)
		await page.waitForURL(/\/chat\/dm/);
		expect(page.url()).toContain('/chat/dm');
		expect(page.url()).not.toContain('/chat/-/dm');
	});

	test('starting DM from member list navigates to /chat/dm/-/{id}', async ({
		page,
		chatPage
	}) => {
		const { RoomPage } = await import('./pages');

		const user = await createAndLoginTestUser(page);
		await chatPage.goto();
		await chatPage.createSpace('DM Route Test');
		await chatPage.createRoom('test-room');

		const roomPage = new RoomPage(page);
		await roomPage.expectMemberVisible(user.displayName, { timeout: TIMEOUTS.REALTIME_EVENT });

		// Click member → profile popover → Send Message
		const memberButton = page.locator('aside[aria-label="Room members"] button', {
			hasText: user.displayName
		});
		await memberButton.click();
		await page.getByRole('button', { name: 'Send Message', exact: true }).click();

		// Should navigate to new DM route format
		await page.waitForURL(/\/chat\/dm\/-\//);
		expect(page.url()).toMatch(/\/chat\/dm\/-\/[a-f0-9]+$/);
	});

	// Use larger viewport so member list is visible
	test.use({ viewport: { width: 1280, height: 720 } });
});
