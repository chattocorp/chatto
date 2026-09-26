import { beforeEach, describe, expect, it, vi } from 'vitest';
import { load as settingsRootLoad } from './+page';

const mocks = vi.hoisted(() => ({ redirect: vi.fn() }));

vi.mock('@sveltejs/kit', () => ({ redirect: mocks.redirect }));
vi.mock('$app/paths', () => ({
  resolve: (path: string, params: { serverId: string }) =>
    path.replace('[serverId]', params.serverId)
}));

describe('settings root route', () => {
  beforeEach(() => mocks.redirect.mockReset());

  it('redirects to the Appearance settings route', () => {
    settingsRootLoad({
      params: { serverId: 'remote' },
      url: new URL('https://chatto.test/old?from=bookmark')
    } as never);

    expect(mocks.redirect).toHaveBeenCalledWith(
      308,
      '/chat/remote/settings/appearance?from=bookmark'
    );
  });
});
