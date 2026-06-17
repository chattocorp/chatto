import { expect } from '@playwright/test';
import { test } from './setup';
import { createAndLoginTestUser } from './fixtures/testUser';
import { TIMEOUTS } from './constants';

test.describe('Code block rendering', () => {
  test('posted fenced code block renders through markdown with app styles', async ({
    page,
    chatPage,
    roomPage
  }) => {
    await createAndLoginTestUser(page);
    await chatPage.goto();
    await chatPage.enterRoom('general');

    const longLine = 'const result = ' + 'veryLongVariableName + '.repeat(20) + '"end"';
    await roomPage.messageInput.fill('```javascript\n' + longLine + '\n```');
    await roomPage.sendButton.click();

    const preBlock = page.locator('pre.hljs');
    await expect(preBlock).toBeVisible({ timeout: TIMEOUTS.UI_STANDARD });
    await expect(page.getByText('```javascript')).not.toBeVisible();

    const codeElement = preBlock.locator('code');
    await codeElement.evaluate((el) => {
      el.scrollLeft = 200;
    });

    expect(await preBlock.evaluate((el) => window.getComputedStyle(el).overflowX)).toBe('hidden');
    expect(await codeElement.evaluate((el) => window.getComputedStyle(el).overflowX)).toBe('auto');
  });
});
