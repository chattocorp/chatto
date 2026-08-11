import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import type { PublicServerInfo } from '$lib/api-client/server';
import RegisterPage from './+page.svelte';

const navigation = vi.hoisted(() => ({ goto: vi.fn(), replaceState: vi.fn() }));

vi.mock('$app/navigation', async (importOriginal) => ({
  ...(await importOriginal<typeof import('$app/navigation')>()),
  goto: navigation.goto,
  replaceState: navigation.replaceState
}));

function serverInfo(overrides: Partial<PublicServerInfo> = {}): PublicServerInfo {
  return {
    name: 'Invited Community',
    version: '0.5.0',
    authorizeUrl: '/oauth/authorize',
    directRegistrationEnabled: true,
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
    navigation.replaceState.mockReset();
  });

  afterEach(() => {
    window.location.hash = '';
    vi.unstubAllGlobals();
  });

  it('removes invitation capabilities from the URL even when admission is open', async () => {
    window.location.hash = 'invite=shared-capability';
    render(RegisterPage, {
      props: {
        data: {
          user: null,
          serverInfoLoaded: true,
          serverInfo: serverInfo({ accountCreationPolicy: 'open' })
        }
      }
    });

    await vi.waitFor(() =>
      expect(navigation.replaceState).toHaveBeenCalledWith('/register', history.state)
    );
  });

  it('requires a validated invitation before showing account creation choices', async () => {
    const fetchMock = vi.fn(
      async () =>
        new Response(JSON.stringify({ valid: true }), {
          status: 200,
          headers: { 'Content-Type': 'application/json' }
        })
    );
    vi.stubGlobal('fetch', fetchMock);

    const { getByRole, getByLabelText } = render(RegisterPage, {
      props: { data: { user: null, serverInfo: serverInfo(), serverInfoLoaded: true } }
    });

    await expect.element(getByRole('heading', { name: 'Enter invitation' })).toBeVisible();
    await getByLabelText('Invitation code').fill('cht_INV1.example.signature');
    await getByRole('button', { name: 'Continue' }).click();

    expect(fetchMock).toHaveBeenCalledWith(
      '/auth/invitation',
      expect.objectContaining({
        method: 'POST',
        body: JSON.stringify({ code: 'cht_INV1.example.signature' })
      })
    );
    await expect.element(getByLabelText('Email')).toBeVisible();
  });

  it('shows external providers only after invitation validation', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn(
        async () =>
          new Response(JSON.stringify({ valid: true }), {
            status: 200,
            headers: { 'Content-Type': 'application/json' }
          })
      )
    );
    const { getByRole, getByLabelText } = render(RegisterPage, {
      props: {
        data: {
          user: null,
          serverInfoLoaded: true,
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

    await expect
      .element(getByRole('link', { name: 'Continue with Company SSO' }))
      .not.toBeInTheDocument();
    await getByLabelText('Invitation code').fill('cht_INV1.example.signature');
    await getByRole('button', { name: 'Continue' }).click();
    await expect.element(getByRole('link', { name: 'Continue with Company SSO' })).toBeVisible();
  });

  it('does not offer sign-in-only providers as registration options', async () => {
    const { getByText } = render(RegisterPage, {
      props: {
        data: {
          user: null,
          serverInfoLoaded: true,
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
