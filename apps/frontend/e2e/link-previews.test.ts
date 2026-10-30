import { expect } from '@playwright/test';
import { test } from './setup';
import { createAndLoginTestUser } from './fixtures/testUser';
import { startOGMockServer, type OGMockServer } from './fixtures/ogMockServer';
import { TIMEOUTS } from './constants';
import { getIdsFromUrlViaConnect, postMessageViaConnect } from './fixtures/connectHelpers';

let ogServer: OGMockServer;

test.beforeAll(async () => {
  ogServer = await startOGMockServer();
});

test.afterAll(async () => {
  await ogServer?.close();
});

test.describe('Automatic URL links', () => {
  test('filenames and bare domains stay plain in posted messages', async ({
    page,
    chatPage,
    roomPage
  }) => {
    await createAndLoginTestUser(page);
    await chatPage.goto();
    await chatPage.enterRoom('general');

    const messageText = 'Files agents.md readme.md foo.py and example.com';
    await roomPage.sendMessage(messageText);

    const message = page.locator('[role="article"]', { hasText: messageText });
    await expect(message).toBeVisible();
    await expect(message.getByText(messageText, { exact: true }).locator('a')).toHaveCount(0);
    await expect(message.getByTestId('link-preview-card')).toHaveCount(0);
  });

  test('www-prefixed bare domain is auto-linked in posted message', async ({
    page,
    chatPage,
    roomPage
  }) => {
    await createAndLoginTestUser(page);
    await chatPage.goto();
    await chatPage.enterRoom('general');

    const messageText = 'Visit www.example.com for details';
    // URL pills include a hidden full-URL description between visible text nodes.
    // Locate the message by its prose, then assert the link separately.
    await roomPage.sendMessage(messageText, 'Visit');

    // The message should be rendered with the bare domain as a clickable link
    const message = page.locator('[role="article"]', { hasText: 'Visit' });
    const link = message.locator('a[href="https://www.example.com"]');
    await expect(link).toBeVisible({ timeout: TIMEOUTS.UI_STANDARD });
    await expect(link).toHaveText('www.example.com');
    await expect(link).toHaveAttribute('target', '_blank');
    await expect(link).toHaveAttribute('rel', 'noopener noreferrer');
  });

  test('www URL with .dev TLD is auto-linked', async ({ page, chatPage, roomPage }) => {
    await createAndLoginTestUser(page);
    await chatPage.goto();
    await chatPage.enterRoom('general');

    await roomPage.sendMessage('Check www.hmans.dev');

    const message = page.locator('[role="article"]', { hasText: 'Check www.hmans.dev' });
    const link = message.locator('a[href="https://www.hmans.dev"]');
    await expect(link).toBeVisible({ timeout: TIMEOUTS.UI_STANDARD });
    await expect(link).toHaveText('www.hmans.dev');
  });
});

test.describe('Link previews', () => {
  for (const authored of [false, true]) {
    test(`${authored ? 'Markdown links fetch' : 'URL pills reuse attached'} hover previews through the server`, async ({
      page,
      chatPage,
      roomPage
    }) => {
      await createAndLoginTestUser(page);
      await chatPage.goto();
      await chatPage.enterRoom('general');
      const testUrl = `${ogServer.baseURL}/og-basic`;
      if (authored) {
        const { roomId } = await getIdsFromUrlViaConnect(page);
        await postMessageViaConnect(page, roomId, `Hover check [the useful guide](${testUrl})`);
      } else {
        await roomPage.waitForInputEditable();
        await roomPage.messageInput.fill(`Hover check ${testUrl}`);
        await expect(page.getByTestId('composer-link-preview')).toBeVisible();
        await expect(roomPage.sendButton).toBeEnabled();
        await roomPage.messageInput.press('Control+Enter');
      }
      const chip = authored
        ? page
            .locator('[role="article"]')
            .getByRole('link', { name: 'the useful guide', exact: true })
        : page.locator(`[role="article"] .url-link[data-content-href="${testUrl}"]`);
      if (!authored) await expect(chip).toHaveText('Test Page Title');
      let requests = 0;
      page.on('request', (request) => {
        if (request.url().includes('/FetchLinkPreview')) requests++;
      });
      await chip.hover();
      const card = page.getByTestId('link-hover-card');
      await expect(chip).not.toHaveAttribute('title');
      await expect(card).toContainText('Test Page Title');
      await expect(card).toContainText('Test Site');
      if (authored) await expect(chip).not.toHaveClass(/url-link|entity-link/);
      expect(requests).toBe(authored ? 1 : 0);
      await page.keyboard.press('Escape');
      await expect(card).not.toBeVisible();
    });
  }
  test('link preview card appears on posted message', async ({ page, chatPage, roomPage }) => {
    await createAndLoginTestUser(page);
    await chatPage.goto();
    await chatPage.enterRoom('general');

    const testUrl = `${ogServer.baseURL}/og-basic`;
    const messageText = `Check out ${testUrl}`;

    // Type the message and wait for the composer to fetch the preview
    await roomPage.waitForInputEditable();
    await roomPage.messageInput.fill(messageText);

    // Wait for the composer preview to appear (debounced URL detection + server fetch)
    const composerPreview = page.getByTestId('composer-link-preview');
    await expect(composerPreview).toBeVisible({ timeout: TIMEOUTS.COMPLEX_OPERATION });

    // Now send — the preview data is included in the mutation
    await roomPage.messageInput.press('Control+Enter');
    const postedMessage = page
      .locator('[role="article"]')
      .filter({ has: page.locator(`a[data-content-href="${testUrl}"]`) });
    await expect(postedMessage).toBeVisible();

    // The preview should be visible on the posted message (stored at post-time)
    const previewCard = postedMessage.getByTestId('link-preview-card');
    await expect(previewCard).toBeVisible({ timeout: TIMEOUTS.COMPLEX_OPERATION });

    // Verify the OG metadata is rendered correctly
    await expect(previewCard.getByText('Test Page Title')).toBeVisible();
    await expect(previewCard.getByText('This is a test description')).toBeVisible();
    await expect(previewCard.getByText('Test Site')).toBeVisible();
  });

  test('composer shows live link preview while typing', async ({ page, chatPage, roomPage }) => {
    await createAndLoginTestUser(page);
    await chatPage.goto();
    await chatPage.enterRoom('general');

    const testUrl = `${ogServer.baseURL}/og-basic`;
    await roomPage.waitForInputEditable();
    await roomPage.messageInput.fill(`Look at ${testUrl}`);

    // The composer debounces URL detection (500ms), then queries the server.
    // Wait for the preview card to appear above the input.
    const previewCard = page.getByTestId('composer-link-preview');
    await expect(previewCard).toBeVisible({ timeout: TIMEOUTS.COMPLEX_OPERATION });

    await expect(previewCard.getByText('Test Page Title')).toBeVisible();
    await expect(previewCard.getByText('Test Site')).toBeVisible();
  });

  test('author can delete a link preview from posted message', async ({
    page,
    chatPage,
    roomPage
  }) => {
    await createAndLoginTestUser(page);
    await chatPage.goto();
    await chatPage.enterRoom('general');

    const testUrl = `${ogServer.baseURL}/og-basic`;
    const messageText = `Preview to delete ${testUrl}`;

    // Type message and wait for the composer preview before sending
    await roomPage.waitForInputEditable();
    await roomPage.messageInput.fill(messageText);
    await expect(page.getByTestId('composer-link-preview')).toBeVisible({
      timeout: TIMEOUTS.COMPLEX_OPERATION
    });

    // Send the message (preview data included in mutation)
    await roomPage.messageInput.press('Control+Enter');
    const postedMessage = page
      .locator('[role="article"]')
      .filter({ has: page.locator(`a[data-content-href="${testUrl}"]`) });
    await expect(postedMessage).toBeVisible();

    // Wait for the preview to appear on the posted message
    const previewCard = postedMessage.getByTestId('link-preview-card');
    await expect(previewCard).toBeVisible({ timeout: TIMEOUTS.COMPLEX_OPERATION });

    // Hover to reveal the delete button, then click it
    await previewCard.hover();
    await page.getByRole('button', { name: 'Delete preview' }).click();

    // Confirmation dialog should appear
    const dialog = page.getByRole('dialog');
    await expect(dialog).toBeVisible({ timeout: TIMEOUTS.UI_FAST });
    await expect(
      dialog.getByText('Are you sure you want to remove this link preview?')
    ).toBeVisible();

    // Confirm deletion
    await dialog.getByRole('button', { name: 'Delete' }).click();

    // Preview should disappear after deletion
    await expect(previewCard).not.toBeVisible({ timeout: TIMEOUTS.REALTIME_EVENT });
  });

  test('message with multiple URLs shows only one link preview', async ({
    page,
    chatPage,
    roomPage
  }) => {
    await createAndLoginTestUser(page);
    await chatPage.goto();
    await chatPage.enterRoom('general');

    const firstUrl = `${ogServer.baseURL}/og-basic`;
    const secondUrl = `${ogServer.baseURL}/og-second`;
    const messageText = `Check ${firstUrl} and ${secondUrl}`;

    // Type message and wait for the composer preview before sending
    await roomPage.waitForInputEditable();
    await roomPage.messageInput.fill(messageText);
    await expect(page.getByTestId('composer-link-preview')).toBeVisible({
      timeout: TIMEOUTS.COMPLEX_OPERATION
    });

    // Send the message (preview data included in mutation)
    await roomPage.messageInput.press('Control+Enter');
    const postedMessage = page
      .locator('[role="article"]')
      .filter({ has: page.locator(`a[data-content-href="${firstUrl}"]`) });
    await expect(postedMessage).toBeVisible();

    // Wait for the first preview to appear
    const previewCard = postedMessage.getByTestId('link-preview-card');
    await expect(previewCard).toBeVisible({ timeout: TIMEOUTS.COMPLEX_OPERATION });

    // Only the first URL should have a preview
    await expect(previewCard.getByText('Test Page Title')).toBeVisible();

    // There should be exactly one preview card, not two
    await expect(postedMessage.getByTestId('link-preview-card')).toHaveCount(1);
  });

  test('YouTube URL shows a compact preview without a player in composer', async ({
    page,
    chatPage,
    roomPage
  }) => {
    await createAndLoginTestUser(page);
    await chatPage.goto();
    await chatPage.enterRoom('general');

    await roomPage.waitForInputEditable();
    await roomPage.messageInput.fill('Watch https://www.youtube.com/watch?v=dQw4w9WgXcQ');

    // The server detects YouTube from the URL without a fetch, but input is still debounced.
    const composerPreview = page.getByTestId('composer-link-preview');
    await expect(composerPreview).toBeVisible({ timeout: TIMEOUTS.UI_STANDARD });
    await expect(composerPreview.getByText('YouTube Video')).toBeVisible();

    // The composer must not load the third-party player while the user types.
    await expect(page.getByTestId('youtube-embed')).toHaveCount(0);
  });
});
