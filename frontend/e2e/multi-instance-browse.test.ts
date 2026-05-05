import { test, expect } from './setup';
import { createAndLoginTestUser } from './fixtures/testUser';
import {
	startSecondServer,
	stopSecondServer,
	createUserOnRemote,
	createSpaceOnRemote,
	connectRemoteInstance
} from './fixtures/multiInstance';
import { ExplorePage } from './pages/ExplorePage';
import type { ServerInfo } from './fixtures/server';
import { TIMEOUTS } from './constants';

test.describe('Multi-Instance Browse Spaces', () => {
	let remoteServer: ServerInfo;

	test.beforeEach(async ({}, testInfo) => {
		remoteServer = await startSecondServer(testInfo);
	});

	test.afterEach(async ({}, testInfo) => {
		if (remoteServer) {
			await stopSecondServer(remoteServer, testInfo);
		}
	});

	test('shows spaces from multiple instances in a single list', async ({ page, chatPage }) => {
		// Set up home instance: create user and a space
		await createAndLoginTestUser(page);
		await chatPage.goto();
		await chatPage.createSpace('Home Space');

		// Set up remote instance: create user and a space
		const remoteUser = await createUserOnRemote(remoteServer.baseURL, 'remoteuser1', 'password123');
		await createSpaceOnRemote(remoteServer.baseURL, remoteUser.token, 'Remote Space');

		// Connect remote instance via the real /instances/add → OAuth → callback flow
		await connectRemoteInstance(page, remoteServer, remoteUser.userId);

		// Navigate to Browse Spaces
		const explorePage = new ExplorePage(page);
		await explorePage.goto();

		// Wait for the space directory to load
		await expect(page.locator('input[placeholder="Filter spaces..."]')).toBeVisible({
			timeout: TIMEOUTS.REALTIME_EVENT
		});

		// Should see spaces from both instances in one flat list (no instance headers)
		await explorePage.expectSpaceVisible('Home Space');
		await explorePage.expectSpaceVisible('Remote Space');
		await expect(page.locator('[data-testid="instance-header"]')).toHaveCount(0);
	});


});
