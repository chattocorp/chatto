import { expect } from '@playwright/test';
import { test } from './setup';
import { createAndLoginTestUser } from './fixtures/testUser';
import { withLoggedInServerWindow } from './fixtures/serverUser';
import { waitForRoomReady } from './fixtures/realtimeSync';
import { TIMEOUTS } from './constants';
import * as routes from './routes';

test.describe('Composer drafts', () => {
  test('drafts are tab-specific and do not leak to other tabs', async ({
    page,
    chatPage,
    roomPage,
    browser,
    serverURL
  }) => {
    // Create user and load the primary server
    const user = await createAndLoginTestUser(page);
    await chatPage.goto();
    await chatPage.enterRoom('general');

    // Get the room URL for the second tab
    const roomUrl = page.url();

    // Type a draft message in tab 1
    const draftText = `Draft message ${Date.now()}`;
    await roomPage.messageInput.fill(draftText);

    // Verify the draft is in tab 1
    await expect(roomPage.messageInput).toHaveText(draftText);

    // Open a second tab with the same user in the same room
    await withLoggedInServerWindow(
      browser!,
      serverURL,
      user,
      async ({ page: page2, roomPage: roomPage2 }) => {
        // Navigate to the same room
        await page2.goto(roomUrl);
        await page2.waitForURL(routes.patterns.anyRoom);

        // The message input in tab 2 should be empty (not showing tab 1's draft)
        await expect(roomPage2.messageInput).toHaveText('');

        // Type a different draft in tab 2
        const draftText2 = `Different draft ${Date.now()}`;
        await roomPage2.messageInput.fill(draftText2);

        // Verify tab 2 has its own draft
        await expect(roomPage2.messageInput).toHaveText(draftText2);

        // Go back to tab 1 and verify its draft is unchanged
        await expect(roomPage.messageInput).toHaveText(draftText);
      },
      { viewport: { width: 1280, height: 720 } }
    );
  });

  test('draft persists when navigating away and back to room', async ({
    page,
    chatPage,
    roomPage
  }) => {
    // Create user and load the primary server
    await createAndLoginTestUser(page);
    await chatPage.goto();
    await chatPage.enterRoom('general');

    // Type a draft message
    const draftText = `Draft that should persist ${Date.now()}`;
    await roomPage.messageInput.fill(draftText);

    // Navigate to a different room
    await chatPage.enterRoom('announcements');

    // The input should be empty in the new room
    await expect(roomPage.messageInput).toHaveText('');

    // Navigate back to general
    await chatPage.enterRoom('general');

    // The draft should be restored
    await expect(roomPage.messageInput).toHaveText(draftText);
  });

  test('draft image attachments persist when navigating away and back to room', async ({
    page,
    chatPage,
    roomPage
  }) => {
    await createAndLoginTestUser(page);
    await chatPage.goto();
    await chatPage.enterRoom('general');

    // Attach an image in general
    await roomPage.selectAttachment('e2e/fixtures/brighton.jpg');
    await expect(roomPage.attachmentPreview).toBeVisible();

    // Navigate away to announcements
    await chatPage.enterRoom('announcements');
    await expect(roomPage.attachmentPreview).not.toBeVisible();

    // Navigate back to general - attachment should be restored
    await chatPage.enterRoom('general');
    await expect(roomPage.attachmentPreview).toBeVisible();
  });
});

test.describe('Composer focus', () => {
  test('clicking empty area in composer focuses the text input', async ({
    page,
    chatPage,
    roomPage
  }) => {
    await createAndLoginTestUser(page);
    await chatPage.goto();
    await chatPage.enterRoom('general');
    await waitForRoomReady(page, 'general');

    // Click somewhere else first to blur the composer
    await page.getByRole('heading').first().click();

    // Verify the editor is not focused
    const editor = roomPage.messageInput;
    await expect(editor).not.toBeFocused();

    // Click the composer container area (the outer padding area, not the editor itself).
    // The composer wrapper contains the input container with the editor inside.
    // Clicking its padding should focus the editor.
    const composerContainer = page.locator('.flex.flex-col.gap-2.p-2').filter({
      has: editor
    });
    const box = await composerContainer.boundingBox();
    expect(box).not.toBeNull();

    // Click near the top-left padding area of the composer (away from buttons and editor)
    await page.mouse.click(box!.x + 5, box!.y + 5);

    // The editor should now be focused
    await expect(editor).toBeFocused({ timeout: TIMEOUTS.UI_FAST });
  });

  test('clicking attach button opens file dialog, not just focus', async ({ page, chatPage }) => {
    await createAndLoginTestUser(page);
    await chatPage.goto();
    await chatPage.enterRoom('general');
    await waitForRoomReady(page, 'general');

    // The attach button should trigger its own behavior, not be intercepted
    const attachButton = page.getByTitle('Attach file');
    await expect(attachButton).toBeVisible();

    // Set up a listener for the file chooser dialog
    const fileChooserPromise = page.waitForEvent('filechooser', { timeout: TIMEOUTS.UI_STANDARD });
    await attachButton.click();

    // The file dialog should open (proving the button handled the click, not the composer)
    const fileChooser = await fileChooserPromise;
    expect(fileChooser).toBeTruthy();
  });
});

test.describe('Composer simple/rich keyboard modes', () => {
  test('sends simple text with Enter and hides the rich shortcut hint', async ({
    page,
    chatPage,
    roomPage
  }) => {
    await createAndLoginTestUser(page);
    await chatPage.goto();
    await chatPage.enterRoom('general');
    await waitForRoomReady(page, 'general');

    const message = `Return again send ${Date.now()}`;
    await roomPage.waitForInputEditable();
    await roomPage.messageInput.fill(message);
    await expect(page.getByText(/(?:Cmd|Ctrl)\+Return to Send/)).not.toBeVisible();

    await roomPage.messageInput.press('Enter');
    await expect(roomPage.getMessage(message).locator).toBeVisible({ timeout: TIMEOUTS.UI_FAST });
    await expect(roomPage.messageInput).toHaveText('');
  });

  test('activates rich mode with Control+Enter before sending', async ({
    page,
    chatPage,
    roomPage
  }) => {
    await createAndLoginTestUser(page);
    await chatPage.goto();
    await chatPage.enterRoom('general');
    await waitForRoomReady(page, 'general');

    const message = `Manual rich send ${Date.now()}`;
    await roomPage.waitForInputEditable();
    await roomPage.messageInput.fill(message);

    await roomPage.messageInput.press('Control+Enter');
    await expect(roomPage.getMessage(message).locator).not.toBeVisible();
    await expect(roomPage.messageInput.locator(':scope > p')).toHaveCount(2);
    await expect(page.getByText(/(?:Return|Enter) again to Send/)).toBeVisible();

    await roomPage.messageInput.press('Control+Enter');
    await expect(roomPage.getMessage(message).locator).toBeVisible({ timeout: TIMEOUTS.UI_FAST });
    await expect(roomPage.messageInput).toHaveText('');
  });

  test('sends from the visible trailing paragraph after exiting a list', async ({
    page,
    chatPage,
    roomPage
  }) => {
    await createAndLoginTestUser(page);
    await chatPage.goto();
    await chatPage.enterRoom('general');
    await waitForRoomReady(page, 'general');

    await roomPage.waitForInputEditable();
    await roomPage.messageInput.fill('- first');
    await expect(page.getByText(/(?:Cmd|Ctrl)\+Return to Send/)).toBeVisible();

    await roomPage.messageInput.press('Enter');
    await expect(roomPage.messageInput.locator('ul li')).toHaveCount(2);
    await expect(page.getByText(/(?:Cmd|Ctrl)\+Return to Send/)).toBeVisible();

    await roomPage.messageInput.press('Enter');
    await expect(roomPage.messageInput.locator('ul li')).toHaveCount(1);
    await expect(roomPage.messageInput.locator(':scope > p')).toHaveCount(1);
    await expect(page.getByText(/(?:Return|Enter) again to Send/)).toBeVisible();

    await roomPage.messageInput.press('Enter');
    await expect(roomPage.getMessage('first').locator).toBeVisible({ timeout: TIMEOUTS.UI_FAST });
    await expect(roomPage.messageInput).toHaveText('');
  });
});

test.describe('Composer links', () => {
  test('typing space after a pasted autolink leaves the link', async ({
    page,
    chatPage,
    roomPage
  }) => {
    await createAndLoginTestUser(page);
    await chatPage.goto();
    await chatPage.enterRoom('general');
    await waitForRoomReady(page, 'general');

    const url = 'https://www.spiegel.de/';
    await roomPage.waitForInputEditable();
    await roomPage.messageInput.click();
    await roomPage.messageInput.evaluate((element, pastedUrl) => {
      const dataTransfer = new DataTransfer();
      dataTransfer.setData('text/plain', pastedUrl);
      element.dispatchEvent(
        new ClipboardEvent('paste', {
          bubbles: true,
          cancelable: true,
          clipboardData: dataTransfer
        })
      );
    }, url);

    const link = roomPage.messageInput.locator('a');
    await expect(link).toHaveText(url);
    await expect(link).toHaveAttribute('href', url);

    await roomPage.messageInput.pressSequentially(' after');

    await expect(link).toHaveText(url);
    await expect(roomPage.messageInput).toHaveText(`${url} after`);
  });
});

// Use #general (postable) as the starting room and a freshly-created custom
// room (also postable) as the navigation target. We can't use #announcements
// — its special permissions deny message.post for regular members, which
// leaves the composer's contenteditable disabled, so focus can never land
// on it regardless of the navigation behaviour we're testing.

async function setupTwoRooms(
  page: import('@playwright/test').Page,
  chatPage: import('./pages').ChatPage
): Promise<string> {
  await createAndLoginTestUser(page);
  await chatPage.goto();
  const targetRoom = await chatPage.createRoom();
  await chatPage.enterRoom('general');
  await waitForRoomReady(page, 'general');
  return targetRoom;
}

async function navigateViaSidebar(
  page: import('@playwright/test').Page,
  chatPage: import('./pages').ChatPage,
  targetRoom: string
) {
  // Move focus off the composer onto a sidebar link — a faithful proxy for
  // "user clicked a sidebar room link, then we navigate".
  const targetLink = chatPage.roomList.getByRole('link', { name: `# ${targetRoom}` });
  await targetLink.focus();
  await targetLink.click();
  await waitForRoomReady(page, targetRoom);
}

async function navigateViaQuickSwitcher(page: import('@playwright/test').Page, targetRoom: string) {
  const isMac = process.platform === 'darwin';
  await page.keyboard.press(isMac ? 'Meta+k' : 'Control+k');
  const dialog = page.locator('dialog.quick-switcher');
  await expect(dialog).toBeVisible({ timeout: TIMEOUTS.UI_FAST });

  // Filter to the target room and pick it via Enter. The <dialog>'s close()
  // wants to return focus to its invoker — the composer must win that race
  // on desktop, and stay out of the way on touch devices.
  await dialog.getByPlaceholder('Go to server, room, or conversation...').fill(`#${targetRoom}`);
  await expect(
    dialog.locator('button.sidebar-item').filter({ hasText: `#${targetRoom}` })
  ).toBeVisible({ timeout: TIMEOUTS.UI_STANDARD });
  await page.keyboard.press('Enter');

  await expect(dialog).not.toBeVisible({ timeout: TIMEOUTS.UI_FAST });
  await waitForRoomReady(page, targetRoom);
}

test.describe('Composer auto-focus on navigation (desktop)', () => {
  test('clicking a room in the sidebar focuses the composer', async ({
    page,
    chatPage,
    roomPage
  }) => {
    const targetRoom = await setupTwoRooms(page, chatPage);
    await navigateViaSidebar(page, chatPage, targetRoom);
    await expect(roomPage.messageInput).toBeFocused({ timeout: TIMEOUTS.UI_STANDARD });
  });

  test('selecting a room in the quick switcher focuses the composer', async ({
    page,
    chatPage,
    roomPage
  }) => {
    const targetRoom = await setupTwoRooms(page, chatPage);
    await navigateViaQuickSwitcher(page, targetRoom);
    await expect(roomPage.messageInput).toBeFocused({ timeout: TIMEOUTS.UI_STANDARD });
  });
});

test.describe('Composer auto-focus on navigation (touch device)', () => {
  // `isMobile: true` on Chromium makes `(pointer: coarse)` match, which is
  // what `shouldAutoFocus()` reads. We deliberately keep a desktop-sized
  // viewport so the sidebar is visible (no hamburger) — this isolates the
  // touch-detection gate from the mobile-layout chrome.
  test.use({ hasTouch: true, isMobile: true, viewport: { width: 1280, height: 720 } });

  test('does NOT focus the composer on sidebar navigation', async ({
    page,
    chatPage,
    roomPage
  }) => {
    const targetRoom = await setupTwoRooms(page, chatPage);
    await navigateViaSidebar(page, chatPage, targetRoom);

    // Wait for canPost to load (editor becomes editable). On desktop this is
    // when the autofocus effect fires — proves we've waited long enough that
    // any focus would have landed.
    await expect(roomPage.messageInput).toHaveAttribute('contenteditable', 'true', {
      timeout: TIMEOUTS.UI_STANDARD
    });
    await expect(roomPage.messageInput).not.toBeFocused();
  });

  test('does NOT focus the composer on quick switcher selection', async ({
    page,
    chatPage,
    roomPage
  }) => {
    const targetRoom = await setupTwoRooms(page, chatPage);
    await navigateViaQuickSwitcher(page, targetRoom);

    await expect(roomPage.messageInput).toHaveAttribute('contenteditable', 'true', {
      timeout: TIMEOUTS.UI_STANDARD
    });
    await expect(roomPage.messageInput).not.toBeFocused();
  });
});
