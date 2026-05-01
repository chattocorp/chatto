import type { Page } from '@playwright/test';
import { test, expect } from './setup';
import { createAndLoginTestUser } from './fixtures/testUser';
import {
	startSecondServer,
	stopSecondServer,
	createUserOnRemote,
	connectRemoteInstance
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

	test('authenticated user with instances is redirected to Browse Spaces', async ({
		page
	}) => {
		await createAndLoginTestUser(page);
		await page.goto('/');

		// Should redirect to /chat/spaces (Browse Spaces)
		await page.waitForURL(routes.spaces);
		await expect(page.getByRole('heading', { name: 'Browse Spaces' })).toBeVisible();
	});
});

test.describe('Origin Auto-Registration', () => {
	test('origin instance appears on instances page after probe completes', async ({ page }) => {
		await createAndLoginTestUser(page);

		// Navigate to instances page — origin should be auto-registered
		await page.goto(routes.instances);

		// The origin instance should be listed (its name comes from the instance config)
		await expect(page.getByRole('heading', { name: 'Connected Instances' })).toBeVisible();
		// Origin instance should not have a Disconnect button
		await expect(page.getByRole('button', { name: 'Disconnect' })).not.toBeVisible();
	});

	test('origin re-registers after reload if user is authenticated', async ({ page }) => {
		await createAndLoginTestUser(page);

		// Navigate to a page first so localStorage is accessible
		await page.goto('/');
		await page.waitForLoadState('networkidle');

		// Clear localStorage to remove all instances
		await page.evaluate(() => localStorage.removeItem('chatto:instances'));
		await page.reload();
		await page.waitForLoadState('networkidle');

		// Origin should be re-registered via probeOrigin — give it time
		await page.goto(routes.instances);
		await expect(page.getByTestId('instance-row')).toBeVisible({
			timeout: TIMEOUTS.UI_STANDARD
		});
	});
});

test.describe('Add Instance Dialog', () => {
	// Target the page-header trigger by visible text; the sidebar "+" button
	// also exposes the accessible name "Add Instance" (via its title attribute),
	// so getByRole('button', { name: 'Add Instance' }) would be ambiguous.
	const headerAddInstanceButton = (page: Page) =>
		page.getByRole('button', { name: 'Add Instance', exact: true }).filter({ hasText: 'Add Instance' });

	test('shows URL input for connecting to remote instances', async ({ page }) => {
		await createAndLoginTestUser(page);
		await page.goto(routes.instances);
		await headerAddInstanceButton(page).click();

		// URL input should be visible
		await expect(page.getByLabel('Instance URL')).toBeVisible();
		await expect(page.getByRole('button', { name: 'Connect' })).toBeVisible();
	});

	test('shows error for invalid instance URL', async ({ page }) => {
		await createAndLoginTestUser(page);
		await page.goto(routes.instances);
		await headerAddInstanceButton(page).click();

		// Enter an unreachable URL
		await page.getByLabel('Instance URL').fill('https://nonexistent.invalid');
		await page.getByRole('button', { name: 'Connect' }).click();

		// Should show a connection error
		await expect(page.getByText('Could not connect')).toBeVisible({
			timeout: TIMEOUTS.REALTIME_EVENT
		});
	});
});

test.describe('Add Instance - Remote Auth Flow', () => {
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

	test('probing a valid remote instance redirects to remote OAuth login', async ({ page }) => {
		await createAndLoginTestUser(page);

		// Navigate directly to the hostname page — it probes, detects authorizeUrl,
		// and auto-redirects to the remote's OAuth login page
		const baseURL = remoteBaseURL(remoteServer);
		const hostname = new URL(baseURL).host;
		await page.goto(`/instances/add/${hostname}`);

		// Should redirect to the remote's OAuth login page
		await expect(page).toHaveURL(/\/login\?redirect=/, {
			timeout: TIMEOUTS.REALTIME_EVENT
		});
		await expect(page.locator('input[autocomplete="username"]')).toBeVisible();
	});

	test('signing in to remote instance via OAuth flow adds it to sidebar', async ({ page }) => {
		await createAndLoginTestUser(page);

		// Create a user on the remote instance
		const baseURL = remoteBaseURL(remoteServer);
		const hostname = new URL(baseURL).host;
		await createUserOnRemote(baseURL, 'remoteuser', 'password123');

		// Navigate to the add-instance page — auto-redirects to remote's OAuth login
		await page.goto(`/instances/add/${hostname}`);
		await expect(page).toHaveURL(/\/login\?redirect=/, {
			timeout: TIMEOUTS.REALTIME_EVENT
		});

		// Fill in credentials on the remote's login page
		await page.locator('input[autocomplete="username"]').fill('remoteuser');
		await page.locator('input[autocomplete="current-password"]').fill('password123');
		await page.getByRole('button', { name: 'Sign In' }).click();

		// Should redirect back to home and end up on browse spaces
		await page.waitForURL(routes.spaces, { timeout: TIMEOUTS.COMPLEX_OPERATION });

		// The remote instance should now be visible on the instances page
		await page.goto(routes.instances);
		// Should have at least 1 Disconnect button (remote — origin has no Disconnect)
		await expect(async () => {
			const count = await page.getByRole('button', { name: 'Disconnect' }).count();
			expect(count).toBeGreaterThanOrEqual(1);
		}).toPass({ timeout: TIMEOUTS.UI_STANDARD });
	});

	test('invalid credentials show error on remote OAuth login page', async ({ page }) => {
		await createAndLoginTestUser(page);

		const baseURL = remoteBaseURL(remoteServer);
		const hostname = new URL(baseURL).host;

		// Navigate — auto-redirects to remote's OAuth login
		await page.goto(`/instances/add/${hostname}`);
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

test.describe('Create Space - Multi-Instance', () => {
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

	test('create space page shows instance picker when multiple instances are connected', async ({
		page,
		chatPage
	}) => {
		await createAndLoginTestUser(page);
		await chatPage.goto();

		// Add remote instance via the real /instances/add → OAuth → callback flow
		const baseURL = remoteBaseURL(remoteServer);
		const remoteUser = await createUserOnRemote(baseURL, 'remoteuser', 'password123');
		await connectRemoteInstance(page, { ...remoteServer, baseURL }, remoteUser.userId);

		await page.goto(routes.newSpace);

		// Confirm we landed on the new-space page (auth could spuriously redirect us
		// elsewhere if currentUser hydration lagged in CI — fail fast with a clear error).
		await expect(page).toHaveURL(routes.newSpace, { timeout: TIMEOUTS.UI_STANDARD });

		// Instance picker should be visible (since we have 2 instances).
		// Use REALTIME_EVENT to give multi-instance hydration enough headroom on slow CI.
		await expect(page.getByLabel('Instance')).toBeVisible({ timeout: TIMEOUTS.REALTIME_EVENT });
	});

	test('create space works on selected instance', async ({ page, chatPage }) => {
		await createAndLoginTestUser(page);
		await chatPage.goto();

		// Create a space (single instance — no picker shown)
		await page.goto(routes.newSpace);

		await page.getByLabel('Name').fill('Test New Space');
		await page.locator('button[type="submit"]').click();

		// Should redirect to the new space
		await page.waitForURL(routes.patterns.anySpace);
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
