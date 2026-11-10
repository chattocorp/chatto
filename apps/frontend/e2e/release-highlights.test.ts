import { test, expect } from './setup';
import { createAndLoginTestUser, loginTestUser } from './fixtures/testUser';

/** Real sessions verify that acknowledgements reach another device and survive reload. */
test('release highlights synchronize dismissal and retain readable history', async ({
  page,
  browser,
  serverURL
}) => {
  test.setTimeout(90_000);
  const errors: string[] = [];
  page.on('pageerror', (error) => errors.push(error.message));
  const user = await createAndLoginTestUser(page);
  await page.goto('/chat');
  const badge = page.getByTestId('release-highlights-unread-dot');
  await expect(badge).toBeVisible();
  await page.locator('header').getByRole('link', { name: "What's new", exact: true }).click();
  await expect(page).toHaveURL(/\/help\/whats-new\?server=.*#highlight-help-guides/);
  const dismiss = page.getByRole('button', { name: 'Dismiss', exact: true });
  await expect(dismiss).toBeEnabled();

  const otherDevice = await browser.newContext({ baseURL: serverURL });
  try {
    const receiver = await otherDevice.newPage();
    receiver.on('pageerror', (error) => errors.push(error.message));
    await loginTestUser(receiver, user);
    await receiver.goto('/chat');
    await expect(receiver.getByTestId('release-highlights-unread-dot')).toBeVisible();
    await dismiss.click();
    await expect(badge).toHaveCount(0);
    // Keep the other page loaded: this assertion needs real realtime delivery.
    await expect(receiver.getByTestId('release-highlights-unread-dot')).toHaveCount(0);
    await receiver.reload();
    await expect(
      receiver.locator('header').getByRole('link', { name: 'Help', exact: true })
    ).toBeVisible();
    await expect(receiver.getByTestId('release-highlights-unread-dot')).toHaveCount(0);
    await page.reload();
    await expect(page.locator('#highlight-help-guides')).toBeVisible();
    await expect(page.getByRole('button', { name: 'Dismiss', exact: true })).toHaveCount(0);
    await expect(page.getByRole('button', { name: 'Dismiss all', exact: true })).toHaveCount(0);
  } finally {
    await otherDevice.close();
  }
  expect(errors).toEqual([]);
});
