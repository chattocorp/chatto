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
import * as routes from './routes';

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

	// FIXME: multi-space test — creates multiple spaces per instance which
	// the URL collapse can't represent. Re-enable in next phase-2 PR.
	test.skip('search filters across all instances', async ({ page, chatPage }) => {
		// Set up home instance
		await createAndLoginTestUser(page);
		await chatPage.goto();
		await chatPage.createSpace('Alpha Home');
		await chatPage.createSpace('Beta Home');

		// Set up remote instance
		const remoteUser = await createUserOnRemote(remoteServer.baseURL, 'remoteuser2', 'password123');
		await createSpaceOnRemote(remoteServer.baseURL, remoteUser.token, 'Alpha Remote');
		await createSpaceOnRemote(remoteServer.baseURL, remoteUser.token, 'Gamma Remote');

		// Connect remote instance via the real /instances/add → OAuth → callback flow
		await connectRemoteInstance(page, remoteServer, remoteUser.userId);

		const explorePage = new ExplorePage(page);
		await explorePage.goto();

		// Wait for the space directory to load (search input appears after loading)
		await expect(page.locator('input[placeholder="Filter spaces..."]')).toBeVisible({
			timeout: TIMEOUTS.REALTIME_EVENT
		});

		// All spaces should be visible initially
		await explorePage.expectSpaceVisible('Alpha Home');
		await explorePage.expectSpaceVisible('Beta Home');
		await explorePage.expectSpaceVisible('Alpha Remote');
		await explorePage.expectSpaceVisible('Gamma Remote');

		// Filter by "Alpha" — should show spaces from both instances
		await page.locator('input[placeholder="Filter spaces..."]').fill('Alpha');
		await explorePage.expectSpaceVisible('Alpha Home');
		await explorePage.expectSpaceVisible('Alpha Remote');
		await explorePage.expectSpaceNotVisible('Beta Home');
		await explorePage.expectSpaceNotVisible('Gamma Remote');
	});

	// FIXME: post-URL-collapse, joining a non-primary space from Browse
	// Spaces can't navigate to it (the URL has no spaceId segment).
	// Re-enable when the next phase-2 PR narrows discovery to the primary
	// space and removes the "join arbitrary space" flow.
	test.skip('joining a space on remote instance navigates to it', async ({ page, chatPage }) => {
		// Set up home instance (need a user logged in)
		await createAndLoginTestUser(page);
		await chatPage.goto();

		// Set up remote instance: one user creates the space, another user will browse it
		const remoteOwner = await createUserOnRemote(remoteServer.baseURL, 'remoteowner3', 'password123');
		await createSpaceOnRemote(remoteServer.baseURL, remoteOwner.token, 'Join Me Remote');
		const remoteBrowser = await createUserOnRemote(remoteServer.baseURL, 'remotebrowser3', 'password123');

		// Connect remote instance with the browser user (who hasn't joined the space)
		await connectRemoteInstance(page, remoteServer, remoteBrowser.userId);

		const explorePage = new ExplorePage(page);
		await explorePage.goto();

		// Wait for the space directory to load
		await expect(page.locator('input[placeholder="Filter spaces..."]')).toBeVisible({
			timeout: TIMEOUTS.REALTIME_EVENT
		});

		// The remote space should be joinable
		await explorePage.expectSpaceJoinable('Join Me Remote');

		// Join the remote space
		await explorePage.joinSpace('Join Me Remote');

		// Should navigate to the space
		await page.waitForURL(routes.patterns.anySpace, { timeout: TIMEOUTS.UI_STANDARD });
	});
});
