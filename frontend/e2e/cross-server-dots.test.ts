import { expect } from '@playwright/test';
import { test } from './setup';
import { createAndLoginTestUser, joinSpace } from './fixtures/testUser';
import {
	startSecondServer,
	stopSecondServer,
	createUserOnRemote,
	createSpaceOnRemote,
	joinSpaceOnRemote,
	getRoomOnRemote,
	postMessageOnRemote,
	connectRemoteInstance
} from './fixtures/multiServer';
import {
	postMessageViaAPI,
	postThreadReplyViaAPI,
	getRoomIdByName
} from './fixtures/graphqlHelpers';
import type { ServerInfo } from './fixtures/server';
import { TIMEOUTS } from './constants';
import * as routes from './routes';

/**
 * Returns the remote server's base URL using 127.0.0.1 instead of localhost so
 * the SPA can resolve it as a distinct instance hostname.
 */
function remoteBaseURL(server: ServerInfo): string {
	return server.baseURL.replace('localhost', '127.0.0.1');
}

/**
 * Cross-instance dot indicator coverage.
 *
 * Most dot-rendering code is instance-agnostic (one render path keyed by
 * `serverId`), but a few timing windows and aggregation paths only manifest
 * for remote instances on cold loads. These tests cover those windows.
 */
test.describe('Cross-instance dots', () => {
	let remoteServer: ServerInfo;

	test.beforeEach(async ({}, testInfo) => {
		remoteServer = await startSecondServer(testInfo);
	});

	test.afterEach(async ({}, testInfo) => {
		if (remoteServer) {
			await stopSecondServer(remoteServer, testInfo);
		}
	});

	test('@mention on a remote space lights up its space icon in real time', async ({ page, chatPage }) => {
		// Home: log in so the SPA boots.
		await createAndLoginTestUser(page);
		await chatPage.goto();

		// Remote: owner creates a space, viewer joins, mentioner joins.
		const baseURL = remoteBaseURL(remoteServer);
		const ts = Date.now();
		const viewerLogin = `xviewer${ts}`;
		const owner = await createUserOnRemote(baseURL, `xowner${ts}`, 'password123');
		const spaceId = await createSpaceOnRemote(baseURL, owner.token, 'Cross Instance Mention');
		const viewer = await createUserOnRemote(baseURL, viewerLogin, 'password123');
		await joinSpaceOnRemote(baseURL, viewer.token);
		const mentioner = await createUserOnRemote(baseURL, `xmentioner${ts}`, 'password123');
		await joinSpaceOnRemote(baseURL, mentioner.token);
		const generalRoomId = await getRoomOnRemote(baseURL, owner.token, 'general');

		// Connect the remote instance as `viewer` and stay on /chat (away from the
		// remote space). This is the cold-load timing window where the bus has to
		// be ready and consumers have to attach reactively.
		await connectRemoteInstance(page, { ...remoteServer, baseURL }, viewer.userId);
		await page.waitForLoadState('networkidle');

		// Sanity: no dot on the remote space icon yet. Issue #330: home and
		// remote share the bootstrap space name "E2E Test Server", so
		// disambiguate the remote icon by the host segment in its href —
		// home links use "/chat/-" while remote links use "/chat/<host>".
		const remoteHostSegment = new URL(baseURL).hostname;
		const remoteSpaceWrapper = page
			.locator('.server-gutter .server-icon-wrapper')
			.filter({ has: page.locator(`a[data-testid="server-icon"][href*="/chat/${remoteHostSegment}"]`) });
		const remoteSpaceBadge = remoteSpaceWrapper.getByTestId('server-notification-badge');
		await expect(remoteSpaceBadge).not.toBeVisible();

		// Mentioner posts an @mention of the viewer in the remote space. No reload.
		await postMessageOnRemote(
			baseURL,
			mentioner.token,
			generalRoomId,
			`hey @${viewerLogin} ping ${ts}`
		);

		// The remote space icon should light up in real time, no reload.
		await expect(remoteSpaceBadge).toBeVisible({ timeout: TIMEOUTS.REALTIME_EVENT });
		await expect(remoteSpaceBadge).toHaveText('1');
	});

	// "DM on a remote instance lights up the DM icon" was removed with the
	// cross-instance DM icon (#330 phase 3). Cross-server DM aggregation will
	// be re-tested when that view is reintroduced.

	test('mention on a thread message: clicking the space badge opens the thread', async ({
		page,
		chatPage,
		roomPage,
		browser,
		serverURL
	}) => {
		// Home: User A creates a space, posts a root message, then leaves the room.
		const userA = await createAndLoginTestUser(page);
		await chatPage.goto();
		await chatPage.createSpace();
		const spaceId = await chatPage.getSpaceId();

		await chatPage.enterRoom('general');
		const generalRoomId = await getRoomIdByName(page, 'general');
		const rootBody = `Thread root ${Date.now()}`;
		const rootEventId = await postMessageViaAPI(page, generalRoomId, rootBody);

		// Move A away from the room so the notification badge can show on the space.
		await chatPage.enterRoom('announcements');

		// User B joins, then posts a thread reply that @-mentions User A.
		const ctxB = await browser!.newContext({ baseURL: serverURL });
		const pageB = await ctxB.newPage();
		try {
			await createAndLoginTestUser(pageB);
			await joinSpace(pageB);
			await postThreadReplyViaAPI(
				pageB,
				generalRoomId,
				`@${userA.login} look at this`,
				rootEventId
			);

			// User A: notification badge appears on the space icon.
			const spaceIcon = page.locator('.server-gutter [data-testid="server-icon"]').first();
			const spaceBadge = spaceIcon.locator('..').getByTestId('server-notification-badge');
			await expect(spaceBadge).toBeVisible({ timeout: TIMEOUTS.REALTIME_EVENT });
			// The reply both mentions User A and replies to their message, so the
			// count badge reflects both pending notification records.
			await expect(spaceBadge).toHaveText('2');

			// Click the badge. The mention is on a thread message, so clicking should
			// land in #general with the thread pane open and the reply highlighted.
			await spaceBadge.click();

			// Should land on the thread URL (/chat/-/{spaceId}/{roomId}/{threadId}).
			await page.waitForURL(routes.patterns.anyThread);
			await expect(page.getByRole('heading', { name: '# general' })).toBeVisible();
			await roomPage.expectThreadPaneVisible();
		} finally {
			await ctxB.close();
		}
	});
});
