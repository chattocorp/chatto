import { expect, type Locator, type Page } from '@playwright/test';
import { TIMEOUTS, POLLING_INTERVALS } from './constants';
import { createAndLoginTestUser } from './fixtures/testUser';
import { test } from './setup';

test.use({ serverOptions: { searchProvider: true } });

const SEARCH_PLACEHOLDER = 'Words, phrases, or filters...';

function searchResult(page: Page, body: string): Locator {
  return page.locator('ol [role="article"]', { hasText: body });
}

async function followSearchResult(result: Locator): Promise<void> {
  await result.locator('a:has(time)').click();
}

async function openSearch(page: Page): Promise<void> {
  const link = page.getByRole('link', { name: 'Search', exact: true });
  await expect(link).toBeVisible({ timeout: TIMEOUTS.COMPLEX_OPERATION });
  await link.click();
  await expect(page.getByRole('heading', { name: 'Search messages' })).toBeVisible();
  await expect(page.getByPlaceholder(SEARCH_PLACEHOLDER)).toBeEnabled();
}

async function submitSearch(page: Page, query: string): Promise<void> {
  await page.getByPlaceholder(SEARCH_PLACEHOLDER).fill(query);
  await page.getByRole('button', { name: 'Search', exact: true }).click();
}

async function expectSearchResult(page: Page, query: string, body: string): Promise<Locator> {
  const result = searchResult(page, body);
  await expect(async () => {
    await submitSearch(page, query);
    await expect(result).toBeVisible({ timeout: TIMEOUTS.UI_FAST });
  }).toPass({ timeout: TIMEOUTS.POLLING_EXTENDED, intervals: [...POLLING_INTERVALS] });
  return result;
}

async function expectNoSearchResult(page: Page, query: string, body: string): Promise<void> {
  const result = searchResult(page, body);
  await expect(async () => {
    await submitSearch(page, query);
    await expect(page.getByText('No messages found', { exact: true })).toBeVisible({
      timeout: TIMEOUTS.UI_FAST
    });
    await expect(result).toHaveCount(0);
  }).toPass({ timeout: TIMEOUTS.POLLING_EXTENDED, intervals: [...POLLING_INTERVALS] });
}

test.describe('message search', () => {
  test.describe.configure({ timeout: 60_000 });

  test('indexes messages, follows results, tracks edits and deletion, and enforces room access', async ({
    page,
    chatPage,
    roomPage
  }) => {
    await createAndLoginTestUser(page);
    await chatPage.goto();
    await chatPage.enterRoom('general');
    const generalRoomPath = new URL(page.url()).pathname;

    const suffix = Date.now();
    const originalTerm = `original${suffix}`;
    const editedTerm = `edited${suffix}`;
    const originalBody = `The runners were running with ${originalTerm}`;
    const editedBody = `The runners kept running with ${editedTerm}`;

    const message = await roomPage.sendMessage(originalBody);
    const messageId = await message.getEventId();
    expect(messageId).toBeTruthy();

    await test.step('find a newly indexed message through the sidebar and follow its result', async () => {
      await openSearch(page);

      // "run" exercises the configured English analyzer: the body contains
      // only inflected forms, while the unique term keeps this result isolated.
      const result = await expectSearchResult(page, `run ${originalTerm}`, originalBody);
      await followSearchResult(result);

      await expect(page).toHaveURL((url) => url.pathname === generalRoomPath);
      await roomPage.expectMessageVisible(originalBody, { timeout: TIMEOUTS.REALTIME_EVENT });
    });

    await test.step('replace the searchable body after editing', async () => {
      await roomPage.getMessageByEventId(messageId!).startEdit();
      await roomPage.completeEdit(editedBody);
      await roomPage.expectMessageVisible(editedBody);

      await openSearch(page);
      await expectNoSearchResult(page, originalTerm, originalBody);
      await expectSearchResult(page, editedTerm, editedBody);
    });

    await test.step('remove a deleted message from search', async () => {
      await followSearchResult(searchResult(page, editedBody));
      await roomPage.getMessageByEventId(messageId!).delete();

      await openSearch(page);
      await expectNoSearchResult(page, editedTerm, editedBody);
    });

    await test.step('exclude retained index documents after room access is lost', async () => {
      const roomName = await chatPage.createRoom(`search-access-${suffix}`);
      const privateTerm = `private${suffix}`;
      const privateBody = `A searchable room secret named ${privateTerm}`;
      await roomPage.sendMessage(privateBody);

      await openSearch(page);
      const result = await expectSearchResult(page, privateTerm, privateBody);
      await followSearchResult(result);
      await chatPage.expectRoomHeaderVisible(roomName);

      const roomURL = page.url();
      await page.getByTitle('Leave room').click();
      await page.getByRole('dialog').getByRole('button', { name: 'Leave Room' }).click();
      await page.waitForURL((url) => url.href !== roomURL, {
        timeout: TIMEOUTS.REALTIME_EVENT
      });

      await openSearch(page);
      await expectNoSearchResult(page, privateTerm, privateBody);
    });
  });
});
