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
	let remoteServer: ServerInfo | undefined;

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

	test('Leave Server icon is hidden on remote instances', async ({ page, chatPage }) => {
		await createAndLoginTestUser(page);
		await chatPage.goto();

		const baseURL = remoteBaseURL(remoteServer!);
		const remoteHostname = new URL(baseURL).hostname;
		const remoteUser = await createUserOnRemote(baseURL, 'remoteuser-hidden', 'password123');
		await connectRemoteInstance(page, { ...remoteServer, baseURL }, remoteUser.userId);

		// The remote should have been added to the sidebar.
		const remoteSidebarIcon = page
			.locator(`[data-testid="server-icon"][href*="${remoteHostname}"]`)
			.first();
		await expect(remoteSidebarIcon).toBeVisible({ timeout: TIMEOUTS.REALTIME_EVENT });

		// Navigate into the remote server.
		await remoteSidebarIcon.click();
		await page.waitForURL(new RegExp(`/chat/${remoteHostname.replace(/\./g, '\\.')}`));

		// The leave-server affordance was removed from the server header.
		await expect(page.getByTitle('Leave server')).not.toBeVisible();
	});

	test('can sign out of only the selected remote server', async ({ page, chatPage }) => {
		await createAndLoginTestUser(page);
		await chatPage.goto();

		const baseURL = remoteBaseURL(remoteServer!);
		const remoteHostname = new URL(baseURL).hostname;
		const remoteUser = await createUserOnRemote(baseURL, 'remoteuser-signout', 'password123');
		await connectRemoteInstance(page, { ...remoteServer!, baseURL }, remoteUser.userId);

		await page.waitForURL(new RegExp(`/chat/${remoteHostname.replace(/\./g, '\\.')}`));
		await page.getByTitle('Sign out').click();
		await expect(page.getByRole('dialog')).toBeVisible({ timeout: TIMEOUTS.UI_FAST });
		await page.getByRole('button', { name: 'Current Server' }).click();

		await expect(page).toHaveURL(/\/chat\/-/);
		await expect(
			page.locator(`[data-testid="server-icon"][href*="${remoteHostname}"]`)
		).not.toBeVisible({ timeout: TIMEOUTS.UI_STANDARD });
		await expect(page.getByTitle('Sign out')).toBeVisible();
	});

	test('can remove the selected remote server when it is unreachable', async (
		{ page, chatPage },
		testInfo
	) => {
		await createAndLoginTestUser(page);
		await chatPage.goto();

		const baseURL = remoteBaseURL(remoteServer!);
		const remoteHostname = new URL(baseURL).hostname;
		const remoteUser = await createUserOnRemote(baseURL, 'remoteuser-dead', 'password123');
		await connectRemoteInstance(page, { ...remoteServer!, baseURL }, remoteUser.userId);

		await page.waitForURL(new RegExp(`/chat/${remoteHostname.replace(/\./g, '\\.')}`));
		await stopSecondServer(remoteServer!, testInfo);
		remoteServer = undefined;

		await page.getByTitle('Sign out').click();
		await expect(page.getByRole('dialog')).toBeVisible({ timeout: TIMEOUTS.UI_FAST });
		await page.getByRole('button', { name: 'Current Server' }).click();

		await expect(page).toHaveURL(/\/chat\/-/);
		await expect(
			page.locator(`[data-testid="server-icon"][href*="${remoteHostname}"]`)
		).not.toBeVisible({ timeout: TIMEOUTS.UI_STANDARD });
		await expect(page.getByTitle('Sign out')).toBeVisible();
	});
});

test.describe('Origin Server', () => {
	test('Leave Server icon is hidden on the origin instance', async ({ page, chatPage }) => {
		await createAndLoginTestUser(page);
		await chatPage.goto();

		// On origin: the leave-server affordance should not be present.
		await expect(page.getByTitle('Leave server')).not.toBeVisible();
	});
});
