import { expect } from '@playwright/test';
import { test } from './setup';
import { createAndLoginTestUser } from './fixtures/testUser';
import { TIMEOUTS } from './constants';

test.describe('Code block rendering', () => {
  test('message starting with code block WITH language renders correctly', async ({
    page,
    chatPage,
    roomPage
  }) => {
    await createAndLoginTestUser(page);
    await chatPage.goto();
    await chatPage.createSpace();
    await chatPage.enterRoom('general');

    // Send a message that STARTS with a code block with language
    await roomPage.messageInput.fill('```javascript\nconsole.log("hello");\n```');
    await roomPage.sendButton.click();

    // The code block should render with syntax highlighting
    await expect(page.locator('pre.hljs')).toBeVisible({ timeout: TIMEOUTS.UI_STANDARD });

    // Should NOT see raw markdown backticks
    await expect(page.getByText('```javascript')).not.toBeVisible();
  });

  test('message starting with code block WITHOUT language renders correctly', async ({
    page,
    chatPage,
    roomPage
  }) => {
    await createAndLoginTestUser(page);
    await chatPage.goto();
    await chatPage.createSpace();
    await chatPage.enterRoom('general');

    // Send a message that starts with a code block without language
    await roomPage.messageInput.fill('```\nconsole.log("hello");\n```');
    await roomPage.sendButton.click();

    // Should render as a code block
    await expect(page.locator('pre')).toBeVisible({ timeout: TIMEOUTS.UI_STANDARD });

    // Should NOT see raw markdown
    await expect(page.getByText('```\nconsole')).not.toBeVisible();
  });

  test('code block with language AFTER text renders correctly', async ({
    page,
    chatPage,
    roomPage
  }) => {
    await createAndLoginTestUser(page);
    await chatPage.goto();
    await chatPage.createSpace();
    await chatPage.enterRoom('general');

    // Send a message with text BEFORE the code block
    await roomPage.messageInput.fill('Check this out:\n```javascript\nconsole.log("hello");\n```');
    await roomPage.sendButton.click();

    // Should render with syntax highlighting
    await expect(page.locator('pre.hljs')).toBeVisible({ timeout: TIMEOUTS.UI_STANDARD });
  });

  test('language label stays fixed when code block is scrolled horizontally', async ({
    page,
    chatPage,
    roomPage
  }) => {
    await createAndLoginTestUser(page);
    await chatPage.goto();
    await chatPage.createSpace();
    await chatPage.enterRoom('general');

    // Send a code block with a very long line to trigger horizontal scroll
    const longLine = 'const result = ' + 'veryLongVariableName + '.repeat(20) + '"end"';
    await roomPage.messageInput.fill('```javascript\n' + longLine + '\n```');
    await roomPage.sendButton.click();

    const preBlock = page.locator('pre.hljs');
    await expect(preBlock).toBeVisible({ timeout: TIMEOUTS.UI_STANDARD });

    // Get the code element inside the pre block (the scrollable container)
    const codeElement = preBlock.locator('code');

    // Scroll the code element to the right
    await codeElement.evaluate((el) => {
      el.scrollLeft = 200;
    });

    // The language label (::after pseudo-element) should stay within the
    // visible bounds of the pre block. We verify this by checking that
    // the pre block's overflow is hidden (label is positioned on pre, not code)
    const preOverflow = await preBlock.evaluate((el) => {
      return window.getComputedStyle(el).overflowX;
    });
    expect(preOverflow).toBe('hidden');

    // And the code element should be the one that scrolls
    const codeOverflow = await codeElement.evaluate((el) => {
      return window.getComputedStyle(el).overflowX;
    });
    expect(codeOverflow).toBe('auto');
  });
});
