import { describe, expect, it } from 'vitest';
import type { ExternalIdentityProviderInfo } from '$lib/api/externalIdentities';
import {
  reverificationErrorMessage,
  reverificationProviders,
  reverificationStartUrl,
  wasReverified
} from './freshAuthReverification';

function provider(
  id: string,
  linked: boolean,
  reverificationAvailable: boolean
): ExternalIdentityProviderInfo {
  return {
    id,
    type: 'oidc',
    label: id,
    loginUrl: `https://chat.example/auth/providers/${id}`,
    linkUrl: `https://chat.example/auth/providers/${id}?intent=link`,
    linked,
    linkedIdentitySubjectHash: linked ? `${id}-subject` : null,
    reverificationAvailable
  };
}

describe('fresh-auth re-verification', () => {
  it('starts on the provider login URL with the return path', () => {
    const url = new URL(
      reverificationStartUrl(provider('sso', true, true), '/sso/link?link_provider=github')
    );
    expect(url.origin + url.pathname).toBe('https://chat.example/auth/providers/sso');
    expect(url.searchParams.get('intent')).toBe('reverify');
    expect(url.searchParams.get('redirect')).toBe('/sso/link?link_provider=github');
  });

  it('offers only linked providers that the server marks available', () => {
    const providers = [
      provider('sso', true, true),
      provider('unlinked', false, true),
      provider('github', true, false)
    ];
    expect(reverificationProviders(providers).map((candidate) => candidate.id)).toEqual(['sso']);
  });

  it('reads the callback result', () => {
    expect(wasReverified(new URL('https://chat.example/sso/link?reverified=1'))).toBe(true);
    expect(wasReverified(new URL('https://chat.example/sso/link'))).toBe(false);
    expect(reverificationErrorMessage('reverification_mismatch')).toMatch(/different account/);
    expect(reverificationErrorMessage('unknown')).toMatch(/could not confirm/);
  });
});
