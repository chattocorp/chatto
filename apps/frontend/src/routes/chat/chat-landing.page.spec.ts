import { describe, expect, it } from 'vitest';
import { load } from './+page';

async function expectRedirect(
  url: string,
  user: { id: string } | null,
  location: string
): Promise<void> {
  await expect(
    load({
      parent: async () => ({ user }),
      url: new URL(url)
    } as never)
  ).rejects.toMatchObject({ status: 302, location });
}

describe('chat landing load', () => {
  it('redirects authenticated users to the origin segment without waiting for a viewer projection', async () => {
    await expectRedirect('https://chat.example.test/chat', { id: 'user-1' }, '/chat/-');
  });

  it('preserves the welcome signal for the origin route', async () => {
    await expectRedirect(
      'https://chat.example.test/chat?welcome=true',
      { id: 'user-1' },
      '/chat/-?welcome=true'
    );
  });
});
