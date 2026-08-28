import { randomUUID } from 'node:crypto';
import { waitForEmailChangeCode } from './fixtures/mailpit';
import { completeSignup } from './fixtures/signup';
import { expect, test } from './setup';

const password = 'correct horse battery staple';

test('changes the verified email without changing sub and invalidates other sessions', async ({
  browser,
  page,
  request,
  stack
}) => {
  const originalEmail = `email-before-${randomUUID()}@example.invalid`;
  const changedEmail = `email-after-${randomUUID()}@example.invalid`;
  const accountID = await completeSignup(page, request, stack, originalEmail, password);

  const otherContext = await browser.newContext({ baseURL: stack.baseURL });
  const otherPage = await otherContext.newPage();
  await otherPage.goto('/login');
  await otherPage.getByLabel('Email address').fill(originalEmail);
  await otherPage.getByLabel('Password').fill(password);
  await otherPage.getByRole('button', { name: 'Sign in' }).click();
  await expect(otherPage.getByRole('heading', { name: 'Your account' })).toBeVisible();

  await page.getByRole('link', { name: 'Change email address' }).click();
  await expect(page.getByRole('heading', { name: 'Change your email address' })).toBeVisible();
  await page.getByLabel('New email address').fill(changedEmail.toUpperCase());
  await page.getByLabel('Current password').fill(password);
  await page.getByRole('button', { name: 'Email me a change code' }).click();

  const code = await waitForEmailChangeCode(request, stack.mailpitURL);
  await page.getByLabel('Email change code').fill(code);
  await page.getByRole('button', { name: 'Verify code' }).click();
  await expect(page.getByRole('heading', { name: 'Your new email address is verified' })).toBeVisible();
  await page.getByRole('button', { name: 'Change email address' }).click();
  await expect(page.getByRole('status')).toContainText('Your email address was changed');
  await page.reload();
  await expect(page.getByRole('status')).toContainText('Your email address was changed');
  await expect(page.locator('code')).toHaveText(accountID);

  await otherPage.goto('/account');
  await expect(otherPage).toHaveURL(/\/login$/);
  await otherContext.close();

  await page.getByRole('button', { name: 'Sign out' }).click();
  await page.getByLabel('Email address').fill(originalEmail);
  await page.getByLabel('Password').fill(password);
  await page.getByRole('button', { name: 'Sign in' }).click();
  await expect(page.getByRole('alert')).toHaveText('The email address or password is incorrect.');

  await page.getByLabel('Email address').fill(changedEmail);
  await page.getByLabel('Password').fill(password);
  await page.getByRole('button', { name: 'Sign in' }).click();
  await expect(page.getByRole('heading', { name: 'Your account' })).toBeVisible();
  await expect(page.locator('code')).toHaveText(accountID);
});

test('does not disclose that a requested email address is already claimed', async ({
  browser,
  page,
  request,
  stack
}) => {
  const originalEmail = `email-owner-${randomUUID()}@example.invalid`;
  const claimedEmail = `email-claimed-${randomUUID()}@example.invalid`;
  const accountID = await completeSignup(page, request, stack, originalEmail, password);

  const claimedContext = await browser.newContext({ baseURL: stack.baseURL });
  const claimedPage = await claimedContext.newPage();
  await completeSignup(claimedPage, request, stack, claimedEmail, `${password} claimed`);
  await claimedContext.close();

  await page.getByRole('link', { name: 'Change email address' }).click();
  await page.getByLabel('New email address').fill(claimedEmail);
  await page.getByLabel('Current password').fill(password);
  await page.getByRole('button', { name: 'Email me a change code' }).click();
  await expect(page.getByRole('heading', { name: 'Check your new email address' })).toBeVisible();

  const code = await waitForEmailChangeCode(request, stack.mailpitURL);
  await page.getByLabel('Email change code').fill(code);
  await page.getByRole('button', { name: 'Verify code' }).click();
  await expect(page.getByRole('heading', { name: 'Your new email address is verified' })).toBeVisible();
  await page.getByRole('button', { name: 'Change email address' }).click();

  await expect(page.getByRole('heading', { name: 'Change your email address' })).toBeVisible();
  await expect(page.getByRole('alert')).toHaveText("We couldn't change that email address. Start again.");
  await page.goto('/account');
  await expect(page.locator('code')).toHaveText(accountID);
});
