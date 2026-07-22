import { expect } from '@playwright/test';
import { test } from './setup';
import * as routes from './routes';

test.use({
  serverOptions: {
    env: {
      CHATTO_AUTH_DIRECT_REGISTRATION: 'false'
    }
  }
});

test.describe('Registration disabled', () => {
  test('login page hides "Create account" registration link when registration is disabled', async ({
    page
  }) => {
    await page.goto(routes.login);
    await expect(page.getByRole('button', { name: 'Sign In' })).toBeVisible();
    await expect(page.getByRole('link', { name: 'Create account' })).toBeHidden();
  });

  test('register page shows disabled message', async ({ page }) => {
    await page.goto(routes.register);
    await expect(page.getByText('Registration is not available on this instance.')).toBeVisible();
  });

  test('register API returns 403', async ({ page }) => {
    const response = await page.request.post('/auth/register', {
      headers: { 'Content-Type': 'application/json' },
      data: { email: 'test@example.com' }
    });

    expect(response.status()).toBe(403);
    const body = await response.json();
    expect(body.error).toBe('Registration is disabled');
  });
});
