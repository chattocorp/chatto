// SPDX-License-Identifier: Apache-2.0

import { test, expect } from './setup';
import { createAndLoginTestUser } from './fixtures/testUser';
import { startServer, stopServer, type ServerInfo } from './fixtures/server';
import { createUserOnRemote, connectRemoteInstance } from './fixtures/multiServer';

test.describe('Remote bearer renewal', () => {
  test.setTimeout(90_000);

  let remote: ServerInfo | undefined;

  test.beforeEach(async ({}, testInfo) => {
    remote = await startServer(testInfo, {
      instanceId: 'remote-renewal',
      portOffset: 5,
      env: { CHATTO_AUTH_ACCESS_TOKEN_TTL: '5s' }
    });
  });

  test.afterEach(async ({}, testInfo) => {
    if (remote) await stopServer(remote, testInfo);
  });

  test('keeps both tabs connected after the remote access token expires', async ({ page }) => {
    await createAndLoginTestUser(page);
    const baseURL = remote!.baseURL.replace('localhost', '127.0.0.1');
    const user = await createUserOnRemote(baseURL, 'remote-renewal-viewer', 'password123');
    await connectRemoteInstance(page, { ...remote!, baseURL }, user.userId);
    const initialExpiry = await page.evaluate(() => {
      const server = JSON.parse(localStorage.getItem('chatto:instances') ?? '[]').find(
        (entry: { url: string }) => new URL(entry.url).hostname === '127.0.0.1'
      );
      const auth = JSON.parse(
        localStorage.getItem(`chatto:i:${server.id}:authentication`) ?? 'null'
      );
      return auth.accessTokenExpiresAt as number;
    });

    const secondPage = await page.context().newPage();
    await secondPage.goto(page.url());
    const icon = secondPage.locator('[data-testid="server-icon"][href*="127.0.0.1"]').first();
    await expect(icon).toBeVisible();

    await expect
      .poll(
        async () =>
          secondPage.evaluate(() => {
            const server = JSON.parse(localStorage.getItem('chatto:instances') ?? '[]').find(
              (entry: { url: string }) => new URL(entry.url).hostname === '127.0.0.1'
            );
            if (!server) return false;
            const auth = JSON.parse(
              localStorage.getItem(`chatto:i:${server.id}:authentication`) ?? 'null'
            );
            return (
              !!auth?.accessTokenExpiresAt &&
              auth.accessTokenExpiresAt > Date.now() &&
              auth.reauthRequiredAt === null
            );
          }),
        { timeout: 20_000 }
      )
      .toBe(true);

    await page.bringToFront();
    await expect.poll(() => Date.now(), { timeout: 20_000 }).toBeGreaterThan(initialExpiry + 1_000);
    await secondPage.bringToFront();
    await secondPage.reload();
    await expect(icon).not.toHaveAttribute('title', /Sign in to reconnect/, { timeout: 20_000 });
    await expect
      .poll(
        async () =>
          secondPage.evaluate(() => {
            const server = JSON.parse(localStorage.getItem('chatto:instances') ?? '[]').find(
              (entry: { url: string }) => new URL(entry.url).hostname === '127.0.0.1'
            );
            const auth = JSON.parse(
              localStorage.getItem(`chatto:i:${server.id}:authentication`) ?? 'null'
            );
            return auth.accessTokenExpiresAt as number;
          }),
        { timeout: 20_000 }
      )
      .toBeGreaterThan(initialExpiry);
  });

  test('recovers a lost refresh response with the persisted request ID', async ({ page }) => {
    await createAndLoginTestUser(page);
    const baseURL = remote!.baseURL.replace('localhost', '127.0.0.1');
    const user = await createUserOnRemote(baseURL, 'remote-lost-response', 'password123');
    let releaseLostResponse!: () => void;
    const lostResponse = new Promise<void>((resolve) => {
      releaseLostResponse = resolve;
    });
    let loseNextRefresh = true;
    await page.context().route(`${baseURL}/oauth/token`, async (route) => {
      const grant = (route.request().postDataJSON() as { grant_type?: string }).grant_type;
      if (grant !== 'refresh_token' || !loseNextRefresh) {
        await route.continue();
        return;
      }
      loseNextRefresh = false;
      await route.fetch();
      await route.abort('failed');
      releaseLostResponse();
    });
    await connectRemoteInstance(page, { ...remote!, baseURL }, user.userId);
    await lostResponse;

    const pending = await page.evaluate(() => {
      const server = JSON.parse(localStorage.getItem('chatto:instances') ?? '[]').find(
        (entry: { url: string }) => new URL(entry.url).hostname === '127.0.0.1'
      );
      const auth = JSON.parse(
        localStorage.getItem(`chatto:i:${server.id}:authentication`) ?? 'null'
      );
      return { pendingRefresh: !!auth?.refreshRequestId, reauthRequired: !!auth?.reauthRequiredAt };
    });
    expect(pending).toEqual({ pendingRefresh: true, reauthRequired: false });

    await page.reload();
    const icon = page.locator('[data-testid="server-icon"][href*="127.0.0.1"]').first();
    await expect(icon).not.toHaveAttribute('title', /Sign in to reconnect/, { timeout: 20_000 });
    await expect
      .poll(
        async () =>
          page.evaluate(() => {
            const server = JSON.parse(localStorage.getItem('chatto:instances') ?? '[]').find(
              (entry: { url: string }) => new URL(entry.url).hostname === '127.0.0.1'
            );
            const auth = JSON.parse(
              localStorage.getItem(`chatto:i:${server.id}:authentication`) ?? 'null'
            );
            return {
              pendingRefresh: !!auth?.refreshRequestId,
              reauthRequired: !!auth?.reauthRequiredAt
            };
          }),
        { timeout: 20_000 }
      )
      .toEqual({ pendingRefresh: false, reauthRequired: false });
  });

  test('retries viewer recovery when an API rejects a newly rotated access token', async ({
    page
  }) => {
    await createAndLoginTestUser(page);
    const baseURL = remote!.baseURL.replace('localhost', '127.0.0.1');
    const user = await createUserOnRemote(baseURL, 'remote-retry-viewer', 'password123');
    await connectRemoteInstance(page, { ...remote!, baseURL }, user.userId);

    const viewerURL = `${baseURL}/api/connect/chatto.api.v1.ViewerService/GetViewer`;
    let rejected = 0;
    let recovered = 0;
    page.on('response', (response) => {
      if (response.url() === viewerURL && response.status() === 200) recovered++;
    });
    await page.context().route(viewerURL, async (route) => {
      if (rejected >= 2) {
        await route.continue();
        return;
      }
      rejected++;
      const response = await route.fetch();
      await route.fulfill({
        response,
        status: 401,
        contentType: 'application/json',
        body: JSON.stringify({ code: 'unauthenticated', message: 'temporary' })
      });
    });

    await page.reload();
    await expect.poll(() => rejected, { timeout: 20_000 }).toBe(2);
    await expect.poll(() => recovered, { timeout: 20_000 }).toBeGreaterThan(0);
    const icon = page.locator('[data-testid="server-icon"][href*="127.0.0.1"]').first();
    await expect(icon).toBeVisible();
    await expect(icon).not.toHaveAttribute('title', /Sign in to reconnect/);
    const reauthRequired = await page.evaluate(() => {
      const server = JSON.parse(localStorage.getItem('chatto:instances') ?? '[]').find(
        (entry: { url: string }) => new URL(entry.url).hostname === '127.0.0.1'
      );
      const auth = JSON.parse(
        localStorage.getItem(`chatto:i:${server.id}:authentication`) ?? 'null'
      );
      return !!auth?.reauthRequiredAt;
    });
    expect(reauthRequired).toBe(false);
  });

  test('requires sign-in after the local server revokes a renewable session', async ({ page }) => {
    await createAndLoginTestUser(page);
    const baseURL = remote!.baseURL.replace('localhost', '127.0.0.1');
    const user = await createUserOnRemote(baseURL, 'remote-revoked-viewer', 'password123');
    await connectRemoteInstance(page, { ...remote!, baseURL }, user.userId);
    const refreshToken = await page.evaluate(() => {
      const server = JSON.parse(localStorage.getItem('chatto:instances') ?? '[]').find(
        (entry: { url: string }) => new URL(entry.url).hostname === '127.0.0.1'
      );
      const auth = JSON.parse(
        localStorage.getItem(`chatto:i:${server.id}:authentication`) ?? 'null'
      );
      return auth.refreshToken as string;
    });

    const revoked = await page.request.post(`${baseURL}/auth/logout`, {
      data: { refreshToken }
    });
    expect(revoked.ok()).toBe(true);
    await page.reload();
    const icon = page.locator('[data-testid="server-icon"][href*="127.0.0.1"]').first();
    await expect(icon).toHaveAttribute('title', /Sign in to reconnect/, { timeout: 20_000 });
  });
});
