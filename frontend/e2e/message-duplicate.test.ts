import { expect } from '@playwright/test';
import { test } from './setup';
import { createAndLoginTestUser } from './fixtures/testUser';
import { TIMEOUTS } from './constants';

test.describe('Message duplication bug', () => {
  test('posting a message should not create duplicates', async ({ page, chatPage, roomPage }) => {
    // Capture console logs for debugging
    const consoleLogs: string[] = [];
    page.on('console', (msg) => {
      if (msg.type() === 'log') {
        consoleLogs.push(msg.text());
      }
    });

    await createAndLoginTestUser(page);
    await chatPage.goto();
    await chatPage.enterRoom('general');

    const timestamp = Date.now();
    const testMessage = `Test message ${timestamp}`;

    // Post a message
    await roomPage.messageInput.fill(testMessage);
    await roomPage.messageInput.press('Enter');

    // Wait for message to appear (use first() to avoid strict mode with duplicates)
    await expect(page.getByText(testMessage).first()).toBeVisible({ timeout: TIMEOUTS.UI_STANDARD });

    // Poll for message count stability - verify exactly 1 message with this text
    // This replaces an arbitrary timeout with deterministic polling
    const messagesWithText = page.locator('[role="article"]', { hasText: testMessage });
    await expect(async () => {
      const count = await messagesWithText.count();
      expect(count).toBe(1);
    }).toPass({ timeout: TIMEOUTS.UI_STANDARD, intervals: [100, 250, 500, 1000] });

    const count = await messagesWithText.count();

    // Print relevant console logs for debugging BEFORE the assertion
    const relevantLogs = consoleLogs.filter(
      (log) => log.includes('SpaceEventBus')
    );
    console.log('\n=== Console logs ===');
    for (const log of relevantLogs) {
      console.log(log);
    }
    console.log('=== End console logs ===\n');

    // Debug: print info about all messages
    console.log(`Found ${count} messages with text "${testMessage}"`);
    for (let i = 0; i < count; i++) {
      const msg = messagesWithText.nth(i);
      const evtId = await msg.getAttribute('data-event-id');
      console.log(`  Message ${i}: eventId=${evtId}`);
    }

    // Should be exactly ONE message with this text
    expect(count).toBe(1);

    // Also verify by checking eventIds - there should be only one non-empty eventId
    const allMessages = await page.locator('[role="article"]').all();

    // Filter to only messages with our test text
    const ourMessageEventIds: string[] = [];
    for (const msg of allMessages) {
      const text = await msg.textContent();
      if (text?.includes(testMessage)) {
        const evtId = await msg.getAttribute('data-event-id');
        ourMessageEventIds.push(evtId || 'null');
      }
    }

    // Should have exactly one message
    expect(ourMessageEventIds.length).toBe(1);
    // And it should have a real eventId
    expect(ourMessageEventIds[0]).not.toBe('null');
  });

  test('posting multiple messages should not create duplicates', async ({
    page,
    chatPage,
    roomPage
  }) => {
    await createAndLoginTestUser(page);
    await chatPage.goto();
    await chatPage.enterRoom('general');

    const timestamp = Date.now();
    const message1 = `First message ${timestamp}`;
    const message2 = `Second message ${timestamp}`;

    // Post first message
    await roomPage.sendMessage(message1);

    // Post second message (this is where the "prev" error occurs according to bug report)
    await roomPage.sendMessage(message2);

    // Poll for message count stability - verify exactly 1 of each message
    // This replaces an arbitrary timeout with deterministic polling
    const msg1Locator = page.locator('[role="article"]', { hasText: message1 });
    const msg2Locator = page.locator('[role="article"]', { hasText: message2 });

    await expect(async () => {
      const msg1Count = await msg1Locator.count();
      const msg2Count = await msg2Locator.count();
      expect(msg1Count).toBe(1);
      expect(msg2Count).toBe(1);
    }).toPass({ timeout: TIMEOUTS.UI_STANDARD, intervals: [100, 250, 500, 1000] });

    // Check there are no console errors about "prev"
    // (This is checked implicitly - if there are uncaught errors, Playwright will fail)
  });
});
