import { test, expect } from './setup';

test('a server-hosted frontend retains its origin and local sign-in entry', async ({ page }) => {
  await page.goto('/');
  await expect(page).toHaveURL(/\/login$/);
  await expect(page.getByLabel('Username or Email')).toBeVisible();
  await page.getByRole('link', { name: 'Add Server', exact: true }).click();
  await expect(page).toHaveURL(/\/chat\/servers$/);
  await expect(page.getByRole('heading', { name: 'Your servers' })).toBeVisible();
  const saved = page.getByTestId('saved-server-entry');
  await expect(saved).toHaveCount(1);
  await saved.getByRole('link', { name: 'Open', exact: true }).click();
  await expect(page).toHaveURL(/\/login(?:\?|$)/);
  await expect(page.getByLabel('Username or Email')).toBeVisible();
});

test('a static frontend starts at the directory without registering its host', async ({ page }) => {
  // Serve the normal static app shell, but provide no Chatto discovery API.
  await page.route('**/chatto.discovery.v1.ServerDiscoveryService/GetServer', (route) =>
    route.fulfill({ status: 404, body: 'Not found' })
  );
  await page.goto('/');
  await expect(page).toHaveURL(/\/chat\/servers$/);
  await expect(page.getByLabel('Server URL')).toBeVisible();
  await expect(page.getByTestId('saved-server-entry')).toHaveCount(0);
  await expect(page.getByRole('heading', { name: 'Choose a server to get started' })).toHaveCount(
    0
  );
  await expect
    .poll(() => page.evaluate(() => JSON.parse(localStorage.getItem('chatto:instances') ?? '[]')))
    .toEqual([]);
  await page.goto('/login');
  await expect(page).toHaveURL(/\/chat\/servers$/);
});
