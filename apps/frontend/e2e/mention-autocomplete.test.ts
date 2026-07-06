import { expect, type Page } from '@playwright/test';
import { test } from './setup';
import {
  createAndLoginTestUser,
  generateRoleName,
  loginAsAdminAndUsePrimaryServer
} from './fixtures/testUser';
import { connectPost, type E2EAdminRole, unwrapAdminRole } from './fixtures/connectHelpers';
import { TIMEOUTS } from './constants';

async function createPingableRole(page: Page, roleName: string) {
  const data = await connectPost<{ role?: E2EAdminRole }>(
    page,
    'chatto.admin.v1.AdminRoleService/CreateRole',
    {
      name: roleName,
      displayName: roleName,
      description: 'E2E role mention confirmation',
      pingable: true
    }
  );
  const role = unwrapAdminRole(data.role);
  expect(role?.name).toBe(roleName);
  expect(role?.pingable).toBe(true);
}

/**
 * E2E coverage for the TipTap-driven mention integration keeps full app smoke
 * tests for user autocomplete and role-mention confirmation. Local popup
 * behaviour, ranking, threshold show/hide, Escape, and cursor replacement are
 * covered by browser/unit specs for MessageComposer, AutocompleteState, and
 * MentionAutocomplete.
 */
test.describe('Mention autocomplete', () => {
  test('Tab completes @mention with matching username and sends it', async ({
    page,
    chatPage,
    roomPage
  }) => {
    const user = await createAndLoginTestUser(page);
    await chatPage.goto();
    await chatPage.enterRoom('general');

    await roomPage.messageInput.click();
    const partialMention = `@${user.login.slice(0, 4)}`;
    await roomPage.messageInput.pressSequentially(partialMention);
    await roomPage.messageInput.press('Tab');

    await expect(roomPage.messageInput).toHaveText(`@${user.login} `);
    await roomPage.messageInput.pressSequentially('hello');
    await roomPage.messageInput.press('Enter');

    await expect(page.locator('[role="article"]', { hasText: `@${user.login} hello` })).toBeVisible(
      { timeout: TIMEOUTS.UI_STANDARD }
    );
  });

  test('confirms a real role mention before posting', async ({ page, chatPage, roomPage }) => {
    await loginAsAdminAndUsePrimaryServer(page);
    const roleName = generateRoleName('ping');
    await createPingableRole(page, roleName);

    await chatPage.goto();
    await chatPage.enterRoom('general');

    const messageText = `@${roleName} e2e role mention confirmation ${Date.now()}`;
    let createMessageRequests = 0;
    page.on('request', (request) => {
      if (request.url().includes('/chatto.api.v1.MessageService/CreateMessage')) {
        createMessageRequests += 1;
      }
    });

    await roomPage.waitForInputEditable();
    await roomPage.messageInput.fill(messageText);
    await roomPage.sendButton.click();

    const postedMessage = page.locator('[role="article"]', { hasText: messageText });
    await expect(page.getByRole('dialog', { name: 'Send mention?' })).toBeVisible({
      timeout: TIMEOUTS.UI_STANDARD
    });
    expect(createMessageRequests).toBe(0);
    await expect(postedMessage).toHaveCount(0);

    await page.getByRole('button', { name: 'Send Anyway' }).click();

    await expect.poll(() => createMessageRequests).toBe(1);
    await expect(postedMessage).toBeVisible({ timeout: TIMEOUTS.UI_STANDARD });
  });
});
