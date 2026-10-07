import { beforeEach, describe, expect, it, vi } from 'vitest';
import { load } from './[name]/+page';

const mocks = vi.hoisted(() => ({ redirect: vi.fn() }));

vi.mock('@sveltejs/kit', () => ({ redirect: mocks.redirect }));
vi.mock('$app/paths', () => ({
  resolve: (path: string, params: Record<string, string>) =>
    Object.entries(params).reduce(
      (resolved, [name, value]) => resolved.replace(`[${name}]`, value),
      path
    )
}));

describe('legacy role page redirect', () => {
  beforeEach(() => mocks.redirect.mockReset());

  it.each([
    ['moderator', '/chat/remote/manage/server/roles/moderator'],
    ['new', '/chat/remote/manage/server/roles/new']
  ])('redirects permissions/%s to its Roles page', (name, destination) => {
    load({
      params: { serverId: 'remote', name },
      url: new URL('https://chatto.test/old?from=bookmark')
    } as never);

    expect(mocks.redirect).toHaveBeenCalledWith(308, `${destination}?from=bookmark`);
  });
});
