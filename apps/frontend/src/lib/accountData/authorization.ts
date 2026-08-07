import { getClientConfiguration } from '$lib/clientConfig';
import { generateCodeChallenge, generateCodeVerifier, generateState } from '$lib/oauth/pkce';
import { OAuthPopupError, openOAuthPopup } from '$lib/oauth/popup';

const ACCOUNT_DATA_SCOPE = 'account_data';
const CALLBACK_PATH = '/servers/callback?mode=authling-account-data';

export type AccountDataAuthorization = {
  accessToken: string;
  expiresAt: number;
  issuer: string;
  clientId: string;
  accountId: string;
  providerLabel: string;
};

type OIDCDiscovery = {
  issuer?: unknown;
  authorization_endpoint?: unknown;
  token_endpoint?: unknown;
  userinfo_endpoint?: unknown;
  scopes_supported?: unknown;
  code_challenge_methods_supported?: unknown;
  token_endpoint_auth_methods_supported?: unknown;
};

/** Authorize this browser origin to access the user's Authling account data. */
export async function authorizeAccountData(): Promise<AccountDataAuthorization> {
  const verifier = generateCodeVerifier();
  const state = generateState();
  // Open before the first await so browsers keep this action associated with
  // the user's click while discovery and PKCE preparation run.
  const popup = openOAuthPopup(state);

  try {
    const clientConfiguration = await getClientConfiguration();
    if (!clientConfiguration.authling) {
      throw new Error('This client does not configure an Authling issuer.');
    }
    const discovery = await discoverAccountDataProvider(clientConfiguration.authling.issuer);
    const redirectUri = window.location.origin + CALLBACK_PATH;
    const clientId = clientConfiguration.authling.clientId;
    const challenge = await generateCodeChallenge(verifier);
    const authorizeURL = new URL(discovery.authorizationEndpoint);
    authorizeURL.search = new URLSearchParams({
      response_type: 'code',
      client_id: clientId,
      redirect_uri: redirectUri,
      scope: `openid ${ACCOUNT_DATA_SCOPE}`,
      code_challenge: challenge,
      code_challenge_method: 'S256',
      state
    }).toString();
    popup.navigate(authorizeURL.toString());

    const response = await popup.response;
    if (response.error) throw new OAuthPopupError(response.errorDescription || response.error);
    if (!response.code)
      throw new OAuthPopupError('The provider did not return an authorization code.');

    const tokenResponse = await fetch(discovery.tokenEndpoint, {
      method: 'POST',
      headers: { 'Content-Type': 'application/x-www-form-urlencoded' },
      body: new URLSearchParams({
        grant_type: 'authorization_code',
        client_id: clientId,
        redirect_uri: redirectUri,
        code: response.code,
        code_verifier: verifier
      }),
      signal: AbortSignal.timeout(10000)
    });
    const token = (await tokenResponse.json()) as {
      access_token?: unknown;
      expires_in?: unknown;
      scope?: unknown;
      error?: unknown;
      error_description?: unknown;
    };
    if (!tokenResponse.ok || typeof token.access_token !== 'string') {
      throw new Error(
        typeof token.error_description === 'string'
          ? token.error_description
          : typeof token.error === 'string'
            ? token.error
            : 'The provider did not return an access token.'
      );
    }
    if (typeof token.scope !== 'string' || !token.scope.split(' ').includes(ACCOUNT_DATA_SCOPE)) {
      throw new Error('The access token does not grant account-data access.');
    }
    const expiresIn = typeof token.expires_in === 'number' ? token.expires_in : 300;
    const userinfoResponse = await fetch(discovery.userinfoEndpoint, {
      headers: { Authorization: `Bearer ${token.access_token}` },
      signal: AbortSignal.timeout(10000)
    });
    const userinfo = (await userinfoResponse.json()) as { sub?: unknown };
    if (!userinfoResponse.ok || typeof userinfo.sub !== 'string' || !userinfo.sub) {
      throw new Error('The provider did not return an account identity.');
    }
    return {
      accessToken: token.access_token,
      expiresAt: Date.now() + expiresIn * 1000,
      issuer: discovery.issuer,
      clientId,
      accountId: userinfo.sub,
      providerLabel: 'Authling'
    };
  } catch (error) {
    popup.close();
    throw error;
  }
}

async function discoverAccountDataProvider(issuerURL: string): Promise<{
  issuer: string;
  authorizationEndpoint: string;
  tokenEndpoint: string;
  userinfoEndpoint: string;
}> {
  const issuer = issuerURL.replace(/\/$/, '');
  const response = await fetch(`${issuer}/.well-known/openid-configuration`, {
    signal: AbortSignal.timeout(10000)
  });
  if (!response.ok) throw new Error('OIDC discovery failed.');
  const document = (await response.json()) as OIDCDiscovery;
  if (document.issuer !== issuer) throw new Error('OIDC discovery returned a different issuer.');
  if (
    !Array.isArray(document.scopes_supported) ||
    !document.scopes_supported.includes(ACCOUNT_DATA_SCOPE)
  ) {
    throw new Error('This OIDC provider does not support account data.');
  }
  if (
    !Array.isArray(document.code_challenge_methods_supported) ||
    !document.code_challenge_methods_supported.includes('S256')
  ) {
    throw new Error('This OIDC provider does not support secure browser authorization.');
  }
  if (
    !Array.isArray(document.token_endpoint_auth_methods_supported) ||
    !document.token_endpoint_auth_methods_supported.includes('none')
  ) {
    throw new Error('This OIDC provider does not support public clients.');
  }
  if (
    typeof document.authorization_endpoint !== 'string' ||
    typeof document.token_endpoint !== 'string' ||
    typeof document.userinfo_endpoint !== 'string'
  ) {
    throw new Error('OIDC discovery is incomplete.');
  }
  assertProviderURL(document.authorization_endpoint, issuer);
  assertProviderURL(document.token_endpoint, issuer);
  assertProviderURL(document.userinfo_endpoint, issuer);
  return {
    issuer,
    authorizationEndpoint: document.authorization_endpoint,
    tokenEndpoint: document.token_endpoint,
    userinfoEndpoint: document.userinfo_endpoint
  };
}

function assertProviderURL(value: string, issuer: string): void {
  const endpoint = new URL(value);
  const issuerOrigin = new URL(issuer).origin;
  if (endpoint.origin !== issuerOrigin) {
    throw new Error('OIDC discovery returned a cross-origin endpoint.');
  }
  if (endpoint.protocol !== 'https:' && endpoint.hostname !== 'localhost') {
    throw new Error('OIDC endpoints must use HTTPS.');
  }
}
