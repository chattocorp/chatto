import { test, expect } from './setup';
import { createAndLoginTestUser } from './fixtures/testUser';
import { withServerUser } from './fixtures/serverUser';
import { TIMEOUTS } from './constants';

test('renders canonical message plaintext before resource hydration completes', async ({
  page,
  browser,
  serverURL,
  chatPage,
  roomPage
}) => {
  await createAndLoginTestUser(page);
  await chatPage.goto();
  await chatPage.enterRoom('general');

  let releaseHydration = () => {};
  const hydrationRelease = new Promise<void>((resolve) => {
    releaseHydration = resolve;
  });
  let reportHydrationStarted = () => {};
  const hydrationStarted = new Promise<void>((resolve) => {
    reportHydrationStarted = resolve;
  });
  let hydrationRequests = 0;
  const getMessageRoute = '**/api/connect/chatto.api.v1.MessageService/GetMessage';
  await page.route(getMessageRoute, async (route) => {
    hydrationRequests += 1;
    reportHydrationStarted();
    await hydrationRelease;
    await route.continue();
  });

  const body = `Canonical plaintext before hydration ${Date.now()}`;
  try {
    const post = withServerUser(
      browser!,
      serverURL,
      async ({ chatPage: senderChat, roomPage: senderRoom }) => {
        await senderChat.enterRoom('general');
        await senderRoom.fileInput.setInputFiles('e2e/fixtures/brighton.jpg');
        await expect(senderRoom.attachmentPreview).toBeVisible();
        await senderRoom.messageInput.fill(body);
        await senderRoom.messageInput.press('Control+Enter');
        await expect(senderRoom.getMessage(body).locator).toBeVisible({
          timeout: TIMEOUTS.UI_FAST
        });
      }
    );

    await hydrationStarted;
    const provisionalMessage = roomPage.getMessage(body);
    await expect(provisionalMessage.locator).toBeVisible({
      timeout: TIMEOUTS.UI_FAST
    });
    await expect(provisionalMessage.attachmentImage).toHaveCount(0);
    expect(hydrationRequests).toBe(1);

    releaseHydration();
    await post;
    await expect(provisionalMessage.attachmentImage).toBeVisible({
      timeout: TIMEOUTS.COMPLEX_OPERATION
    });
  } finally {
    releaseHydration();
    await page.unroute(getMessageRoute);
  }
});
