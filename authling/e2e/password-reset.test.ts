import { randomUUID } from 'node:crypto';
import { completeSignup } from './fixtures/signup';
import { waitForPasswordResetCode } from './fixtures/mailpit';
import { expect, test } from './setup';

const originalPassword = 'correct horse battery staple';
const replacementPassword = 'an entirely new uncommon password';

test('resets the password without changing sub and invalidates other browser sessions', async ({
  browser,
  page,
  request,
  stack
}) => {
  const email = `reset-${randomUUID()}@example.invalid`;
  const accountID = await completeSignup(page, request, stack, email, originalPassword);

  const otherContext = await browser.newContext({ baseURL: stack.baseURL });
  const otherPage = await otherContext.newPage();
  await otherPage.goto('/login');
  await otherPage.getByLabel('Email address').fill(email);
  await otherPage.getByLabel('Password').fill(originalPassword);
  await otherPage.getByRole('button', { name: 'Sign in' }).click();
  await expect(otherPage.getByRole('heading', { name: 'Your account' })).toBeVisible();

  await page.goto('/login');
  await page.getByRole('link', { name: 'Forgot your password?' }).click();
  await expect(page.getByRole('heading', { name: 'Reset your password' })).toBeVisible();
  await page.getByLabel('Email address').fill(email.toUpperCase());
  await page.getByRole('button', { name: 'Email me a reset code' }).click();
  await expect(page.getByRole('heading', { name: 'Check your email' })).toBeVisible();

  const code = await waitForPasswordResetCode(request, stack.mailpitURL);
  await page.getByLabel('Password reset code').fill(code);
  await page.getByRole('button', { name: 'Verify code' }).click();
  await expect(page.getByRole('heading', { name: 'Choose a new password' })).toBeVisible();
  await expect(page.getByText('signs out your other Authling browser sessions')).toBeVisible();
  await page.getByLabel('New password').fill(replacementPassword);
  await page.getByRole('button', { name: 'Reset password' }).click();
  await expect(page.getByRole('heading', { name: 'Your account' })).toBeVisible();
  await expect(page.locator('code')).toHaveText(accountID);

  await otherPage.goto('/account');
  await expect(otherPage).toHaveURL(/\/login$/);
  await otherContext.close();

  await page.getByRole('button', { name: 'Sign out' }).click();
  await page.getByLabel('Email address').fill(email);
  await page.getByLabel('Password').fill(originalPassword);
  await page.getByRole('button', { name: 'Sign in' }).click();
  await expect(page.getByRole('alert')).toHaveText('The email address or password is incorrect.');

  await page.getByLabel('Email address').fill(email);
  await page.getByLabel('Password').fill(replacementPassword);
  await page.getByRole('button', { name: 'Sign in' }).click();
  await expect(page.getByRole('heading', { name: 'Your account' })).toBeVisible();
  await expect(page.locator('code')).toHaveText(accountID);
});
