import { expect, test } from './setup';

test('serves the public pages with browser security headers', async ({ page }) => {
  const response = await page.goto('/');

  expect(response).not.toBeNull();
  expect(response?.headers()['content-security-policy']).toContain("default-src 'none'");
  expect(response?.headers()['content-security-policy']).toContain("form-action 'self'");
  expect(response?.headers()['content-security-policy']).toContain("frame-ancestors 'none'");
  expect(response?.headers()['referrer-policy']).toBe('origin');
  expect(response?.headers()['x-content-type-options']).toBe('nosniff');
  expect(response?.headers()['x-frame-options']).toBe('DENY');

  await expect(page).toHaveTitle('Authling');
  await page.getByRole('link', { name: 'Create account' }).click();
  await expect(page.getByRole('heading', { name: 'Create your account' })).toBeVisible();
});

for (const path of [
  '/login',
  '/logout',
  '/signup',
  '/signup/verify',
  '/signup/complete',
  '/password-reset',
  '/password-reset/verify',
  '/password-reset/complete',
  '/oidc/consent'
]) {
  test(`rejects cross-origin form submission to ${path}`, async ({ request, stack }) => {
    const response = await request.post(`${stack.baseURL}${path}`, {
      headers: {
        'Content-Type': 'application/x-www-form-urlencoded',
        Origin: 'https://attacker.example',
        'Sec-Fetch-Site': 'cross-site'
      },
      data: 'email=attacker%40example.invalid&flow=forged&code=000000&password=forged-password'
    });

    expect(response.status()).toBe(403);
    expect(await response.text()).toBe('cross-origin request rejected\n');
  });
}
