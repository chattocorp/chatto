import { expect } from '@playwright/test';
import { test } from './setup';
import { loginAsAdmin, verifyAdminEmail } from './fixtures/testUser';
import {
	startSecondServer,
	stopSecondServer,
	loginAdminOnRemote,
	setMotdOnRemote,
	connectRemoteInstance
} from './fixtures/multiServer';
import type { ServerInfo } from './fixtures/server';

/**
 * Returns the remote server's base URL using 127.0.0.1 instead of localhost so
 * the SPA resolves it as a distinct instance hostname.
 */
function remoteBaseURL(server: ServerInfo): string {
	return server.baseURL.replace('localhost', '127.0.0.1');
}

/**
 * The header MOTD is bound to the URL-resolved active instance. With multiple
 * registered servers, navigating between them must flip the header text to
 * reflect the currently-viewed server's MOTD.
 */
test.describe('Multi-instance MOTD', () => {
	let remoteServer: ServerInfo;

	test.beforeEach(async ({}, testInfo) => {
		remoteServer = await startSecondServer(testInfo);
	});

	test.afterEach(async ({}, testInfo) => {
		if (remoteServer) {
			await stopSecondServer(remoteServer, testInfo);
		}
	});

	test('header MOTD reflects the currently-viewed server', async ({ page, adminPage }) => {
		// Origin: log in as admin and set MOTD via the admin UI.
		const adminUser = await loginAsAdmin(page);
		await verifyAdminEmail(page, adminUser.id!);
		await adminPage.gotoServerSettings();
		await adminPage.fillServerSettings({ motd: 'ORIGIN MOTD' });
		await adminPage.saveServerSettings();

		// Remote: set a different MOTD via the admin GraphQL mutation.
		const baseURL = remoteBaseURL(remoteServer);
		const remoteAdmin = await loginAdminOnRemote(baseURL);
		await setMotdOnRemote(baseURL, remoteAdmin.token, 'REMOTE MOTD');

		// On the origin, the header should show the origin's MOTD.
		await page.goto('/chat');
		const motd = page.getByTestId('motd-content');
		await expect(motd).toHaveText('ORIGIN MOTD');

		// Connect the remote instance — connectRemoteInstance lands on
		// /chat/<remote-host>/..., which should flip the header to the
		// remote's MOTD.
		await connectRemoteInstance(page, { ...remoteServer, baseURL }, remoteAdmin.userId);
		await expect(motd).toHaveText('REMOTE MOTD');

		// Navigate back to the origin — header should flip back.
		await page.goto('/chat/-');
		await expect(motd).toHaveText('ORIGIN MOTD');
	});
});
