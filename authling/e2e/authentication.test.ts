import { randomUUID } from 'node:crypto';
import { completeSignup } from './fixtures/signup';
import { restartAuthling } from './fixtures/stack';
import { expect, test } from './setup';

const password = 'correct horse battery staple';

test('signs in after signup, logs out, and signs back in', async ({ page, request, stack }) => {
  const email = `login-${randomUUID()}@example.invalid`;
  const accountID = await completeSignup(page, request, stack, email, password);

  const cookies = await page.context().cookies(stack.baseURL);
  const session = cookies.find((cookie) => cookie.name === 'authling_session');
  expect(session).toMatchObject({ httpOnly: true, secure: false, sameSite: 'Lax', path: '/' });
  expect(session?.expires).toBe(-1);

  await page.getByRole('button', { name: 'Sign out' }).click();
  await expect(page.getByRole('heading', { name: 'Sign in' })).toBeVisible();
  expect((await page.context().cookies(stack.baseURL)).some((cookie) => cookie.name === 'authling_session')).toBe(false);

  await page.context().addCookies([
    {
      name: 'authling_session',
      value: session?.value ?? '',
      url: stack.baseURL,
      httpOnly: true,
      sameSite: 'Lax'
    }
  ]);

  await page.goto('/account');
  await expect(page).toHaveURL(/\/login$/);

  await page.getByLabel('Email address').fill(email.toUpperCase());
  await page.getByLabel('Password').fill(password);
  await page.getByRole('button', { name: 'Sign in' }).click();
  await expect(page.getByRole('heading', { name: 'Your account' })).toBeVisible();
  await expect(page.locator('code')).toHaveText(accountID);
});

test('uses the same login failure for unknown emails and wrong passwords', async ({
  page,
  request,
  stack
}) => {
  const email = `failure-${randomUUID()}@example.invalid`;
  await completeSignup(page, request, stack, email, password);
  await page.getByRole('button', { name: 'Sign out' }).click();

  for (const [candidateEmail, candidatePassword] of [
    [email, 'the wrong password'],
    [`absent-${randomUUID()}@example.invalid`, 'the wrong password']
  ]) {
    await page.getByLabel('Email address').fill(candidateEmail);
    await page.getByLabel('Password').fill(candidatePassword);
    await page.getByRole('button', { name: 'Sign in' }).click();
    await expect(page.getByRole('alert')).toHaveText('The email address or password is incorrect.');
    await expect(page).toHaveURL(/\/login$/);
  }
});

test('rejects and clears a forged session cookie', async ({ page, stack }) => {
  await page.context().addCookies([
    {
      name: 'authling_session',
      value: 'AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA',
      url: stack.baseURL,
      httpOnly: true,
      sameSite: 'Lax'
    }
  ]);

  await page.goto('/account');
  await expect(page).toHaveURL(/\/login$/);
  expect((await page.context().cookies(stack.baseURL)).some((cookie) => cookie.name === 'authling_session')).toBe(false);
});

test('keeps an authenticated session across an Authling restart', async ({
  page,
  request,
  stack
}, testInfo) => {
  const email = `restart-${randomUUID()}@example.invalid`;
  const accountID = await completeSignup(page, request, stack, email, password);

  await restartAuthling(stack, testInfo);
  await page.reload();

  await expect(page.getByRole('heading', { name: 'Your account' })).toBeVisible();
  await expect(page.locator('code')).toHaveText(accountID);
});

test('throttles an identifier after ten failed password attempts', async ({ page, request, stack }) => {
  const email = `throttle-${randomUUID()}@example.invalid`;
  await completeSignup(page, request, stack, email, password);
  await page.getByRole('button', { name: 'Sign out' }).click();

  for (let attempt = 0; attempt < 10; attempt++) {
    await page.getByLabel('Email address').fill(email);
    await page.getByLabel('Password').fill(`wrong password ${attempt}`);
    await page.getByRole('button', { name: 'Sign in' }).click();
    await expect(page.getByRole('alert')).toHaveText('The email address or password is incorrect.');
  }

  await page.getByLabel('Email address').fill(email);
  await page.getByLabel('Password').fill(password);
  await page.getByRole('button', { name: 'Sign in' }).click();
  await expect(page.getByRole('alert')).toHaveText('The email address or password is incorrect.');
});
