import { randomUUID } from 'node:crypto';
import { completeSignup } from './fixtures/signup';
import { expect, test } from './setup';

const password = 'correct horse battery staple';

test('reviews and signs out other browser sessions', async ({ browser, page, request, stack }) => {
  const email = `sessions-${randomUUID()}@example.invalid`;
  await completeSignup(page, request, stack, email, password);

  const signInElsewhere = async () => {
    const context = await browser.newContext({ baseURL: stack.baseURL });
    const otherPage = await context.newPage();
    await otherPage.goto('/login');
    await otherPage.getByLabel('Email address').fill(email);
    await otherPage.getByLabel('Password').fill(password);
    await otherPage.getByRole('button', { name: 'Sign in' }).click();
    await expect(otherPage.getByRole('heading', { name: 'Your account' })).toBeVisible();
    return { context, otherPage };
  };

  const firstOther = await signInElsewhere();
  await page.reload();
  let sessionSection = page.getByRole('heading', { name: 'Browser sessions' }).locator('..');
  await expect(sessionSection.getByText('Browser session', { exact: true })).toHaveCount(2);
  await expect(sessionSection.getByText('This browser')).toHaveCount(1);
  await expect(
    sessionSection.getByText('Authling does not store browser names, IP addresses, or locations.')
  ).toBeVisible();
  await sessionSection.getByRole('button', { name: 'Sign out', exact: true }).click();
  await expect(page.getByRole('status')).toHaveText('The other browser session was signed out.');

  await firstOther.otherPage.goto('/account');
  await expect(firstOther.otherPage).toHaveURL(/\/login$/);
  await firstOther.context.close();

  const secondOther = await signInElsewhere();
  const thirdOther = await signInElsewhere();
  await page.reload();
  sessionSection = page.getByRole('heading', { name: 'Browser sessions' }).locator('..');
  await expect(sessionSection.getByText('Browser session', { exact: true })).toHaveCount(3);
  await sessionSection.getByRole('button', { name: 'Sign out all other browsers' }).click();
  await expect(page.getByRole('status')).toHaveText('Your other browser sessions were signed out.');
  await expect(sessionSection.getByText('Browser session', { exact: true })).toHaveCount(1);

  for (const other of [secondOther, thirdOther]) {
    await other.otherPage.goto('/account');
    await expect(other.otherPage).toHaveURL(/\/login$/);
    await other.context.close();
  }

  await page.reload();
  await expect(page.getByRole('heading', { name: 'Your account' })).toBeVisible();
});
