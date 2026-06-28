import { expect } from '@playwright/test';
import { TIMEOUTS } from './constants';
import { createAndLoginTestUser } from './fixtures/testUser';
import { withServerUser } from './fixtures/serverUser';
import { waitForRoomReady } from './fixtures/realtimeSync';
import { test } from './setup';
import { SettingsPage } from './pages';
import * as routes from './routes';

test.describe('Presence indicators', () => {
  test('shows online indicator when another user opens the server', async ({
    page,
    chatPage,
    browser,
    serverURL
  }) => {
    // User A: Create account and navigate to general room
    const userA = await createAndLoginTestUser(page);
    await chatPage.goto();

    // Navigate to "general" room to see member list
    const roomPage = await chatPage.enterRoom('general');

    // User A should see themselves in the member list with online indicator
    await expect(roomPage.memberList).toBeVisible();

    // User A should be in the member list
    await roomPage.expectMemberVisible(userA.login);

    // User A's presence indicator should be green (online)
    // Use a longer timeout as presence update requires subscription establishment
    const userAPresenceDot = roomPage.getMemberPresenceDot(userA.login);
    await expect(userAPresenceDot).toHaveClass(/bg-green-500/, {
      timeout: TIMEOUTS.REALTIME_EVENT
    });

    // User B: Create account and open the server
    let userBLogin: string;

    await withServerUser(
      browser!,
      serverURL,
      async ({ page: page2, user: userB, chatPage: chatPage2 }) => {
        userBLogin = userB.login;

        // User B is auto-joined to "general" room - click it in the sidebar
        await chatPage2.enterRoom('general');
        await waitForRoomReady(page2, 'general');

        // Wait for User B to be visible in User A's member list (presence broadcast)
        await roomPage.expectMemberVisible(userB.login, { timeout: TIMEOUTS.REALTIME_EVENT });

        // User B's presence indicator should be green (online)
        const userBPresenceDot = roomPage.getMemberPresenceDot(userB.login);
        await expect(userBPresenceDot).toHaveClass(/bg-green-500/, {
          timeout: TIMEOUTS.UI_STANDARD
        });
      }
    );

    // After User B's context closes, they remain online until TTL expires (60s).
    // We use TTL-based expiry instead of immediate deletion to support multi-device
    // scenarios where another connection may still be active.
    // The offline transition is tested in unit tests which can control timing.

    // Verify User B is still in the member list
    const userBListItem = roomPage.getMember(userBLogin!);
    await expect(userBListItem).toBeVisible({ timeout: TIMEOUTS.REALTIME_EVENT });

    // User B should still show as online (TTL hasn't expired yet)
    const userBPresenceDot = roomPage.getMemberPresenceDot(userBLogin!);
    await expect(userBPresenceDot).toHaveClass(/bg-green-500/, { timeout: TIMEOUTS.UI_STANDARD });
  });

  test('user sees their own presence as online immediately', async ({ page, chatPage }) => {
    // Create user and load the primary server
    const user = await createAndLoginTestUser(page);
    await chatPage.goto();

    // Navigate to general room
    const roomPage = await chatPage.enterRoom('general');

    // User should see themselves in the member list
    await roomPage.expectMemberVisible(user.login);

    // User's presence indicator should be green (online)
    // Use a longer timeout as presence update requires subscription establishment
    const presenceDot = roomPage.getMemberPresenceDot(user.login);
    await expect(presenceDot).toHaveClass(/bg-green-500/, { timeout: TIMEOUTS.REALTIME_EVENT });
  });
});

test.describe('Message avatar presence', () => {
  test('does not show presence dot on message avatars', async ({ page, chatPage }) => {
    // Create user and load the primary server
    await createAndLoginTestUser(page);
    await chatPage.goto();

    // Navigate to general room and send a message
    const roomPage = await chatPage.enterRoom('general');
    await roomPage.sendMessage('Hello without presence!');

    // The message avatar should NOT have a presence indicator
    // DOM: button.absolute > div.relative (UserAvatar) — no span.rounded-full child
    const messageArticle = page.locator('[role="article"]', { hasText: 'Hello without presence!' });
    const avatarPresenceDot = messageArticle.locator('button.absolute span.rounded-full');
    await expect(avatarPresenceDot).not.toBeVisible();
  });
});

test.describe('Member list display format', () => {
  test('shows display name and @username in two rows', async ({ page, chatPage }) => {
    // Create user with specific login
    const user = await createAndLoginTestUser(page);
    await chatPage.goto();

    // Navigate to general room to see member list
    const roomPage = await chatPage.enterRoom('general');

    // Wait for member list to load
    await roomPage.expectMemberVisible(user.login);

    // Verify display name is shown (test users get "Test User {timestamp}")
    await roomPage.expectMemberDisplayName(user.login, user.displayName);

    // Verify @username is shown with muted styling
    await roomPage.expectMemberUsernameFormat(user.login, user.login);
  });

  test('shows custom display name when set', async ({ page, chatPage }) => {
    // Create user and set a custom display name
    const user = await createAndLoginTestUser(page);
    await chatPage.goto();

    // Set a custom display name via settings
    const settingsPage = new SettingsPage(page);
    await settingsPage.goto();
    await settingsPage.updateDisplayName('Custom Name');

    // Create account and navigate to room
    await chatPage.goto();
    const roomPage = await chatPage.enterRoom('general');

    // Wait for member list to load
    await roomPage.expectMemberVisible(user.login);

    // Verify custom display name is shown
    await roomPage.expectMemberDisplayName(user.login, 'Custom Name');

    // Verify @username still shows the login
    await roomPage.expectMemberUsernameFormat(user.login, user.login);
  });

  test('shows both users with correct display format in member list', async ({
    page,
    chatPage,
    browser,
    serverURL
  }) => {
    // User A: Create account
    const userA = await createAndLoginTestUser(page);
    await chatPage.goto();

    const roomPage = await chatPage.enterRoom('general');

    // Verify User A's display format (test users get "Test User {timestamp}")
    await roomPage.expectMemberVisible(userA.login);
    await roomPage.expectMemberDisplayName(userA.login, userA.displayName);
    await roomPage.expectMemberUsernameFormat(userA.login, userA.login);

    // User B: Create account with custom display name
    await withServerUser(
      browser!,
      serverURL,
      async ({ page: page2, user: userB, chatPage: chatPage2 }) => {
        // Set custom display name for User B
        await page2.goto(routes.settings);
        await page2.waitForURL(routes.settings);
        const displayNameInput = page2.getByPlaceholder('Enter your display name');
        await displayNameInput.fill('Bob Builder');
        await page2.getByRole('button', { name: 'Save Changes' }).click();
        await expect(page2.getByText('Profile updated')).toBeVisible();

        // User B opens the server
        await chatPage2.goto();
        await chatPage2.enterRoom('general');
        await waitForRoomReady(page2, 'general');

        // User A should see User B with custom display name
        await roomPage.expectMemberVisible(userB.login, { timeout: TIMEOUTS.REALTIME_EVENT });
        await roomPage.expectMemberDisplayName(userB.login, 'Bob Builder');
        await roomPage.expectMemberUsernameFormat(userB.login, userB.login);
      }
    );
  });
});

test.describe('Member list grouping', () => {
  test('shows Online section header with member count', async ({ page, chatPage }) => {
    // Create user and load the primary server
    const user = await createAndLoginTestUser(page);
    await chatPage.goto();

    // Navigate to general room
    const roomPage = await chatPage.enterRoom('general');

    // Wait for member list to be populated first
    await roomPage.expectMemberVisible(user.login);

    // Bootstrap server members include e2eadmin (the owner) plus the test
    // user. Connect-based presence reports the bootstrap admin as online, so
    // Online is exactly 2.
    await expect(roomPage.onlineSectionHeader).toHaveText('Online (2)', {
      timeout: TIMEOUTS.REALTIME_EVENT
    });
  });

  test('online members remain in the Online section until presence TTL expiry', async ({
    page,
    chatPage,
    browser,
    serverURL
  }) => {
    // User A: Create account and navigate to general room
    const userA = await createAndLoginTestUser(page);
    await chatPage.goto();

    const roomPage = await chatPage.enterRoom('general');

    // Initially the bootstrap admin and User A are online.
    await expect(roomPage.onlineSectionHeader).toHaveText('Online (2)', {
      timeout: TIMEOUTS.REALTIME_EVENT
    });

    // User B: Create account and open the server
    let userBLogin: string;

    await withServerUser(
      browser!,
      serverURL,
      async ({ page: page2, user: userB, chatPage: chatPage2 }) => {
        userBLogin = userB.login;

        // User B opens the server
        await chatPage2.enterRoom('general');
        await waitForRoomReady(page2, 'general');

        // User A should see the bootstrap admin, User A, and User B online.
        await expect(roomPage.onlineSectionHeader).toHaveText('Online (3)', {
          timeout: TIMEOUTS.REALTIME_EVENT
        });

        // User B should be visible and not dimmed
        const userBItem = roomPage.getMember(userB.login);
        await expect(userBItem).toBeVisible();
        await expect(userBItem).not.toHaveClass(/opacity-50/);
      }
    );

    // After User B disconnects, they remain online until TTL expires (60s).
    // We use TTL-based expiry instead of immediate deletion to support multi-device
    // scenarios. The offline transition is tested in unit tests.

    // User B should still be in the Online section (TTL hasn't expired)
    await expect(roomPage.onlineSectionHeader).toHaveText('Online (3)', {
      timeout: TIMEOUTS.UI_STANDARD
    });

    // User B should still be visible and not dimmed
    const userBItem = roomPage.getMember(userBLogin!);
    await expect(userBItem).toBeVisible();
    await expect(userBItem).not.toHaveClass(/opacity-50/);

    // User A should also not be dimmed
    const userAItem = roomPage.getMember(userA.login);
    await expect(userAItem).not.toHaveClass(/opacity-50/);
  });

  test('section counts update when members come online', async ({
    page,
    chatPage,
    browser,
    serverURL
  }) => {
    // User A: Create account and navigate to general room
    const userA = await createAndLoginTestUser(page);
    await chatPage.goto();

    const roomPage = await chatPage.enterRoom('general');

    // Wait for member list to load
    await roomPage.expectMemberVisible(userA.login);

    // Initially the bootstrap admin and User A are online.
    await expect(roomPage.onlineSectionHeader).toHaveText('Online (2)', {
      timeout: TIMEOUTS.REALTIME_EVENT
    });

    // User B: Create account and open the server
    await withServerUser(browser!, serverURL, async ({ page: page2, chatPage: chatPage2 }) => {
      await chatPage2.enterRoom('general');
      await waitForRoomReady(page2, 'general');

      // Bootstrap admin, User A, and User B should all be online.
      await expect(roomPage.onlineSectionHeader).toHaveText('Online (3)', {
        timeout: TIMEOUTS.REALTIME_EVENT
      });
    });

    // User B remains online until TTL expires (60s). We use TTL-based expiry
    // to support multi-device scenarios. The offline transition is tested in
    // unit tests which can control timing.
    await expect(roomPage.onlineSectionHeader).toHaveText('Online (3)', {
      timeout: TIMEOUTS.UI_STANDARD
    });
  });
});
