import { expect, type Locator, type Page } from '@playwright/test';
import { TIMEOUTS } from '../constants';
import { csrfHeaders } from '../fixtures/csrf';
import * as routes from '../routes';

/**
 * Page object for authentication flows (login, registration, email verification).
 * Handles form interactions and navigation between auth pages.
 */
export class AuthPage {
  constructor(readonly page: Page) {}

  // --- Locators: Registration Form (Step 1 — email only) ---

  /** Email input on registration page */
  get emailInput(): Locator {
    return this.page.getByLabel('Email');
  }

  /** Continue button on registration page (step 1) */
  get continueButton(): Locator {
    return this.page.getByRole('button', { name: 'Continue' });
  }

  // --- Locators: Registration Complete Form (Step 2 — username + password) ---

  /** Username input on registration complete page */
  get completeUsernameInput(): Locator {
    return this.page.getByLabel('Username');
  }

  /** Password input (works for login and registration complete) */
  get passwordInput(): Locator {
    return this.page.locator('#password');
  }

  /** Confirm Password input on registration complete page */
  get completeConfirmPasswordInput(): Locator {
    return this.page.getByLabel('Confirm Password');
  }

  /** Create Account button on registration complete page */
  get createAccountButton(): Locator {
    return this.page.getByRole('button', { name: 'Create Account' });
  }

  /** First verification-code input on the registration page */
  get firstCodeInput(): Locator {
    return this.page.getByLabel('Digit 1');
  }

  /** Submit button on the verification-code step */
  get submitCodeButton(): Locator {
    return this.page.getByRole('button', { name: 'Submit' });
  }

  /** Complete Registration heading */
  get completeRegistrationHeading(): Locator {
    return this.page.getByRole('heading', { name: 'Complete Registration' });
  }

  // --- Locators: Login Form ---

  /** Username or Email input on login page */
  get identifierInput(): Locator {
    return this.page.getByLabel('Username or Email');
  }

  /** Sign In button on login page */
  get signInButton(): Locator {
    return this.page.getByRole('button', { name: 'Sign In' });
  }

  // --- Locators: Navigation Links ---

  /** Link to login page from registration */
  get signInLink(): Locator {
    return this.page.getByRole('link', { name: 'Sign in' });
  }

  /** Link to registration page from login */
  get createAccountLink(): Locator {
    return this.page.getByRole('link', { name: 'Create Account' });
  }

  /** Link to forgot password page from login */
  get forgotPasswordLink(): Locator {
    return this.page.getByRole('link', { name: 'Forgot password?' });
  }

  // --- Locators: Forgot Password Form ---

  /** Email input on forgot password page */
  get forgotPasswordEmailInput(): Locator {
    return this.page.getByLabel('Email');
  }

  /** Send Reset Link button on forgot password page */
  get sendResetLinkButton(): Locator {
    return this.page.getByRole('button', { name: 'Send Reset Link' });
  }

  /** Forgot password page heading */
  get forgotPasswordHeading(): Locator {
    return this.page.getByRole('heading', { name: 'Forgot Password' });
  }

  // --- Locators: Reset Password Form ---

  /** New password input on reset password page */
  get newPasswordInput(): Locator {
    return this.page.getByLabel('New Password');
  }

  /** Confirm password input on reset password page */
  get confirmPasswordInput(): Locator {
    return this.page.getByLabel('Confirm Password');
  }

  /** Reset Password button on reset password page */
  get resetPasswordButton(): Locator {
    return this.page.getByRole('button', { name: 'Reset Password' });
  }

  /** Reset password page heading */
  get resetPasswordHeading(): Locator {
    return this.page.getByRole('heading', { name: 'Set New Password' });
  }

  /** Password reset success banner on login page */
  get passwordResetSuccessBanner(): Locator {
    return this.page.getByText('Password reset successful');
  }

  // --- Locators: Common Elements ---

  /** Sign out button (visible when authenticated) */
  get logoutButton(): Locator {
    return this.page.getByTitle('Sign out');
  }

  /** Logout confirmation dialog */
  get logoutDialog(): Locator {
    return this.page.getByRole('dialog');
  }

  /** Confirm sign out button in dialog */
  get confirmLogoutButton(): Locator {
    return this.page.getByRole('dialog').getByRole('button', { name: 'All Servers' });
  }

  /** Cancel logout button in dialog */
  get cancelLogoutButton(): Locator {
    return this.page.getByRole('dialog').getByRole('button', { name: 'Cancel' });
  }

  /** Sign In heading on login page */
  get signInHeading(): Locator {
    return this.page.getByRole('heading', { name: /sign in/i });
  }

  /** Create Account heading on registration page */
  get createAccountHeading(): Locator {
    return this.page.getByRole('heading', { name: /create account/i });
  }

  // --- Navigation Methods ---

  /**
   * Navigate to the login page.
   */
  async gotoLogin(): Promise<void> {
    await this.page.goto(routes.login);
    await expect(this.signInHeading).toBeVisible();
  }

  /**
   * Navigate to the registration page.
   */
  async gotoRegister(): Promise<void> {
    await this.page.goto(routes.register);
    await expect(this.createAccountHeading).toBeVisible();
  }

  // --- Registration Methods ---

  /**
   * Fill the registration complete form (step 2) with username and password.
   */
  async fillRegistrationCompleteForm(
    username: string,
    password: string,
    confirmPassword: string
  ): Promise<void> {
    await this.completeUsernameInput.fill(username);
    await this.passwordInput.fill(password);
    await this.completeConfirmPasswordInput.fill(confirmPassword);
  }

  /**
   * Submit the registration complete form and wait for redirect to /chat.
   */
  async submitRegistrationComplete(): Promise<void> {
    await this.createAccountButton.click();
    // New users get redirected to /chat/spaces (browse spaces) since they have no joined spaces
    await this.page.waitForURL(routes.patterns.chatRedirect);
  }

  /**
   * Create a registration completion token via the test endpoint (bypasses code entry).
   */
  async createRegistrationTokenViaTestEndpoint(email: string): Promise<string> {
    const response = await this.page.request.post('/auth/test/create-registration-token', {
      headers: { 'Content-Type': 'application/json' },
      data: { email }
    });
    expect(response.ok()).toBeTruthy();
    const data = await response.json();
    return data.token;
  }

  /**
   * Create a registration code via the test endpoint (bypasses email delivery).
   */
  async createRegistrationCodeViaTestEndpoint(email: string): Promise<string> {
    const response = await this.page.request.post('/auth/test/create-registration-code', {
      headers: { 'Content-Type': 'application/json' },
      data: { email }
    });
    expect(response.ok()).toBeTruthy();
    const data = await response.json();
    return data.code;
  }

  /**
   * Exchange a registration code for a completion token via the public API.
   */
  async verifyRegistrationCodeViaApi(email: string, code: string): Promise<string> {
    const response = await this.page.request.post('/auth/register/verify-code', {
      headers: { 'Content-Type': 'application/json' },
      data: { email, code }
    });
    expect(response.ok()).toBeTruthy();
    const data = await response.json();
    return data.completionToken;
  }

  /**
   * Navigate to the registration complete page with a token.
   */
  async gotoRegisterComplete(token: string): Promise<void> {
    await this.page.goto(routes.registerComplete(token));
    await expect(this.completeRegistrationHeading).toBeVisible();
  }

  /**
   * Fill the verification code step.
   */
  async fillRegistrationCode(code: string): Promise<void> {
    await this.firstCodeInput.fill(code);
  }

  /**
   * Register a new user via the email → code → account details flow.
   */
  async register(username: string, email: string, password: string): Promise<void> {
    await this.gotoRegister();
    await this.emailInput.fill(email);
    await this.continueButton.click();

    const emailData = await this.getLastVerificationEmail();
    const code = this.extractVerificationCode(emailData.body);
    await this.fillRegistrationCode(code);
    await this.submitCodeButton.click();
    await expect(this.completeRegistrationHeading).toBeVisible();

    await this.fillRegistrationCompleteForm(username, password, password);
    await this.submitRegistrationComplete();
  }

  /**
   * Register a new user with auto-generated credentials.
   * Returns the generated credentials for later use.
   */
  async registerWithGeneratedCredentials(): Promise<{
    username: string;
    email: string;
    password: string;
  }> {
    const timestamp = Date.now();
    const credentials = {
      username: `user${timestamp}`,
      email: `user${timestamp}@example.com`,
      password: 'testpassword123'
    };
    await this.register(credentials.username, credentials.email, credentials.password);
    return credentials;
  }

  // --- Login Methods ---

  /**
   * Fill the login form with the provided credentials.
   */
  async fillLoginForm(identifier: string, password: string): Promise<void> {
    await this.identifierInput.fill(identifier);
    await this.passwordInput.fill(password);
  }

  /**
   * Submit the login form and wait for redirect to /chat.
   */
  async submitLogin(): Promise<void> {
    await this.signInButton.click();
    // Users may be redirected to /chat/spaces, their last chat route, or /chat depending on history.
    await this.page.waitForURL(routes.patterns.chatRedirect);
  }

  /**
   * Login with the provided credentials.
   * Navigates to /login, fills the form, and submits.
   */
  async login(identifier: string, password: string): Promise<void> {
    await this.gotoLogin();
    await this.fillLoginForm(identifier, password);
    await this.submitLogin();
  }

  // --- Logout ---

  /**
   * Logout the current user.
   *
   * A pure API call (`page.request.post('/auth/logout')`) is not enough on its
   * own: the response Set-Cookie doesn't reliably overwrite the page-side jar
   * that the SPA's subsequent fetches use, so the SPA stays authenticated and
   * a follow-up `goto('/login')` redirects back into the app instead of
   * showing the sign-in form.
   *
   * Going through the UI button triggers the SPA's full sign-out path
   * (serverRegistry.removeAll() + `window.location.href = '/'`), which
   * forces a hard reload and lands cleanly on the landing page.
   */
  async logout(): Promise<void> {
    await this.logoutViaUI();
  }

  /**
   * Open the logout confirmation dialog. Idempotent: if the dialog is already
   * open, this is a no-op. Otherwise it clicks the Sign Out button and retries
   * the click if the first attempt didn't open the dialog (Svelte hydration
   * race: actionability checks pass before onclick is attached, so the first
   * click can be dropped).
   */
  private async openLogoutDialog(): Promise<void> {
    await expect(async () => {
      if (!(await this.logoutDialog.isVisible())) {
        await this.logoutButton.click();
      }
      await expect(this.logoutDialog).toBeVisible({ timeout: 1000 });
    }).toPass({ timeout: TIMEOUTS.REALTIME_EVENT, intervals: [200, 500, 1000] });
  }

  /**
   * Logout the current user by clicking the logout button.
   * Confirms the logout dialog and waits for redirect to home page.
   */
  async logoutViaUI(): Promise<void> {
    await this.openLogoutDialog();
    await this.confirmLogoutButton.click();
    await this.page.waitForURL('/');
  }

  /**
   * Click logout button and then cancel the confirmation.
   * Verifies the dialog closes without logging out.
   */
  async cancelLogoutViaUI(): Promise<void> {
    await this.openLogoutDialog();
    await this.cancelLogoutButton.click();
    await expect(this.logoutDialog).not.toBeVisible();
  }

  // --- Email Verification Methods ---

  /**
   * Get the last verification email from the test endpoint.
   * Returns the email data including body.
   */
  async getLastVerificationEmail(): Promise<{ to: string; subject: string; body: string }> {
    const response = await this.page.request.get('/auth/test/last-email');
    expect(response.ok()).toBeTruthy();
    return response.json();
  }

  /**
   * Extract a six-digit verification code from an email body.
   */
  extractVerificationCode(emailBody: string): string {
    const match = emailBody.match(/\b\d{6}\b/);
    if (!match) {
      throw new Error('Verification code not found in email body');
    }
    return match[0];
  }

  /**
   * Complete email verification by getting the email and submitting the code.
   * Assumes a verification email was just sent.
   */
  async completeEmailVerification(email: string): Promise<void> {
    const emailData = await this.getLastVerificationEmail();
    const code = this.extractVerificationCode(emailData.body);
    const response = await this.page.request.post('/auth/verify-email/confirm-code', {
      headers: { 'Content-Type': 'application/json', ...(await csrfHeaders(this.page)) },
      data: { email, code }
    });
    expect(response.ok()).toBeTruthy();
  }

  /**
   * Verify an email for a user via the test endpoint (bypasses actual email).
   */
  async verifyEmailViaTestEndpoint(userId: string, email: string): Promise<void> {
    const response = await this.page.request.post('/auth/test/verify-email', {
      headers: { 'Content-Type': 'application/json' },
      data: { userId, email }
    });
    expect(response.ok()).toBeTruthy();
  }

  // --- Password Reset Methods ---

  /**
   * Navigate to the forgot password page.
   */
  async gotoForgotPassword(): Promise<void> {
    await this.page.goto(routes.forgotPassword);
    await expect(this.forgotPasswordHeading).toBeVisible();
  }

  /**
   * Navigate to the reset password page with a token.
   */
  async gotoResetPassword(token: string): Promise<void> {
    await this.page.goto(routes.resetPassword(token));
    await expect(this.resetPasswordHeading).toBeVisible();
  }

  /**
   * Fill the forgot password form.
   */
  async fillForgotPasswordForm(email: string): Promise<void> {
    await this.forgotPasswordEmailInput.fill(email);
  }

  /**
   * Submit the forgot password form.
   */
  async submitForgotPassword(): Promise<void> {
    await this.sendResetLinkButton.click();
  }

  /**
   * Fill the reset password form.
   */
  async fillResetPasswordForm(password: string, confirmPassword: string): Promise<void> {
    await this.newPasswordInput.fill(password);
    await this.confirmPasswordInput.fill(confirmPassword);
  }

  /**
   * Submit the reset password form and wait for redirect to login.
   */
  async submitResetPassword(): Promise<void> {
    await this.resetPasswordButton.click();
    await this.page.waitForURL(routes.loginResetSuccess);
  }

  /**
   * Request a password reset by filling and submitting the forgot password form.
   */
  async requestPasswordReset(email: string): Promise<void> {
    await this.gotoForgotPassword();
    await this.fillForgotPasswordForm(email);
    await this.submitForgotPassword();
  }

  /**
   * Extract the password reset URL from an email body.
   */
  extractPasswordResetUrl(emailBody: string): string {
    const match = emailBody.match(/http[s]?:\/\/[^\s]+reset-password\?token=[^\s]+/);
    if (!match) {
      throw new Error('Password reset URL not found in email body');
    }
    return match[0];
  }

  /**
   * Complete password reset by getting the email, visiting the link, and setting a new password.
   * Assumes a password reset email was just sent.
   */
  async completePasswordReset(newPassword: string): Promise<void> {
    const emailData = await this.getLastVerificationEmail();
    const resetUrl = this.extractPasswordResetUrl(emailData.body);
    await this.page.goto(resetUrl);
    await expect(this.resetPasswordHeading).toBeVisible();
    await this.fillResetPasswordForm(newPassword, newPassword);
    await this.submitResetPassword();
  }

  // --- API Methods ---

  /**
   * Create a user directly via the test-only HTTP endpoint.
   * Useful for setting up test fixtures. The production create-user mutation
   * was removed for security (#175); the test endpoint is gated
   * behind the `test_endpoints` build tag and never compiled into release
   * binaries.
   */
  async createUserViaApi(login: string, password: string): Promise<{ id: string; login: string }> {
    const response = await this.page.request.post('/auth/test/create-user', {
      headers: { 'Content-Type': 'application/json' },
      data: { login, displayName: login, password }
    });
    expect(response.ok()).toBeTruthy();
    const data = (await response.json()) as { id: string; login: string };
    return { id: data.id, login: data.login };
  }

  /**
   * Register via the REST API using the code-based flow.
   * Creates a registration code via test endpoint, exchanges it, then completes registration.
   * Returns the response from the complete step.
   */
  async registerViaApi(
    login: string,
    email: string,
    password: string
  ): Promise<{
    success: boolean;
    user: { id: string; login: string };
  }> {
    const code = await this.createRegistrationCodeViaTestEndpoint(email);
    const token = await this.verifyRegistrationCodeViaApi(email, code);
    const response = await this.page.request.post('/auth/register/complete', {
      headers: { 'Content-Type': 'application/json' },
      data: { token, login, password, passwordConfirmation: password }
    });
    expect(response.ok()).toBeTruthy();
    return response.json();
  }

  /**
   * Simulate OAuth callback via test endpoint.
   * Returns user data and whether it was a new user.
   */
  async simulateOAuthCallback(
    email: string,
    displayName: string
  ): Promise<{
    success: boolean;
    isNewUser: boolean;
    user: { id: string };
  }> {
    const response = await this.page.request.post('/auth/test/oauth-callback', {
      data: { email, displayName }
    });
    expect(response.ok()).toBeTruthy();
    return response.json();
  }

  /**
   * Create a pending external identity flow through the test-only endpoint.
   * The returned URL drives the production /sso/confirm route and ConnectRPC
   * confirmation methods.
   */
  async createExternalIdentityFlow(input: {
    kind: 'create' | 'link';
    providerId: string;
    providerType: string;
    providerLabel?: string;
    issuer?: string;
    subject: string;
    verifiedEmail?: string;
    loginHint?: string;
    displayNameHint?: string;
    boundUserId?: string;
  }): Promise<{ token: string; confirmUrl: string }> {
    const response = await this.page.request.post('/auth/test/external-identity-flow', {
      data: input
    });
    expect(response.ok()).toBeTruthy();
    return response.json();
  }

  // --- Assertions ---

  /**
   * Assert that the user is logged in (logout button visible).
   */
  async expectLoggedIn(): Promise<void> {
    await expect(this.logoutButton).toBeVisible();
  }

  /**
   * Assert that the user is logged out (sees sign-in prompt or login link).
   */
  async expectLoggedOut(): Promise<void> {
    // After logout, user ends up at /login (via / redirect).
    // Use a generous timeout — auth failure → redirect → re-render can take time.
    await expect(this.signInHeading).toBeVisible({ timeout: TIMEOUTS.UI_STANDARD });
  }

  /**
   * Assert that an error message is visible.
   */
  async expectError(pattern: RegExp): Promise<void> {
    await expect(this.page.getByText(pattern)).toBeVisible();
  }

  /**
   * Assert that the registration form heading is visible.
   */
  async expectOnRegisterPage(): Promise<void> {
    await expect(this.createAccountHeading).toBeVisible();
  }

  /**
   * Assert that the login form heading is visible.
   */
  async expectOnLoginPage(): Promise<void> {
    await expect(this.signInHeading).toBeVisible();
  }

  /**
   * Assert that the forgot password page is visible.
   */
  async expectOnForgotPasswordPage(): Promise<void> {
    await expect(this.forgotPasswordHeading).toBeVisible();
  }

  /**
   * Assert that the reset password page is visible.
   */
  async expectOnResetPasswordPage(): Promise<void> {
    await expect(this.resetPasswordHeading).toBeVisible();
  }

  /**
   * Assert that the password reset success banner is visible on the login page.
   */
  async expectPasswordResetSuccessBanner(): Promise<void> {
    await expect(this.passwordResetSuccessBanner).toBeVisible();
  }
}
