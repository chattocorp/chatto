import { describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import PasswordLoginPage from './+page.svelte';

// Page titles are tested separately from this page's partial route/server fixtures.
vi.mock('$lib/render/pageTitle', () => ({ formatPageTitle: () => 'Chatto' }));

const provider = {
  id: 'company',
  type: 'oidc',
  issuerUrl: 'https://id.example',
  label: 'Company SSO',
  loginUrl: '/auth/providers/company',
  autoProvision: false
};

const serverInfo = {
  name: 'Community',
  version: '0.6.0',
  authorizeUrl: '/oauth/authorize',
  directRegistrationEnabled: true,
  directLoginEnabled: true,
  accountCreationPolicy: 'open' as const,
  welcomeMessage: null,
  description: null,
  iconUrl: null,
  bannerUrl: null,
  authProviders: [provider]
};

const data = {
  user: null,
  serverInfo,
  serverInfoLoaded: true,
  redirectUrl: '/chat/abc',
  loginErrorCode: '',
  passwordResetSuccess: false
};

describe('password login page', () => {
  it('offers password sign-in, registration, and a way back to the provider choices', async () => {
    const { getByRole, getByLabelText } = render(PasswordLoginPage, { props: { data } });

    await expect.element(getByLabelText('Username or Email')).toBeVisible();
    await expect.element(getByRole('link', { name: 'Forgot password?' })).toBeVisible();
    await expect
      .element(getByRole('link', { name: 'Create an account with email', exact: true }))
      .toHaveAttribute('href', '/register?method=email&redirect=%2Fchat%2Fabc');
    await expect
      .element(getByRole('link', { name: 'Other sign-in options' }))
      .toHaveAttribute('href', '/login?redirect=%2Fchat%2Fabc');
    await expect
      .element(getByRole('link', { name: 'Company SSO', exact: true }))
      .not.toBeInTheDocument();
  });

  it('hides registration and the provider link when neither applies', async () => {
    const { getByRole } = render(PasswordLoginPage, {
      props: {
        data: {
          ...data,
          serverInfo: { ...serverInfo, directRegistrationEnabled: false, authProviders: [] }
        }
      }
    });

    await expect.element(getByRole('button', { name: 'Sign In' })).toBeVisible();
    await expect
      .element(getByRole('link', { name: 'Create an account with email', exact: true }))
      .not.toBeInTheDocument();
    await expect
      .element(getByRole('link', { name: 'Other sign-in options' }))
      .not.toBeInTheDocument();
  });

  it('confirms a completed password reset', async () => {
    const { getByText } = render(PasswordLoginPage, {
      props: { data: { ...data, passwordResetSuccess: true } }
    });

    await expect.element(getByText('Password reset successful')).toBeVisible();
  });
});
