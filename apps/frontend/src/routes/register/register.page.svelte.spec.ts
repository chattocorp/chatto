import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { userEvent } from 'vitest/browser';
import type { PublicServerInfo } from '@chatto/client/api/server';
import RegisterPage from './+page.svelte';

const navigation = vi.hoisted(() => ({ goto: vi.fn() }));

// Page titles are tested separately from this page's partial route/server fixtures.
vi.mock('$lib/render/pageTitle', () => ({ formatPageTitle: () => 'Chatto' }));

vi.mock('$app/navigation', async (importOriginal) => ({
  ...(await importOriginal<typeof import('$app/navigation')>()),
  goto: navigation.goto
}));

function serverInfo(overrides: Partial<PublicServerInfo> = {}): PublicServerInfo {
  return {
    name: 'Invited Community',
    version: '0.5.0',
    authorizeUrl: '/oauth/authorize',
    directRegistrationEnabled: true,
    directLoginEnabled: true,
    accountCreationPolicy: 'invite_only',
    welcomeMessage: null,
    description: null,
    iconUrl: null,
    bannerUrl: null,
    authProviders: [],
    ...overrides
  };
}

describe('invite-only registration', () => {
  beforeEach(() => {
    navigation.goto.mockReset();
  });

  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it('validates a code before showing email registration and keeps its selector for resend', async () => {
    const fetcher = vi.fn<typeof fetch>(async (input) => {
      const path = String(input);
      if (path === '/auth/browser/registration-invitation')
        return Response.json({ admissionId: 'tab-a', externalIdentityToken: '' });
      if (path === '/auth/register') return Response.json({ message: 'sent' });
      throw new Error('Unexpected request');
    });
    vi.stubGlobal('fetch', fetcher);
    const page = render(RegisterPage, {
      props: {
        data: {
          user: null,
          serverInfo: serverInfo(),
          serverInfoLoaded: true,
          admissionId: '',
          externalIdentityToken: ''
        }
      }
    });
    await userEvent.fill(page.getByLabelText('Invitation code'), 'ABCD-EFGH-IJKL-MNOP-QRST');
    await userEvent.click(page.getByRole('button', { name: 'Continue' }));
    await expect.element(page.getByLabelText('Email')).toBeVisible();
    await userEvent.fill(page.getByLabelText('Email'), 'invited@example.test');
    await userEvent.click(page.getByRole('button', { name: 'Continue' }));
    await userEvent.click(page.getByRole('button', { name: 'Resend', exact: true }));
    const submissions = fetcher.mock.calls.filter(([path]) => path === '/auth/register');
    expect(submissions).toHaveLength(2);
    for (const submission of submissions) {
      expect(JSON.parse(submission[1]?.body as string)).toMatchObject({ admissionId: 'tab-a' });
    }
  });

  it('requires an invitation code before showing account creation choices', async () => {
    const { getByText, getByLabelText } = render(RegisterPage, {
      props: {
        data: {
          user: null,
          serverInfo: serverInfo(),
          serverInfoLoaded: true,
          admissionId: '',
          externalIdentityToken: ''
        }
      }
    });

    await expect
      .element(getByText('Enter an invitation code to create an account on this server.'))
      .toBeVisible();
    await expect.element(getByLabelText('Email')).not.toBeInTheDocument();
  });

  it('continues a verified SSO flow after code admission without showing the email form', async () => {
    const fetcher = vi.fn<typeof fetch>(async () =>
      Response.json({ admissionId: 'sso-tab', externalIdentityToken: 'admitted-flow' })
    );
    vi.stubGlobal('fetch', fetcher);
    const page = render(RegisterPage, {
      props: {
        data: {
          user: null,
          serverInfo: serverInfo(),
          serverInfoLoaded: true,
          admissionId: '',
          externalIdentityToken: 'verified-flow'
        }
      }
    });
    await userEvent.fill(page.getByLabelText('Invitation code'), 'ABCD-EFGH-IJKL-MNOP-QRST');
    await userEvent.click(page.getByRole('button', { name: 'Continue' }));
    expect(JSON.parse(fetcher.mock.calls[0][1]?.body as string)).toMatchObject({
      externalIdentityToken: 'verified-flow'
    });
    expect(navigation.goto).toHaveBeenCalledWith('/sso/confirm?token=admitted-flow', {
      replaceState: true
    });
    await expect.element(page.getByLabelText('Email')).not.toBeInTheDocument();
  });

  it('shows account creation choices after a code is accepted', async () => {
    const { getByRole, getByLabelText } = render(RegisterPage, {
      props: {
        data: {
          user: null,
          serverInfoLoaded: true,
          admissionId: 'admission-1',
          externalIdentityToken: '',
          serverInfo: serverInfo({
            directRegistrationEnabled: false,
            authProviders: [
              {
                id: 'company',
                type: 'oidc',
                label: 'Company SSO',
                loginUrl: '/auth/providers/company',
                issuerUrl: 'https://id.example',
                autoProvision: true
              }
            ]
          })
        }
      }
    });

    await expect.element(getByLabelText('Email')).not.toBeInTheDocument();
    await expect.element(getByRole('link', { name: 'Continue with Company SSO' })).toBeVisible();
  });

  it('does not offer sign-in-only providers as registration options', async () => {
    const { getByText } = render(RegisterPage, {
      props: {
        data: {
          user: null,
          serverInfoLoaded: true,
          admissionId: '',
          externalIdentityToken: '',
          serverInfo: serverInfo({
            directRegistrationEnabled: false,
            authProviders: [
              {
                id: 'sign-in-only',
                type: 'oidc',
                label: 'Sign-in only',
                loginUrl: '/auth/providers/sign-in-only',
                issuerUrl: 'https://id.example',
                autoProvision: false
              }
            ]
          })
        }
      }
    });

    await expect
      .element(getByText('Registration is not available on this instance.'))
      .toBeVisible();
  });
});
