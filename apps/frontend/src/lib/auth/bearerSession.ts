// SPDX-License-Identifier: Apache-2.0

/** Show-once bearer credentials returned by a human authentication flow. */
export type NewBearerSession = {
  token: string;
  refreshToken: string;
  expiresIn: number;
  refreshTokenExpiresIn: number;
  oauthClientId: string | null;
};

type AuthenticationResponseShape = {
  token?: unknown;
  refreshToken?: unknown;
  expiresIn?: unknown;
  refreshTokenExpiresIn?: unknown;
};

type OAuthTokenResponseShape = {
  access_token?: unknown;
  refresh_token?: unknown;
  expires_in?: unknown;
  refresh_token_expires_in?: unknown;
};

function validLifetime(value: unknown): value is number {
  return typeof value === 'number' && Number.isFinite(value) && value > 0;
}

/** Parse the camelCase response used by direct login and registration. */
export function directBearerSession(
  value: AuthenticationResponseShape,
  oauthClientId: string | null = null
): NewBearerSession | null {
  if (
    typeof value.token !== 'string' ||
    value.token.length === 0 ||
    typeof value.refreshToken !== 'string' ||
    value.refreshToken.length === 0 ||
    !validLifetime(value.expiresIn) ||
    !validLifetime(value.refreshTokenExpiresIn)
  ) {
    return null;
  }
  return {
    token: value.token,
    refreshToken: value.refreshToken,
    expiresIn: value.expiresIn,
    refreshTokenExpiresIn: value.refreshTokenExpiresIn,
    oauthClientId
  };
}

/** Parse the standard snake_case OAuth token response. */
export function oauthBearerSession(
  value: OAuthTokenResponseShape,
  oauthClientId: string | null
): NewBearerSession | null {
  if (
    typeof value.access_token !== 'string' ||
    value.access_token.length === 0 ||
    typeof value.refresh_token !== 'string' ||
    value.refresh_token.length === 0 ||
    !validLifetime(value.expires_in) ||
    !validLifetime(value.refresh_token_expires_in)
  ) {
    return null;
  }
  return {
    token: value.access_token,
    refreshToken: value.refresh_token,
    expiresIn: value.expires_in,
    refreshTokenExpiresIn: value.refresh_token_expires_in,
    oauthClientId
  };
}

/** Convert relative response lifetimes into the persisted absolute shape. */
export function persistedBearerSession(credentials: NewBearerSession, now = Date.now()) {
  return {
    token: credentials.token,
    refreshToken: credentials.refreshToken,
    accessTokenExpiresAt: now + credentials.expiresIn * 1000,
    refreshTokenExpiresAt: now + credentials.refreshTokenExpiresIn * 1000,
    oauthClientId: credentials.oauthClientId,
    refreshRequestId: null
  } as const;
}
