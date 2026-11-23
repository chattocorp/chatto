import { describe, expect, it } from 'vitest';
import { load } from './+page';

const url = new URL('https://chat.example.test/sso/link?link_provider=github-main&link_user=u1');

describe('identity-link window load', () => {
  it('sends a signed-out window to sign-in and back to the same link', async () => {
    await expect(
      load({ url, parent: async () => ({ user: null, serverInfo: null }) } as never)
    ).rejects.toMatchObject({
      status: 302,
      location: '/login?redirect=%2Fsso%2Flink%3Flink_provider%3Dgithub-main%26link_user%3Du1'
    });
  });

  it('passes the signed-in account and password sign-in availability', async () => {
    const user = { id: 'u1', hasPassword: true };
    await expect(
      load({
        url,
        parent: async () => ({ user, serverInfo: { directLoginEnabled: false } })
      } as never)
    ).resolves.toEqual({ user, directLoginEnabled: false });
  });
});
