import { ImageFitMode } from '@chatto/api-types/api/v1/common_pb';
import '../../app.css';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import MessagePreviewCard from './MessagePreviewCard.svelte';
import type { MessageLink } from '$lib/messageLinks';

import { TimelineEventKind } from '$lib/render/timelineEvents';
import type { RefreshedAttachmentUrls } from '$lib/attachments/attachmentUrls';

const { getRoomEventsAroundMock, timelineResults, refreshAssetUrlsMock } = vi.hoisted(() => ({
  getRoomEventsAroundMock: vi.fn(),
  timelineResults: [] as unknown[],
  refreshAssetUrlsMock: vi.fn()
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

vi.mock('$lib/api-client/roomTimeline', () => ({
  createRoomTimelineAPI: vi.fn(() => ({
    getRoomEventsAround: getRoomEventsAroundMock
  }))
}));

vi.mock('$lib/api-client/attachments', async (importActual) => ({
  ...(await importActual<typeof import('$lib/api-client/attachments')>()),
  createAttachmentAPI: vi.fn(() => ({
    refreshAssetUrls: refreshAssetUrlsMock
  }))
}));

vi.mock('$lib/state/server/registry.svelte', () => ({
  serverRegistry: {
    tryGetStore: () => ({
      currentUser: {
        user: { login: 'viewer' }
      },
      navigation: {
        rooms: [{ id: 'room_1', name: 'general' }]
      }
    }),
    getServer: (id: string) =>
      id === 'server_1'
        ? { id: 'server_1', url: window.location.origin, name: 'Test Server', token: null }
        : undefined,
    isOriginServer: (id: string) => id === 'server_1',
    get originServer() {
      return { id: 'server_1', url: window.location.origin, name: 'Test Server', token: null };
    },
    servers: [{ id: 'server_1', url: window.location.origin, name: 'Test Server', token: null }]
  }
}));

vi.mock('$lib/state/activeServer.svelte', () => ({
  getActiveServer: () => 'server_1'
}));

vi.mock('$lib/state/server/serverConnection.svelte', () => ({
  serverConnectionManager: {
    getClient: () => ({
      getAPI: (factory: (config: never) => unknown) => factory({} as never)
    })
  }
}));

function link(): MessageLink {
  return {
    serverSegment: '-',
    serverId: 'server_1',
    roomId: 'room_1',
    messageId: 'event_1'
  };
}

function previewPage(event: unknown) {
  return {
    events: [
      {
        id: 'event_1',
        createdAt: '2027-05-29T15:00:00Z',
        actor: null,
        event
      }
    ],
    startCursor: null,
    endCursor: null,
    hasOlder: false,
    hasNewer: false
  };
}

function previewResult(thumbnailUrl: string) {
  return previewPage({
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
  return previewPage({
    kind: TimelineEventKind.MessagePosted,
    body,
    attachments: []
  });
}

function videoPreviewResult(videoThumbnailUrl: string | null) {
  return previewPage({
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

beforeEach(() => {
  getRoomEventsAroundMock.mockReset();
  refreshAssetUrlsMock.mockReset();
  timelineResults.length = 0;
  getRoomEventsAroundMock.mockImplementation(() => Promise.resolve(timelineResults.shift()));
  refreshAssetUrlsMock.mockResolvedValue(new Map());
});

describe('MessagePreviewCard', () => {
  it('renders a deleted author as an italicized placeholder', async () => {
    timelineResults.push(bodyPreviewResult('A deleted user message'));

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
    timelineResults.push(
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
    const scrollViewport = container.querySelector<HTMLElement>('.max-h-52.overflow-y-auto');
    expect(scrollViewport).not.toBeNull();
    expect(
      [...container.querySelectorAll('[data-testid="message-preview-card"] bdi')]
        .map((value) => value.textContent)
        .filter((value) => value === 'Test Server' || value === '#general')
    ).toEqual(['Test Server', '#general']);
    const fades = container.querySelectorAll<HTMLElement>('[aria-hidden="true"]');
    expect(fades).toHaveLength(2);
    expect(fades[0].className).toContain('from-surface');
    expect(fades[0].className).toContain('z-30');
    expect(fades[1].className).toContain('bg-gradient-to-t');
    await vi.waitFor(() => {
      expect(scrollViewport!.scrollHeight).toBeLessThanOrEqual(scrollViewport!.clientHeight + 1);
      expect(fades[0].classList).toContain('opacity-0');
      expect(fades[1].classList).toContain('opacity-0');
    });
  });

  it('shows only the applicable fade at each edge of an overflowing linked message', async () => {
    timelineResults.push(
      bodyPreviewResult(
        Array.from({ length: 30 }, (_, index) => `Paragraph ${index + 1}`).join('\n\n')
      )
    );

    const { container } = render(MessagePreviewCard, {
      props: { link: link(), showDismiss: false }
    });

    const scrollViewport = await vi.waitFor(() => {
      const viewport = container.querySelector<HTMLElement>('.max-h-52.overflow-y-auto');
      expect(viewport).not.toBeNull();
      expect(viewport!.scrollHeight).toBeGreaterThan(viewport!.clientHeight + 1);
      return viewport!;
    });
    const fades = container.querySelectorAll<HTMLElement>('[aria-hidden="true"]');

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
    timelineResults.push(previewResult(testImageUrl('old-image')));
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
    timelineResults.push(previewResult(testImageUrl('old-image')));
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
    timelineResults.push(videoPreviewResult(testImageUrl('old-video')));

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
    timelineResults.push(videoPreviewResult(testImageUrl('old-video')));
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
    timelineResults.push(videoPreviewResult(testImageUrl('old-video')));
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
    timelineResults.push(videoPreviewResult(testImageUrl('old-video')));
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
