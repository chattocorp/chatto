import { test, expect } from './setup';
import { createAndLoginTestUser, loginTestUser } from './fixtures/testUser';
import {
  startSecondServer,
  stopSecondServer,
  createUserOnRemote,
  connectRemoteInstance
} from './fixtures/multiServer';

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

// The frontend origin needs no account when the selected server owns the reader.
test('release history keeps the remote account with an anonymous origin', async ({
  page
}, testInfo) => {
  test.setTimeout(90_000);
  const remote = await startSecondServer(testInfo);
  try {
    const baseURL = remote.baseURL.replace('localhost', '127.0.0.1');
    const reader = await createUserOnRemote(baseURL, 'highlight-remote', 'password123');
    await connectRemoteInstance(page, { ...remote, baseURL }, reader.userId);
    await expect(page.getByTestId('release-highlights-unread-dot')).toBeVisible();
    await page.locator('header').getByRole('link', { name: "What's new", exact: true }).click();
    await expect(page).toHaveURL(/\/help\/whats-new\?server=.*#highlight-help-guides/);
    const selectedServer = new URL(page.url()).searchParams.get('server');
    expect(selectedServer).toBeTruthy();
    expect(selectedServer).not.toBe('-');
    const destinations: string[] = [];
    page.on('request', (request) => {
      const url = new URL(request.url());
      if (url.pathname.endsWith('/DismissReleaseHighlights')) destinations.push(url.origin);
    });
    await page.getByRole('button', { name: 'Dismiss', exact: true }).click();
    await expect(page.getByTestId('release-highlights-unread-dot')).toHaveCount(0);
    expect(destinations).toEqual([new URL(baseURL).origin]);
    await page.locator('header').getByRole('link', { name: 'Help', exact: true }).click();
    await expect(page).toHaveURL(
      (url) => url.pathname === '/help' && url.searchParams.get('server') === selectedServer
    );
    await page
      .getByRole('navigation', { name: 'Help', exact: true })
      .getByRole('link', { name: "What's new", exact: true })
      .click();
    await expect(page.locator('#highlight-help-guides')).toBeVisible();
    await expect(page.getByRole('button', { name: 'Dismiss', exact: true })).toHaveCount(0);
    await page.getByRole('link', { name: 'Read the guide', exact: true }).click();
    await expect(page).toHaveURL(/\/help\/conversations\?server=/);
    expect(new URL(page.url()).searchParams.get('server')).toBe(selectedServer);
    await page
      .locator('article')
      .getByRole('link', { name: 'Install Chatto on your phone', exact: true })
      .click();
    await expect(page).toHaveURL(
      (url) => url.pathname === '/help/phone' && url.searchParams.get('server') === selectedServer
    );
    await page.goBack();
    await page.goBack();
    await page.goForward();
    await expect(page).toHaveURL(/\/help\/conversations\?server=/);
    await page.goBack();
    await page.reload();
    expect(new URL(page.url()).searchParams.get('server')).toBe(selectedServer);
    await expect(page.getByRole('button', { name: 'Dismiss', exact: true })).toHaveCount(0);
  } finally {
    await stopSecondServer(remote, testInfo);
  }
});
