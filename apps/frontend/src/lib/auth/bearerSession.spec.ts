// SPDX-License-Identifier: Apache-2.0

import { describe, expect, it } from 'vitest';
import { oauthBearerSession, persistedBearerSession } from './bearerSession';

describe('bearer session responses', () => {
  it('converts a complete OAuth response to absolute persisted expiries', () => {
    const credentials = oauthBearerSession({
      access_token: 'access',
      refresh_token: 'refresh',
      expires_in: 900,
      refresh_token_expires_in: 86_400
    }, 'https://client.example/oauth/client-metadata.json');

    expect(credentials).not.toBeNull();
    if (!credentials) throw new Error('complete OAuth response was rejected');
    expect(persistedBearerSession(credentials, 1_000)).toEqual({
      token: 'access',
      refreshToken: 'refresh',
      accessTokenExpiresAt: 901_000,
      refreshTokenExpiresAt: 86_401_000,
      oauthClientId: 'https://client.example/oauth/client-metadata.json',
      refreshRequestId: null
    });
  });
});
