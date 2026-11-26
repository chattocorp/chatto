import { expect, test } from './setup';

test('a web handoff waits for a click, retries discovery and remembers the server', async ({
  page
}) => {
  const host = 'book-club.example.com';
  const link = '/chat/' + host;
  const origin = 'https://' + host;
  const requests: string[] = [];
  let online = false;
  await page.route(origin + '/**', (route) => {
    requests.push(route.request().url());
    if (online && route.request().url().includes('ServerDiscoveryService/GetServer?')) {
      expect(route.request().headers().authorization).toBeUndefined();
      expect(route.request().headers().referer).toBeUndefined();
      return route.fulfill({
        json: {
          profile: { name: 'Book Club', version: '0.6.0' },
          login: { authorizeUrl: '/oauth/authorize' }
        }
      });
    }
    return route.fulfill({ status: 503, body: 'Unavailable' });
  });
  const registrations = () =>
    page.evaluate((origin) => {
      const servers: { url: string }[] = JSON.parse(
        localStorage.getItem('chatto:instances') ?? '[]'
      );
      return servers.filter((server) => server.url === origin).length;
    }, origin);

  await page.goto(link);
  await expect(page.getByRole('heading', { name: host })).toBeVisible();
  expect(requests).toEqual([]);
  expect(await registrations()).toBe(0);
  await page.getByRole('button', { name: 'Add Server', exact: true }).click();
  const retry = page.getByRole('button', { name: 'Try Again' });
  await expect(retry).toBeVisible();
  await expect(page.getByRole('heading', { name: host })).toBeVisible();
  expect(await registrations()).toBe(0);
  online = true;
  await retry.click();
  await expect(page.getByTestId('server-signed-out')).toBeVisible();
  await expect(page).toHaveURL(/\/chat\/book-club\.example\.com\/overview$/);
  expect(await registrations()).toBe(1);
  await page.goto(link);
  await expect(page.getByTestId('server-signed-out')).toBeVisible();
  await expect(page).toHaveURL(/\/chat\/book-club\.example\.com\/overview$/);
  expect(await registrations()).toBe(1);
});

test('an incompatible server offers its exact website and stays out of the switcher', async ({
  page
}) => {
  const origin = 'https://old.example.com';
  await page.route(origin + '/**', (route) =>
    route.fulfill({
      json: {
        profile: { name: 'Old server', version: '0.1.0' },
        login: { authorizeUrl: '/oauth/authorize' }
      }
    })
  );
  await page.goto('/chat/old.example.com');
  await page.getByRole('button', { name: 'Add Server', exact: true }).click();
  await expect(page.getByRole('link', { name: 'Open in new tab' })).toHaveAttribute('href', origin);
  await page.reload();
  await expect(page.getByRole('button', { name: 'Add Server', exact: true })).toBeVisible();
});

test('a saved server opens by hostname using its registered port', async ({ page }) => {
  const hostname = 'book-club.example.com';
  await page.addInitScript((hostname) => {
    localStorage.setItem(
      'chatto:instances',
      JSON.stringify([
        {
          id: 'other-port',
          url: 'https://' + hostname + ':8443',
          name: 'Other port',
          addedAt: 1
        }
      ])
    );
  }, hostname);
  await page.route('https://' + hostname + ':8443/**', (route) => route.abort());
  await page.goto('/chat/' + hostname);
  await expect(page).toHaveURL(/\/chat\/book-club\.example\.com\/overview$/);
  await expect(page.getByRole('button', { name: 'Add Server', exact: true })).toHaveCount(0);
});

test('production links reject an explicit port before discovery', async ({ page }) => {
  const requests: string[] = [];
  await page.route('https://book-club.example.com:8443/**', (route) => {
    requests.push(route.request().url());
    return route.abort();
  });
  await page.goto('/chat/book-club.example.com:8443');
  await expect(page.getByText('Please enter a valid URL.')).toBeVisible();
  expect(requests).toEqual([]);
});
