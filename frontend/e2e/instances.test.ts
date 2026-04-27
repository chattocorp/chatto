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

test.describe('Instances Page', () => {
	test('shows home instance on the instances page', async ({ page, chatPage }) => {
		await createAndLoginTestUser(page);
		await chatPage.goto();

		// Navigate to instances page
		await page.goto(routes.instances);

		// Should show the origin instance
		await expect(page.getByRole('heading', { name: 'Connected Instances' })).toBeVisible();

		// Origin instance should NOT have a Disconnect button
		await expect(page.getByRole('button', { name: 'Disconnect' })).not.toBeVisible();
	});

	test('sidebar "+" navigates to add instance page', async ({ page, chatPage }) => {
		await createAndLoginTestUser(page);
		await chatPage.goto();

		// Click the "+" button in sidebar — should navigate to add instance page
		await page.getByTitle('Add Instance').click();
		await page.waitForURL('/instances/add');
		await expect(page.getByRole('heading', { name: 'Add Instance' })).toBeVisible({
			timeout: TIMEOUTS.UI_FAST
		});
	});

	test('header "Manage Instances" icon navigates to instances page', async ({ page, chatPage }) => {
		await createAndLoginTestUser(page);
		await chatPage.goto();

		// Click the server icon in the header
		await page.getByTitle('Manage Instances').click();
		await page.waitForURL(routes.instances);

		await expect(page.getByRole('heading', { name: 'Connected Instances' })).toBeVisible();
	});

	test('"Add Instance" link in header navigates to add instance page', async ({ page }) => {
		await createAndLoginTestUser(page);
		await page.goto(routes.instances);

		// Click the Add Instance link in the pane header (not the sidebar "+" icon)
		await page.getByText('Add Instance').last().click();
		await page.waitForURL('/instances/add');

		// The Add Instance page should be shown
		await expect(page.getByRole('heading', { name: 'Add Instance' })).toBeVisible({
			timeout: TIMEOUTS.UI_FAST
		});
	});
});

test.describe('Instances Page - Multi-Instance', () => {
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

	test('shows remote instance on the instances page', async ({ page, chatPage }) => {
		await createAndLoginTestUser(page);
		await chatPage.goto();

		// Set up remote instance via the real /instances/add → OAuth → callback flow
		const baseURL = remoteBaseURL(remoteServer);
		const remoteHostname = new URL(baseURL).hostname;
		const remoteUser = await createUserOnRemote(baseURL, 'remoteuser1', 'password123');
		await connectRemoteInstance(page, { ...remoteServer, baseURL }, remoteUser.userId);

		// Navigate to instances page
		await page.goto(routes.instances);

		// Should show the remote instance (identified by hostname, since the
		// display name comes from the server's GraphQL config, not localStorage)
		const remoteRow = page.getByTestId('instance-row').filter({ hasText: remoteHostname });
		await expect(remoteRow).toBeVisible();
		await expect(page.getByText('Connected').first()).toBeVisible();

		// Remote instance should have a Disconnect button (origin does not)
		await expect(remoteRow.getByRole('button', { name: 'Disconnect' })).toBeVisible();
	});

	test('disconnecting a remote instance removes it from the list', async ({
		page,
		chatPage
	}) => {
		await createAndLoginTestUser(page);
		await chatPage.goto();

		// Set up remote instance via the real /instances/add → OAuth → callback flow
		const baseURL = remoteBaseURL(remoteServer);
		const remoteHostname = new URL(baseURL).hostname;
		const remoteUser = await createUserOnRemote(baseURL, 'remoteuser2', 'password123');
		await connectRemoteInstance(page, { ...remoteServer, baseURL }, remoteUser.userId);

		await page.goto(routes.instances);

		// Scope to the remote instance's row (identified by hostname)
		const remoteRow = page.getByTestId('instance-row').filter({ hasText: remoteHostname });
		await expect(remoteRow).toBeVisible();

		// Click its Disconnect button
		await remoteRow.getByRole('button', { name: 'Disconnect' }).click();

		// Confirmation modal should appear
		const dialog = page.getByRole('dialog');
		await expect(dialog).toBeVisible({ timeout: TIMEOUTS.UI_FAST });

		// Confirm disconnect
		await dialog.getByRole('button', { name: 'Disconnect' }).click();

		// Remote instance should be gone
		await expect(remoteRow).not.toBeVisible({ timeout: TIMEOUTS.UI_FAST });

		// Only the origin instance remains
		await expect(page.getByRole('heading', { name: 'Connected Instances' })).toBeVisible();
	});

	test('cancelling disconnect keeps the instance', async ({ page, chatPage }) => {
		await createAndLoginTestUser(page);
		await chatPage.goto();

		// Set up remote instance via the real /instances/add → OAuth → callback flow
		const baseURL = remoteBaseURL(remoteServer);
		const remoteHostname = new URL(baseURL).hostname;
		const remoteUser = await createUserOnRemote(baseURL, 'remoteuser3', 'password123');
		await connectRemoteInstance(page, { ...remoteServer, baseURL }, remoteUser.userId);

		await page.goto(routes.instances);

		// Confirm we landed on /instances. The page redirects to /login if origin auth
		// hasn't hydrated yet — assert here to fail fast with a clear error instead of
		// timing out 30s later trying to click a button on the wrong page.
		await expect(page).toHaveURL(routes.instances, { timeout: TIMEOUTS.UI_STANDARD });

		// Scope to the remote instance's row (identified by hostname).
		// Wait explicitly with REALTIME_EVENT — multi-instance hydration can be slow in CI.
		const remoteRow = page.getByTestId('instance-row').filter({ hasText: remoteHostname });
		await expect(remoteRow).toBeVisible({ timeout: TIMEOUTS.REALTIME_EVENT });
		await remoteRow.getByRole('button', { name: 'Disconnect' }).click();

		// Confirmation modal should appear — click Cancel
		const dialog = page.getByRole('dialog');
		await expect(dialog).toBeVisible({ timeout: TIMEOUTS.UI_FAST });
		await dialog.getByRole('button', { name: 'Cancel' }).click();

		// Instance should still be there
		await expect(remoteRow).toBeVisible();
		// Dialog should be closed
		await expect(dialog).not.toBeVisible();
	});
});
