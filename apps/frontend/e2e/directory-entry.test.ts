import { test, expect } from './setup';

test('first-run lookup remembers sources and modal lookup opens sign-in', async ({ page }) => {
  const queries: string[] = [];
  await page.route('**/chatto.discovery.v1.ServerDiscoveryService/GetServer', (route) => {
    if (new URL(route.request().url()).origin !== 'https://known.example')
      return route.fulfill({ status: 404, body: 'Not found' });
    return route.fulfill({
      json: {
        profile: { name: 'Known community', version: '0.5.0', description: 'A remembered server.' },
        login: { authorizeUrl: '/oauth/authorize' }
      }
    });
  });
  await page.route(
    '**/chatto.discovery.v1.ServerDiscoveryService/ListNeighborhoodServers',
    (route) => {
      queries.push(new URL(route.request().url()).origin);
      return route.fulfill({
        json: {
          servers: [
            {
              origin: 'https://neighbor.example',
              directNeighbor: true,
              profile: { name: 'Recommended community', version: '0.5.0' }
            }
          ]
        }
      });
    }
  );
  await page.goto('/');
  await page.getByLabel('Server URL').fill('https://known.example');
  await page.getByRole('button', { name: 'Find server', exact: true }).click();
  const grid = page.getByTestId('server-directory-grid');
  await expect(grid.getByRole('heading', { name: 'Known community', exact: true })).toBeVisible();
  await expect(
    grid.getByRole('heading', { name: 'Recommended community', exact: true })
  ).toBeVisible();
  await expect(page.getByTestId('saved-server-entry')).toHaveCount(0);
  await expect(page.getByRole('heading', { name: 'Chat just got real!' })).toHaveCount(0);
  await page.reload();
  await expect(grid.getByRole('heading', { name: 'Known community', exact: true })).toBeVisible();
  await page.getByRole('button', { name: 'Connect by address', exact: true }).click();
  const dialog = page.getByRole('dialog');
  await dialog.getByLabel('Server URL').fill('https://known.example');
  await dialog.getByRole('button', { name: 'Find server', exact: true }).click();
  await expect(dialog).toBeHidden();
  await expect(page).toHaveURL(/\/chat\/known\.example\/overview$/);
  await expect(page.getByTestId('server-signed-out')).toBeVisible();
  await page.goBack();
  await expect(page).toHaveURL(/\/chat\/servers$/);
  await expect(page.getByRole('dialog')).toBeHidden();
  await expect(page.getByTestId('saved-server-entry')).toHaveCount(1);
  await page.goForward();
  await expect(page.getByTestId('server-signed-out')).toBeVisible();
  await page.reload();
  await expect(page.getByTestId('server-signed-out')).toBeVisible();
  expect(queries.length).toBeGreaterThan(1);
  expect(new Set(queries)).toEqual(new Set(['https://known.example']));
  expect(
    await page.evaluate(() => JSON.parse(localStorage.getItem('chatto:instances') ?? '[]'))
  ).toEqual([expect.objectContaining({ url: 'https://known.example' })]);
});

test('a server-hosted frontend retains its origin and local sign-in entry', async ({ page }) => {
  await page.goto('/');
  await expect(page).toHaveURL(/\/login$/);
  await expect(page.getByLabel('Username or Email')).toBeVisible();
  await page.getByRole('link', { name: 'Add Server', exact: true }).click();
  await expect(page).toHaveURL(/\/chat\/servers$/);
  await expect(page.getByTestId('server-directory-grid')).toBeVisible();
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
  await expect(page.getByRole('heading', { name: 'Chat just got real!' })).toBeVisible();
  await expect
    .poll(() => page.evaluate(() => JSON.parse(localStorage.getItem('chatto:instances') ?? '[]')))
    .toEqual([]);
  await page.goto('/login');
  await expect(page).toHaveURL(/\/chat\/servers$/);
});
