import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import type { RegisteredServer } from '../server/registry.js';

const { servers } = vi.hoisted(() => ({
  servers: new Map<string, RegisteredServer>()
}));

vi.mock('../server/serverIds.js', () => ({
  ownerOfServerId: (id: string) =>
    servers.has(id) ? { getServer: (serverId: string) => servers.get(serverId) } : undefined
}));

import { assetUrlForServer } from './assetUrls.js';

const ORIGIN = 'https://app.example';

function server(overrides: Partial<RegisteredServer> = {}): RegisteredServer {
  return {
    id: 'remote',
    url: 'https://remote.example',
    name: 'Remote',
    iconUrl: null,
    token: 'token',
    userId: 'user',
    userLogin: 'alice',
    userDisplayName: 'Alice',
    userAvatarUrl: null,
    reauthRequiredAt: null,
    addedAt: 1,
    ...overrides
  };
}

function stubBrowser() {
  vi.stubGlobal('window', { location: { origin: ORIGIN } });
}

describe('assetUrlForServer', () => {
  beforeEach(() => {
    servers.clear();
    servers.set('remote', server());
  });

  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it('resolves remote relative asset URLs to direct signed server URLs', () => {
    stubBrowser();

    expect(assetUrlForServer('remote', '/assets/files/att_1?access=ticket')).toBe(
      'https://remote.example/assets/files/att_1?access=ticket'
    );
  });

  it('keeps transformed asset URLs signed and direct', () => {
    stubBrowser();

    expect(
      assetUrlForServer('remote', '/assets/files/att_1/image/960x800/contain?access=ticket')
    ).toBe('https://remote.example/assets/files/att_1/image/960x800/contain?access=ticket');
  });

  it('leaves non-stable asset URLs unchanged', () => {
    stubBrowser();

    expect(assetUrlForServer('remote', '/assets/attachments/locator.sig')).toBe(
      '/assets/attachments/locator.sig'
    );
  });
});
