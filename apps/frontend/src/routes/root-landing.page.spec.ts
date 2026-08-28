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

describe('root landing load', () => {
  it('redirects unauthenticated visitors to login before rendering', async () => {
    await expectRedirect('https://chat.example.test/', null, '/login');
  });

  it('preserves query parameters when redirecting authenticated visitors to chat', async () => {
    await expectRedirect('https://chat.example.test/?welcome=true', { id: 'user-1' }, '/chat?welcome=true');
  });
});
