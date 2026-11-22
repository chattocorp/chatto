import { describe, expect, it } from 'vitest';
import { load } from './+page';

describe('account settings load', () => {
  it('forwards a legacy link window URL with its query to the link window', () => {
    const url = new URL(
      'https://chat.example.test/chat/-/settings/account?link_provider=github-main&link_user=u1&link_complete=1&error=provider_failed'
    );
    expect(() => load({ url } as never)).toThrow(
      expect.objectContaining({
        status: 302,
        location:
          '/sso/link?link_provider=github-main&link_user=u1&link_complete=1&error=provider_failed'
      })
    );
  });

  it('shows account settings without link parameters', () => {
    const url = new URL('https://chat.example.test/chat/-/settings/account?tab=x');
    expect(load({ url } as never)).toBeUndefined();
  });
});
