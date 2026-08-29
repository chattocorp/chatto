import { Code, ConnectError } from '@connectrpc/connect';
import { test, expect } from './setup';
import { createAndLoginTestUser } from './fixtures/testUser';
import {
	startSecondServer,
	stopSecondServer,
	createUserOnRemote,
	getViewerOnRemote
} from './fixtures/multiServer';
import type { ServerInfo } from './fixtures/server';
import { TIMEOUTS } from './constants';

/**
 * Returns the remote server's hostname:port (e.g., "127.0.0.1:4050")
 * using 127.0.0.1 to give it a distinct hostname from the home server's "localhost".
 */
function remoteHostPort(server: ServerInfo): string {
	const url = new URL(server.baseURL);
	return `127.0.0.1:${url.port}`;
}

function remoteBaseURL(server: ServerInfo): string {
	return server.baseURL.replace('localhost', '127.0.0.1');
}

test.describe('OAuth Authorization Code + PKCE Flow', () => {
	let remoteServer: ServerInfo;

	test.beforeEach(async ({}, testInfo) => {
		remoteServer = await startSecondServer(testInfo);
	});

	test.afterEach(async ({}, testInfo) => {
		if (remoteServer) {
			await stopSecondServer(remoteServer, testInfo);
		}
	});

	test('full OAuth flow: add server via popup-based auth', async ({ page, chatPage }) => {
		// 1. Home instance: log in so the SPA works
		await createAndLoginTestUser(page);
		await chatPage.goto();

		// 2. Remote instance: create a user via API so we have credentials to use
		const baseURL = remoteBaseURL(remoteServer);
		await createUserOnRemote(baseURL, 'remoteuser', 'password123');

		// 3. Drive the Server Directory: open it from the sidebar `+` button,
		// fill the URL, find the server, then click the static "Join" button on
		// the result card. The page generates the PKCE verifier/challenge
		// and opens the remote's /oauth/authorize in a popup.
		const hostPort = remoteHostPort(remoteServer);
		await page.getByTitle('Add Server').click();
		await page.getByLabel('Server URL').fill(hostPort);
		await page.getByRole('button', { name: 'Find server' }).click();
		await expect(page.getByRole('button', { name: 'Join', exact: true })).toBeVisible({
			timeout: TIMEOUTS.REALTIME_EVENT
		});
		const popupPromise = page.waitForEvent('popup');
		await page.getByRole('button', { name: 'Join', exact: true }).click();
		const remoteAuthPage = await popupPromise;

		// 4. The popup should land on the remote instance's OAuth login page.
		// The flow: redirect to remote's /oauth/authorize → /login?redirect=/oauth/authorize
		const identifierInput = remoteAuthPage.locator('input[autocomplete="username"]');
		await expect(identifierInput).toBeVisible({ timeout: TIMEOUTS.REALTIME_EVENT });
		await expect(remoteAuthPage).toHaveURL(/127\.0\.0\.1.*\/login\?redirect=/);

		// 5. Fill in credentials for the remote user
		await identifierInput.fill('remoteuser');
		await remoteAuthPage
			.locator('input[autocomplete="current-password"]')
			.fill('password123');

		// 6. Submit the login form on the remote instance.
		// Backend detects pending OAuth flow and asks for consent before
		// generating the code.
		await remoteAuthPage.getByRole('button', { name: /Sign In/i }).click();
		await expect(remoteAuthPage).toHaveURL(/127\.0\.0\.1.*\/oauth\/consent/, {
			timeout: TIMEOUTS.REALTIME_EVENT
		});
		await expect(remoteAuthPage.getByText(/^localhost:\d+$/)).toBeVisible();
		await expect(remoteAuthPage.getByText(/instances\/callback/)).toHaveCount(0);
		const popupClosed = remoteAuthPage.waitForEvent('close');
		await remoteAuthPage.getByRole('button', { name: 'Allow Access' }).click();
		await popupClosed;

		// 7. Wait for the callback page to redirect into the newly-added
		// remote server's chat tree. Its URL segment is its hostname.
		await expect(page).toHaveURL(/\/chat\/127\.0\.0\.1(\/|$)/, {
			timeout: TIMEOUTS.COMPLEX_OPERATION
		});

		// 8. Verify the remote instance was registered in localStorage
		const instances = await page.evaluate(() => {
			return JSON.parse(localStorage.getItem('chatto:instances') || '[]');
		});

		const remoteInstance = instances.find((i: { url: string }) =>
			i.url.includes('127.0.0.1')
		);
		expect(remoteInstance).toBeTruthy();
		expect(remoteInstance.token).toBeTruthy();
		expect(remoteInstance.userId).toBeTruthy();
		expect(remoteInstance.userLogin).toBe('remoteuser');

		// 9. Forget the local client-side registration and connect the same
		// remote again. The remote user session and remembered OAuth consent
		// remain on the remote server, so this second authorize flow should skip
		// both login and consent and return directly to the callback.
		await page.evaluate(() => {
			const instances = JSON.parse(localStorage.getItem('chatto:instances') || '[]');
			localStorage.setItem(
				'chatto:instances',
				JSON.stringify(instances.filter((i: { url: string }) => !i.url.includes('127.0.0.1')))
			);
		});
		await page.goto('/chat/-');
		await page.getByTitle('Add Server').click();
		await page.getByLabel('Server URL').fill(hostPort);
		await page.getByRole('button', { name: 'Find server' }).click();
		await expect(page.getByRole('button', { name: 'Join', exact: true })).toBeVisible({
			timeout: TIMEOUTS.REALTIME_EVENT
		});
		const secondPopupPromise = page.waitForEvent('popup');
		const secondPopupClosed = secondPopupPromise.then((popup) => popup.waitForEvent('close'));
		await page.getByRole('button', { name: 'Join', exact: true }).click();
		await secondPopupClosed;
		await expect(page).toHaveURL(/\/chat\/127\.0\.0\.1(\/|$)/, {
			timeout: TIMEOUTS.COMPLEX_OPERATION
		});
		await expect(page).not.toHaveURL(/\/oauth\/consent/);

		// 10. Remote multi-server access must be carried by the stored bearer
		// token, not by ambient browser cookies from the remote OAuth login.
		await page.context().clearCookies();
		await page.reload();
		await expect(page).toHaveURL(/\/chat\/127\.0\.0\.1(\/|$)/, {
			timeout: TIMEOUTS.COMPLEX_OPERATION
		});
		await expect(page.getByTitle('Sign out')).toBeVisible();
	});

	test('token exchange rejects invalid code_verifier', async () => {
		const baseURL = remoteBaseURL(remoteServer);
		await createUserOnRemote(baseURL, 'codeuser', 'password123');

		// Test the /oauth/token endpoint directly with a bogus code
		const tokenResponse = await fetch(`${baseURL}/oauth/token`, {
			method: 'POST',
			headers: { 'Content-Type': 'application/json' },
			body: JSON.stringify({
				grant_type: 'authorization_code',
				client_id: 'https://client.example/oauth/client-metadata.json',
				code: 'cht_ACnonexistent12',
				code_verifier: 'wrong-verifier',
				redirect_uri: 'https://example.com/callback'
			})
		});

		expect(tokenResponse.status).toBe(400);
		const errorData = await tokenResponse.json();
		expect(errorData.error).toBe('invalid_grant');
	});

	test('remote admin can see and block the authorized client, revoking access and reconnects', async ({
		page,
		chatPage,
		browser,
		serverURL
	}) => {
		await createAndLoginTestUser(page);
		await chatPage.goto();

		const baseURL = remoteBaseURL(remoteServer);
		await createUserOnRemote(baseURL, 'blockedremoteuser', 'password123');
		const hostPort = remoteHostPort(remoteServer);

		await page.getByTitle('Add Server').click();
		await page.getByLabel('Server URL').fill(hostPort);
		await page.getByRole('button', { name: 'Find server' }).click();
		await expect(page.getByRole('button', { name: 'Join', exact: true })).toBeVisible({
			timeout: TIMEOUTS.REALTIME_EVENT
		});
		const popupPromise = page.waitForEvent('popup');
		await page.getByRole('button', { name: 'Join', exact: true }).click();
		const remoteAuthPage = await popupPromise;
		await expect(remoteAuthPage.locator('input[autocomplete="username"]')).toBeVisible({
			timeout: TIMEOUTS.REALTIME_EVENT
		});
		await remoteAuthPage.locator('input[autocomplete="username"]').fill('blockedremoteuser');
		await remoteAuthPage.locator('input[autocomplete="current-password"]').fill('password123');
		await remoteAuthPage.getByRole('button', { name: 'Sign In' }).click();
		await expect(remoteAuthPage).toHaveURL(/\/oauth\/consent/, {
			timeout: TIMEOUTS.REALTIME_EVENT
		});
		const popupClosed = remoteAuthPage.waitForEvent('close');
		await remoteAuthPage.getByRole('button', { name: 'Allow Access' }).click();
		await popupClosed;
		await expect(page).toHaveURL(/\/chat\/127\.0\.0\.1(\/|$)/, {
			timeout: TIMEOUTS.COMPLEX_OPERATION
		});

		const remoteToken = await page.evaluate(() => {
			const instances = JSON.parse(localStorage.getItem('chatto:instances') || '[]') as Array<{
				url: string;
				token?: string;
			}>;
			return instances.find((instance) => instance.url.includes('127.0.0.1'))?.token ?? null;
		});
		expect(remoteToken).toBeTruthy();

		const adminContext = await browser.newContext();
		try {
			const adminPage = await adminContext.newPage();
			await adminPage.goto(`${baseURL}/login`);
			await adminPage.locator('input[autocomplete="username"]').fill('e2eadmin');
			await adminPage
				.locator('input[autocomplete="current-password"]')
				.fill('adminpassword123');
			await adminPage.getByRole('button', { name: 'Sign In' }).click();
			await expect(adminPage).toHaveURL(/\/chat(\/|$)/, {
				timeout: TIMEOUTS.REALTIME_EVENT
			});
			await adminPage.goto(`${baseURL}/chat/-/manage/server/security`);

			const clientID = `${new URL(serverURL).origin}/oauth/frontend-client-metadata.json`;
			const clientRow = adminPage.getByRole('row').filter({ hasText: 'Chatto Web' });
			await expect(clientRow).toContainText(clientID, { timeout: TIMEOUTS.COMPLEX_OPERATION });
			await expect(clientRow.getByRole('cell', { name: '1', exact: true })).toBeVisible();

			const policy = clientRow.getByRole('combobox', { name: 'Policy for Chatto Web' });
			const updateResponse = adminPage.waitForResponse(
				(response) =>
					response.url().includes('AdminOAuthClientService/UpdateOAuthClientPolicy') &&
					response.request().method() === 'POST'
			);
			await policy.selectOption('blocked');
			expect((await updateResponse).ok()).toBeTruthy();
			await expect(policy).toHaveValue('blocked');
		} finally {
			await adminContext.close();
		}

		// The remote connection was already live before the policy change. The
		// server must terminate that exact OAuth client's socket so the bundled
		// frontend observes the revoked credential without a reload.
		await expect(
			page.getByRole('status').filter({ hasText: 'E2E Test Server needs sign-in' })
		).toBeVisible({ timeout: TIMEOUTS.REALTIME_EVENT });
		await expect(page.getByRole('button', { name: 'Reconnect', exact: true })).toBeVisible();

		try {
			await getViewerOnRemote(baseURL, remoteToken!);
			throw new Error('blocked OAuth access token remained valid');
		} catch (error) {
			expect(ConnectError.from(error).code).toBe(Code.Unauthenticated);
		}

		await page.evaluate(() => {
			const instances = JSON.parse(localStorage.getItem('chatto:instances') || '[]') as Array<{
				url: string;
			}>;
			localStorage.setItem(
				'chatto:instances',
				JSON.stringify(instances.filter((instance) => !instance.url.includes('127.0.0.1')))
			);
		});
		await page.goto('/chat/-');
		await page.getByTitle('Add Server').click();
		await page.getByLabel('Server URL').fill(hostPort);
		await page.getByRole('button', { name: 'Find server' }).click();
		await expect(page.getByRole('button', { name: 'Join', exact: true })).toBeVisible({
			timeout: TIMEOUTS.REALTIME_EVENT
		});
		const blockedPopupPromise = page.waitForEvent('popup');
		await page.getByRole('button', { name: 'Join', exact: true }).click();
		const blockedPopup = await blockedPopupPromise;
		await expect(blockedPopup.locator('body')).toContainText('invalid_client', {
			timeout: TIMEOUTS.REALTIME_EVENT
		});
		await expect(blockedPopup.locator('body')).toContainText('blocked by this server');
		await blockedPopup.close();
	});
});
