import { randomUUID } from 'node:crypto';
import { completeSignup } from './fixtures/signup';
import { expect, test } from './setup';

const originalPassword = 'correct horse battery staple';
const replacementPassword = 'an entirely new uncommon password';

test('changes a signed-in password and invalidates other browser sessions', async ({
  browser,
  page,
  request,
  stack
}) => {
  const email = `password-change-${randomUUID()}@example.invalid`;
  const accountID = await completeSignup(page, request, stack, email, originalPassword);

  const otherContext = await browser.newContext({ baseURL: stack.baseURL });
  const otherPage = await otherContext.newPage();
  await otherPage.goto('/login');
  await otherPage.getByLabel('Email address').fill(email);
  await otherPage.getByLabel('Password').fill(originalPassword);
  await otherPage.getByRole('button', { name: 'Sign in' }).click();
  await expect(otherPage.getByRole('heading', { name: 'Your account' })).toBeVisible();

  await page.getByRole('link', { name: 'Change password' }).click();
  await expect(page.getByRole('heading', { name: 'Change your password' })).toBeVisible();
  await expect(page.getByText('signs out your other Authling browser sessions')).toBeVisible();

  await page.getByLabel('Current password').fill('the wrong current password');
  await page.getByLabel('New password', { exact: true }).fill(replacementPassword);
  await page.getByLabel('Confirm new password').fill(replacementPassword);
  await page.getByRole('button', { name: 'Change password' }).click();
  await expect(page.getByRole('alert')).toHaveText('The current password is incorrect.');

  await page.getByLabel('Current password').fill(originalPassword);
  await page.getByLabel('New password', { exact: true }).fill(originalPassword);
  await page.getByLabel('Confirm new password').fill(originalPassword);
  await page.getByRole('button', { name: 'Change password' }).click();
  await expect(page.getByRole('alert')).toHaveText('new password must be different from the current password');

  await page.getByLabel('Current password').fill(originalPassword);
  await page.getByLabel('New password', { exact: true }).fill(replacementPassword);
  await page.getByLabel('Confirm new password').fill(`${replacementPassword} mismatch`);
  await page.getByRole('button', { name: 'Change password' }).click();
  await expect(page.getByRole('alert')).toHaveText('New passwords do not match.');

  await page.getByLabel('Current password').fill(originalPassword);
  await page.getByLabel('New password', { exact: true }).fill(replacementPassword);
  await page.getByLabel('Confirm new password').fill(replacementPassword);
  await page.getByRole('button', { name: 'Change password' }).click();
  await expect(page.getByRole('status')).toContainText('Your password was changed');
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
