import { expect } from '@playwright/test';
import { createAndLoginTestUser } from './fixtures/testUser';
import { waitForRoomUnread, waitForRoomRead, getRoomIdByName } from './fixtures/graphqlHelpers';
import { waitForRoomReady } from './fixtures/realtimeSync';
import { test } from './setup';
import { TIMEOUTS, POLLING_INTERVALS } from './constants';
import * as routes from './routes';
import {
  withBootstrapAdminRequest,
  withLoggedInServerWindow,
  withServerUser
} from './fixtures/serverUser';

test.describe('Multi-Tab Unread Sync', () => {
  test('entering room clears unread in other tabs via RoomMarkedAsReadEvent', async ({
    page,
    chatPage,
    browser,
    serverURL
  }) => {
    test.setTimeout(60000);

    // User A: Create account (auto-enters a room due to redirect behavior)
    const userA = await createAndLoginTestUser(page);
    await chatPage.goto();

    // Navigate User A to announcements room (not general) so general stays unread
    await chatPage.enterRoom('announcements');

    // Get room ID for general (the room that will have unread messages)
    const roomId = await getRoomIdByName(page, 'general');

    // User B: Open the server and send a message that creates unread state for User A
    await withServerUser(
      browser!,
      serverURL,
      async ({ page: page2, chatPage: chatPage2, roomPage: roomPage2 }) => {
        await chatPage2.enterRoom('general');
        await waitForRoomReady(page2, 'general');

        // User B sends message - creates unread state for User A
        const testMessage = `Test message ${Date.now()}`;
        await roomPage2.sendMessage(testMessage);

        // Wait for server to register unread state for User A
        await waitForRoomUnread(page, roomId, true);

        await withLoggedInServerWindow(
          browser!,
          serverURL,
          userA,
          async ({ page: page3, chatPage: chatPage3 }) => {
            // Tab 2 navigates to the server and enters announcements room (not general)
            // This way Tab 2 can see general's unread indicator in the room list
            await chatPage3.enterRoom('announcements');

            // Wait for Tab 2 to show room-level unread indicator for general
            await expect(async () => {
              const roomUnreadDot = page3.locator('[data-testid="room-unread-dot"]');
              await expect(roomUnreadDot).toBeVisible();
            }).toPass({ timeout: TIMEOUTS.REALTIME_EVENT, intervals: [100, 250, 500, 1000] });

            // Wait for WebSocket subscription to be established
            // networkidle waits until no network requests for 500ms, ensuring the
            // GraphQL subscription connection is established before we trigger events
            await page3.waitForLoadState('networkidle');

            // Tab 1: User A enters general room (this auto-marks room as read and emits RoomMarkedAsReadEvent)
            await chatPage.enterRoom('general');
            await waitForRoomReady(page, 'general');

            // Tab 2: Should receive RoomMarkedAsReadEvent and clear room-level unread indicator
            await expect(async () => {
              const roomUnreadDot = page3.locator('[data-testid="room-unread-dot"]');
              await expect(roomUnreadDot).not.toBeVisible();
            }).toPass({ timeout: TIMEOUTS.REALTIME_EVENT, intervals: [100, 250, 500, 1000] });
          }
        );
      }
    );
  });
});

test.describe('Multi-window unread sync', () => {
  test('unread indicator appears in second window when message posted by another user', async ({
    page,
    chatPage,
    roomPage,
    browser,
    serverURL
  }) => {
    test.setTimeout(60000); // Multi-user test with real-time events needs more time

    // User A: Create account
    const userA = await createAndLoginTestUser(page);
    await chatPage.goto();

    // User A visits general room then leaves to announcements
    await chatPage.enterRoom('general');
    await waitForRoomReady(page, 'general');
    await roomPage.sendMessage('First window ready');

    // Get the general room ID for polling
    const generalRoomId = await getRoomIdByName(page, 'general');

    // Wait for server to confirm room is read
    await waitForRoomRead(page, generalRoomId);

    // User A navigates away from general to announcements
    await chatPage.enterRoom('announcements');
    await waitForRoomReady(page, 'announcements');

    // User A opens second window (same account) - also in announcements
    await withLoggedInServerWindow(
      browser!,
      serverURL,
      userA,
      async ({ page: page2, chatPage: chatPage2 }) => {
        // Navigate to announcements in second window (not general)
        await chatPage2.enterRoom('announcements');
        await waitForRoomReady(page2, 'announcements');

        // User B: Create account, open the server, post in general
        await withServerUser(
          browser!,
          serverURL,
          async ({ page: page3, chatPage: chatPage3, roomPage: roomPage3 }) => {
            // User B posts in general room
            await chatPage3.enterRoom('general');
            await waitForRoomReady(page3, 'general');
            await roomPage3.sendMessage(`Message from User B at ${Date.now()}`);

            // Wait for server to register the unread state for User A
            await waitForRoomUnread(page2, generalRoomId, true);

            // Both windows should see unread indicator on general room
            const generalLink1 = page.locator('nav').locator('a', { hasText: '# general' });
            const generalLink2 = page2.locator('nav').locator('a', { hasText: '# general' });

            await expect(async () => {
              await expect(generalLink1).toHaveClass(/font-semibold/);
              await expect(generalLink2).toHaveClass(/font-semibold/);
            }).toPass({ timeout: TIMEOUTS.REALTIME_EVENT, intervals: [100, 250, 500, 1000] });
          }
        );
      }
    );
  });
});

test.describe('Unread indicators', () => {
  test('shows unread indicator when another user posts a message to a different room', async ({
    page,
    chatPage,
    browser,
    serverURL
  }) => {
    test.setTimeout(60000); // Multi-user test with real-time events needs more time

    // User A: Create account and navigate to announcements room
    // (User A stays in announcements while User B posts in general)
    await createAndLoginTestUser(page);
    await chatPage.goto();

    // Navigate to "announcements" room (User A will observe from here)
    await chatPage.enterRoom('announcements');
    await waitForRoomReady(page, 'announcements');

    // Get the general room ID for polling
    const generalRoomId = await getRoomIdByName(page, 'general');

    // Verify general has no unread indicator
    const generalLink = chatPage.roomList.locator('a', { hasText: '# general' });
    await expect(generalLink).toBeVisible();
    await expect(generalLink).not.toHaveClass(/font-semibold/);

    // User B: Create account and open the server
    await withServerUser(
      browser!,
      serverURL,
      async ({ page: page2, chatPage: chatPage2, roomPage: roomPage2 }) => {
        // User B enters general room
        await chatPage2.enterRoom('general');
        await waitForRoomReady(page2, 'general');

        // User B sends a message
        const testMessage = `Hello from User B at ${Date.now()}`;
        await roomPage2.sendMessage(testMessage);

        // Wait for server to register the unread state
        await waitForRoomUnread(page, generalRoomId, true);

        // User A: Verify unread indicator appears on "general"
        await expect(async () => {
          await expect(generalLink).toHaveClass(/font-semibold/);
          const unreadDot = generalLink.locator('[data-testid="room-unread-dot"]');
          await expect(unreadDot).toBeVisible();
        }).toPass({ timeout: TIMEOUTS.REALTIME_EVENT, intervals: [100, 250, 500, 1000] });

        // User A: Navigate to general room
        await generalLink.click();
        await page.waitForURL(routes.patterns.anyRoom);

        // Verify the message is visible
        await expect(page.getByText(testMessage)).toBeVisible();

        // Wait for server to confirm room is read
        await waitForRoomRead(page, generalRoomId);

        // Verify the unread indicator is now gone
        await expect(async () => {
          await expect(generalLink).not.toHaveClass(/font-semibold/);
          const unreadDot = generalLink.locator('[data-testid="room-unread-dot"]');
          await expect(unreadDot).not.toBeVisible();
        }).toPass({ timeout: TIMEOUTS.REALTIME_EVENT, intervals: [100, 250, 500, 1000] });
      }
    );
  });

  test('unread indicator clears when navigating to room', async ({ page, chatPage, roomPage }) => {
    await createAndLoginTestUser(page);
    await chatPage.goto();

    // Navigate to general room first
    await chatPage.enterRoom('general');
    await waitForRoomReady(page, 'general');

    // Send a message to mark general as "active"
    await roomPage.sendMessage('Hello from general');

    // Navigate to announcements
    await chatPage.enterRoom('announcements');
    await waitForRoomReady(page, 'announcements');

    // Both rooms should have no unread indicator since we've viewed them
    const generalLink = chatPage.roomList.locator('a', { hasText: '# general' });
    const announcementsLink = chatPage.roomList.locator('a', { hasText: '# announcements' });

    await expect(generalLink).not.toHaveClass(/font-semibold/);
    await expect(announcementsLink).not.toHaveClass(/font-semibold/);
  });
});

test.describe('Room unread separator', () => {
  test('shows unread separator when entering room with new messages', async ({
    page,
    chatPage,
    roomPage,
    browser,
    serverURL
  }) => {
    test.setTimeout(60000); // Multi-user test with real-time events needs more time

    // User A: Create account
    await createAndLoginTestUser(page);
    await chatPage.goto();

    // User A enters general room and posts initial messages
    await chatPage.enterRoom('general');
    await waitForRoomReady(page, 'general');
    await roomPage.sendMessage('Message 1 from User A');
    await roomPage.sendMessage('Message 2 from User A');

    // Get the general room ID for polling
    const generalRoomId = await getRoomIdByName(page, 'general');

    // User B: Create account, open the server
    await withServerUser(
      browser!,
      serverURL,
      async ({ page: page2, chatPage: chatPage2, roomPage: roomPage2 }) => {
        // User B enters general room (this records their last-read position)
        await chatPage2.enterRoom('general');
        await waitForRoomReady(page2, 'general');
        await roomPage2.expectMessageVisible('Message 2 from User A');

        // Wait for room to be fully loaded
        await expect(roomPage2.messageInput).toBeEnabled();

        // Wait for server to confirm room is read (replaces arbitrary timeout)
        await waitForRoomRead(page2, generalRoomId);

        // User B leaves room by navigating to announcements
        await chatPage2.enterRoom('announcements');
        await waitForRoomReady(page2, 'announcements');

        // User A posts a new message while User B is away
        const newMessage = `New message ${Date.now()}`;
        await roomPage.sendMessage(newMessage);

        // Wait for server to register the unread state for User B
        await waitForRoomUnread(page2, generalRoomId, true);

        // User B re-enters general room
        await chatPage2.enterRoom('general');
        await waitForRoomReady(page2, 'general');

        // Wait for the message to arrive, then check separator
        await roomPage2.expectMessageVisible(newMessage);
        await roomPage2.expectUnreadSeparator();
      }
    );
  });

  test('does not show unread separator when entering room for the first time', async ({
    page,
    chatPage,
    roomPage,
    browser,
    serverURL
  }) => {
    test.setTimeout(60000); // Multi-user test needs more time

    // User A: Create account
    await createAndLoginTestUser(page);
    await chatPage.goto();

    // User A enters general room and posts a message
    await chatPage.enterRoom('general');
    await waitForRoomReady(page, 'general');
    await roomPage.sendMessage('Welcome message from creator');

    // User B: Create account, open the server
    await withServerUser(
      browser!,
      serverURL,
      async ({ page: page2, chatPage: chatPage2, roomPage: roomPage2 }) => {
        // User B enters general room for the first time
        await chatPage2.enterRoom('general');
        await waitForRoomReady(page2, 'general');
        await roomPage2.expectMessageVisible('Welcome message from creator');

        // No unread separator should be shown - this is the first visit
        await roomPage2.expectNoUnreadSeparator();
      }
    );
  });

  test('unread separator position stays fixed when new messages arrive', async ({
    page,
    chatPage,
    roomPage,
    browser,
    serverURL
  }) => {
    test.setTimeout(60000); // Multi-user test with real-time events needs more time

    // User A: Create account
    await createAndLoginTestUser(page);
    await chatPage.goto();

    // User A enters general room and posts initial message
    await chatPage.enterRoom('general');
    await waitForRoomReady(page, 'general');
    await roomPage.sendMessage('Initial message');

    // Get the general room ID for polling
    const generalRoomId = await getRoomIdByName(page, 'general');

    // User B: Create account, open the server
    await withServerUser(
      browser!,
      serverURL,
      async ({ page: page2, chatPage: chatPage2, roomPage: roomPage2 }) => {
        // User B enters general, then leaves
        await chatPage2.enterRoom('general');
        await waitForRoomReady(page2, 'general');
        await roomPage2.expectMessageVisible('Initial message');

        // Wait for server to confirm room is read
        await waitForRoomRead(page2, generalRoomId);

        await chatPage2.enterRoom('announcements');
        await waitForRoomReady(page2, 'announcements');

        // User A posts first unread message
        const unreadMsg1 = `Unread 1 ${Date.now()}`;
        await roomPage.sendMessage(unreadMsg1);

        // Wait for server to register unread state
        await waitForRoomUnread(page2, generalRoomId, true);

        // User B re-enters - should see separator before unreadMsg1
        await chatPage2.enterRoom('general');
        await waitForRoomReady(page2, 'general');
        await roomPage2.expectMessageVisible(unreadMsg1);
        await roomPage2.expectUnreadSeparator();

        // User A posts another message while User B is viewing
        const unreadMsg2 = `Unread 2 ${Date.now()}`;
        await roomPage.sendMessage(unreadMsg2);

        // Wait for message to arrive
        await roomPage2.expectMessageVisible(unreadMsg2);

        // Separator should still be visible (position doesn't change)
        await roomPage2.expectUnreadSeparator();
      }
    );
  });

  test('unread separator is deferred until the hidden tab returns', async ({
    page,
    chatPage,
    roomPage,
    browser,
    serverURL
  }) => {
    test.setTimeout(60000); // Multi-user test with real-time events needs more time

    // User A: Create account, post an initial message in general.
    await createAndLoginTestUser(page);
    await chatPage.goto();

    await chatPage.enterRoom('general');
    await waitForRoomReady(page, 'general');
    await roomPage.sendMessage('Initial message');

    const generalRoomId = await getRoomIdByName(page, 'general');

    // User B: Create account, open the server, and sit in general — present and
    // caught up, never navigating away.
    await withServerUser(
      browser!,
      serverURL,
      async ({ page: page2, chatPage: chatPage2, roomPage: roomPage2 }) => {
        await chatPage2.enterRoom('general');
        await waitForRoomReady(page2, 'general');
        await roomPage2.expectMessageVisible('Initial message');
        await waitForRoomRead(page2, generalRoomId);

        // No separator yet — User B has read everything.
        await roomPage2.expectNoUnreadSeparator();

        // User B's tab goes to the background. They stay in the room, but the
        // rendered separator should not change until they return.
        await page2.evaluate(() => {
          Object.defineProperty(document, 'visibilityState', {
            value: 'hidden',
            writable: true,
            configurable: true
          });
          document.dispatchEvent(new Event('visibilitychange'));
        });

        // User A posts while User B's tab is still hidden.
        const awayMessage = `Posted while hidden ${Date.now()}`;
        await roomPage.sendMessage(awayMessage);

        // The message streams in over the live subscription, but the in-room
        // separator is deferred so Chatto does not visibly repaint the marker
        // while the user is away.
        await roomPage2.expectMessageVisible(awayMessage);
        await expect(async () => {
          await roomPage2.expectNoUnreadSeparator();
        }).toPass({ timeout: TIMEOUTS.UI_STANDARD, intervals: POLLING_INTERVALS });

        // Re-focusing the tab reveals the deferred separator and keeps it
        // stable across the mark-read round-trip.
        await page2.evaluate(() => {
          Object.defineProperty(document, 'visibilityState', {
            value: 'visible',
            writable: true,
            configurable: true
          });
          document.dispatchEvent(new Event('visibilitychange'));
        });

        await expect(async () => {
          await roomPage2.expectUnreadSeparator();
        }).toPass({ timeout: TIMEOUTS.UI_STANDARD, intervals: POLLING_INTERVALS });
      }
    );
  });

  test('unread separator is deferred until the blurred window is focused again', async ({
    page,
    chatPage,
    roomPage,
    browser,
    serverURL
  }) => {
    test.setTimeout(60000); // Multi-user test with real-time events needs more time

    // User A: Create account, post an initial message in general.
    await createAndLoginTestUser(page);
    await chatPage.goto();

    await chatPage.enterRoom('general');
    await waitForRoomReady(page, 'general');
    await roomPage.sendMessage('Initial blur message');

    const generalRoomId = await getRoomIdByName(page, 'general');

    // User B: Stay in general, then switch focus away without hiding the tab.
    await withServerUser(
      browser!,
      serverURL,
      async ({ page: page2, chatPage: chatPage2, roomPage: roomPage2 }) => {
        await chatPage2.enterRoom('general');
        await waitForRoomReady(page2, 'general');
        await roomPage2.expectMessageVisible('Initial blur message');
        await waitForRoomRead(page2, generalRoomId);
        await roomPage2.expectNoUnreadSeparator();

        await page2.evaluate(() => {
          window.dispatchEvent(new Event('blur'));
        });

        const awayMessage = `Posted while blurred ${Date.now()}`;
        await roomPage.sendMessage(awayMessage);
        await roomPage2.expectMessageVisible(awayMessage);
        await expect(async () => {
          await roomPage2.expectNoUnreadSeparator();
        }).toPass({ timeout: TIMEOUTS.UI_STANDARD, intervals: POLLING_INTERVALS });

        await page2.evaluate(() => {
          window.dispatchEvent(new Event('focus'));
        });

        await expect(async () => {
          await roomPage2.expectUnreadSeparator();
        }).toPass({ timeout: TIMEOUTS.UI_STANDARD, intervals: POLLING_INTERVALS });
      }
    );
  });

  test('refocus keeps the separator stable when only non-message events arrived while hidden', async ({
    page,
    chatPage,
    roomPage,
    browser,
    serverURL
  }) => {
    test.setTimeout(60000); // Multi-user test with real-time events needs more time

    // Regression test for the bug where the "New messages" separator would
    // flicker out on tab refocus when the only thing that arrived while the
    // tab was hidden was a non-message room event (join, leave). The server-
    // side read cursor only tracks root messages, so a refocus mutation
    // round-trip returned previousLastReadAt === lastReadAt and the bounded
    // window collapsed to empty. The marker is now deferred until refocus,
    // and the same-room refocus must not overwrite that deferred anchor.

    // User A: Create account, enter general, post the initial message
    // so User B has a real read cursor anchored on a root message.
    const userA = await createAndLoginTestUser(page);
    await chatPage.goto();

    await chatPage.enterRoom('general');
    await waitForRoomReady(page, 'general');
    await roomPage.sendMessage('Anchor message from User A');

    const generalRoomId = await getRoomIdByName(page, 'general');

    // User B: Open the server, enter general, read the anchor message — caught
    // up, no separator yet.
    await withServerUser(
      browser!,
      serverURL,
      async ({ page: page2, chatPage: chatPage2, roomPage: roomPage2 }) => {
        await chatPage2.enterRoom('general');
        await waitForRoomReady(page2, 'general');
        await roomPage2.expectMessageVisible('Anchor message from User A');
        await waitForRoomRead(page2, generalRoomId);
        await roomPage2.expectNoUnreadSeparator();

        // User B's tab goes hidden. The unread anchor is captured, but the
        // rendered separator is deferred until User B returns.
        await page2.evaluate(() => {
          Object.defineProperty(document, 'visibilityState', {
            value: 'hidden',
            writable: true,
            configurable: true
          });
          document.dispatchEvent(new Event('visibilitychange'));
        });

        // User A leaves general. The "User A left the room" event is a non-
        // message room event — it does NOT advance the server's root-message
        // read cursor.
        await page.getByTitle('Leave room').click();
        await page.getByRole('dialog').getByRole('button', { name: 'Leave Room' }).click();

        // User B (still hidden) receives the leave event over the live
        // subscription, but the separator should not render while hidden.
        await expect(page2.getByText(`${userA.displayName} left the room`)).toBeVisible({
          timeout: TIMEOUTS.REALTIME_EVENT
        });
        await expect(async () => {
          await roomPage2.expectNoUnreadSeparator();
        }).toPass({ timeout: TIMEOUTS.UI_STANDARD, intervals: POLLING_INTERVALS });

        // User B's tab returns to the foreground. With the bug, this refocus
        // would fire markRoomAsRead which returns previousLastReadAt ===
        // lastReadAt (server cursor never moved), the .then() would overwrite
        // the deferred open-bound anchor with an empty window, and the
        // separator would blink out.
        await page2.evaluate(() => {
          Object.defineProperty(document, 'visibilityState', {
            value: 'visible',
            writable: true,
            configurable: true
          });
          document.dispatchEvent(new Event('visibilitychange'));
        });

        // Separator must remain visible across the focus transition — it
        // shouldn't toggle just because the user came back to the tab.
        await expect(async () => {
          await roomPage2.expectUnreadSeparator();
        }).toPass({ timeout: TIMEOUTS.UI_STANDARD, intervals: POLLING_INTERVALS });

        // A second hide/show cycle to be sure: with the old code the marker
        // would reappear on blur and vanish again on focus, so a second
        // refocus must also leave it in place.
        await page2.evaluate(() => {
          Object.defineProperty(document, 'visibilityState', {
            value: 'hidden',
            writable: true,
            configurable: true
          });
          document.dispatchEvent(new Event('visibilitychange'));
        });
        await roomPage2.expectUnreadSeparator();

        await page2.evaluate(() => {
          Object.defineProperty(document, 'visibilityState', {
            value: 'visible',
            writable: true,
            configurable: true
          });
          document.dispatchEvent(new Event('visibilitychange'));
        });
        await expect(async () => {
          await roomPage2.expectUnreadSeparator();
        }).toPass({ timeout: TIMEOUTS.UI_STANDARD, intervals: POLLING_INTERVALS });
      }
    );
  });

  test('backgrounding the tab does not strand the user own latest message below the separator', async ({
    page,
    chatPage,
    roomPage,
    browser,
    serverURL
  }) => {
    test.setTimeout(60000); // Multi-user test needs more time

    // User A: Create account, post an existing message in general.
    await createAndLoginTestUser(page);
    await chatPage.goto();

    await chatPage.enterRoom('general');
    await waitForRoomReady(page, 'general');
    await roomPage.sendMessage('Existing message from User A');

    const generalRoomId = await getRoomIdByName(page, 'general');

    // User B: Create account, open the server, enter the (non-empty) room — this
    // gives User B a real read cursor — then post their own message and
    // background the tab.
    await withServerUser(
      browser!,
      serverURL,
      async ({ page: page2, chatPage: chatPage2, roomPage: roomPage2 }) => {
        await chatPage2.enterRoom('general');
        await waitForRoomReady(page2, 'general');
        await roomPage2.expectMessageVisible('Existing message from User A');
        await waitForRoomRead(page2, generalRoomId);

        // User B posts their own message. Posting auto-advances the read cursor
        // server-side, so the client cursor must follow.
        const ownMessage = `User B own message ${Date.now()}`;
        await roomPage2.sendMessage(ownMessage);
        await roomPage2.expectMessageVisible(ownMessage);
        await waitForRoomRead(page2, generalRoomId);
        await roomPage2.expectNoUnreadSeparator();

        // User B's tab goes to the background — still in the room.
        await page2.evaluate(() => {
          Object.defineProperty(document, 'visibilityState', {
            value: 'hidden',
            writable: true,
            configurable: true
          });
          document.dispatchEvent(new Event('visibilitychange'));
        });

        // The separator must not appear above User B's own latest message.
        // Negative assertion — give events time to settle before asserting absence.
        await expect(async () => {
          await roomPage2.expectNoUnreadSeparator();
        }).toPass({ timeout: TIMEOUTS.UI_STANDARD, intervals: POLLING_INTERVALS });
      }
    );
  });

  test('no unread separator for own messages after posting and reloading', async ({
    page,
    chatPage,
    roomPage
  }) => {
    // User creates account
    await createAndLoginTestUser(page);
    await chatPage.goto();

    // Enter room
    await chatPage.enterRoom('general');
    await waitForRoomReady(page, 'general');

    // Get the general room ID for polling
    const generalRoomId = await getRoomIdByName(page, 'general');

    // Post a message (this marks the room as read)
    await roomPage.sendMessage('Initial message');

    // Wait for server to confirm room is read
    await waitForRoomRead(page, generalRoomId);

    // Leave room by going to announcements
    await chatPage.enterRoom('announcements');
    await waitForRoomReady(page, 'announcements');

    // Go back and post another message
    await chatPage.enterRoom('general');
    await waitForRoomReady(page, 'general');

    // Wait for room to fully load
    await roomPage.expectMessageVisible('Initial message');

    // Post a second message (this should also update our last-read position)
    const ownMessage = `My own message ${Date.now()}`;
    await roomPage.sendMessage(ownMessage);

    // Reload the page
    await page.reload();
    await page.waitForURL(routes.patterns.anyRoom);

    // Wait for room to load
    await roomPage.expectMessageVisible(ownMessage);

    // The user's own message should NOT show the unread separator
    // (they clearly saw it since they posted it)
    await roomPage.expectNoUnreadSeparator();
  });

  test('background then refocus then post does not strand own latest message below separator', async ({
    page,
    chatPage,
    roomPage
  }) => {
    // Regression test for: user posts a message, backgrounds the tab,
    // refocuses (same room, no navigation), then posts another message —
    // a "New messages" separator must NOT appear between the two own
    // messages. The presence-false branch in useRoomUnread anchors the
    // open-window separator at lastCursor; without advancing that anchor
    // on subsequent own posts, the second message renders below the
    // separator even though the user obviously saw their first message.
    await createAndLoginTestUser(page);
    await chatPage.goto();

    await chatPage.enterRoom('general');
    await waitForRoomReady(page, 'general');
    const generalRoomId = await getRoomIdByName(page, 'general');

    // First own message — establishes a real client-side lastCursor that
    // the presence-false branch will anchor on.
    const firstMessage = `First own message ${Date.now()}`;
    await roomPage.sendMessage(firstMessage);
    await roomPage.expectMessageVisible(firstMessage);
    await waitForRoomRead(page, generalRoomId);

    // Background the tab. useRoomUnread's effect fires presence-false and
    // anchors unreadAfterTime = lastCursor, unreadBeforeTime = null (the
    // open-upper-bound window).
    await page.evaluate(() => {
      Object.defineProperty(document, 'visibilityState', {
        value: 'hidden',
        writable: true,
        configurable: true
      });
      document.dispatchEvent(new Event('visibilitychange'));
    });

    // Refocus. The effect's presence-true branch does NOT overwrite the
    // open-bound separator (intentional — preserves "you missed messages"
    // markers across blur/focus cycles).
    await page.evaluate(() => {
      Object.defineProperty(document, 'visibilityState', {
        value: 'visible',
        writable: true,
        configurable: true
      });
      document.dispatchEvent(new Event('visibilitychange'));
    });

    // Second own message — the bug case. Pre-fix this would render below
    // a "New messages" separator anchored at the first message's time.
    const secondMessage = `Second own message ${Date.now()}`;
    await roomPage.sendMessage(secondMessage);
    await roomPage.expectMessageVisible(secondMessage);

    // Give time for the separator to render if it were going to. Negative
    // assertion needs a settling window.
    await expect(async () => {
      await roomPage.expectNoUnreadSeparator();
    }).toPass({ timeout: TIMEOUTS.UI_STANDARD, intervals: POLLING_INTERVALS });
  });
});

test.describe('Unread dot stability after loadRooms refresh', () => {
  test('room unread dot does not reappear after clearing when loadRooms is triggered', async ({
    page,
    chatPage,
    roomPage,
    browser,
    serverURL
  }) => {
    test.setTimeout(60000);

    // User A: Create account
    await createAndLoginTestUser(page);
    await chatPage.goto();

    // User A enters general room and posts a message
    await chatPage.enterRoom('general');
    await waitForRoomReady(page, 'general');
    await roomPage.sendMessage('Initial message from User A');

    const generalRoomId = await getRoomIdByName(page, 'general');

    // User B: Create account, open the server
    await withServerUser(
      browser!,
      serverURL,
      async ({ page: page2, chatPage: chatPage2, roomPage: roomPage2 }) => {
        // User B enters general room (marks as read)
        await chatPage2.enterRoom('general');
        await waitForRoomReady(page2, 'general');
        await roomPage2.expectMessageVisible('Initial message from User A');
        await waitForRoomRead(page2, generalRoomId);

        // User B navigates to announcements
        await chatPage2.enterRoom('announcements');
        await waitForRoomReady(page2, 'announcements');

        // User A posts a new message → User B should see unread dot on general
        const testMessage = `Trigger unread ${Date.now()}`;
        await roomPage.sendMessage(testMessage);

        // Wait for server to register unread state
        await waitForRoomUnread(page2, generalRoomId, true);

        // User B should see unread dot
        const generalLink = chatPage2.roomList.locator('a', { hasText: '# general' });
        await expect(async () => {
          await expect(generalLink).toHaveClass(/font-semibold/);
        }).toPass({ timeout: TIMEOUTS.REALTIME_EVENT, intervals: [100, 250, 500, 1000] });

        // User B enters general room → dot should clear
        await chatPage2.enterRoom('general');
        await waitForRoomReady(page2, 'general');
        await roomPage2.expectMessageVisible(testMessage);

        // Wait for server to confirm room is read
        await waitForRoomRead(page2, generalRoomId);

        // Verify the unread dot is gone
        await expect(async () => {
          await expect(generalLink).not.toHaveClass(/font-semibold/);
          const unreadDot = generalLink.locator('[data-testid="room-unread-dot"]');
          await expect(unreadDot).not.toBeVisible();
        }).toPass({ timeout: TIMEOUTS.REALTIME_EVENT, intervals: [100, 250, 500, 1000] });

        // User B navigates to announcements (so general is not active)
        await chatPage2.enterRoom('announcements');
        await waitForRoomReady(page2, 'announcements');

        // Rename the general room → triggers RoomUpdatedEvent → loadRooms() in
        // User B. Issue #330: regular members can't manage rooms on the
        // bootstrap server, so do the rename as e2eadmin through a side request
        // context that leaves user A's page session intact.
        await withBootstrapAdminRequest(serverURL, async (adminRequest) => {
          await adminRequest.post('/api/graphql', {
            headers: { 'Content-Type': 'application/json', 'X-REQUEST-TYPE': 'GraphQL' },
            data: {
              query: `mutation($input: UpdateRoomInput!) { updateRoom(input: $input) { id name } }`,
              variables: { input: { roomId: generalRoomId, name: 'general-renamed' } }
            }
          });
        });

        // Wait for the rename to be visible in User B's room list
        const renamedLink = chatPage2.roomList.locator('a', { hasText: '# general-renamed' });
        await expect(renamedLink).toBeVisible({ timeout: TIMEOUTS.REALTIME_EVENT });

        // The renamed room should NOT show an unread dot (the loadRooms refresh
        // should not have restored the stale unread state)
        await expect(renamedLink).not.toHaveClass(/font-semibold/);
        const unreadDot = renamedLink.locator('[data-testid="room-unread-dot"]');
        await expect(unreadDot).not.toBeVisible();
      }
    );
  });
});

test.describe('Thread reply unread behavior', () => {
  test('thread reply does not cause unread dot on room or server', async ({
    page,
    chatPage,
    roomPage,
    browser,
    serverURL
  }) => {
    test.setTimeout(60000);

    // User A: Create account
    await createAndLoginTestUser(page);
    await chatPage.goto();

    // User A enters general room and posts a root message
    await chatPage.enterRoom('general');
    await waitForRoomReady(page, 'general');
    const rootMessage = `Root message ${Date.now()}`;
    const rootMsg = await roomPage.sendMessage(rootMessage);

    const generalRoomId = await getRoomIdByName(page, 'general');

    // User B: Create account, open the server
    await withServerUser(
      browser!,
      serverURL,
      async ({ page: page2, chatPage: chatPage2, roomPage: roomPage2 }) => {
        // User B enters general room (marks as read)
        await chatPage2.enterRoom('general');
        await waitForRoomReady(page2, 'general');
        await roomPage2.expectMessageVisible(rootMessage);

        // Wait for server to confirm room is read for User B
        await waitForRoomRead(page2, generalRoomId);

        // User B navigates to announcements (so general is not active)
        await chatPage2.enterRoom('announcements');
        await waitForRoomReady(page2, 'announcements');

        // User A posts a thread reply to the root message
        await rootMsg.openThread();
        await roomPage.expectThreadPaneVisible();
        const threadReply = `Thread reply ${Date.now()}`;
        await roomPage.postThreadReply(threadReply);

        // Verify server-side: room should still be read for User B
        // (waitForRoomRead polls the server, giving events time to propagate)
        await waitForRoomRead(page2, generalRoomId);

        // Verify UI: no unread dot on room — use toPass() to allow events to settle
        // before asserting absence (negative assertions need extra care)
        const generalLink = chatPage2.roomList.locator('a', { hasText: '# general' });
        const roomUnreadDot = generalLink.locator('[data-testid="room-unread-dot"]');
        await expect(async () => {
          await expect(generalLink).not.toHaveClass(/font-semibold/);
          await expect(roomUnreadDot).not.toBeVisible();
        }).toPass({ timeout: TIMEOUTS.UI_STANDARD, intervals: POLLING_INTERVALS });

        // Now User A posts a new ROOT message — this SHOULD cause unread
        await roomPage.closeThread();
        const newRootMessage = `New root message ${Date.now()}`;
        await roomPage.sendMessage(newRootMessage);

        // Wait for server to register unread state
        await waitForRoomUnread(page2, generalRoomId, true);

        // User B should see unread dot on general room
        await expect(async () => {
          await expect(generalLink).toHaveClass(/font-semibold/);
        }).toPass({ timeout: TIMEOUTS.REALTIME_EVENT, intervals: [100, 250, 500, 1000] });
      }
    );
  });
});
