import { test, expect } from './setup';
import { createAndLoginTestUser } from './fixtures/testUser';
import { withServerUser } from './fixtures/serverUser';
import { DMPage } from './pages/DMPage';

for (const touch of [false, true]) {
  test.describe(touch ? 'Touch message bodies' : 'Mouse message bodies', () => {
    test.use(
      touch ? { isMobile: true, hasTouch: true, viewport: { width: 1280, height: 900 } } : {}
    );

    test('opens a newly posted message and reopens its thread from the body', async ({
      page,
      chatPage,
      roomPage
    }) => {
      const errors: string[] = [];
      page.on('pageerror', (error) => errors.push(error.message));
      await createAndLoginTestUser(page);
      await chatPage.goto();
      await chatPage.enterRoom('general');
      // Room entry uses the desktop sidebar helper; exercise the message at phone size.
      if (touch) await page.setViewportSize({ width: 390, height: 844 });
      const message = await roomPage.sendMessage('Body activation regression root');
      const body = message.locator.locator('.prose');
      // Click the rendered text inside the real timeline, not the toolbar.
      if (touch) await body.tap();
      else await body.click();
      await roomPage.expectThreadPaneVisible();
      await expect(roomPage.threadReplyInput).toBeEditable();
      await roomPage.postThreadReply('Reply created through body activation');
      await roomPage.closeThread();
      if (touch) await body.tap();
      else await body.click();
      await roomPage.expectThreadPaneVisible();
      await roomPage.expectTextInThreadPane('Reply created through body activation');
      expect(errors).toEqual([]);
    });
  });
}

test('keeps links and mouse text selection independent of body activation', async ({
  page,
  chatPage,
  roomPage,
  context
}) => {
  await createAndLoginTestUser(page);
  await chatPage.goto();
  await chatPage.enterRoom('general');
  const message = await roomPage.sendMessage(
    'Selectable body text https://example.com/body-activation'
  );
  const body = message.locator.locator('.prose');
  await context.route('https://example.com/body-activation', (route) =>
    route.fulfill({ contentType: 'text/html', body: '<p>Test link destination</p>' })
  );
  const popupPromise = page.waitForEvent('popup');
  await body.getByRole('link').click();
  const popup = await popupPromise;
  await expect(popup.getByText('Test link destination')).toBeVisible();
  await popup.close();
  await page.bringToFront();
  await expect(roomPage.threadPane).not.toBeVisible();

  const box = await body.boundingBox();
  if (!box) throw new Error('Message body has no bounds');
  await page.mouse.move(box.x + 3, box.y + 10);
  await page.mouse.down();
  await page.mouse.move(box.x + 120, box.y + 10, { steps: 10 });
  await page.mouse.up();
  await expect.poll(() => page.evaluate(() => window.getSelection()?.toString())).not.toBe('');
  await expect(roomPage.threadPane).not.toBeVisible();
  // A normal click clears the browser selection and must open on that same click.
  await body.click({ position: { x: 20, y: 10 } });
  await roomPage.expectThreadPaneVisible();
});

test('opens a thread by clicking a newly posted direct message', async ({
  page,
  browser,
  serverURL
}) => {
  const recipient = await createAndLoginTestUser(page);
  await withServerUser(browser, serverURL, async ({ page: senderPage }) => {
    const room = await new DMPage(senderPage).startConversation(recipient.login);
    const message = await room.sendMessage('Direct message body activation');
    await message.locator.locator('.prose').click();
    await room.expectThreadPaneVisible();
    await expect(room.threadReplyInput).toBeEditable();
  });
});
