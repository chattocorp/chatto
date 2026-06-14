import { test, expect } from './setup';
import { createAndLoginTestUser } from './fixtures/testUser';
import {
	startSecondServer,
	stopSecondServer,
	createUserOnRemote,
	connectRemoteInstance
} from './fixtures/multiServer';
import type { ServerInfo } from './fixtures/server';
import { TIMEOUTS } from './constants';

test.describe('Add Server (sidebar entry point)', () => {
	test('sidebar "+" opens the Add Server dialog', async ({ page, chatPage }) => {
		await createAndLoginTestUser(page);
		await chatPage.goto();

		await page.getByTitle('Add Server').click();
		await expect(page.getByRole('heading', { name: 'Add Server' })).toBeVisible({
			timeout: TIMEOUTS.UI_FAST
		});
		await expect(page.getByLabel('Server URL')).toBeVisible();
	});
});

test.describe('Leave Server', () => {
	let remoteServer: ServerInfo;

	test.beforeEach(async ({}, testInfo) => {
		remoteServer = await startSecondServer(testInfo);
	});

	test.afterEach(async ({}, testInfo) => {
		if (remoteServer) {
			await stopSecondServer(remoteServer, testInfo);
		}
	});

	function remoteBaseURL(server: ServerInfo): string {
		return server.baseURL.replace('localhost', '127.0.0.1');
	}

	test('leaving a remote server unregisters it from the sidebar', async ({ page, chatPage }) => {
		await createAndLoginTestUser(page);
		await chatPage.goto();

		const baseURL = remoteBaseURL(remoteServer);
		const remoteHostname = new URL(baseURL).hostname;
		const remoteUser = await createUserOnRemote(baseURL, 'remoteuser-leave', 'password123');
		await connectRemoteInstance(page, { ...remoteServer, baseURL }, remoteUser.userId);

		// The remote should have been added to the sidebar.
		const remoteSidebarIcon = page
			.locator(`[data-testid="server-icon"][href*="${remoteHostname}"]`)
			.first();
		await expect(remoteSidebarIcon).toBeVisible({ timeout: TIMEOUTS.REALTIME_EVENT });

		// Navigate into the remote server.
		await remoteSidebarIcon.click();
		await page.waitForURL(new RegExp(`/chat/${remoteHostname.replace(/\./g, '\\.')}`));

		// Click the Leave Server icon in the space header.
		await page.getByTitle('Leave server').click();

		// Confirmation dialog should appear with Leave Server copy.
		const dialog = page.getByRole('dialog');
		await expect(dialog).toBeVisible({ timeout: TIMEOUTS.UI_FAST });
		await expect(dialog.getByRole('heading', { name: 'Leave Server' })).toBeVisible();

		// Confirm.
		await dialog.getByRole('button', { name: 'Leave Server' }).click();

		// Should land back on the origin instance.
		await page.waitForURL(/\/chat\/-/, { timeout: TIMEOUTS.UI_STANDARD });

		// Remote should no longer be in the sidebar or localStorage.
		await expect(remoteSidebarIcon).not.toBeVisible({ timeout: TIMEOUTS.UI_STANDARD });

		const stored = await page.evaluate(() =>
			JSON.parse(localStorage.getItem('chatto:instances') ?? '[]')
		);
		expect(stored.find((i: { url: string }) => i.url.includes(remoteHostname))).toBeUndefined();
	});

	test('cancelling Leave Server keeps the instance', async ({ page, chatPage }) => {
		await createAndLoginTestUser(page);
		await chatPage.goto();

		const baseURL = remoteBaseURL(remoteServer);
		const remoteHostname = new URL(baseURL).hostname;
		const remoteUser = await createUserOnRemote(baseURL, 'remoteuser-cancel', 'password123');
		await connectRemoteInstance(page, { ...remoteServer, baseURL }, remoteUser.userId);

		const remoteSidebarIcon = page
			.locator(`[data-testid="server-icon"][href*="${remoteHostname}"]`)
			.first();
		await expect(remoteSidebarIcon).toBeVisible({ timeout: TIMEOUTS.REALTIME_EVENT });
		await remoteSidebarIcon.click();
		await page.waitForURL(new RegExp(`/chat/${remoteHostname.replace(/\./g, '\\.')}`));

		await page.getByTitle('Leave server').click();
		const dialog = page.getByRole('dialog');
		await expect(dialog).toBeVisible({ timeout: TIMEOUTS.UI_FAST });
		await dialog.getByRole('button', { name: 'Cancel' }).click();
		await expect(dialog).not.toBeVisible();

		// Still in the remote server, still in the sidebar.
		await expect(remoteSidebarIcon).toBeVisible();
	});

	test('Leave Server icon is hidden on the origin instance', async ({ page, chatPage }) => {
		await createAndLoginTestUser(page);
		await chatPage.goto();

		// On origin: the leave-server affordance should not be present.
		await expect(page.getByTitle('Leave server')).not.toBeVisible();
	});
});
