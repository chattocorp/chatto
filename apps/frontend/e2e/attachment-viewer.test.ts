import { readFile, writeFile } from 'node:fs/promises';
import { test, expect } from './setup';
import { createAndLoginTestUser } from './fixtures/testUser';
import {
  createUserOnRemote,
  joinDefaultRoomsOnRemote,
  getRoomOnRemote,
  postMessageAttachmentOnRemote
} from './fixtures/multiServer';

test('another user can download unsupported files and play audio in the shared mobile viewer', async ({
  page,
  chatPage,
  serverURL
}, testInfo) => {
  const sender = await createUserOnRemote(serverURL, 'filesender', 'password123');
  await joinDefaultRoomsOnRemote(serverURL, sender.token);
  const roomId = await getRoomOnRemote(serverURL, sender.token, 'general');
  await createAndLoginTestUser(page);
  await chatPage.goto();
  await chatPage.enterRoom('general');
  await page.setViewportSize({ width: 360, height: 640 });
  const errors: string[] = [];
  page.on('pageerror', (error) => errors.push(error.message));
  for (const [filename, contentType] of [
    ['Long document name with spaces and Unicode ü.pdf', 'application/pdf'],
    ['Archive.zip', 'application/zip'],
    ['Notes.txt', 'text/plain'],
    ['test-audio.mp3', 'audio/mpeg']
  ]) {
    const audio = contentType.startsWith('audio/');
    const path = audio ? 'e2e/fixtures/test-audio.mp3' : testInfo.outputPath(filename);
    if (!audio) await writeFile(path, `Original bytes for ${filename}\n`);
    const description = `Notes for ${filename}\n${'A long description that stays readable on a small screen.\n'.repeat(14)}`;
    await postMessageAttachmentOnRemote(
      serverURL,
      sender.token,
      roomId,
      filename,
      path,
      filename,
      contentType,
      description
    );
    const trigger = page.getByRole('button', { name: `View ${filename}`, exact: true });
    await trigger.click();
    const dialog = page.getByRole('dialog', { name: filename, exact: true });
    await expect(dialog).toBeVisible();
    await expect(dialog.getByText(contentType, { exact: true })).toBeVisible();
    await expect(dialog.locator('iframe')).toHaveCount(0);
    const caption = dialog.locator('p[id]');
    await expect(caption).toHaveText(description);
    await expect(dialog).toHaveAttribute('aria-describedby', (await caption.getAttribute('id'))!);
    const captionSize = await caption.evaluate((element) => ({
      height: element.clientHeight,
      scrollHeight: element.scrollHeight
    }));
    expect(captionSize.height).toBeLessThanOrEqual(128);
    expect(captionSize.scrollHeight).toBeGreaterThan(captionSize.height);
    if (audio) {
      await expect
        .poll(() => dialog.locator('audio').evaluate((element) => element.readyState))
        .toBeGreaterThanOrEqual(2);
      await dialog.locator('audio').evaluate(async (element) => {
        element.muted = true;
        await element.play();
      });
      await expect(dialog.locator('audio')).toHaveJSProperty('paused', false);
    } else {
      await expect(
        dialog.getByText('No preview is available for this file. Use Download to save it.')
      ).toBeVisible();
      await expect(dialog.locator('img, audio, video')).toHaveCount(0);
    }
    const link = dialog.getByRole('link', { name: 'Download', exact: true });
    await expect(link).toBeInViewport();
    await expect(dialog.getByRole('button', { name: 'Close', exact: true })).toBeInViewport();
    const pending = page.waitForEvent('download');
    await link.click();
    const downloaded = await pending;
    expect(downloaded.suggestedFilename()).toBe(filename);
    expect(await readFile((await downloaded.path())!)).toEqual(await readFile(path));
    await page.goBack();
    await expect(dialog).not.toBeVisible();
    await expect(trigger).toBeFocused();
    await expect(page.locator('dialog audio')).toHaveCount(0);
  }
  expect(errors).toEqual([]);
});
