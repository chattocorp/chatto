import { flushSync } from 'svelte';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import type { NeighborhoodServer } from '$lib/api-client/server';
import type { Neighbor } from '$lib/api-client/neighbors';
import { queryClient } from '$lib/query/client';

const mocks = vi.hoisted(() => ({
  list: vi.fn(),
  create: vi.fn(),
  update: vi.fn(),
  delete: vi.fn(),
  listNeighborhoodServers: vi.fn()
}));

vi.mock('$lib/state/server/scope.svelte', () => ({
  useServerScope: () => ({
    serverId: 'origin',
    connection: {
      queryScope: 'neighbors-test',
      connectBaseUrl: 'https://self.example/api/connect',
      getAPI: () => ({
        list: mocks.list,
        create: mocks.create,
        update: mocks.update,
        delete: mocks.delete
      })
    },
    isCurrent: () => true
  })
}));

vi.mock('$lib/api-client/server', async (importOriginal) => {
  const actual = await importOriginal<typeof import('$lib/api-client/server')>();
  return { ...actual, listNeighborhoodServers: mocks.listNeighborhoodServers };
});

import Page from './+page.svelte';

function neighbor(origin: string): Neighbor {
  return { id: 'neighbor-1', origin, revision: 'revision-1' };
}

/** A cached Neighborhood entry; image paths identify copies on the current server. */
function cachedServer(origin: string): NeighborhoodServer {
  return {
    origin,
    profile: {
      name: 'Preview Chatto',
      version: '0.5.0',
      description: 'A public description from the advertised server.',
      iconUrl: '/assets/neighborhood/logo',
      bannerUrl: '/assets/neighborhood/banner'
    },
    directNeighbor: true,
    recommendedByOrigins: []
  };
}

function input(container: HTMLElement, selector: string, value: string) {
  const element = container.querySelector<HTMLInputElement>(selector)!;
  element.value = value;
  element.dispatchEvent(new Event('input', { bubbles: true }));
  flushSync();
}

function button(container: HTMLElement, label: string): HTMLButtonElement {
  return Array.from(container.querySelectorAll<HTMLButtonElement>('button')).find(
    (candidate) => candidate.textContent?.trim() === label
  )!;
}

describe('Neighbor management page', () => {
  beforeEach(() => {
    queryClient.clear();
    vi.clearAllMocks();
    mocks.list.mockResolvedValue([]);
    mocks.create.mockImplementation(async (origin: string) => neighbor(origin));
    mocks.update.mockImplementation(async (_current: Neighbor, origin: string) => neighbor(origin));
    mocks.delete.mockResolvedValue(undefined);
    mocks.listNeighborhoodServers.mockResolvedValue([]);
  });

  afterEach(() => {
    vi.unstubAllGlobals();
    vi.restoreAllMocks();
  });

  it('renders each Neighbor with its cached profile from the current server', async () => {
    const png = Uint8Array.from(
      atob(
        'iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mP8z8DwHwAFBQIAX8jx0gAAAABJRU5ErkJggg=='
      ),
      (character) => character.charCodeAt(0)
    );
    const imageFetch = vi.fn(
      async () =>
        new Response(new Blob([png], { type: 'image/png' }), {
          status: 200,
          headers: { 'Content-Type': 'image/png' }
        })
    );
    vi.stubGlobal('fetch', imageFetch);
    const current = neighbor('https://dev.preview.chatto.run');
    const cached = cachedServer(current.origin);
    mocks.list.mockResolvedValue([current]);
    mocks.listNeighborhoodServers.mockResolvedValue([cached]);

    const { container } = render(Page);

    await vi.waitFor(() => {
      expect(container.querySelectorAll('[data-testid="neighbor-card"]')).toHaveLength(1);
      expect(container.textContent).toContain('Preview Chatto');
      expect(container.textContent).toContain(cached.profile.description);
    });
    expect(
      Array.from(container.querySelectorAll('h2')).some((heading) =>
        heading.textContent?.trim().startsWith('Neighbors')
      )
    ).toBe(true);
    await vi.waitFor(() => {
      expect(container.querySelector<HTMLImageElement>('img')?.src).toContain('blob:');
    });
    expect(mocks.listNeighborhoodServers).toHaveBeenCalledWith(
      'https://self.example',
      expect.objectContaining({ signal: expect.any(AbortSignal) })
    );
    // The browser loads only the current server's copies, never the Neighbor.
    for (const [url] of imageFetch.mock.calls as unknown as [string][]) {
      expect(new URL(url).origin).toBe('https://self.example');
    }
    expect(imageFetch).toHaveBeenCalledWith(
      'https://self.example/assets/neighborhood/banner',
      expect.objectContaining({
        credentials: 'omit',
        redirect: 'error',
        referrerPolicy: 'no-referrer'
      })
    );
  });

  it.each([
    ['dev.preview.chatto.run', 'https://dev.preview.chatto.run'],
    ['https://dev.preview.chatto.run/chat/-/RMch1OYtMwZ7sOJ', 'https://dev.preview.chatto.run']
  ])('adds %s as the canonical origin', async (entered, expected) => {
    const { container } = render(Page);
    await vi.waitFor(() => expect(mocks.list).toHaveBeenCalled());

    input(container, '#new-neighbor-origin', entered);
    container.querySelector('form')!.requestSubmit();

    await vi.waitFor(() => expect(mocks.create).toHaveBeenCalledWith(expected));
  });

  it('shows a new Neighbor as loading until discovery caches its profile', async () => {
    const added = 'https://new.example';
    mocks.listNeighborhoodServers.mockResolvedValue([]);
    const { container } = render(Page);
    await vi.waitFor(() => expect(mocks.list).toHaveBeenCalled());

    input(container, '#new-neighbor-origin', added);
    container.querySelector('form')!.requestSubmit();

    await vi.waitFor(() => {
      expect(container.querySelectorAll('[data-testid="neighbor-card"]')).toHaveLength(1);
      expect(mocks.listNeighborhoodServers).toHaveBeenCalled();
    });
    expect(container.textContent).not.toContain('public profile could not be loaded');

    mocks.listNeighborhoodServers.mockResolvedValue([cachedServer(added)]);
    await vi.waitFor(() => expect(container.textContent).toContain('Preview Chatto'), {
      timeout: 5_000
    });
  });

  it('canonicalizes a full URL when editing a Neighbor', async () => {
    const current = neighbor('https://old.example');
    mocks.list.mockResolvedValue([current]);

    const { container } = render(Page);
    await vi.waitFor(() => {
      expect(button(container, 'Edit')).toBeDefined();
      expect(container.querySelectorAll('[data-testid="neighbor-card"]')).toHaveLength(1);
      expect(container.textContent).toContain('public profile could not be loaded');
    });
    flushSync(() => button(container, 'Edit').click());
    await vi.waitFor(() => {
      expect(container.querySelector('#neighbor-origin-neighbor-1')).not.toBeNull();
    });
    input(container, '#neighbor-origin-neighbor-1', 'https://new.example/chat/-/room');
    button(container, 'Save').click();

    await vi.waitFor(() => {
      expect(mocks.update).toHaveBeenCalledWith(current, 'https://new.example');
    });
  });
});
