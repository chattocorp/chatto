import { flushSync } from 'svelte';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import type { PublicServerInfo } from '$lib/api-client/server';
import type { Neighbor } from '$lib/api-client/neighbors';
import { queryClient } from '$lib/query/client';

const mocks = vi.hoisted(() => ({
  list: vi.fn(),
  create: vi.fn(),
  update: vi.fn(),
  delete: vi.fn(),
  loadServerProfiles: vi.fn()
}));

vi.mock('$lib/state/server/scope.svelte', () => ({
  useServerScope: () => ({
    serverId: 'origin',
    connection: {
      queryScope: 'neighbors-test',
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

vi.mock('$lib/serverDirectory', async (importOriginal) => {
  const actual = await importOriginal<typeof import('$lib/serverDirectory')>();
  return { ...actual, loadServerProfiles: mocks.loadServerProfiles };
});

import Page from './+page.svelte';

function neighbor(origin: string, testimonial: string | null = null): Neighbor {
  return { id: 'neighbor-1', origin, testimonial, revision: 'revision-1' };
}

function profile(): PublicServerInfo {
  return {
    name: 'Preview Chatto',
    version: '0.5.0',
    authorizeUrl: '/oauth/authorize',
    directRegistrationEnabled: true,
    directLoginEnabled: true,
    accountCreationPolicy: 'open',
    welcomeMessage: null,
    description: 'A public description from the advertised server.',
    iconUrl: 'https://dev.preview.chatto.run/logo.webp',
    bannerUrl: 'https://dev.preview.chatto.run/banner.webp',
    authProviders: []
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
    mocks.create.mockImplementation(async (origin: string, testimonial: string) =>
      neighbor(origin, testimonial || null)
    );
    mocks.update.mockImplementation(
      async (_current: Neighbor, origin: string, testimonial: string) =>
        neighbor(origin, testimonial || null)
    );
    mocks.delete.mockResolvedValue(undefined);
    mocks.loadServerProfiles.mockResolvedValue([]);
  });

  it('renders each Neighbor with its public server profile', async () => {
    const current = neighbor('https://dev.preview.chatto.run');
    const publicProfile = profile();
    mocks.list.mockResolvedValue([current]);
    mocks.loadServerProfiles.mockResolvedValue([
      { origin: current.origin, profile: publicProfile }
    ]);

    const { container } = render(Page);

    await vi.waitFor(() => {
      expect(container.querySelectorAll('[data-testid="neighbor-card"]')).toHaveLength(1);
      expect(container.textContent).toContain('Preview Chatto');
      expect(container.textContent).toContain(publicProfile.description);
    });
    expect(
      Array.from(container.querySelectorAll('h2')).some((heading) =>
        heading.textContent?.trim().startsWith('Neighbors')
      )
    ).toBe(true);
    expect(container.querySelector<HTMLImageElement>('img')?.src).toContain('/banner.webp');
  });

  it.each([
    ['dev.preview.chatto.run', 'https://dev.preview.chatto.run'],
    ['https://dev.preview.chatto.run/chat/-/RMch1OYtMwZ7sOJ', 'https://dev.preview.chatto.run']
  ])('adds %s as the canonical origin', async (entered, expected) => {
    const { container } = render(Page);
    await vi.waitFor(() => expect(mocks.list).toHaveBeenCalled());

    input(container, '#new-neighbor-origin', entered);
    container.querySelector('form')!.requestSubmit();

    await vi.waitFor(() => expect(mocks.create).toHaveBeenCalledWith(expected, ''));
  });

  it('canonicalizes a full URL when editing a Neighbor', async () => {
    const current = neighbor('https://old.example');
    mocks.list.mockResolvedValue([current]);
    mocks.loadServerProfiles.mockResolvedValue([{ origin: current.origin, profile: null }]);

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
      expect(mocks.update).toHaveBeenCalledWith(current, 'https://new.example', '');
    });
  });

  it('displays and edits a public testimonial', async () => {
    const current = neighbor(
      'https://neighbor.example',
      'A **welcoming** community.\n\nPeople listen.'
    );
    mocks.list.mockResolvedValue([current]);
    mocks.loadServerProfiles.mockResolvedValue([{ origin: current.origin, profile: profile() }]);

    const { container } = render(Page);
    await vi.waitFor(() => expect(container.textContent).toContain('A welcoming community.'));
    const testimonial = container.querySelector('[data-testid="neighbor-testimonial"]')!;
    expect(testimonial.querySelectorAll('p')).toHaveLength(2);
    expect(testimonial.querySelector('strong')?.textContent).toBe('welcoming');

    flushSync(() => button(container, 'Edit').click());
    input(container, '#neighbor-testimonial-neighbor-1', '  Clear and kind moderation.  ');
    button(container, 'Save').click();
    await vi.waitFor(() => {
      expect(mocks.update).toHaveBeenCalledWith(
        current,
        'https://neighbor.example',
        'Clear and kind moderation.'
      );
    });

    await vi.waitFor(() => expect(button(container, 'Edit')).toBeDefined());
    flushSync(() => button(container, 'Edit').click());
    input(container, '#neighbor-testimonial-neighbor-1', '');
    button(container, 'Save').click();
    await vi.waitFor(() => {
      expect(mocks.update).toHaveBeenLastCalledWith(
        expect.objectContaining({ origin: 'https://neighbor.example' }),
        'https://neighbor.example',
        ''
      );
    });
  });

  it('creates a Neighbor with a trimmed testimonial', async () => {
    const { container } = render(Page);
    await vi.waitFor(() => expect(mocks.list).toHaveBeenCalled());
    input(container, '#new-neighbor-origin', 'new.example');
    input(container, '#new-neighbor-testimonial', '  A small, lively server.  ');
    container.querySelector('form')!.requestSubmit();
    await vi.waitFor(() => {
      expect(mocks.create).toHaveBeenCalledWith('https://new.example', 'A small, lively server.');
    });
  });
});
