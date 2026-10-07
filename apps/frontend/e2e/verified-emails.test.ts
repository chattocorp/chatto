import { expect, test } from './setup';
import type { APIResponse } from '@playwright/test';
import { createAndLoginTestUser, loginAsAdmin, logoutCurrentUser } from './fixtures/testUser';
import { connectPostResponse } from './fixtures/connectHelpers';
import * as routes from './routes';

const CONFIRM_EMAIL_ROUTE =
  '**/api/connect/chatto.api.v1.MyAccountService/ConfirmEmailVerification';
const LIST_EMAILS_ROUTE = '**/api/connect/chatto.api.v1.MyAccountService/ListVerifiedEmails';

test.describe('Verified email settings', () => {
  test('adds an address only after verification and then allows primary selection', async ({
    accountPage,
    authPage,
    page
  }) => {
    const user = await createAndLoginTestUser(page, { loginPrefix: 'verifiedemail' });
    const newEmail = `${user.login}.secondary@example.com`;

    await accountPage.goto();
    await page.getByRole('button', { name: 'Add email address' }).click();

    const dialog = page.getByRole('dialog', { name: 'Add email address' });
    await dialog.getByLabel('Email address', { exact: true }).fill(newEmail);
    await dialog.getByLabel('Confirm email address').fill(newEmail);
    await dialog.getByRole('button', { name: 'Send verification code' }).click();

    await page.waitForURL(routes.settingsVerifyEmail);
    await expect(page.getByText(newEmail, { exact: true })).toBeVisible();

    // The pending address survives a reload, but it is not yet account state.
    await page.reload();
    await expect(page.getByText(newEmail, { exact: true })).toBeVisible();
    await page.goto(routes.settingsAccount);
    await expect(page.getByText(newEmail, { exact: true })).toHaveCount(0);

    await page.goto(routes.settingsVerifyEmail);
    const message = await authPage.getLastVerificationEmail();
    expect(message.to).toBe(newEmail);
    await page.getByLabel('Digit 1').fill(authPage.extractVerificationCode(message.body));
    await page.getByRole('button', { name: 'Verify email' }).click();

    await page.waitForURL(routes.settingsAccount);
    const newEmailRow = page.getByRole('row', { name: `${newEmail} Make primary` });
    await expect(newEmailRow).toBeVisible();
    await newEmailRow.getByRole('button', { name: 'Make primary' }).click();
    await expect(page.getByRole('row', { name: `${newEmail} Primary` })).toBeVisible();

    await page.reload();
    await expect(page.getByRole('row', { name: `${newEmail} Primary` })).toBeVisible();

    await logoutCurrentUser(page);
    await loginAsAdmin(page);
    await page.goto(routes.serverAdminMembers);

    const adminMemberRow = page.getByRole('row').filter({ hasText: `@${user.login}` });
    await expect(adminMemberRow).toContainText(newEmail);
    await expect(adminMemberRow).not.toContainText(`${user.login}@example.com`);
  });

  test('rejects a request captured before another tab replaces its cookie session', async ({
    accountPage,
    page
  }) => {
    const firstUser = await createAndLoginTestUser(page, { loginPrefix: 'emailbindingfirst' });
    await accountPage.goto();
    await expect(page.getByText(firstUser.id ?? '', { exact: true })).toBeVisible();
    await expect(page.getByText(`${firstUser.login}@example.com`, { exact: true })).toBeVisible();
    await page.getByRole('link', { name: 'Profile', exact: true }).click();
    await page.waitForURL(routes.settingsProfile);

    let markListStarted = () => {};
    let releaseList = () => {};
    const listStarted = new Promise<void>((resolve) => (markListStarted = resolve));
    const listGate = new Promise<void>((resolve) => (releaseList = resolve));
    let completeList = (_response: APIResponse) => {};
    const listResponse = new Promise<APIResponse>((resolve) => (completeList = resolve));
    await page.route(
      LIST_EMAILS_ROUTE,
      async (route) => {
        markListStarted();
        await listGate;

        // The request started while this SPA still displayed the first user.
        // Send it with the cookie that the other tab installed afterward.
        const cookies = await page.context().cookies(route.request().url());
        const response = await route.fetch({
          headers: {
            ...route.request().headers(),
            cookie: cookies.map(({ name, value }) => `${name}=${value}`).join('; ')
          }
        });
        // Session revocation can navigate the SPA before this response arrives.
        // Inspect the server response even if the browser aborted its request.
        completeList(response);
        await route.fulfill({ response });
      },
      { times: 1 }
    );

    await page.getByRole('link', { name: 'Account', exact: true }).click();
    await page.waitForURL(routes.settingsAccount);
    await listStarted;

    const otherTab = await page.context().newPage();
    const secondUser = await createAndLoginTestUser(otherTab, {
      loginPrefix: 'emailbindingsecond'
    });

    const staleResponse = await connectPostResponse(
      page,
      'chatto.api.v1.MyAccountService/ListVerifiedEmails',
      { expectedUserId: firstUser.id }
    );
    expect(staleResponse.ok()).toBe(false);
    await expect(staleResponse.json()).resolves.toMatchObject({ code: 'failed_precondition' });

    const currentResponse = await connectPostResponse(
      page,
      'chatto.api.v1.MyAccountService/ListVerifiedEmails',
      { expectedUserId: secondUser.id }
    );
    expect(currentResponse.ok()).toBe(true);

    releaseList();
    const rejectedList = await listResponse;
    expect(rejectedList.ok()).toBe(false);
    await expect(rejectedList.json()).resolves.toMatchObject({ code: 'failed_precondition' });

    const firstUserId = page.getByText(firstUser.id ?? '', { exact: true });
    const firstUserEmail = page.getByText(`${firstUser.login}@example.com`, { exact: true });
    const secondUserId = page.getByText(secondUser.id ?? '', { exact: true });
    const secondUserEmail = page.getByText(`${secondUser.login}@example.com`, { exact: true });
    const currentLogin = page.getByTestId('current-user-login');
    const accountChangedError = page.getByText(/authenticated account changed/);
    const sessionExpired = page.getByText('Session expired', { exact: true });

    // The root session can independently detect the new cookie. It must keep
    // Alice's view with the rejection, keep it after it marks Alice's session
    // expired, switch the complete view to Bob, or end the revoked session.
    // The new cookie can also redirect sign-in to Bob's overview. An expired
    // session holds private reads, so the email list can stay loading instead
    // of showing the rejection. Bob's address must never appear while the
    // page still identifies Alice.
    await expect
      .poll(async () => {
        const pathname = new URL(page.url()).pathname;
        if (pathname === routes.root || pathname === routes.login) {
          return (
            !(await firstUserId.isVisible()) &&
            !(await firstUserEmail.isVisible()) &&
            !(await secondUserId.isVisible()) &&
            !(await secondUserEmail.isVisible())
          );
        }
        if (await firstUserId.isVisible()) {
          return (
            ((await accountChangedError.isVisible()) || (await sessionExpired.isVisible())) &&
            !(await secondUserEmail.isVisible())
          );
        }
        if (pathname !== routes.settingsAccount && (await currentLogin.isVisible())) {
          return (
            (await currentLogin.textContent())?.trim() === `@${secondUser.login}` &&
            !(await firstUserEmail.isVisible())
          );
        }
        return (await secondUserId.isVisible()) && (await secondUserEmail.isVisible());
      })
      .toBe(true);
    expect((await firstUserId.isVisible()) && (await secondUserEmail.isVisible())).toBe(false);
    await otherTab.close();
  });

  test('clears a consumed challenge when confirmation finishes after navigation', async ({
    accountPage,
    authPage,
    page
  }) => {
    const user = await createAndLoginTestUser(page, { loginPrefix: 'emailnavigation' });
    const newEmail = `${user.login}.secondary@example.com`;

    await accountPage.goto();
    await page.getByRole('button', { name: 'Add email address' }).click();
    const dialog = page.getByRole('dialog', { name: 'Add email address' });
    await dialog.getByLabel('Email address', { exact: true }).fill(newEmail);
    await dialog.getByLabel('Confirm email address').fill(newEmail);
    await dialog.getByRole('button', { name: 'Send verification code' }).click();
    await page.waitForURL(routes.settingsVerifyEmail);

    const message = await authPage.getLastVerificationEmail();
    let releaseResponse: (() => void) | undefined;
    let markConfirmed: (() => void) | undefined;
    const responseGate = new Promise<void>((resolve) => {
      releaseResponse = resolve;
    });
    const serverConfirmed = new Promise<void>((resolve) => {
      markConfirmed = resolve;
    });
    await page.route(CONFIRM_EMAIL_ROUTE, async (route) => {
      const response = await route.fetch();
      markConfirmed?.();
      await responseGate;
      await route.fulfill({ response });
    });

    await page.getByLabel('Digit 1').fill(authPage.extractVerificationCode(message.body));
    await page.getByRole('button', { name: 'Verify email' }).click();
    await serverConfirmed;
    await page.getByRole('link', { name: 'Profile', exact: true }).click();
    await page.waitForURL(routes.settingsProfile);
    const challengeKey = await page.evaluate(() =>
      Object.keys(sessionStorage).find((key) => key.endsWith(':pending-email-verification'))
    );
    expect(challengeKey).toBeDefined();
    releaseResponse?.();

    // Let the old page's response handler finish before a full document reload
    // destroys it. The first navigation above already unmounted that page.
    await expect
      .poll(() => page.evaluate((key) => sessionStorage.getItem(key), challengeKey!))
      .toBeNull();
    await page.goto(routes.settingsVerifyEmail);
    await expect(page.getByText('No email verification is in progress.')).toBeVisible();
  });
});
