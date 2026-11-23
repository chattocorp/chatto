import { Code, ConnectError } from '@connectrpc/connect';
import { describe, expect, it } from 'vitest';
import {
  hasIdentityLinkParams,
  identityLinkErrorMessage,
  identityLinkPath,
  isIdentityLinkPath,
  identityLinkReturnPath,
  identityLinkReverificationPath,
  linkStartFailure,
  parseIdentityLinkRequest,
  remoteIdentityLinkUrl
} from './identityLink';

const freshAuthRequired = new ConnectError(
  'fresh authentication is required',
  Code.FailedPrecondition
);
const options = { passwordSent: false, canUsePasswordProof: true, directLoginEnabled: true };

describe('identity-link window URLs', () => {
  it('returns the callback on the settings path that older replicas serve', () => {
    const path = identityLinkReturnPath('github-main', 'user-alice');
    expect(path).toBe(
      '/chat/-/settings/account?link_provider=github-main&link_user=user-alice&link_complete=1'
    );
    const url = new URL(`${path}&error=provider_failed`, 'https://chat.example');
    expect(hasIdentityLinkParams(url)).toBe(true);
    expect(parseIdentityLinkRequest(url)).toEqual({
      providerId: 'github-main',
      userId: 'user-alice',
      completed: true,
      errorCode: 'provider_failed',
      reverified: false,
      reverificationRequired: false
    });
  });

  it('opens a remote window on the settings path that older servers know', () => {
    const url = new URL(
      remoteIdentityLinkUrl('https://remote.example/api/connect', 'github-main', 'user-alice')
    );
    expect(url.origin + url.pathname).toBe('https://remote.example/chat/-/settings/account');
    expect(parseIdentityLinkRequest(url)).toMatchObject({
      providerId: 'github-main',
      userId: 'user-alice',
      completed: false
    });
  });

  it('starts again without the callback result', () => {
    expect(identityLinkPath('github-main', 'user-alice')).toBe(
      '/sso/link?link_provider=github-main&link_user=user-alice'
    );
  });

  it('recognises link window paths for sign-in redirects', () => {
    expect(isIdentityLinkPath('/sso/link?link_provider=github-main')).toBe(true);
    expect(isIdentityLinkPath('/sso/linked')).toBe(false);
    expect(isIdentityLinkPath('/chat/-/settings/account')).toBe(false);
  });

  it('ignores URLs without link parameters', () => {
    expect(hasIdentityLinkParams(new URL('https://chat.example/chat/-/settings/account'))).toBe(
      false
    );
  });

  it('maps callback error codes', () => {
    expect(identityLinkErrorMessage('external_identity_conflict')).toBe(
      'That provider identity is already linked to a different account.'
    );
    expect(identityLinkErrorMessage('unknown')).toBe('Failed to link sign-on provider');
  });
});

describe('linkStartFailure', () => {
  it('asks for the password when it can prove a fresh sign-in', () => {
    expect(linkStartFailure(freshAuthRequired, options)).toEqual({ kind: 'password_required' });
  });

  it('asks for a new sign-in when a password cannot prove it', () => {
    expect(linkStartFailure(freshAuthRequired, { ...options, canUsePasswordProof: false })).toEqual(
      {
        kind: 'failed',
        message: 'Sign in again before linking another provider.',
        retryable: false
      }
    );
    expect(
      linkStartFailure(freshAuthRequired, {
        ...options,
        canUsePasswordProof: false,
        directLoginEnabled: false
      })
    ).toMatchObject({
      kind: 'failed',
      retryable: false,
      message: expect.stringContaining('through your sign-in provider')
    });
  });

  it('re-verifies through a linked provider when one can and a password cannot', () => {
    const sso = { ...options, canUsePasswordProof: false, directLoginEnabled: false };
    expect(linkStartFailure(freshAuthRequired, { ...sso, canReverify: true })).toEqual({
      kind: 'reverification_required'
    });
    expect(linkStartFailure(freshAuthRequired, { ...options, canReverify: true })).toEqual({
      kind: 'password_required'
    });
    expect(
      linkStartFailure(freshAuthRequired, { ...sso, canReverify: true, passwordSent: true })
    ).toMatchObject({ kind: 'password_rejected' });
  });

  it('opens the window on the re-verification explanation', () => {
    const path = identityLinkReverificationPath('github-main', 'user-alice');
    expect(path).toBe('/sso/link?link_provider=github-main&link_user=user-alice&reverify=required');
    expect(parseIdentityLinkRequest(new URL(path, 'https://chat.example'))).toMatchObject({
      reverificationRequired: true,
      completed: false
    });
  });

  it('reads a successful re-verification from the window URL', () => {
    const url = new URL(
      `${identityLinkPath('github-main', 'user-alice')}&reverified=1`,
      'https://chat.example'
    );
    expect(parseIdentityLinkRequest(url)).toMatchObject({ completed: false, reverified: true });
  });

  it('keeps a failure after a sent password with the password', () => {
    expect(linkStartFailure(freshAuthRequired, { ...options, passwordSent: true })).toEqual({
      kind: 'password_rejected',
      message: 'fresh authentication is required'
    });
  });

  it('reports other failures', () => {
    expect(linkStartFailure(new Error('boom'), options)).toMatchObject({
      kind: 'failed',
      retryable: true
    });
  });
});
