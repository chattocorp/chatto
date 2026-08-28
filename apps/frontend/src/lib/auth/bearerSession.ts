// SPDX-License-Identifier: Apache-2.0

/** Show-once bearer credentials returned by a human authentication flow. */
export type NewBearerSession = {
  token: string;
  refreshToken: string;
  expiresIn: number;
  /** Remaining lifetime of the current renewable-session window. */
  refreshTokenExpiresIn: number;
  oauthClientId: string | null;
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
