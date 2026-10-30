import { ImageFitMode } from '@chatto/api-types/api/v1/common_pb';
import { Code, ConnectError } from '@connectrpc/connect';
import '../../app.css';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { tick } from 'svelte';
import MessagePreviewCard from './MessagePreviewCard.svelte';
import type { MessageLink } from '$lib/chatLinks';

import { TimelineEventKind } from '@chatto/client/timeline/timelineEvents';
import type { RefreshedAttachmentUrls } from '@chatto/client/attachments/attachmentUrls';

const { getMessageMock, messageResults, refreshAssetUrlsMock, registryState } = vi.hoisted(() => ({
  getMessageMock: vi.fn(),
  messageResults: [] as unknown[],
  refreshAssetUrlsMock: vi.fn(),
  // Reactive server records, so tests can change session state under a mounted card.
  registryState: {} as {
    servers: Map<string, Record<string, unknown>>;
    stores: Map<string, Record<string, unknown>>;
    connections: Map<string, { queryScope: string; getAPI: unknown }>;
  }
}));

function testImageUrl(label: string): string {
  return `data:image/svg+xml,${encodeURIComponent(
    `<svg xmlns="http://www.w3.org/2000/svg"><title>${label}</title></svg>`
  )}`;
}

function deferred<T>() {
  let resolve!: (value: T) => void;
  const promise = new Promise<T>((resolvePromise) => {
    resolve = resolvePromise;
  });
  return { promise, resolve };
}

// The store mock also carries the frontend UI state of its server.
vi.mock(
  '$lib/state/server/serverUi',
  async () => (await import('$lib/test-utils/serverUiMock')).serverUiIsStore
);

vi.mock('@chatto/client/api/roomTimeline', () => ({
  createRoomTimelineAPI: vi.fn(() => ({
    getMessage: getMessageMock
  }))
}));

vi.mock('@chatto/client/api/attachments', async (importActual) => ({
  ...(await importActual<typeof import('@chatto/client/api/attachments')>()),
  createAttachmentAPI: vi.fn(() => ({
    refreshAssetUrls: refreshAssetUrlsMock
  }))
}));

vi.mock('$lib/client', async () => {
  const original = (await import('$lib/test-utils/clientMock')).clientMockDefaults;
  const { SvelteMap } = await import('svelte/reactivity');
  registryState.servers = new SvelteMap([
    ['server_1', { id: 'server_1', url: window.location.origin, name: 'Test Server', token: null }]
  ]);
  registryState.stores = new SvelteMap();
  registryState.connections = new Map();
  return {
    ...original,
    serverRegistry: {
      tryGetStore: (id: string) => registryState.stores.get(id),
      getServer: (id: string) => registryState.servers.get(id),
      isOriginServer: (id: string) => id === 'server_1',
      get originServer() {
        return { id: 'server_1', url: window.location.origin, name: 'Test Server', token: null };
      },
      servers: [{ id: 'server_1', url: window.location.origin, name: 'Test Server', token: null }]
    },
    serverConnectionManager: {
      getClient: (id: string) => registryState.connections.get(id)
    }
  };
});

vi.mock('$lib/state/activeServer.svelte', () => ({
  getActiveServer: () => 'server_1'
}));

function link(): MessageLink {
  return {
    serverId: 'server_1',
    roomId: 'room_1',
    messageId: 'event_1'
  };
}

function previewEvent(event: unknown) {
  return {
    id: 'event_1',
    createdAt: '2027-05-29T15:00:00Z',
    actor: null,
    event
  };
}

function previewResult(thumbnailUrl: string) {
  return previewEvent({
    kind: TimelineEventKind.MessagePosted,
    body: null,
    attachments: [
      {
        id: 'att_1',
        filename: 'photo.jpg',
        contentType: 'image/jpeg',
        thumbnailAssetUrl: {
          url: thumbnailUrl,
          expiresAt: '2027-05-29T15:00:00Z'
        },
        videoProcessing: null
      }
    ]
  });
}

function bodyPreviewResult(body: string) {
  return previewEvent({
    kind: TimelineEventKind.MessagePosted,
    body,
    attachments: []
  });
}

function videoPreviewResult(videoThumbnailUrl: string | null) {
  return previewEvent({
    kind: TimelineEventKind.MessagePosted,
    body: null,
    attachments: [
      {
        id: 'att_video',
        filename: 'clip.mp4',
        contentType: 'video/mp4',
        thumbnailAssetUrl: null,
        videoProcessing: videoThumbnailUrl
          ? {
              thumbnailAssetUrl: {
                url: videoThumbnailUrl,
                expiresAt: '2027-05-29T15:00:00Z'
              }
            }
          : null
      }
    ]
  });
}

function refreshResult(thumbnailUrl: string) {
  return new Map<string, RefreshedAttachmentUrls>([
    [
      'att_1',
      {
        assetUrl: {
          url: '/assets/files/att_1?access=fresh-original',
          expiresAt: '2027-05-29T15:00:00Z'
        },
        thumbnailAssetUrl: {
          url: thumbnailUrl,
          expiresAt: '2027-05-29T15:00:00Z'
        },
        videoThumbnailAssetUrl: null,
        variantAssetUrls: new Map()
      }
    ]
  ]);
}

function videoRefreshResult(videoThumbnailUrl: string) {
  return new Map<string, RefreshedAttachmentUrls>([
    [
      'att_video',
      {
        assetUrl: {
          url: '/assets/files/att_video?access=fresh-original',
          expiresAt: '2027-05-29T15:00:00Z'
        },
        thumbnailAssetUrl: null,
        videoThumbnailAssetUrl: {
          url: videoThumbnailUrl,
          expiresAt: '2027-05-29T15:00:00Z'
        },
        variantAssetUrls: new Map()
      }
    ]
  ]);
}

function clearedRefreshResult(attachmentId: string) {
  return new Map<string, RefreshedAttachmentUrls>([
    [
      attachmentId,
      {
        assetUrl: null,
        thumbnailAssetUrl: null,
        videoThumbnailAssetUrl: null,
        variantAssetUrls: new Map()
      }
    ]
  ]);
}

function testConnection(queryScope: string) {
  return {
    queryScope,
    getAPI: (factory: (config: never) => unknown) => factory({} as never)
  };
}

function testStore() {
  return {
    currentUser: {
      user: { login: 'viewer' }
    },
    navigation: {
      rooms: [{ id: 'room_1', name: 'general' }]
    }
  };
}

beforeEach(() => {
  registryState.connections.set('server_1', testConnection('session-1'));
  registryState.stores.set('server_1', testStore());
  registryState.servers.set('server_1', {
    id: 'server_1',
    url: window.location.origin,
    name: 'Test Server',
    token: null
  });
  getMessageMock.mockReset();
  refreshAssetUrlsMock.mockReset();
  messageResults.length = 0;
  getMessageMock.mockImplementation(() => Promise.resolve(messageResults.shift()));
  refreshAssetUrlsMock.mockResolvedValue(new Map());
});

describe('MessagePreviewCard', () => {
  it('shows the linked message timestamp', async () => {
    messageResults.push(bodyPreviewResult('Dated message'));
    const { container } = render(MessagePreviewCard, { props: { link: link() } });
    await vi.waitFor(() => {
      const time = container.querySelector('time');
      expect(time?.dateTime).toBe('2027-05-29T15:00:00Z');
      expect(time?.textContent).toContain('2027');
    });
  });
  it('hides a message when access is denied', async () => {
    getMessageMock.mockRejectedValue(new ConnectError('Access denied', Code.PermissionDenied));
    const { container } = render(MessagePreviewCard, { props: { link: link() } });
    await vi.waitFor(() => expect(getMessageMock).toHaveBeenCalled());
    await tick();
    expect(container.querySelector('[data-testid="message-preview-card"]')).toBeNull();
  });

  it('shows attachments as one text line without thumbnails in compact mode', async () => {
    messageResults.push(previewResult(testImageUrl('compact')));
    const { container } = render(MessagePreviewCard, { props: { link: link(), compact: true } });

    await vi.waitFor(() =>
      expect(
        container.querySelector('[data-testid="message-preview-attachment-summary"]')?.textContent
      ).toContain('Image')
    );
    expect(container.querySelector('img[alt="photo.jpg"]')).toBeNull();
  });

  it('renders an attachment-only thread reply', async () => {
    messageResults.push(previewResult(testImageUrl('thread reply')));
    const { container } = render(MessagePreviewCard, {
      props: { link: { ...link(), threadRootEventId: 'thread_1' } }
    });
    await vi.waitFor(() => expect(container.textContent).toContain('Image'));
    expect(getMessageMock).toHaveBeenCalledWith(
      expect.objectContaining({
        roomId: 'room_1',
        eventId: 'event_1'
      })
    );
  });

  it.each([
    null,
    previewEvent({ kind: TimelineEventKind.MessagePosted, body: '', attachments: [] })
  ])('hides unavailable or empty messages', async (result) => {
    messageResults.push(result);
    const { container } = render(MessagePreviewCard, { props: { link: link() } });
    await vi.waitFor(() => expect(getMessageMock).toHaveBeenCalledOnce());
    await tick();
    expect(container.querySelector('[data-testid="message-preview-card"]')).toBeNull();
  });
  it('preserves the preview when an equivalent link object replaces the prop', async () => {
    messageResults.push(bodyPreviewResult('Stable preview'));
    const { container, rerender } = render(MessagePreviewCard, {
      props: { link: link(), showDismiss: false }
    });
    const card = await vi.waitFor(() => {
      const node = container.querySelector('[data-testid="message-preview-card"]');
      expect(node).not.toBeNull();
      return node!;
    });

    await rerender({ link: link() });

    expect(getMessageMock).toHaveBeenCalledTimes(1);
    expect(container.querySelector('[data-testid="message-preview-card"]')).toBe(card);
    expect(card.textContent).toContain('Stable preview');
  });

  it('keeps the rendered preview when server session state changes', async () => {
    messageResults.push(bodyPreviewResult('Resumed preview'));
    const { container } = render(MessagePreviewCard, {
      props: { link: link(), showDismiss: false }
    });
    const card = await vi.waitFor(() => {
      const node = container.querySelector('[data-testid="message-preview-card"]');
      expect(node).not.toBeNull();
      return node!;
    });

    // A reconnect after the app resumes updates the server's session record.
    registryState.servers.set('server_1', {
      ...registryState.servers.get('server_1'),
      token: 'renewed-token'
    });
    await tick();

    expect(getMessageMock).toHaveBeenCalledTimes(1);
    expect(container.querySelector('[data-testid="message-preview-card"]')).toBe(card);
    expect(card.textContent).toContain('Resumed preview');
  });

  it('reloads the preview when the server store is replaced', async () => {
    messageResults.push(bodyPreviewResult('First account preview'));
    const second = deferred<ReturnType<typeof bodyPreviewResult>>();
    getMessageMock
      .mockImplementationOnce(() => Promise.resolve(messageResults.shift()))
      .mockReturnValueOnce(second.promise);
    const { container } = render(MessagePreviewCard, {
      props: { link: link(), showDismiss: false }
    });
    await vi.waitFor(() => expect(container.textContent).toContain('First account preview'));

    // Sign-out and account changes replace the server's store and connection.
    registryState.connections.set('server_1', testConnection('session-2'));
    registryState.stores.set('server_1', testStore());
    await tick();

    expect(getMessageMock).toHaveBeenCalledTimes(2);
    expect(container.querySelector('[data-testid="message-preview-card"]')).toBeNull();
    second.resolve(bodyPreviewResult('Second account preview'));
    await vi.waitFor(() => expect(container.textContent).toContain('Second account preview'));
  });

  it('preserves an outstanding request when an equivalent link object arrives', async () => {
    const pending = deferred<ReturnType<typeof bodyPreviewResult>>();
    getMessageMock.mockReturnValueOnce(pending.promise);
    const { container, rerender } = render(MessagePreviewCard, {
      props: { link: link(), showDismiss: false }
    });
    await vi.waitFor(() => expect(getMessageMock).toHaveBeenCalledTimes(1));

    await rerender({ link: link() });
    expect(getMessageMock).toHaveBeenCalledTimes(1);
    pending.resolve(bodyPreviewResult('Pending preview'));

    await vi.waitFor(() => expect(container.textContent).toContain('Pending preview'));
  });

  it.each([
    ['server', { serverId: null }],
    ['room', { roomId: 'room_2' }],
    ['message', { messageId: 'event_2' }]
  ] as const)('clears the preview when the %s target changes', async (_field, change) => {
    messageResults.push(bodyPreviewResult('Previous preview'));
    const { container, rerender } = render(MessagePreviewCard, {
      props: { link: link(), showDismiss: false }
    });
    await vi.waitFor(() => expect(container.textContent).toContain('Previous preview'));
    getMessageMock.mockReturnValue(new Promise(() => {}));

    await rerender({ link: { ...link(), ...change } });

    expect(container.querySelector('[data-testid="message-preview-card"]')).toBeNull();
    expect(getMessageMock).toHaveBeenCalledTimes('serverId' in change ? 1 : 2);
  });

  it('keeps the preview when only the thread of the link changes', async () => {
    messageResults.push(bodyPreviewResult('Thread preview'));
    const { container, rerender } = render(MessagePreviewCard, {
      props: { link: link(), showDismiss: false }
    });
    const card = await vi.waitFor(() => {
      const node = container.querySelector('[data-testid="message-preview-card"]');
      expect(node).not.toBeNull();
      return node!;
    });

    await rerender({ link: { ...link(), threadRootEventId: 'thread_2' } });

    expect(getMessageMock).toHaveBeenCalledTimes(1);
    expect(container.querySelector('[data-testid="message-preview-card"]')).toBe(card);
  });

  it('reads with a cancellable request', async () => {
    messageResults.push(bodyPreviewResult('Bounded preview'));
    const { container } = render(MessagePreviewCard, {
      props: { link: link(), showDismiss: false }
    });
    await vi.waitFor(() => expect(container.textContent).toContain('Bounded preview'));

    expect(getMessageMock).toHaveBeenCalledWith(
      expect.objectContaining({
        roomId: 'room_1',
        eventId: 'event_1',
        signal: expect.any(AbortSignal)
      })
    );
  });

  it('ignores a late response after the target changes', async () => {
    const previous = deferred<ReturnType<typeof bodyPreviewResult>>();
    getMessageMock.mockReturnValueOnce(previous.promise);
    const { container, rerender } = render(MessagePreviewCard, {
      props: { link: link(), showDismiss: false }
    });
    await vi.waitFor(() => expect(getMessageMock).toHaveBeenCalledTimes(1));
    messageResults.push(bodyPreviewResult('Current preview'));

    await rerender({ link: { ...link(), roomId: 'room_2' } });
    await vi.waitFor(() => expect(container.textContent).toContain('Current preview'));
    previous.resolve(bodyPreviewResult('Obsolete preview'));
    await previous.promise;
    await rerender({ link: { ...link(), roomId: 'room_2' } });

    expect(container.textContent).toContain('Current preview');
    expect(container.textContent).not.toContain('Obsolete preview');
    expect(getMessageMock).toHaveBeenCalledTimes(2);
  });

  it('renders a deleted author as an italicized placeholder', async () => {
    messageResults.push(bodyPreviewResult('A deleted user message'));

    const { container } = render(MessagePreviewCard, {
      props: { link: link(), showDismiss: false }
    });

    await vi.waitFor(() => {
      expect(container.querySelector('[data-testid="message-preview-card"] em')?.textContent).toBe(
        '[deleted user]'
      );
    });
  });

  it('renders fitted linked message markdown without scroll fades', async () => {
    messageResults.push(
      bodyPreviewResult('# Release notes\n\n- **Breaking** change\n- More details')
    );

    const { container } = render(MessagePreviewCard, {
      props: { link: link(), showDismiss: false }
    });

    await vi.waitFor(() => {
      expect(container.querySelector('[data-testid="message-preview-card"] h1')?.textContent).toBe(
        'Release notes'
      );
    });

    expect(
      container.querySelector('[data-testid="message-preview-card"] strong')?.textContent
    ).toBe('Breaking');
    expect(container.querySelector('[data-testid="message-preview-card"] ul')).not.toBeNull();
    const scrollViewport = container.querySelector<HTMLElement>('.max-h-32.overflow-y-auto');
    expect(scrollViewport).not.toBeNull();
    expect(getComputedStyle(scrollViewport!).overscrollBehaviorY).toBe('auto');
    expect(
      [...container.querySelectorAll('[data-testid="message-preview-card"] bdi')]
        .map((value) => value.textContent)
        .filter((value) => value === 'Test Server' || value === 'general')
    ).toEqual(['general', 'Test Server']);
    const fades = container.querySelectorAll<HTMLElement>(
      '[aria-hidden="true"].pointer-events-none'
    );
    expect(fades).toHaveLength(2);
    expect(fades[0].className).toContain('from-background');
    expect(fades[0].className).toContain('z-30');
    expect(fades[1].className).toContain('bg-gradient-to-t');
    await vi.waitFor(() => {
      expect(scrollViewport!.scrollHeight).toBeLessThanOrEqual(scrollViewport!.clientHeight + 1);
      expect(fades[0].classList).toContain('opacity-0');
      expect(fades[1].classList).toContain('opacity-0');
    });
  });

  it('shows only the applicable fade at each edge of an overflowing linked message', async () => {
    messageResults.push(
      bodyPreviewResult(
        Array.from({ length: 30 }, (_, index) => `Paragraph ${index + 1}`).join('\n\n')
      )
    );

    const { container } = render(MessagePreviewCard, {
      props: { link: link(), showDismiss: false }
    });

    const scrollViewport = await vi.waitFor(() => {
      const viewport = container.querySelector<HTMLElement>('.max-h-32.overflow-y-auto');
      expect(viewport).not.toBeNull();
      expect(viewport!.scrollHeight).toBeGreaterThan(viewport!.clientHeight + 1);
      return viewport!;
    });
    const fades = container.querySelectorAll<HTMLElement>(
      '[aria-hidden="true"].pointer-events-none'
    );

    expect(getComputedStyle(scrollViewport).overscrollBehaviorY).toBe('auto');

    await vi.waitFor(() => {
      expect(fades[0].classList).toContain('opacity-0');
      expect(fades[1].classList).not.toContain('opacity-0');
    });

    scrollViewport.scrollTop = scrollViewport.scrollHeight;
    scrollViewport.dispatchEvent(new Event('scroll'));

    await vi.waitFor(() => {
      expect(fades[0].classList).not.toContain('opacity-0');
      expect(fades[1].classList).toContain('opacity-0');
    });
  });

  it('refreshes attachment thumbnail asset URLs after image load failure', async () => {
    messageResults.push(previewResult(testImageUrl('old-image')));
    refreshAssetUrlsMock.mockResolvedValueOnce(refreshResult(testImageUrl('fresh-image')));

    const { container } = render(MessagePreviewCard, {
      props: { link: link(), showDismiss: false }
    });

    await vi.waitFor(() => {
      expect(container.querySelector('[data-testid="message-preview-card"]')).not.toBeNull();
    });

    const img = container.querySelector<HTMLImageElement>('img[alt="photo.jpg"]');
    expect(img).not.toBeNull();
    img?.dispatchEvent(new Event('error'));

    await vi.waitFor(() => {
      const refreshed = container.querySelector<HTMLImageElement>('img[alt="photo.jpg"]');
      expect(refreshed?.getAttribute('src')).toContain('fresh-image');
    });
    expect(refreshAssetUrlsMock).toHaveBeenCalledWith('room_1', ['att_1'], {
      width: 120,
      height: 120,
      fit: ImageFitMode.COVER
    });
  });

  it('clears stale preview thumbnail asset URLs when refresh returns null', async () => {
    messageResults.push(previewResult(testImageUrl('old-image')));
    const refresh = deferred<Map<string, RefreshedAttachmentUrls>>();
    refreshAssetUrlsMock.mockReturnValueOnce(refresh.promise);

    const { container } = render(MessagePreviewCard, {
      props: { link: link(), showDismiss: false }
    });

    const img = await vi.waitFor(() => {
      const current = container.querySelector<HTMLImageElement>('img[alt="photo.jpg"]');
      expect(current).not.toBeNull();
      return current!;
    });
    img.dispatchEvent(new Event('error'));

    expect(refreshAssetUrlsMock).toHaveBeenCalled();
    expect(container.querySelector('img[alt="photo.jpg"]')).toBe(img);

    refresh.resolve(clearedRefreshResult('att_1'));

    await vi.waitFor(() => {
      expect(container.querySelector('img[alt="photo.jpg"]')).toBeNull();
    });
    expect(container.textContent).toContain('Image');
  });

  it('renders video attachment thumbnails for linked message previews', async () => {
    messageResults.push(videoPreviewResult(testImageUrl('old-video')));

    const { container } = render(MessagePreviewCard, {
      props: { link: link(), showDismiss: false }
    });

    await vi.waitFor(() => {
      expect(container.querySelector('[data-testid="message-preview-card"]')).not.toBeNull();
    });

    const img = container.querySelector<HTMLImageElement>('img[alt="clip.mp4"]');
    expect(img?.getAttribute('src')).toContain('old-video');
    expect(container.querySelector('[class~="icon-[uil--play]"]')).not.toBeNull();
  });

  it('refreshes video attachment thumbnail asset URLs after image load failure', async () => {
    messageResults.push(videoPreviewResult(testImageUrl('old-video')));
    refreshAssetUrlsMock.mockResolvedValueOnce(videoRefreshResult(testImageUrl('fresh-video')));

    const { container } = render(MessagePreviewCard, {
      props: { link: link(), showDismiss: false }
    });

    await vi.waitFor(() => {
      expect(container.querySelector('[data-testid="message-preview-card"]')).not.toBeNull();
    });

    const img = container.querySelector<HTMLImageElement>('img[alt="clip.mp4"]');
    expect(img).not.toBeNull();
    img?.dispatchEvent(new Event('error'));

    await vi.waitFor(() => {
      const refreshed = container.querySelector<HTMLImageElement>('img[alt="clip.mp4"]');
      expect(refreshed?.getAttribute('src')).toContain('fresh-video');
    });
  });

  it('clears stale preview video thumbnail asset URLs when refresh returns null', async () => {
    messageResults.push(videoPreviewResult(testImageUrl('old-video')));
    const refresh = deferred<Map<string, RefreshedAttachmentUrls>>();
    refreshAssetUrlsMock.mockReturnValueOnce(refresh.promise);

    const { container } = render(MessagePreviewCard, {
      props: { link: link(), showDismiss: false }
    });

    const img = await vi.waitFor(() => {
      const current = container.querySelector<HTMLImageElement>('img[alt="clip.mp4"]');
      expect(current).not.toBeNull();
      return current!;
    });
    img.dispatchEvent(new Event('error'));

    expect(refreshAssetUrlsMock).toHaveBeenCalled();
    expect(container.querySelector('img[alt="clip.mp4"]')).toBe(img);

    refresh.resolve(clearedRefreshResult('att_video'));

    await vi.waitFor(() => {
      expect(container.querySelector('img[alt="clip.mp4"]')).toBeNull();
    });
    expect(container.querySelector('[class~="icon-[uil--play]"]')).not.toBeNull();
  });

  it('falls back to a video tile when the refreshed video thumbnail also fails', async () => {
    messageResults.push(videoPreviewResult(testImageUrl('old-video')));
    refreshAssetUrlsMock.mockResolvedValueOnce(videoRefreshResult(testImageUrl('fresh-video')));

    const { container } = render(MessagePreviewCard, {
      props: { link: link(), showDismiss: false }
    });

    await vi.waitFor(() => {
      expect(container.querySelector('img[alt="clip.mp4"]')).not.toBeNull();
    });

    container
      .querySelector<HTMLImageElement>('img[alt="clip.mp4"]')
      ?.dispatchEvent(new Event('error'));

    await vi.waitFor(() => {
      expect(container.querySelector<HTMLImageElement>('img[alt="clip.mp4"]')?.src).toContain(
        'fresh-video'
      );
    });

    container
      .querySelector<HTMLImageElement>('img[alt="clip.mp4"]')
      ?.dispatchEvent(new Event('error'));

    await vi.waitFor(() => {
      expect(container.querySelector('img[alt="clip.mp4"]')).toBeNull();
    });
    expect(container.querySelector('[class~="icon-[uil--play]"]')).not.toBeNull();
    expect(container.textContent).toContain('Video');
  });
});
