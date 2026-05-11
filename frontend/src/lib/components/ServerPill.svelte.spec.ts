import { describe, it, expect, beforeEach, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { q } from '$lib/test-utils';

interface RegisteredInstanceMock {
  id: string;
  name: string;
  url: string;
  iconUrl: string | null;
}

interface StoreMock {
  instance: {
    iconUrl: string | null;
    bannerUrl: string | null;
    description: string | null;
    welcomeMessage: string | null;
    motd: string | null;
  };
}

const { mockInstances, mockStores } = vi.hoisted(() => ({
  mockInstances: { current: [] as RegisteredInstanceMock[] },
  mockStores: { current: new Map<string, StoreMock>() }
}));

vi.mock('$lib/state/server/registry.svelte', () => ({
  serverRegistry: {
    get instances() {
      return mockInstances.current;
    },
    getInstance: (id: string) =>
      mockInstances.current.find((i) => i.id === id),
    tryGetStore: (id: string) => mockStores.current.get(id)
  }
}));

import ServerPill from './ServerPill.svelte';

function makeInstance(o: Partial<RegisteredInstanceMock> = {}): RegisteredInstanceMock {
  return {
    id: 'a',
    name: 'Instance A',
    url: 'https://a.example.com',
    iconUrl: null,
    ...o
  };
}

function makeStore(o: Partial<StoreMock['instance']> = {}): StoreMock {
  return {
    instance: {
      iconUrl: null,
      bannerUrl: null,
      description: null,
      welcomeMessage: null,
      motd: null,
      ...o
    }
  };
}

beforeEach(() => {
  mockInstances.current = [];
  mockStores.current = new Map();
});

describe('ServerPill', () => {
  describe('single-instance gating', () => {
    it('renders nothing when no instances are registered', () => {
      const { container } = render(ServerPill, { props: { serverId: 'a' } });
      expect(container.querySelector('button[aria-haspopup="dialog"]')).toBeNull();
    });

    it('renders nothing when only a single instance is registered', () => {
      mockInstances.current = [makeInstance({ id: 'a', name: 'Alpha' })];
      mockStores.current.set('a', makeStore());

      const { container } = render(ServerPill, { props: { serverId: 'a' } });

      expect(container.querySelector('button[aria-haspopup="dialog"]')).toBeNull();
      expect(container.textContent ?? '').not.toContain('Alpha');
    });

    it('renders the pill when more than one instance is registered', async () => {
      mockInstances.current = [
        makeInstance({ id: 'a', name: 'Alpha' }),
        makeInstance({ id: 'b', name: 'Beta' })
      ];
      mockStores.current.set('a', makeStore());
      mockStores.current.set('b', makeStore());

      const { container } = render(ServerPill, { props: { serverId: 'a' } });

      await expect
        .element(q(container, 'button[aria-haspopup="dialog"]'))
        .toBeInTheDocument();
      expect(container.textContent).toContain('Alpha');
    });
  });

  describe('rendering', () => {
    beforeEach(() => {
      // Two instances → pill is visible
      mockInstances.current = [
        makeInstance({ id: 'a', name: 'Alpha' }),
        makeInstance({ id: 'b', name: 'Beta' })
      ];
      mockStores.current.set('a', makeStore());
      mockStores.current.set('b', makeStore());
    });

    it('renders the globe icon and the truncated instance name', async () => {
      const { container } = render(ServerPill, { props: { serverId: 'a' } });

      await expect.element(q(container, '.uil--globe')).toBeInTheDocument();
      await expect
        .element(q(container, '.truncate'))
        .toHaveTextContent('Alpha');
    });

    it('reflects the requested instance, not the first registered one', async () => {
      const { container } = render(ServerPill, { props: { serverId: 'b' } });

      expect(container.textContent).toContain('Beta');
      expect(container.textContent).not.toContain('Alpha');
    });
  });
});
