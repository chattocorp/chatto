import type { Page } from '@playwright/test';
import { test, expect } from './setup';
import { createAndLoginTestUser } from './fixtures/testUser';
import {
  connectRemoteInstance,
  createUserOnRemote,
  getRoomOnRemote,
  postMessageAttachmentOnRemote,
  startSecondServer,
  stopSecondServer
} from './fixtures/multiServer';
import type { ServerInfo } from './fixtures/server';
import { waitForRoomReady } from './fixtures/realtimeSync';
import * as routes from './routes';

function remoteBaseURL(server: ServerInfo): string {
  return server.baseURL.replace('localhost', '127.0.0.1');
}

async function ensureServiceWorkerControlsPage(page: Page): Promise<void> {
  await page.evaluate(async () => {
    if (!('serviceWorker' in navigator)) {
      throw new Error('Service workers are not available in this browser');
    }
    await navigator.serviceWorker.ready;
  });

  await page.reload({ waitUntil: 'domcontentloaded' });
  await expect
    .poll(() => page.evaluate(() => Boolean(navigator.serviceWorker.controller)))
    .toBe(true);
}

test.describe('authorized remote asset URLs', () => {
  let remoteServer: ServerInfo | undefined;

  test.afterEach(async ({}, testInfo) => {
    if (remoteServer) {
      await stopSecondServer(remoteServer, testInfo);
      remoteServer = undefined;
    }
  });

  test('renders remote server attachments through signed asset URLs', async ({
    page,
    chatPage
  }) => {
    await createAndLoginTestUser(page);
    await chatPage.goto();
    await ensureServiceWorkerControlsPage(page);

    remoteServer = await startSecondServer(test.info());
    const baseURL = remoteBaseURL(remoteServer);
    const remoteUser = await createUserOnRemote(baseURL, 'remoteassetuser', 'password123');
    const roomId = await getRoomOnRemote(baseURL, remoteUser.token, 'general');
    const body = `Remote attachment ${Date.now()}`;

    const remotePost = await postMessageAttachmentOnRemote(
      baseURL,
      remoteUser.token,
      roomId,
      body,
      'e2e/fixtures/brighton.jpg',
      'brighton.jpg',
      'image/jpeg'
    );

    expect(remotePost.attachmentUrl).toContain('/assets/files/');
    expect(remotePost.attachmentUrl).toContain('access=');

    await connectRemoteInstance(page, { ...remoteServer, baseURL }, remoteUser.userId);
    await page.goto(routes.remote.room('127.0.0.1', roomId));
    await waitForRoomReady(page, 'general');
    await expect(page.getByText(body)).toBeVisible();

    const attachmentImage = page
      .locator(`[data-event-id="${remotePost.eventId}"] button[aria-label^="View"] img`)
      .first();
    await expect(attachmentImage).toBeVisible();
    await expect
      .poll(() =>
        attachmentImage.evaluate((element) => (element as HTMLImageElement).naturalWidth)
      )
      .toBeGreaterThan(0);

    const src = await attachmentImage.getAttribute('src');
    expect(src).toBeTruthy();
    expect(src).toContain('access=');

    const srcUrl = new URL(src!, page.url());
    const expectedRemoteUrl = new URL(baseURL);
    expect(srcUrl.protocol).toBe(expectedRemoteUrl.protocol);
    expect(srcUrl.port).toBe(expectedRemoteUrl.port);
    expect(srcUrl.pathname).toContain('/assets/files/');
    expect(srcUrl.pathname).not.toContain('/__chatto/');
  });
});
