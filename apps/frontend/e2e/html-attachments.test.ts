import { readFile, writeFile } from 'node:fs/promises';
import { test, expect } from './setup';
import { createAndLoginTestUser } from './fixtures/testUser';
import {
  startSecondServer,
  stopSecondServer,
  createUserOnRemote,
  joinDefaultRoomsOnRemote,
  getRoomOnRemote,
  connectRemoteInstance,
  postMessageAttachmentOnRemote
} from './fixtures/multiServer';
import { waitForRoomReady } from './fixtures/realtimeSync';
import * as routes from './routes';

for (const remote of [false, true]) {
  test(`HTML attachments require preview consent and download original bytes (${remote ? 'remote' : 'origin'} server)`, async ({
    page,
    chatPage,
    serverURL
  }, testInfo) => {
    const secondServer = remote ? await startSecondServer(testInfo) : null;
    try {
      const assetServerURL = secondServer
        ? secondServer.baseURL.replace('localhost', '127.0.0.1')
        : serverURL;
      const sender = await createUserOnRemote(assetServerURL, 'htmlsender', 'password123');
      await joinDefaultRoomsOnRemote(assetServerURL, sender.token);
      const roomId = await getRoomOnRemote(assetServerURL, sender.token, 'general');
      await createAndLoginTestUser(page);
      await chatPage.goto();
      if (secondServer) {
        const reader = await createUserOnRemote(assetServerURL, 'htmlreader', 'password123');
        await joinDefaultRoomsOnRemote(assetServerURL, reader.token);
        await connectRemoteInstance(
          page,
          { ...secondServer, baseURL: assetServerURL },
          reader.userId
        );
        await page.goto(routes.remote.room('127.0.0.1', roomId));
        await waitForRoomReady(page, 'general');
      } else {
        await chatPage.enterRoom('general');
      }

      const pageErrors: string[] = [];
      page.on('pageerror', (error) => pageErrors.push(error.message));
      const assetRequests: string[] = [];
      page.on('request', (request) => {
        if (new URL(request.url()).pathname.startsWith('/assets/files/'))
          assetRequests.push(request.url());
      });
      const externalReferrers: (string | undefined)[] = [];
      await page.route('https://preview-resource.example.test/pixel.svg', async (route) => {
        externalReferrers.push(route.request().headers().referer);
        await route.fulfill({
          contentType: 'image/svg+xml',
          body: '<svg xmlns="http://www.w3.org/2000/svg" width="10" height="10"/>'
        });
      });
      const filename = 'Shared report ü.html';
      const html =
        '<!doctype html><html lang="en"><title>Shared report</title><body><h1>HTML preview fixture</h1><script>document.body.dataset.scriptRan="yes"</script><img alt="External fixture" src="https://preview-resource.example.test/pixel.svg"></body></html>';
      const fixturePath = testInfo.outputPath('report.html');
      await writeFile(fixturePath, html);
      await postMessageAttachmentOnRemote(
        assetServerURL,
        sender.token,
        roomId,
        'Shared HTML report',
        fixturePath,
        filename,
        'text/html'
      );
      const trigger = page.getByRole('button', { name: `View ${filename}`, exact: true });
      await trigger.click();
      const dialog = page.getByRole('dialog', { name: filename, exact: true });
      await expect(dialog).toBeVisible();
      await expect(dialog.getByText('HTML document', { exact: true })).toBeVisible();
      await expect(dialog.getByText(`${Buffer.byteLength(html)} B`, { exact: true })).toBeVisible();
      await expect(dialog.locator('iframe')).toHaveCount(0);
      expect(assetRequests).toHaveLength(0);
      expect(externalReferrers).toHaveLength(0);

      const downloadLink = dialog.getByRole('link', { name: 'Download', exact: true });
      expect(new URL((await downloadLink.getAttribute('href'))!, page.url()).origin).toBe(
        new URL(secondServer?.baseURL ?? serverURL).origin
      );
      const downloadPromise = page.waitForEvent('download');
      await downloadLink.click();
      const download = await downloadPromise;
      expect(download.suggestedFilename()).toBe(filename);
      expect(await readFile((await download.path())!, 'utf8')).toBe(html);
      await expect(dialog.locator('iframe')).toHaveCount(0);
      expect(externalReferrers).toHaveLength(0);

      await dialog.getByRole('button', { name: 'Show preview' }).click();
      const frame = dialog.frameLocator('iframe');
      await expect(frame.getByRole('heading', { name: 'HTML preview fixture' })).toBeVisible();
      await expect(dialog.locator('iframe')).toHaveAttribute('sandbox', '');
      await expect(dialog.locator('iframe')).toHaveAttribute('referrerpolicy', 'no-referrer');
      await expect(frame.locator('body')).not.toHaveAttribute('data-script-ran');
      await expect.poll(() => externalReferrers.length).toBe(1);
      expect(externalReferrers[0]).toBeUndefined();
      await expect(downloadLink).toBeVisible();

      // The sandbox isolates frame keyboard events. Tab returns to the outer
      // controls, where Escape dismisses the dialog.
      await frame.getByRole('heading', { name: 'HTML preview fixture' }).click();
      await page.keyboard.press('Tab');
      await expect(downloadLink).toBeFocused();
      await page.keyboard.press('Escape');
      await expect(dialog).not.toBeVisible();
      await expect(trigger).toBeFocused();
      await trigger.click();
      await expect(dialog.locator('iframe')).toHaveCount(0);
      await page.goBack();
      await expect(dialog).not.toBeVisible();
      await expect(trigger).toBeFocused();

      await page.setViewportSize({ width: 390, height: 844 });
      await trigger.click();
      await expect(dialog.getByRole('button', { name: 'Show preview' })).toBeVisible();
      await expect(downloadLink).toBeInViewport();
      await expect(dialog.getByRole('button', { name: 'Close', exact: true })).toBeInViewport();
      const bounds = await dialog.boundingBox();
      expect(bounds!.width).toBeLessThanOrEqual(390);
      await dialog.getByRole('button', { name: 'Close', exact: true }).click();
      await expect(dialog).not.toBeVisible();
      expect(pageErrors).toEqual([]);
    } finally {
      if (secondServer) await stopSecondServer(secondServer, testInfo);
    }
  });
}
