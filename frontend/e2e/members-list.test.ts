import { expect } from '@playwright/test';
import { createAndLoginTestUser, joinSpace } from './fixtures/testUser';
import { waitForRoomReady } from './fixtures/realtimeSync';
import { test } from './setup';
import { ChatPage, SettingsPage } from './pages';
import * as routes from './routes';
import { TIMEOUTS } from './constants';

// FIXME: multi-user createSpace + joinSpace flow times out in CI under the
// URL-collapse transition (#330 phase 2) — second user's GraphQL state
// doesn't always reflect the new membership before the assertion. Re-enable
// after the next PR removes createSpace and tests use the bootstrap space.
test.describe.skip('Member list sorting', () => {
  test('members are sorted alphabetically by display name within each status group', async ({
    page,
    chatPage,
    browser,
    serverURL
  }) => {
    // User A: Create account with display name starting with "Z"
    const userA = await createAndLoginTestUser(page);
    await chatPage.goto();

    // Set User A's display name to start with "Z"
    const settingsPage = new SettingsPage(page);
    await settingsPage.goto();
    await settingsPage.updateDisplayName('Zara Adams');

    // Create space and room
    await chatPage.goto();
    await chatPage.createSpace();
    const spaceId = await chatPage.getSpaceId();
    const roomPage = await chatPage.enterRoom('general');

    // Wait for User A to appear
    await roomPage.expectMemberVisible(userA.login);

    // User B: Create account with display name starting with "A"
    const context2 = await browser!.newContext({ baseURL: serverURL });
    const page2 = await context2.newPage();

    try {
      const userB = await createAndLoginTestUser(page2);

      // Set User B's display name to start with "A"
      await page2.goto(routes.settings);
      await page2.waitForURL(routes.settings);
      const displayNameInput = page2.getByPlaceholder('Enter your display name');
      await displayNameInput.fill('Alice Brown');
      await page2.getByRole('button', { name: 'Save Changes' }).click();
      await expect(page2.getByText('Profile updated')).toBeVisible();

      // User B joins the space
      await joinSpace(page2, spaceId);
      await page2.goto(routes.space());
      await page2.waitForURL(routes.patterns.anySpace);

      const chatPage2 = new ChatPage(page2);
      await chatPage2.enterRoom('general');
      await waitForRoomReady(page2, 'general');

      // Wait for both users to be visible in User A's member list
      await roomPage.expectMemberVisible(userA.login, { timeout: TIMEOUTS.REALTIME_EVENT });
      await roomPage.expectMemberVisible(userB.login, { timeout: TIMEOUTS.REALTIME_EVENT });

      // Wait for presence to update so both are in the Online section
      await expect(roomPage.onlineSectionHeader).toHaveText('Online (2)', { timeout: TIMEOUTS.REALTIME_EVENT });

      // Verify members are sorted alphabetically: Alice before Zara
      await roomPage.expectMembersSortedAlphabetically();

      // Double-check the specific order
      const displayNames = await roomPage.getMemberDisplayNamesInOrder();
      expect(displayNames[0]).toBe('Alice Brown');
      expect(displayNames[1]).toBe('Zara Adams');
    } finally {
      await context2.close();
    }
  });

  test('sorting uses display name, not login', async ({ page, chatPage, browser, serverURL }) => {
    // This test verifies that users with login "aaa_user" but display name "Zach"
    // appear after users with login "zzz_user" but display name "Aaron"

    // User A: Create with login that sorts early alphabetically
    const userA = await createAndLoginTestUser(page);
    await chatPage.goto();

    // Set display name that sorts LATER alphabetically
    const settingsPage = new SettingsPage(page);
    await settingsPage.goto();
    await settingsPage.updateDisplayName('Zach');

    await chatPage.goto();
    await chatPage.createSpace();
    const spaceId = await chatPage.getSpaceId();
    const roomPage = await chatPage.enterRoom('general');

    await roomPage.expectMemberVisible(userA.login);

    // User B: Will have display name that sorts EARLIER
    const context2 = await browser!.newContext({ baseURL: serverURL });
    const page2 = await context2.newPage();

    try {
      const userB = await createAndLoginTestUser(page2);

      // Set display name that sorts earlier
      await page2.goto(routes.settings);
      await page2.waitForURL(routes.settings);
      const displayNameInput = page2.getByPlaceholder('Enter your display name');
      await displayNameInput.fill('Aaron');
      await page2.getByRole('button', { name: 'Save Changes' }).click();
      await expect(page2.getByText('Profile updated')).toBeVisible();

      await joinSpace(page2, spaceId);
      await page2.goto(routes.space());
      await page2.waitForURL(routes.patterns.anySpace);

      const chatPage2 = new ChatPage(page2);
      await chatPage2.enterRoom('general');
      await waitForRoomReady(page2, 'general');

      // Wait for both users
      await roomPage.expectMemberVisible(userA.login, { timeout: TIMEOUTS.REALTIME_EVENT });
      await roomPage.expectMemberVisible(userB.login, { timeout: TIMEOUTS.REALTIME_EVENT });
      await expect(roomPage.onlineSectionHeader).toHaveText('Online (2)', { timeout: TIMEOUTS.REALTIME_EVENT });

      // Aaron should appear before Zach (sorted by display name, not login)
      const displayNames = await roomPage.getMemberDisplayNamesInOrder();
      expect(displayNames[0]).toBe('Aaron');
      expect(displayNames[1]).toBe('Zach');
    } finally {
      await context2.close();
    }
  });
});
