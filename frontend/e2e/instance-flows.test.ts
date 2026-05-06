import type { Page } from '@playwright/test';
import { test, expect } from './setup';
import { createAndLoginTestUser } from './fixtures/testUser';
import {
	startSecondServer,
	stopSecondServer,
	createUserOnRemote
} from './fixtures/multiInstance';
import type { ServerInfo } from './fixtures/server';
import * as routes from './routes';
import { TIMEOUTS } from './constants';

test.describe('Landing Page', () => {
	test('unauthenticated user is redirected to /login', async ({
		browser
	}) => {
		// Fresh context — no localStorage, no cookies
		const context = await browser.newContext();
		const page = await context.newPage();

		await page.goto('/');
		await page.waitForURL(routes.login);

		await context.close();
	});

	test('unauthenticated user does not see sidebar nav icons', async ({ browser }) => {
		const context = await browser.newContext();
		const page = await context.newPage();

		await page.goto(routes.login);

		// Sidebar nav icons for DMs, Browse Spaces, and Create Space should not be present
		await expect(page.getByTestId('dm-icon')).not.toBeVisible();
		await expect(page.getByRole('link', { name: 'Explore Spaces' })).not.toBeVisible();
		await expect(page.getByRole('link', { name: 'Create Space' })).not.toBeVisible();

		await context.close();
	});

	test('authenticated user with instances is redirected into the server', async ({
		page
	}) => {
		await createAndLoginTestUser(page);
		await page.goto('/');

		// Issue #330 / ADR-027: with auto-join, every signed-in user lands in
		// their server's first room (not the Browse Spaces directory).
		await page.waitForURL(routes.patterns.anyRoom);
	});
});

test.describe('Origin Auto-Registration', () => {
	test('origin instance is registered in localStorage after probe', async ({ page, chatPage }) => {
		await createAndLoginTestUser(page);
		await chatPage.goto();

		await expect(async () => {
			const stored = await page.evaluate(() =>
				JSON.parse(localStorage.getItem('chatto:instances') ?? '[]')
			);
			expect(stored.length).toBeGreaterThanOrEqual(1);
		}).toPass({ timeout: TIMEOUTS.UI_STANDARD });
	});

	test('origin re-registers after reload if user is authenticated', async ({ page, chatPage }) => {
		await createAndLoginTestUser(page);
		await chatPage.goto();

		// Clear localStorage to remove all instances
		await page.evaluate(() => localStorage.removeItem('chatto:instances'));
		await page.reload();
		await page.waitForLoadState('networkidle');

		// Origin should be re-registered via probeOrigin — give it time
		await expect(async () => {
			const stored = await page.evaluate(() =>
				JSON.parse(localStorage.getItem('chatto:instances') ?? '[]')
			);
			expect(stored.length).toBeGreaterThanOrEqual(1);
		}).toPass({ timeout: TIMEOUTS.UI_STANDARD });
	});
});

test.describe('Add Server Dialog', () => {
	test('shows URL input for connecting to remote servers', async ({ page, chatPage }) => {
		await createAndLoginTestUser(page);
		await chatPage.goto();
		await page.getByTitle('Add Server').click();

		// URL input should be visible
		await expect(page.getByLabel('Server URL')).toBeVisible();
		await expect(page.getByRole('button', { name: 'Connect' })).toBeVisible();
	});

	test('shows error for invalid server URL', async ({ page, chatPage }) => {
		await createAndLoginTestUser(page);
		await chatPage.goto();
		await page.getByTitle('Add Server').click();

		// Enter an unreachable URL
		await page.getByLabel('Server URL').fill('https://nonexistent.invalid');
		await page.getByRole('button', { name: 'Connect' }).click();

		// Should show a connection error
		await expect(page.getByText('Could not connect')).toBeVisible({
			timeout: TIMEOUTS.REALTIME_EVENT
		});
	});
});

test.describe('Add Server - Remote Auth Flow', () => {
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

	/**
	 * Drive the dialog up to (but not through) the OAuth redirect: open it
	 * from the sidebar `+` button, fill the URL, click Connect to probe, and
	 * click the static "Sign in" button on the preview.
	 */
	async function driveAddServerToOAuth(page: Page, hostname: string): Promise<void> {
		await page.getByTitle('Add Server').click();
		await page.getByLabel('Server URL').fill(hostname);
		await page.getByRole('button', { name: 'Connect' }).click();
		await expect(page.getByRole('button', { name: 'Sign in', exact: true })).toBeVisible({
			timeout: TIMEOUTS.REALTIME_EVENT
		});
		await page.getByRole('button', { name: 'Sign in', exact: true }).click();
	}

	test('previewing a valid remote server then continuing redirects to remote OAuth login', async ({ page, chatPage }) => {
		await createAndLoginTestUser(page);
		await chatPage.goto();

		const baseURL = remoteBaseURL(remoteServer);
		const hostname = new URL(baseURL).host;

		await driveAddServerToOAuth(page, hostname);

		// Should redirect to the remote's OAuth login page
		await expect(page).toHaveURL(/\/login\?redirect=/, {
			timeout: TIMEOUTS.REALTIME_EVENT
		});
		await expect(page.locator('input[autocomplete="username"]')).toBeVisible();
	});

	test('signing in to remote server via OAuth flow adds it to sidebar', async ({ page, chatPage }) => {
		await createAndLoginTestUser(page);
		await chatPage.goto();

		const baseURL = remoteBaseURL(remoteServer);
		const hostname = new URL(baseURL).host;
		const remoteHostname = new URL(baseURL).hostname;
		await createUserOnRemote(baseURL, 'remoteuser', 'password123');

		await driveAddServerToOAuth(page, hostname);
		await expect(page).toHaveURL(/\/login\?redirect=/, {
			timeout: TIMEOUTS.REALTIME_EVENT
		});

		// Fill in credentials on the remote's login page
		await page.locator('input[autocomplete="username"]').fill('remoteuser');
		await page.locator('input[autocomplete="current-password"]').fill('password123');
		await page.getByRole('button', { name: 'Sign In' }).click();

		// Should redirect back to home and end up on browse spaces
		await page.waitForURL(routes.spaces, { timeout: TIMEOUTS.COMPLEX_OPERATION });

		// The remote instance should now appear in the sidebar.
		await expect(
			page.locator(`[data-testid="space-icon"][href*="${remoteHostname}"]`).first()
		).toBeVisible({ timeout: TIMEOUTS.UI_STANDARD });
	});

	test('invalid credentials show error on remote OAuth login page', async ({ page, chatPage }) => {
		await createAndLoginTestUser(page);
		await chatPage.goto();

		const baseURL = remoteBaseURL(remoteServer);
		const hostname = new URL(baseURL).host;

		await driveAddServerToOAuth(page, hostname);
		await expect(page).toHaveURL(/\/login\?redirect=/, {
			timeout: TIMEOUTS.REALTIME_EVENT
		});

		await page.locator('input[autocomplete="username"]').fill('wronguser');
		await page.locator('input[autocomplete="current-password"]').fill('wrongpassword');
		await page.getByRole('button', { name: 'Sign In' }).click();

		// Should show an auth error on the remote's login page
		await expect(page.getByText(/failed|invalid|not found/i)).toBeVisible({
			timeout: TIMEOUTS.UI_STANDARD
		});

		// Should stay on the remote's OAuth login page
		await expect(page).toHaveURL(/\/login\?redirect=/);
	});
});

test.describe('Sign Out', () => {
	test('sign out removes all instances and redirects to landing page', async ({
		page,
		chatPage
	}) => {
		await createAndLoginTestUser(page);
		await chatPage.goto();

		// Click the sign out button in the header
		await page.getByTitle('Sign Out').click();

		// Confirmation dialog should appear
		const dialog = page.getByRole('dialog');
		await expect(dialog).toBeVisible({ timeout: TIMEOUTS.UI_FAST });

		// Confirm sign out
		await dialog.getByRole('button', { name: 'Sign Out' }).click();

		// Should end up at the login page (hard reload clears state)
		await page.waitForURL('/login');

		// No instance should have an authenticated user
		const instances = await page.evaluate(() => localStorage.getItem('chatto:instances'));
		const parsed: { userId: string | null }[] = instances ? JSON.parse(instances) : [];
		expect(parsed.every((i) => !i.userId)).toBe(true);
	});
});


test.describe('/chat backward compatibility', () => {
	test('/chat redirects to / for unauthenticated users', async ({ browser }) => {
		const context = await browser.newContext();
		const page = await context.newPage();

		await page.goto('/chat');
		await page.waitForURL('/');

		await context.close();
	});

	test('/chat redirects authenticated users to /', async ({ page }) => {
		await createAndLoginTestUser(page);
		await page.goto('/chat');

		// / then redirects to /chat/spaces for authenticated users
		await page.waitForURL((url) => url.pathname === '/' || url.pathname.startsWith('/chat/'));
	});
});
