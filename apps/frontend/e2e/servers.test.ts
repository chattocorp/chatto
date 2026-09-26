import { test, expect } from './setup';
import { Code, ConnectError } from '@connectrpc/connect';
import { createAndLoginTestUser } from './fixtures/testUser';
import {
	startSecondServer,
	stopSecondServer,
	createUserOnRemote,
	connectRemoteInstance,
	getViewerOnRemote,
	getRoomOnRemote,
	postMessageOnRemote
} from './fixtures/multiServer';
import type { ServerInfo } from './fixtures/server';
import { TIMEOUTS } from './constants';
import * as routes from './routes';

test.describe('Server Directory (sidebar entry point)', () => {
	test('sidebar "+" opens the Server Directory in a dialog', async ({ page, chatPage }) => {
		await createAndLoginTestUser(page);
		await chatPage.goto();
		const chatURL = page.url();

		await page.getByTitle('Add Server').click();
		const dialog = page.getByRole('dialog', { name: 'Add Server' });
		await expect(dialog).toBeVisible({ timeout: TIMEOUTS.UI_FAST });
		await expect(dialog.getByLabel('Server URL')).toBeVisible();
		expect(page.url()).toBe(chatURL);

		await page.goBack();
		await expect(dialog).toBeHidden({ timeout: TIMEOUTS.UI_FAST });
		expect(page.url()).toBe(chatURL);
	});

	test('the Server Directory route shows the directory as a page', async ({ page }) => {
		await createAndLoginTestUser(page);
		await page.goto('/chat/servers');

		await expect(page.getByRole('heading', { name: 'Server Directory' })).toBeVisible({
			timeout: TIMEOUTS.UI_FAST
		});
		await expect(page.getByLabel('Server URL')).toBeVisible();
		await expect(page.getByRole('dialog')).toHaveCount(0);
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
		const pageErrors: string[] = [];
		page.on('pageerror', (error) => pageErrors.push(error.message));
		await createAndLoginTestUser(page);
		await chatPage.goto();

		const baseURL = remoteBaseURL(remoteServer!);
		const remoteHostname = new URL(baseURL).hostname;
		const remoteUser = await createUserOnRemote(baseURL, 'remoteuser-signout', 'password123');
		await connectRemoteInstance(page, { ...remoteServer!, baseURL }, remoteUser.userId);

		await page.waitForURL(new RegExp(`/chat/${remoteHostname.replace(/\./g, '\\.')}`));
		const previousToken = await page.evaluate((url) => {
			const servers = JSON.parse(localStorage.getItem('chatto:instances') ?? '[]') as Array<{
				id: string;
				url: string;
			}>;
			const server = servers.find((entry) => entry.url === url);
			if (!server) return null;
			const authentication = JSON.parse(
				localStorage.getItem(`chatto:i:${server.id}:authentication`) ?? 'null'
			) as { token?: string } | null;
			return authentication?.token ?? null;
		}, baseURL);
		expect(previousToken).toBeTruthy();
		const remoteSidebarIcon = page
			.locator(`[data-testid="server-icon"][href*="${remoteHostname}"]`)
			.first();
		await remoteSidebarIcon.click({ button: 'right' });
		await page.getByRole('menuitem', { name: 'Sign out of this server' }).click();

		await expect(page).toHaveURL(/\/chat\/-/);
		await expect(remoteSidebarIcon).toHaveAttribute('title', /Sign in to reconnect/, {
			timeout: TIMEOUTS.UI_STANDARD
		});
		try {
			await getViewerOnRemote(baseURL, previousToken!);
			throw new Error('signed-out remote bearer token remained valid');
		} catch (error) {
			expect(ConnectError.from(error).code).toBe(Code.Unauthenticated);
		}

		let openedPopups = 0;
		page.on('popup', () => openedPopups++);
		await remoteSidebarIcon.click();
		await expect(page.getByRole('menuitem', { name: 'Log in to this server' })).toBeVisible();
		expect(openedPopups).toBe(0);
		const popupPromise = page.waitForEvent('popup');
		await page.getByRole('menuitem', { name: 'Log in to this server' }).click();
		const popup = await popupPromise;
		expect(openedPopups).toBe(1);
		await popup.close();
		await expect(page.getByTitle('Sign out')).toBeVisible();
		expect(pageErrors).toEqual([]);
	});

	test('keeps a remote server live after signing out of the origin', async ({ page, chatPage }) => {
		const pageErrors: string[] = [];
		page.on('pageerror', (error) => pageErrors.push(error.message));

		await createAndLoginTestUser(page);
		await chatPage.goto();

		const baseURL = remoteBaseURL(remoteServer!);
		const remoteHostname = new URL(baseURL).hostname;
		const remoteViewer = await createUserOnRemote(
			baseURL,
			'remote-after-origin-signout',
			'password123'
		);
		const remoteSender = await createUserOnRemote(
			baseURL,
			'remote-after-origin-sender',
			'password123'
		);
		const generalRoomId = await getRoomOnRemote(baseURL, remoteViewer.token, 'general');
		await connectRemoteInstance(page, { ...remoteServer!, baseURL }, remoteViewer.userId);

		await page.goto(routes.serverOverview);
		await expect(page.getByRole('heading', { name: 'Overview' })).toBeVisible();
		await page.locator('[data-testid="server-icon"][href$="/chat/-"]').click({ button: 'right' });
		await page.getByRole('menuitem', { name: 'Sign out of this server' }).click();

		const remoteHostnameEsc = remoteHostname.replace(/\./g, '\\.');
		await page.waitForURL(new RegExp(`/chat/${remoteHostnameEsc}(/|$)`), {
			timeout: TIMEOUTS.COMPLEX_OPERATION
		});
		await chatPage.enterRoom('general');

		const liveMessage = 'remote delivery after origin sign-out';
		await postMessageOnRemote(baseURL, remoteSender.token, generalRoomId, liveMessage);
		await expect(page.getByText(liveMessage, { exact: true })).toBeVisible({
			timeout: TIMEOUTS.REALTIME_EVENT
		});
		expect(pageErrors).toEqual([]);
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

		const remoteSidebarIcon = page
			.locator(`[data-testid="server-icon"][href*="${remoteHostname}"]`)
			.first();
		await remoteSidebarIcon.click({ button: 'right' });
		await page.getByRole('menuitem', { name: 'Remove server' }).click();
		await expect(page.getByRole('dialog')).toBeVisible({ timeout: TIMEOUTS.UI_FAST });
		await page.getByRole('button', { name: 'Remove Server' }).click();

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
		await page.locator('[data-testid="server-icon"][href$="/chat/-"]').click({ button: 'right' });
		await expect(page.getByRole('menuitem', { name: 'Sign out of this server' })).toBeVisible();
		await expect(page.getByRole('menuitem', { name: 'Remove server' })).toHaveCount(0);
	});
});
