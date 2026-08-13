import { describe, expect, it } from 'vitest';
import {
  OAuthClient as APIOAuthClient,
  OAuthClientPolicy,
  OAuthClientSource
} from '@chatto/api-types/admin/v1/oauth_clients_pb';
import { mapOAuthClient } from './oauthClients';

describe('OAuth client enum mapping', () => {
  it('preserves future policy and source values as explicit unknown states', () => {
    const mapped = mapOAuthClient(
      new APIOAuthClient({
        clientId: 'https://future.example/oauth/client-metadata.json',
        source: 99 as OAuthClientSource,
        policy: 101 as OAuthClientPolicy
      })
    );

    expect(mapped.source).toBe('unknown');
    expect(mapped.sourceCode).toBe(99);
    expect(mapped.policy).toBe('unknown');
    expect(mapped.policyCode).toBe(101);
  });

  it('does not mislabel unspecified values as CIMD or default policy', () => {
    const mapped = mapOAuthClient(
      new APIOAuthClient({
        source: OAuthClientSource.OAUTH_CLIENT_SOURCE_UNSPECIFIED,
        policy: OAuthClientPolicy.OAUTH_CLIENT_POLICY_UNSPECIFIED
      })
    );

    expect(mapped.source).toBe('unknown');
    expect(mapped.policy).toBe('unknown');
  });

  it('continues mapping every supported policy and source', () => {
    expect(
      mapOAuthClient(
        new APIOAuthClient({
          source: OAuthClientSource.OAUTH_CLIENT_SOURCE_CIMD,
          policy: OAuthClientPolicy.OAUTH_CLIENT_POLICY_DEFAULT
        })
      )
    ).toMatchObject({ source: 'cimd', policy: 'default' });
    expect(
      mapOAuthClient(
        new APIOAuthClient({
          source: OAuthClientSource.OAUTH_CLIENT_SOURCE_BUILT_IN,
          policy: OAuthClientPolicy.OAUTH_CLIENT_POLICY_TRUSTED
        })
      )
    ).toMatchObject({ source: 'built-in', policy: 'trusted' });
    expect(
      mapOAuthClient(
        new APIOAuthClient({
          source: OAuthClientSource.OAUTH_CLIENT_SOURCE_CIMD,
          policy: OAuthClientPolicy.OAUTH_CLIENT_POLICY_BLOCKED
        })
      )
    ).toMatchObject({ source: 'cimd', policy: 'blocked' });
  });
});
