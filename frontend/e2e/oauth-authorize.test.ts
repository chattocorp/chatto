import { test, expect } from './setup';
import { createAndLoginTestUser } from './fixtures/testUser';
import {
	startSecondServer,
	stopSecondServer,
	createUserOnRemote
} from './fixtures/multiInstance';
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

	test('full OAuth flow: add instance via redirect-based auth', async ({ page, chatPage }) => {
		// 1. Home instance: log in so the SPA works
		await createAndLoginTestUser(page);
		await chatPage.goto();

		// 2. Remote instance: create a user via API so we have credentials to use
		const baseURL = remoteBaseURL(remoteServer);
		await createUserOnRemote(baseURL, 'remoteuser', 'password123');

		// 3. Navigate to the add-instance [hostname] page.
		// The page probes /api/instance, sees authorizeUrl, and immediately starts
		// the OAuth PKCE flow (generates verifier/challenge, redirects to remote).
		const hostPort = remoteHostPort(remoteServer);
		await page.goto(`/instances/add/${hostPort}`);

		// 4. We should land on the remote instance's OAuth login page.
		// The flow: probe → auto-redirect to remote's /oauth/authorize → /login?redirect=/oauth/authorize
		const identifierInput = page.locator('input[autocomplete="username"]');
		await expect(identifierInput).toBeVisible({ timeout: TIMEOUTS.REALTIME_EVENT });
		await expect(page).toHaveURL(/127\.0\.0\.1.*\/login\?redirect=/);

		// 5. Fill in credentials for the remote user
		await identifierInput.fill('remoteuser');
		await page.locator('input[autocomplete="current-password"]').fill('password123');

		// 6. Submit the login form on the remote instance.
		// Backend detects pending OAuth flow → generates auth code → redirects back
		// to the home instance's /instances/callback → exchanges code for token.
		await page.getByRole('button', { name: /Sign In/i }).click();

		// 7. Wait for the callback page to complete and redirect to /chat/spaces.
		await expect(page).toHaveURL(/\/chat\/spaces/, { timeout: TIMEOUTS.COMPLEX_OPERATION });

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

	test('OAuth auto-redirect skips interstitial', async ({ page, chatPage }) => {
		// Verify that when authorizeUrl is present, the page auto-redirects
		// to the remote's login page without showing an intermediate screen.
		await createAndLoginTestUser(page);
		await chatPage.goto();

		const baseURL = remoteBaseURL(remoteServer);
		await createUserOnRemote(baseURL, 'autouser', 'password123');

		const hostPort = remoteHostPort(remoteServer);
		await page.goto(`/instances/add/${hostPort}`);

		// Should go directly to the remote's login page (no "Sign in on" button)
		await expect(page).toHaveURL(/127\.0\.0\.1.*\/login\?redirect=/, {
			timeout: TIMEOUTS.REALTIME_EVENT
		});
	});
});
