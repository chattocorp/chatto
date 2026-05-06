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
		// Set up home instance: create user (bootstrap space "E2E Test Server"
		// is already there, owned by e2eadmin).
		await createAndLoginTestUser(page);
		await chatPage.goto();
		await chatPage.createSpace();

		// Set up remote instance user.
		const remoteUser = await createUserOnRemote(remoteServer.baseURL, 'remoteuser1', 'password123');
		await createSpaceOnRemote(remoteServer.baseURL, remoteUser.token, 'unused');

		// Connect remote instance via the real Add-Server → OAuth → callback flow
		await connectRemoteInstance(page, remoteServer, remoteUser.userId);

		// Navigate to Browse Spaces
		const explorePage = new ExplorePage(page);
		await explorePage.goto();

		// Wait for the space directory to load
		await expect(page.locator('input[placeholder="Filter spaces..."]')).toBeVisible({
			timeout: TIMEOUTS.REALTIME_EVENT
		});

		// Issue #330 / ADR-027: every instance is seeded with one bootstrap
		// space ("E2E Test Server"). With two instances connected, we expect
		// two cards with that name in the flat list, no instance headers.
		await expect(page.locator('[data-testid="space-card"]')).toHaveCount(2);
		await expect(page.locator('[data-testid="instance-header"]')).toHaveCount(0);
	});


});
