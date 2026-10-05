import { describe, expect, it } from 'vitest';
import {
  OAuthClient as APIOAuthClient,
  OauthClientPolicy,
  OauthClientSource
} from '@chatto/api-types/admin/v1/oauth_clients_pb';
import { mapOAuthClient } from './oauthClients';

describe('OAuth client enum mapping', () => {
  it('preserves the public JSON enum names after type renaming', () => {
    const json = { source: 'OAUTH_CLIENT_SOURCE_CIMD', policy: 'OAUTH_CLIENT_POLICY_TRUSTED' };
    const client = APIOAuthClient.fromJson(json);
    expect(client.source).toBe(OauthClientSource.CIMD);
    expect(client.policy).toBe(OauthClientPolicy.TRUSTED);
    expect(client.toJson()).toEqual(json);
  });

  it('preserves future policy and source values as explicit unknown states', () => {
    const mapped = mapOAuthClient(
      new APIOAuthClient({
        clientId: 'https://future.example/oauth/client-metadata.json',
        source: 99 as OauthClientSource,
        policy: 101 as OauthClientPolicy
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
        source: OauthClientSource.UNSPECIFIED,
        policy: OauthClientPolicy.UNSPECIFIED
      })
    );

    expect(mapped.source).toBe('unknown');
    expect(mapped.policy).toBe('unknown');
  });

  it('continues mapping every supported policy and source', () => {
    expect(
      mapOAuthClient(
        new APIOAuthClient({
          source: OauthClientSource.CIMD,
          policy: OauthClientPolicy.DEFAULT
        })
      )
    ).toMatchObject({ source: 'cimd', policy: 'default' });
    expect(
      mapOAuthClient(
        new APIOAuthClient({
          source: OauthClientSource.BUILT_IN,
          policy: OauthClientPolicy.TRUSTED
        })
      )
    ).toMatchObject({ source: 'built-in', policy: 'trusted' });
    expect(
      mapOAuthClient(
        new APIOAuthClient({
          source: OauthClientSource.CIMD,
          policy: OauthClientPolicy.BLOCKED
        })
      )
    ).toMatchObject({ source: 'cimd', policy: 'blocked' });
  });
});
