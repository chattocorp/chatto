import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { userEvent } from 'vitest/browser';
import type { PublicServerInfo } from '@chatto/client/api/server';
import { GetPendingExternalIdentityResponse } from '@chatto/api-types/chatto/auth/v1/external_identity_auth_pb';
import RegisterPage from './+page.svelte';
import { load } from './+page';

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
          redirectUrl: '/',
          user: null,
          serverInfo: serverInfo(),
          serverInfoLoaded: true,
          admissionId: '',
          invitationRequired: true,
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
          redirectUrl: '/',
          user: null,
          serverInfo: serverInfo(),
          serverInfoLoaded: true,
          admissionId: '',
          invitationRequired: true,
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
          invitationRequired: true,
          externalIdentityToken: 'verified-flow',
          redirectUrl: '/'
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
          redirectUrl: '/',
          user: null,
          serverInfoLoaded: true,
          admissionId: 'admission-1',
          invitationRequired: true,
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
          redirectUrl: '/',
          serverInfoLoaded: true,
          admissionId: '',
          invitationRequired: true,
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

  it.each([true, false])(
    'honours pending SSO admission with stale open discovery (email enabled: %s)',
    async (directRegistrationEnabled) => {
      const fetcher = vi.fn<typeof fetch>(
        async () =>
          new Response(
            new Uint8Array(
              new GetPendingExternalIdentityResponse({
                pending: { kind: 1, invitationRequired: true }
              }).toBinary()
            ),
            { headers: { 'Content-Type': 'application/proto' } }
          )
      );
      vi.stubGlobal('fetch', fetcher);
      const parentData = {
        user: null,
        serverInfo: serverInfo({ accountCreationPolicy: 'open', directRegistrationEnabled }),
        serverInfoLoaded: true
      };
      const loaded = await load({
        parent: async () => parentData,
        url: new URL('https://chat.example.test/register?external_identity=verified-flow'),
        fetch: fetcher
      } as never);
      const page = render(RegisterPage, { props: { data: { ...parentData, ...loaded } } });

      await expect.element(page.getByLabelText('Invitation code')).toBeVisible();
      await expect.element(page.getByLabelText('Email')).not.toBeInTheDocument();
      await expect
        .element(page.getByRole('link', { name: 'Sign in with existing account' }))
        .toHaveAttribute('href', '/sso/confirm?token=verified-flow&existing=1');
    }
  );

  it('keeps ordinary registration sign-in redirects', async () => {
    const parentData = { user: null, serverInfo: serverInfo(), serverInfoLoaded: true };
    const loaded = await load({
      parent: async () => parentData,
      url: new URL('https://chat.example.test/register?redirect=%2Foauth%2Fauthorize'),
      fetch: async () => Response.json({ admissionId: '' })
    } as never);
    const page = render(RegisterPage, { props: { data: { ...parentData, ...loaded } } });
    await expect.element(page.getByLabelText('Invitation code')).toBeVisible();
    await expect
      .element(page.getByRole('link', { name: 'Sign In', exact: true }))
      .toHaveAttribute('href', '/login?redirect=%2Foauth%2Fauthorize');
  });
});
