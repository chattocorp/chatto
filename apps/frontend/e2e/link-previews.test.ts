import { expect } from '@playwright/test';
import { test } from './setup';
import { createAndLoginTestUser } from './fixtures/testUser';
import { startOGMockServer, type OGMockServer } from './fixtures/ogMockServer';
import { TIMEOUTS } from './constants';

let ogServer: OGMockServer;

test.beforeAll(async () => {
  ogServer = await startOGMockServer();
});

test.afterAll(async () => {
  await ogServer?.close();
});

test.describe('Bare-domain auto-linking', () => {
  test('www-prefixed bare domain is auto-linked in posted message', async ({
    page,
    chatPage,
    roomPage
  }) => {
    await createAndLoginTestUser(page);
    await chatPage.goto();
    await chatPage.enterRoom('general');

    const messageText = 'Visit www.example.com for details';
    await roomPage.sendMessage(messageText);

    // The message should be rendered with the bare domain as a clickable link
    const message = page.locator('[role="article"]', { hasText: messageText });
    const link = message.locator('a[href="http://www.example.com"]');
    await expect(link).toBeVisible({ timeout: TIMEOUTS.UI_STANDARD });
    await expect(link).toHaveText('www.example.com');
    await expect(link).toHaveAttribute('target', '_blank');
    await expect(link).toHaveAttribute('rel', 'noopener noreferrer');
  });

  test('bare domain with .dev TLD is auto-linked', async ({ page, chatPage, roomPage }) => {
    await createAndLoginTestUser(page);
    await chatPage.goto();
    await chatPage.enterRoom('general');

    await roomPage.sendMessage('Check www.hmans.dev');

    const message = page.locator('[role="article"]', { hasText: 'Check www.hmans.dev' });
    const link = message.locator('a[href="http://www.hmans.dev"]');
    await expect(link).toBeVisible({ timeout: TIMEOUTS.UI_STANDARD });
    await expect(link).toHaveText('www.hmans.dev');
  });
});

test.describe('Link previews', () => {
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
    const composerPreview = page.getByTestId('link-preview-card');
    await expect(composerPreview).toBeVisible({ timeout: TIMEOUTS.COMPLEX_OPERATION });

    // Now send — the preview data is included in the mutation
    await roomPage.messageInput.press('Enter');
    await expect(page.getByText(messageText)).toBeVisible();

    // The preview should be visible on the posted message (stored at post-time)
    const previewCard = page.getByTestId('link-preview-card');
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
    const previewCard = page.getByTestId('link-preview-card');
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
    await expect(page.getByTestId('link-preview-card')).toBeVisible({
      timeout: TIMEOUTS.COMPLEX_OPERATION
    });

    // Send the message (preview data included in mutation)
    await roomPage.messageInput.press('Enter');
    await expect(page.getByText(messageText)).toBeVisible();

    // Wait for the preview to appear on the posted message
    const previewCard = page.getByTestId('link-preview-card');
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
    await expect(page.getByTestId('link-preview-card')).toBeVisible({
      timeout: TIMEOUTS.COMPLEX_OPERATION
    });

    // Send the message (preview data included in mutation)
    await roomPage.messageInput.press('Enter');
    await expect(page.getByText(messageText)).toBeVisible();

    // Wait for the first preview to appear
    const previewCard = page.getByTestId('link-preview-card');
    await expect(previewCard).toBeVisible({ timeout: TIMEOUTS.COMPLEX_OPERATION });

    // Only the first URL should have a preview
    await expect(previewCard.getByText('Test Page Title')).toBeVisible();

    // There should be exactly one preview card, not two
    await expect(page.getByTestId('link-preview-card')).toHaveCount(1);
  });

  test('YouTube URL shows embed in composer', async ({ page, chatPage, roomPage }) => {
    await createAndLoginTestUser(page);
    await chatPage.goto();
    await chatPage.enterRoom('general');

    await roomPage.waitForInputEditable();
    await roomPage.messageInput.fill('Watch https://www.youtube.com/watch?v=dQw4w9WgXcQ');

    // YouTube detection is client-side (no server fetch needed), but still debounced
    const youtubeEmbed = page.getByTestId('youtube-embed');
    await expect(youtubeEmbed).toBeVisible({ timeout: TIMEOUTS.UI_STANDARD });

    // Verify the iframe points to the privacy-friendly embed URL
    const iframe = youtubeEmbed.locator('iframe');
    await expect(iframe).toHaveAttribute(
      'src',
      'https://www.youtube-nocookie.com/embed/dQw4w9WgXcQ'
    );
  });
});
