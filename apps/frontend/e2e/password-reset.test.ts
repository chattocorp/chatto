import { test, expect } from './setup';

test.describe('Password Reset', () => {
  test.describe('Forgot Password Page', () => {
    test('login page has forgot password link', async ({ authPage }) => {
      await authPage.gotoLogin();

      await expect(authPage.forgotPasswordLink).toBeVisible();
      await expect(authPage.forgotPasswordLink).toHaveAttribute('href', '/forgot-password');
    });

    test('can navigate to forgot password page', async ({ authPage }) => {
      await authPage.gotoForgotPassword();
      await authPage.expectOnForgotPasswordPage();
    });

    test('shows success message regardless of email existence (no enumeration)', async ({
      authPage
    }) => {
      await authPage.gotoForgotPassword();
      await authPage.fillForgotPasswordForm('nonexistent@example.com');
      await authPage.submitForgotPassword();

      // Should show success message even for non-existent email
      await expect(authPage.page.getByText(/check your email/i)).toBeVisible();
    });

    test('shows validation error for invalid email format', async ({ authPage }) => {
      await authPage.gotoForgotPassword();
      await authPage.fillForgotPasswordForm('notanemail');

      // Tab away to trigger validation
      await authPage.sendResetLinkButton.focus();

      // Should show validation error
      await authPage.expectError(/valid email/i);
    });

    test('sends email for verified user email', async ({ authPage }) => {
      const timestamp = Date.now();
      const testLogin = `resetuser${timestamp}`;
      const testEmail = `resetuser${timestamp}@example.com`;
      const testPassword = 'testpassword123';

      // Create user and verify their email
      const user = await authPage.createUserViaApi(testLogin, testPassword);
      await authPage.verifyEmailViaTestEndpoint(user.id, testEmail);

      // Request password reset
      await authPage.gotoForgotPassword();
      await authPage.fillForgotPasswordForm(testEmail);
      await authPage.submitForgotPassword();

      // Should show success message
      await expect(authPage.page.getByText(/check your email/i)).toBeVisible();

      // Verify email was actually sent
      const emailData = await authPage.getLastVerificationEmail();
      expect(emailData.to).toBe(testEmail);
      expect(emailData.subject).toContain('Reset');
      expect(emailData.body).toContain('reset-password');
    });
  });

  test.describe('Reset Password Page', () => {
    test('shows error state when no token is provided', async ({ page }) => {
      await page.goto('/reset-password');

      // Should show invalid link message
      await expect(page.getByText(/invalid reset link/i)).toBeVisible();
      await expect(page.getByRole('link', { name: 'Request a new link' })).toBeVisible();
    });

    test('shows error for invalid token', async ({ authPage }) => {
      await authPage.page.goto('/reset-password?token=invalidtoken123');

      // Should still show the form (token validation happens on submit)
      await authPage.expectOnResetPasswordPage();

      await authPage.fillResetPasswordForm('newpassword123', 'newpassword123');
      await authPage.resetPasswordButton.click();

      // Should show error
      await authPage.expectError(/invalid|expired|not found/i);
    });

    test('shows validation error for password mismatch', async ({ authPage }) => {
      const timestamp = Date.now();
      const testLogin = `mismatchpw${timestamp}`;
      const testEmail = `mismatchpw${timestamp}@example.com`;
      const testPassword = 'testpassword123';

      // Create user, verify email, and request reset
      const user = await authPage.createUserViaApi(testLogin, testPassword);
      await authPage.verifyEmailViaTestEndpoint(user.id, testEmail);
      await authPage.requestPasswordReset(testEmail);

      // Get the reset URL and navigate to it
      const emailData = await authPage.getLastVerificationEmail();
      const resetUrl = authPage.extractPasswordResetUrl(emailData.body);
      await authPage.page.goto(resetUrl);

      // Fill form with mismatched passwords
      await authPage.fillResetPasswordForm('newpassword123', 'differentpassword');

      // Tab away to trigger validation
      await authPage.resetPasswordButton.focus();

      // Should show mismatch error
      await authPage.expectError(/do not match/i);
    });

    test('shows validation error for short password', async ({ authPage }) => {
      const timestamp = Date.now();
      const testLogin = `shortpw${timestamp}`;
      const testEmail = `shortpw${timestamp}@example.com`;
      const testPassword = 'testpassword123';

      // Create user, verify email, and request reset
      const user = await authPage.createUserViaApi(testLogin, testPassword);
      await authPage.verifyEmailViaTestEndpoint(user.id, testEmail);
      await authPage.requestPasswordReset(testEmail);

      // Get the reset URL and navigate to it
      const emailData = await authPage.getLastVerificationEmail();
      const resetUrl = authPage.extractPasswordResetUrl(emailData.body);
      await authPage.page.goto(resetUrl);

      // Fill form with short password
      await authPage.newPasswordInput.fill('short');

      // Tab away to trigger validation
      await authPage.confirmPasswordInput.focus();

      // Should show length error
      await authPage.expectError(/at least 8 characters/i);
    });
  });

  test.describe('Complete Password Reset Flow', () => {
    test('can reset password and login with new password', async ({ authPage }) => {
      const timestamp = Date.now();
      const testLogin = `fullreset${timestamp}`;
      const testEmail = `fullreset${timestamp}@example.com`;
      const oldPassword = 'oldpassword123';
      const newPassword = 'newpassword456';

      // Step 1: Create user and verify their email
      const user = await authPage.createUserViaApi(testLogin, oldPassword);
      await authPage.verifyEmailViaTestEndpoint(user.id, testEmail);

      // Step 2: Request password reset
      await authPage.requestPasswordReset(testEmail);

      // Wait for success message
      await expect(authPage.page.getByText(/check your email/i)).toBeVisible();

      // Step 3: Complete password reset via email link
      await authPage.completePasswordReset(newPassword);

      // Step 4: Should be on login page with success banner
      await authPage.expectOnLoginPage();
      await authPage.expectPasswordResetSuccessBanner();

      // Step 5: Login with NEW password
      await authPage.fillLoginForm(testLogin, newPassword);
      await authPage.submitLogin();
      await authPage.expectLoggedIn();

      // Step 6: Logout and verify OLD password no longer works
      await authPage.logout();
      await authPage.gotoLogin();
      await authPage.fillLoginForm(testLogin, oldPassword);
      await authPage.signInButton.click();

      // Should show error for old password
      await authPage.expectError(/invalid credentials/i);
    });

    test('reset token can only be used once', async ({ authPage }) => {
      const timestamp = Date.now();
      const testLogin = `singleuse${timestamp}`;
      const testEmail = `singleuse${timestamp}@example.com`;
      const testPassword = 'testpassword123';
      const newPassword = 'newpassword456';

      // Create user, verify email, and request reset
      const user = await authPage.createUserViaApi(testLogin, testPassword);
      await authPage.verifyEmailViaTestEndpoint(user.id, testEmail);
      await authPage.requestPasswordReset(testEmail);

      // Get the reset URL
      const emailData = await authPage.getLastVerificationEmail();
      const resetUrl = authPage.extractPasswordResetUrl(emailData.body);

      // First reset succeeds
      await authPage.page.goto(resetUrl);
      await authPage.fillResetPasswordForm(newPassword, newPassword);
      await authPage.submitResetPassword();
      await authPage.expectOnLoginPage();

      // Try to use the same token again
      await authPage.page.goto(resetUrl);
      await authPage.fillResetPasswordForm('anotherpassword', 'anotherpassword');
      await authPage.resetPasswordButton.click();

      // Should show error (token already used)
      await authPage.expectError(/invalid|expired|not found/i);
    });
  });

  test.describe('Navigation', () => {
    test('forgot password page has sign in link', async ({ authPage }) => {
      await authPage.gotoForgotPassword();

      const signInLink = authPage.page.getByRole('link', { name: 'Sign in' });
      await expect(signInLink).toBeVisible();
      await expect(signInLink).toHaveAttribute('href', '/login');
    });

    test('reset password page has link to login', async ({ authPage }) => {
      const timestamp = Date.now();
      const testLogin = `navtest${timestamp}`;
      const testEmail = `navtest${timestamp}@example.com`;
      const testPassword = 'testpassword123';

      // Create user, verify email, and request reset
      const user = await authPage.createUserViaApi(testLogin, testPassword);
      await authPage.verifyEmailViaTestEndpoint(user.id, testEmail);
      await authPage.requestPasswordReset(testEmail);

      // Get the reset URL and navigate to it
      const emailData = await authPage.getLastVerificationEmail();
      const resetUrl = authPage.extractPasswordResetUrl(emailData.body);
      await authPage.page.goto(resetUrl);

      const signInLink = authPage.page.getByRole('link', { name: 'Sign in' });
      await expect(signInLink).toBeVisible();
      await expect(signInLink).toHaveAttribute('href', '/login');
    });
  });
});
