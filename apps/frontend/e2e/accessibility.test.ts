import AxeBuilder from '@axe-core/playwright';
import type { Page } from '@playwright/test';
import { test, expect } from './setup';
import { createAndLoginTestUser, loginAsAdmin, verifyAdminEmail } from './fixtures/testUser';
import * as routes from './routes';

const wcagTags = ['wcag2a', 'wcag2aa', 'wcag21a', 'wcag21aa', 'wcag22aa'];

/**
 * Scans the page, or only `include` for overlays whose page is scanned
 * elsewhere. An overlay can cover page controls while it is open, which is
 * expected and would otherwise report obscured targets.
 */
async function expectNoAccessibilityViolations(
  page: Page,
  state: string,
  include?: string
): Promise<void> {
  // Wait for a quiet period with no finite animation running. Entrance
  // animations can start after data arrives, so a single check is not enough.
  await page.evaluate(async () => {
    const deadline = performance.now() + 10_000;
    let quietSince = performance.now();
    while (performance.now() < deadline) {
      const animating = document.getAnimations().some((animation) => {
        const endTime = animation.effect?.getComputedTiming().endTime;
        return (
          animation.playState === 'running' &&
          typeof endTime === 'number' &&
          Number.isFinite(endTime)
        );
      });
      if (animating) quietSince = performance.now();
      else if (performance.now() - quietSince >= 500) return;
      await new Promise((resolve) => requestAnimationFrame(resolve));
    }
  });

  const builder = new AxeBuilder({ page }).withTags(wcagTags);
  if (include) builder.include(include);
  const { violations } = await builder.analyze();

  expect(
    violations,
    `${state} has accessibility violations:\n${violations
      .map(
        ({ id, impact, help, nodes }) =>
          `- ${impact ?? 'unknown'} ${id}: ${help}\n${nodes
            .map((node) => `  ${node.target.join(' ')}: ${node.failureSummary ?? node.html}`)
            .join('\n')}`
      )
      .join('\n')}`
  ).toEqual([]);
}

/** Waits until no region reports aria-busy, so scans see loaded content. */
async function settle(page: Page): Promise<void> {
  await page.waitForLoadState('load');
  await expect(page.locator('[aria-busy="true"]')).toHaveCount(0, { timeout: 10_000 });
}

async function scanRoute(page: Page, state: string, route: string): Promise<void> {
  await page.goto(route);
  await settle(page);
  await expectNoAccessibilityViolations(page, state);
}

test.describe('Route accessibility', () => {
  test('public authentication routes meet WCAG A and AA rules', async ({ page }) => {
    for (const [state, route, heading] of [
      ['login', routes.login, /sign in/i],
      ['registration', routes.register, /create.*account|register/i],
      ['forgot password', routes.forgotPassword, /forgot.*password|reset.*password/i]
    ] as const) {
      await page.goto(route);
      await expect(page.getByRole('heading', { name: heading })).toBeVisible();
      await expectNoAccessibilityViolations(page, state);
    }
  });

  test('desktop chat and account settings meet WCAG A and AA rules', async ({ page, chatPage }) => {
    await createAndLoginTestUser(page);
    await chatPage.goto();
    const roomPage = await chatPage.enterRoom('general');
    await expect(roomPage.messageInput).toBeVisible();
    await expectNoAccessibilityViolations(page, 'desktop room');

    await page.goto(routes.settingsAccount);
    await expect(page.getByRole('heading', { name: 'Account', exact: true })).toBeVisible();
    await expectNoAccessibilityViolations(page, 'account settings');

    await page.goto(routes.settingsNotifications);
    await expect(page.getByRole('heading', { name: /notifications/i })).toBeVisible();
    await expectNoAccessibilityViolations(page, 'notification settings');
  });

  test('mobile chat navigation meets WCAG A and AA rules', async ({ page, chatPage }) => {
    await createAndLoginTestUser(page);
    await chatPage.goto();
    const roomPage = await chatPage.enterRoom('general');
    await expect(roomPage.messageInput).toBeVisible();
    await page.setViewportSize({ width: 375, height: 667 });
    await expect(page.getByRole('button', { name: /toggle sidebar/i })).toBeVisible();
    await expectNoAccessibilityViolations(page, 'mobile room');

    await page.getByRole('button', { name: /toggle sidebar/i }).click();
    await expect(chatPage.roomList).toBeVisible();
    await expectNoAccessibilityViolations(page, 'mobile room with navigation open');
  });

  test('server administration and its room dialog meet WCAG A and AA rules', async ({
    page,
    chatPage
  }) => {
    const admin = await loginAsAdmin(page);
    await verifyAdminEmail(page, admin.id!);
    await page.goto(routes.serverAdminGeneral);
    await expect(
      page.getByRole('heading', { level: 1, name: 'General', exact: true })
    ).toBeVisible();
    await expectNoAccessibilityViolations(page, 'server administration');

    await chatPage.openCreateRoomModal();
    await expect(page.getByRole('dialog')).toBeVisible();
    await expectNoAccessibilityViolations(page, 'create room dialog');
  });

  test('public token, callback, and error pages meet WCAG A and AA rules', async ({ page }) => {
    // /setup redirects to login once a server is configured; the login scan
    // above covers that destination.
    for (const [state, route] of [
      ['reset password', routes.resetPassword('invalid-token')],
      ['registration completion', routes.registerComplete('invalid-token')],
      ['SSO confirmation', '/sso/confirm'],
      ['server callback', '/servers/callback'],
      ['error page', '/this-page-does-not-exist']
    ] as const) {
      await scanRoute(page, state, route);
    }
  });

  test('chat surfaces meet WCAG A and AA rules', async ({ page, chatPage, dmPage }) => {
    test.setTimeout(120_000);
    await createAndLoginTestUser(page);
    await chatPage.goto();
    const roomPage = await chatPage.enterRoom('general');
    await roomPage.sendMessage('Accessibility scan thread root');
    const message = roomPage.getMessage('Accessibility scan thread root');
    await message.openThread();
    await roomPage.expectThreadPaneVisible();
    await settle(page);
    await expectNoAccessibilityViolations(page, 'thread pane');

    for (const [state, route] of [
      ['threads list', routes.threads],
      ['notifications', routes.notifications],
      ['server overview', routes.serverOverview],
      ['server directory', '/chat/servers'],
      ['search', '/chat/-/search?q=scan'],
      ['OAuth consent', '/oauth/consent']
    ] as const) {
      await scanRoute(page, state, route);
    }

    await dmPage.startConversation('e2eadmin');
    await settle(page);
    await expectNoAccessibilityViolations(page, 'direct message');
  });

  test('settings pages meet WCAG A and AA rules', async ({ page }) => {
    test.setTimeout(90_000);
    await createAndLoginTestUser(page);
    for (const [state, route] of [
      ['profile settings', routes.settingsProfile],
      ['appearance settings', routes.settingsAppearance],
      ['language settings', routes.settingsLanguage],
      ['time settings', routes.settingsTime],
      ['voice settings', '/chat/-/settings/voice'],
      ['composer settings', routes.settingsComposer]
    ] as const) {
      await scanRoute(page, state, route);
    }
  });

  test('server administration pages meet WCAG A and AA rules', async ({ page, chatPage }) => {
    test.setTimeout(150_000);
    const admin = await loginAsAdmin(page);
    await verifyAdminEmail(page, admin.id!);
    await chatPage.goto();
    await chatPage.enterRoom('general');
    const roomId = page.url().split('/').pop()!;

    for (const [state, route] of [
      ['members', routes.serverAdminMembers],
      ['member detail', routes.serverAdminMember(admin.id!)],
      ['roles', routes.serverAdminPermissions],
      ['role permission matrix', routes.serverAdminPermission('admin')],
      ['bots', routes.serverAdminBots],
      ['event log', routes.serverAdmin('event-log')],
      ['invite links', routes.serverAdmin('invite-links')],
      ['moderation', routes.serverAdmin('moderation')],
      ['neighbours', routes.serverAdmin('neighbors')],
      ['security', routes.serverAdminSecurity],
      ['system', routes.serverAdminSystem],
      ['room layout editor', routes.serverAdminRooms],
      ['room management', `${routes.serverAdminRooms}/${roomId}`]
    ] as const) {
      await scanRoute(page, state, route);
    }
  });

  test('overlays meet WCAG A and AA rules', async ({ page, chatPage }) => {
    test.setTimeout(120_000);
    const user = await createAndLoginTestUser(page);
    await chatPage.goto();
    const roomPage = await chatPage.enterRoom('general');
    await roomPage.sendMessage('Accessibility scan overlay message');
    const message = roomPage.getMessage('Accessibility scan overlay message');
    await expect(message.locator).toBeVisible();

    await page.keyboard.press('ControlOrMeta+k');
    await expect(page.getByRole('dialog')).toBeVisible();
    await expectNoAccessibilityViolations(page, 'quick switcher', 'dialog[open]');
    await page.keyboard.press('Escape');

    await message.locator.hover({ position: { x: 4, y: 4 } });
    await message.locator.locator('.message-content-stack').click({ button: 'right' });
    const actionMenu = page.locator('[popover][role="menu"]');
    await expect(actionMenu).toBeVisible();
    await expectNoAccessibilityViolations(page, 'message action menu', '[popover]');

    await actionMenu.getByLabel('More reactions').first().click();
    const emojiPicker = page.getByRole('dialog', { name: 'Add reaction' });
    await expect(emojiPicker.getByPlaceholder('Search emojis...')).toBeVisible();
    await expectNoAccessibilityViolations(page, 'emoji picker', '[popover]');
    await page.keyboard.press('Escape');
    await expect(emojiPicker).toBeHidden();

    await message.locator.getByRole('button', { name: user.displayName }).first().click();
    await expect(page.locator('[popover]').first()).toBeVisible();
    await expectNoAccessibilityViolations(page, 'user profile card', '[popover]');
    await page.keyboard.press('Escape');

    await roomPage.sendAttachment('e2e/fixtures/brighton.jpg');
    await page
      .getByRole('button', { name: /View brighton\.jpg/ })
      .first()
      .click();
    await expect(page.getByRole('dialog')).toBeVisible();
    await expectNoAccessibilityViolations(page, 'attachment viewer', 'dialog[open]');
    await page.keyboard.press('Escape');
    await expect(page.getByRole('dialog')).toBeHidden();

    await page.getByRole('button', { name: 'Formatting options' }).click();
    await page.getByRole('button', { name: 'Insert timestamp' }).click();
    await expect(page.getByRole('dialog')).toBeVisible();
    await expectNoAccessibilityViolations(page, 'timestamp picker', '[popover]');
    await page.keyboard.press('Escape');
    await expect(page.getByRole('dialog')).toBeHidden();

    await page.getByTitle('Add Server').click();
    await expect(page.getByRole('dialog')).toBeVisible();
    await expectNoAccessibilityViolations(page, 'add server dialog', 'dialog[open]');
    await page.keyboard.press('Escape');
    await expect(page.getByRole('dialog')).toBeHidden();

    await page.getByTitle('Sign out').first().click();
    await expect(page.getByRole('dialog')).toBeVisible();
    await expectNoAccessibilityViolations(page, 'sign-out dialog', 'dialog[open]');
  });

  test('right-to-left layouts meet WCAG A and AA rules', async ({ page, chatPage }) => {
    await page.addInitScript(() => localStorage.setItem('chatto:locale', 'ar'));
    await createAndLoginTestUser(page);
    await chatPage.goto();
    await chatPage.enterRoom('general');
    await expect(page.locator('html')).toHaveAttribute('dir', 'rtl');
    await settle(page);
    await expectNoAccessibilityViolations(page, 'right-to-left room');

    // The navigation sidebar starts at the inline start, which is the right.
    const sidebar = await chatPage.roomList.boundingBox();
    const composer = await page.getByTestId('composer-input-surface').boundingBox();
    expect(sidebar!.x).toBeGreaterThan(composer!.x);

    await scanRoute(page, 'right-to-left profile settings', routes.settingsProfile);
  });
});

test.describe('Call accessibility', () => {
  test.use({
    serverOptions: {
      env: {
        CHATTO_LIVEKIT_ENABLED: 'true',
        CHATTO_LIVEKIT_URL: 'ws://localhost:7880',
        CHATTO_LIVEKIT_API_KEY: 'devkey',
        CHATTO_LIVEKIT_API_SECRET: 'secret'
      }
    }
  });

  test('the call panel meets WCAG A and AA rules', async ({ page, chatPage }) => {
    await createAndLoginTestUser(page);
    await chatPage.goto();
    await chatPage.enterRoom('general');
    await page
      .locator('[data-testid="room-sidebar-toggle"]:visible')
      .getByLabel('Show call')
      .click();
    await expect(page.getByTestId('call-join-button')).toBeVisible();
    await settle(page);
    await expectNoAccessibilityViolations(page, 'call panel');
  });
});
