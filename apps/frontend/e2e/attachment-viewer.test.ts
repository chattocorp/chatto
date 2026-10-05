import { readFile, writeFile } from 'node:fs/promises';
import { test, expect } from './setup';
import { createAndLoginTestUser } from './fixtures/testUser';
import {
  createUserOnRemote,
  joinDefaultRoomsOnRemote,
  getRoomOnRemote,
  postMessageAttachmentOnRemote
} from './fixtures/multiServer';

test('Markdown attachments render automatically and download their original bytes', async ({
  page,
  chatPage,
  serverURL
}, testInfo) => {
  const sender = await createUserOnRemote(serverURL, 'markdownsender', 'password123');
  await joinDefaultRoomsOnRemote(serverURL, sender.token);
  const roomId = await getRoomOnRemote(serverURL, sender.token, 'general');
  await createAndLoginTestUser(page);
  await chatPage.goto();
  await chatPage.enterRoom('general');
  const filename = 'Review.md';
  const path = testInfo.outputPath(filename);
  const source =
    '# Attachment review\n\n**Summary**\n\n- First item\n- Second item\n\n' +
    '| Item | Status |\n| --- | --- |\n| Preview | Ready |\n\n' +
    '```text\noriginal code\n```\n\n<script>alert(1)</script>\n\n' +
    '![External image](https://example.com/tracker.png)\n\n' +
    Array.from({ length: 20 }, (_, i) => `## Section ${i + 1}\n\nMore document content.\n\n`).join(
      ''
    );
  await writeFile(path, source);
  await postMessageAttachmentOnRemote(
    serverURL,
    sender.token,
    roomId,
    filename,
    path,
    filename,
    'text/markdown',
    'Review notes.'
  );
  const externalRequests: string[] = [];
  page.on('request', (request) => {
    if (new URL(request.url()).hostname === 'example.com') externalRequests.push(request.url());
  });
  const trigger = page.getByRole('button', { name: `View ${filename}`, exact: true });
  await trigger.click();
  const dialog = page.getByRole('dialog', { name: filename, exact: true });
  await expect(
    dialog.getByRole('heading', { name: 'Attachment review', exact: true })
  ).toBeVisible();
  await expect(dialog.locator('strong')).toHaveText('Summary');
  await expect(dialog.locator('li')).toHaveCount(2);
  await expect(dialog.locator('table')).toBeVisible();
  await expect(dialog.locator('pre code')).toContainText('original code');
  await expect(dialog.locator('script, img, iframe')).toHaveCount(0);
  await expect(dialog.getByText('Review notes.', { exact: true })).toBeVisible();
  for (const viewport of [
    { width: 1440, height: 1000 },
    { width: 360, height: 640 }
  ]) {
    await page.setViewportSize(viewport);
    const preview = dialog.locator('.markdown-html').locator('..').locator('..');
    const dimensions = await preview.evaluate((element) => ({
      height: element.clientHeight,
      scrollHeight: element.scrollHeight,
      width: element.clientWidth,
      scrollWidth: element.scrollWidth
    }));
    expect(dimensions.scrollHeight).toBeGreaterThan(dimensions.height);
    expect(dimensions.scrollWidth).toBeLessThanOrEqual(dimensions.width);
    await expect(dialog.getByRole('link', { name: 'Download', exact: true })).toBeInViewport();
    await expect(dialog.getByRole('button', { name: 'Close', exact: true })).toBeInViewport();
  }
  const pending = page.waitForEvent('download');
  await dialog.getByRole('link', { name: 'Download', exact: true }).click();
  const downloaded = await pending;
  expect(downloaded.suggestedFilename()).toBe(filename);
  expect(await readFile((await downloaded.path())!)).toEqual(await readFile(path));
  expect(externalRequests).toEqual([]);
  await page.goBack();
  await expect(dialog).not.toBeVisible();
  await expect(trigger).toBeFocused();
});

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
    if (contentType === 'application/pdf') {
      await page.setViewportSize({ width: 1440, height: 1000 });
      const fallback = dialog.getByText(
        'No preview is available for this file. Use Download to save it.'
      );
      const previewBounds = (await fallback.boundingBox())!;
      const captionBounds = (await caption.boundingBox())!;
      expect(captionBounds.x).toBeGreaterThanOrEqual(previewBounds.x + previewBounds.width);
      await expect(dialog.getByRole('link', { name: 'Download', exact: true })).toBeInViewport();
      await page.setViewportSize({ width: 360, height: 640 });
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
