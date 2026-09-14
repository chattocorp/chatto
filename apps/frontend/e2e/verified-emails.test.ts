import { expect, test } from './setup';
import {
  createAndLoginTestUser,
  loginAsAdmin,
  logoutCurrentUser
} from './fixtures/testUser';
import * as routes from './routes';

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
});
