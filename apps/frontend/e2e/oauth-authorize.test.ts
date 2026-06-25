import { test, expect } from './setup';
import { createAndLoginTestUser } from './fixtures/testUser';
import {
	startSecondServer,
	stopSecondServer,
	createUserOnRemote
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

	test('full OAuth flow: add server via redirect-based auth', async ({ page, chatPage }) => {
		// 1. Home instance: log in so the SPA works
		await createAndLoginTestUser(page);
		await chatPage.goto();

		// 2. Remote instance: create a user via API so we have credentials to use
		const baseURL = remoteBaseURL(remoteServer);
		await createUserOnRemote(baseURL, 'remoteuser', 'password123');

		// 3. Drive the Add-Server dialog: open from the sidebar `+` button,
		// fill the URL, click Connect to probe, then click the static "Sign in"
		// button on the preview. The dialog generates the PKCE verifier/challenge
		// and redirects to the remote's /oauth/authorize.
		const hostPort = remoteHostPort(remoteServer);
		await page.getByTitle('Add Server').click();
		await page.getByLabel('Server URL').fill(hostPort);
		await page.getByRole('button', { name: 'Connect' }).click();
		await expect(page.getByRole('button', { name: 'Sign in', exact: true })).toBeVisible({
			timeout: TIMEOUTS.REALTIME_EVENT
		});
		await page.getByRole('button', { name: 'Sign in', exact: true }).click();

		// 4. We should land on the remote instance's OAuth login page.
		// The flow: redirect to remote's /oauth/authorize → /login?redirect=/oauth/authorize
		const identifierInput = page.locator('input[autocomplete="username"]');
		await expect(identifierInput).toBeVisible({ timeout: TIMEOUTS.REALTIME_EVENT });
		await expect(page).toHaveURL(/127\.0\.0\.1.*\/login\?redirect=/);

		// 5. Fill in credentials for the remote user
		await identifierInput.fill('remoteuser');
		await page.locator('input[autocomplete="current-password"]').fill('password123');

		// 6. Submit the login form on the remote instance.
		// Backend detects pending OAuth flow and asks for consent before
		// generating the code.
		await page.getByRole('button', { name: /Sign In/i }).click();
		await expect(page).toHaveURL(/127\.0\.0\.1.*\/oauth\/consent/, {
			timeout: TIMEOUTS.REALTIME_EVENT
		});
		await expect(page.getByText(/^localhost:\d+$/)).toBeVisible();
		await expect(page.getByText(/instances\/callback/)).toHaveCount(0);
		await page.getByRole('button', { name: 'Allow Access' }).click();

		// 7. Wait for the callback page to complete and redirect into the
		// newly-added remote instance's chat tree (`/chat/127.0.0.1/...`).
		// Post-PR(a) there is no `/chat/spaces` landing — the callback drops
		// the user directly into the instance they just connected, whose URL
		// segment is its hostname (see `serverIdToSegment`).
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
		await page.getByRole('button', { name: 'Connect' }).click();
		await expect(page.getByRole('button', { name: 'Sign in', exact: true })).toBeVisible({
			timeout: TIMEOUTS.REALTIME_EVENT
		});
		await page.getByRole('button', { name: 'Sign in', exact: true }).click();
		await expect(page).toHaveURL(/\/chat\/127\.0\.0\.1(\/|$)/, {
			timeout: TIMEOUTS.COMPLEX_OPERATION
		});
		await expect(page).not.toHaveURL(/\/oauth\/consent/);
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
				code: 'cht_ACnonexistent12',
				code_verifier: 'wrong-verifier',
				redirect_uri: 'https://example.com/callback'
			})
		});

		expect(tokenResponse.status).toBe(400);
		const errorData = await tokenResponse.json();
		expect(errorData.error).toBe('invalid_grant');
	});

});
