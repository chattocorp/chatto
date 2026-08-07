import { randomUUID } from 'node:crypto';
import type { APIRequestContext, Page } from '@playwright/test';
import { messageCount, waitForVerificationCode } from './fixtures/mailpit';
import { expect, test } from './setup';

async function startSignup(page: Page, email = `signup-${randomUUID()}@example.invalid`) {
  await page.goto('/signup');
  await page.getByLabel('Email address').fill(email);
  await page.getByRole('button', { name: 'Email me a code' }).click();
  await expect(page.getByRole('heading', { name: 'Check your email' })).toBeVisible();
}

async function postForm(
  request: APIRequestContext,
  url: string,
  origin: string,
  values: Record<string, string>
) {
  return request.post(url, {
    headers: {
      'Content-Type': 'application/x-www-form-urlencoded',
      Origin: origin
    },
    data: new URLSearchParams(values).toString()
  });
}

test('creates an account only after confirming the emailed code', async ({ page, request, stack }) => {
  await startSignup(page);

  const code = await waitForVerificationCode(request, stack.mailpitURL);
  const wrongCode = String((Number.parseInt(code, 10) + 1) % 1_000_000).padStart(6, '0');
  await page.getByLabel('Verification code').fill(wrongCode);
  await page.getByRole('button', { name: 'Verify email' }).click();
  await expect(page.getByRole('alert')).toHaveText('the code is invalid or has expired');

  await page.getByLabel('Verification code').fill(code);
  await page.getByRole('button', { name: 'Verify email' }).click();
  await expect(page.getByRole('heading', { name: 'Choose a password' })).toBeVisible();
  await expect(page.getByLabel('Password')).toHaveAttribute('minlength', '10');
  const flow = await page.locator('input[name="flow"]').inputValue();

  await page.getByLabel('Password').fill('correct horse battery staple');
  await page.getByRole('button', { name: 'Create account' }).click();
  await expect(page.getByRole('heading', { name: 'Your account' })).toBeVisible();
  await expect(page.locator('code')).toHaveText(/^acc_[a-z0-9]+$/);

  const reused = await postForm(request, `${stack.baseURL}/signup/complete`, stack.baseURL, {
    flow,
    password: 'another sufficiently long password'
  });
  expect(reused.status()).toBe(422);
  expect(await reused.text()).toContain('the signup has expired; start again');
});

test('rejects malformed email addresses without delivering mail', async ({ request, stack }) => {
  const response = await postForm(request, `${stack.baseURL}/signup`, stack.baseURL, {
    email: 'not-an-email'
  });

  expect(response.status()).toBe(422);
  expect(await response.text()).toContain('enter a valid email address');
  expect(await messageCount(request, stack.mailpitURL)).toBe(0);
});

test('exhausts a signup flow after five incorrect verification codes', async ({
  page,
  request,
  stack
}) => {
  await startSignup(page);
  const code = await waitForVerificationCode(request, stack.mailpitURL);
  const wrongCode = code === '000000' ? '999999' : '000000';

  for (let attempt = 0; attempt < 5; attempt++) {
    await page.getByLabel('Verification code').fill(wrongCode);
    await page.getByRole('button', { name: 'Verify email' }).click();
    await expect(page.getByRole('alert')).toHaveText('the code is invalid or has expired');
  }

  await page.getByLabel('Verification code').fill(code);
  await page.getByRole('button', { name: 'Verify email' }).click();
  await expect(page.getByRole('alert')).toHaveText('the code is invalid or has expired');
  await expect(page.getByRole('heading', { name: 'Choose a password' })).not.toBeVisible();
});

test('enforces password policy on the server and keeps the verified flow usable', async ({
  page,
  request,
  stack
}) => {
  await startSignup(page);
  const code = await waitForVerificationCode(request, stack.mailpitURL);
  await page.getByLabel('Verification code').fill(code);
  await page.getByRole('button', { name: 'Verify email' }).click();
  await expect(page.getByRole('heading', { name: 'Choose a password' })).toBeVisible();

  const flow = await page.locator('input[name="flow"]').inputValue();
  const rejected = await postForm(request, `${stack.baseURL}/signup/complete`, stack.baseURL, {
    flow,
    password: 'too short'
  });
  expect(rejected.status()).toBe(422);
  expect(await rejected.text()).toContain(
    'password must contain at least 10 characters and at most 1024 bytes'
  );

  const common = await postForm(request, `${stack.baseURL}/signup/complete`, stack.baseURL, {
    flow,
    password: 'Password123'
  });
  expect(common.status()).toBe(422);
  expect(await common.text()).toContain(
    'password is too common; choose a less predictable password'
  );

  await page.getByLabel('Password').fill('password123 is only part of this passphrase');
  await page.getByRole('button', { name: 'Create account' }).click();
  await expect(page.getByRole('heading', { name: 'Your account' })).toBeVisible();
});
