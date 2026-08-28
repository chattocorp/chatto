import { beforeEach, describe, expect, it, vi } from 'vitest';
import { load as legacyProfileLoad } from './+page';
import { load as legacyAppearanceLoad } from './app/+page';
import { load as legacyTimeLoad } from './preferences/+page';

const mocks = vi.hoisted(() => ({ redirect: vi.fn() }));

vi.mock('@sveltejs/kit', () => ({ redirect: mocks.redirect }));
vi.mock('$app/paths', () => ({
  resolve: (path: string, params: { serverId: string }) =>
    path.replace('[serverId]', params.serverId)
}));

describe('legacy settings routes', () => {
  beforeEach(() => mocks.redirect.mockReset());

  it.each([
    ['settings root', legacyProfileLoad, '/chat/remote/settings/profile'],
    ['app', legacyAppearanceLoad, '/chat/remote/settings/appearance'],
    ['preferences', legacyTimeLoad, '/chat/remote/settings/time']
  ])('redirects %s to its named route', (_name, load, destination) => {
    load({
      params: { serverId: 'remote' },
      url: new URL('https://chatto.test/old?from=bookmark')
    } as never);

    expect(mocks.redirect).toHaveBeenCalledWith(308, `${destination}?from=bookmark`);
  });
});
