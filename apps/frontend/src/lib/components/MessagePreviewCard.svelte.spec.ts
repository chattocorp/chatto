import { beforeEach, describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import MessagePreviewCard from './MessagePreviewCard.svelte';
import type { MessageLink } from '$lib/messageLinks';
import { FitMode } from '$lib/render/types';
import { RoomEventKind } from '$lib/render/eventKinds';
import type { RefreshedAttachmentUrls } from '$lib/attachments/attachmentUrls';

const { getRoomEventsAroundMock, timelineResults, refreshAssetUrlsMock } = vi.hoisted(
  () => ({
    getRoomEventsAroundMock: vi.fn(),
    timelineResults: [] as unknown[],
    refreshAssetUrlsMock: vi.fn()
  })
);

const transparentGif = 'data:image/gif;base64,R0lGODlhAQABAIAAAAAAAP///ywAAAAAAQABAAACAUwAOw==';

vi.mock('$lib/api-client/roomTimeline', () => ({
  createRoomTimelineAPI: vi.fn(() => ({
    getRoomEventsAround: getRoomEventsAroundMock
  }))
}));

vi.mock('$lib/api-client/attachments', () => ({
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
      rooms: {
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
    kind: RoomEventKind.MessagePosted,
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
    kind: RoomEventKind.MessagePosted,
    body,
    attachments: []
  });
}

function videoPreviewResult(videoThumbnailUrl: string | null) {
  return previewPage({
    kind: RoomEventKind.MessagePosted,
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
  it('renders the linked message body as markdown in a scrollable preview', async () => {
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
    expect(container.querySelector('.max-h-52.overflow-y-auto')).not.toBeNull();
    expect(container.querySelector('.bg-gradient-to-b')).not.toBeNull();
    expect(container.querySelector('.bg-gradient-to-t')).not.toBeNull();
  });

  it('refreshes attachment thumbnail asset URLs after image load failure', async () => {
    timelineResults.push(previewResult(transparentGif));
    refreshAssetUrlsMock.mockResolvedValueOnce(
      refreshResult(`${transparentGif}#fresh-image`)
    );

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
      expect(refreshed?.getAttribute('src')).toContain('#fresh-image');
    });
    expect(refreshAssetUrlsMock).toHaveBeenCalledWith('room_1', ['att_1'], {
      width: 120,
      height: 120,
      fit: FitMode.Cover
    });
  });

  it('clears stale preview thumbnail asset URLs when refresh returns null', async () => {
    timelineResults.push(previewResult(`${transparentGif}#old-image`));
    refreshAssetUrlsMock.mockResolvedValueOnce(clearedRefreshResult('att_1'));

    const { container } = render(MessagePreviewCard, {
      props: { link: link(), showDismiss: false }
    });

    await vi.waitFor(() => {
      expect(refreshAssetUrlsMock).toHaveBeenCalled();
    });

    await vi.waitFor(() => {
      expect(container.querySelector('img[alt="photo.jpg"]')).toBeNull();
    });
    expect(container.textContent).toContain('Image');
  });

  it('renders video attachment thumbnails for linked message previews', async () => {
    timelineResults.push(videoPreviewResult(`${transparentGif}#old-video`));

    const { container } = render(MessagePreviewCard, {
      props: { link: link(), showDismiss: false }
    });

    await vi.waitFor(() => {
      expect(container.querySelector('[data-testid="message-preview-card"]')).not.toBeNull();
    });

    const img = container.querySelector<HTMLImageElement>('img[alt="clip.mp4"]');
    expect(img?.getAttribute('src')).toContain('#old-video');
    expect(container.querySelector('.uil--play')).not.toBeNull();
  });

  it('refreshes video attachment thumbnail asset URLs after image load failure', async () => {
    timelineResults.push(videoPreviewResult(`${transparentGif}#old-video`));
    refreshAssetUrlsMock.mockResolvedValueOnce(
      videoRefreshResult(`${transparentGif}#fresh-video`)
    );

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
      expect(refreshed?.getAttribute('src')).toContain('#fresh-video');
    });
  });

  it('clears stale preview video thumbnail asset URLs when refresh returns null', async () => {
    timelineResults.push(videoPreviewResult(`${transparentGif}#old-video`));
    refreshAssetUrlsMock.mockResolvedValueOnce(clearedRefreshResult('att_video'));

    const { container } = render(MessagePreviewCard, {
      props: { link: link(), showDismiss: false }
    });

    await vi.waitFor(() => {
      expect(refreshAssetUrlsMock).toHaveBeenCalled();
    });

    await vi.waitFor(() => {
      expect(container.querySelector('img[alt="clip.mp4"]')).toBeNull();
    });
    expect(container.querySelector('.uil--play')).not.toBeNull();
  });

  it('falls back to a video tile when the refreshed video thumbnail also fails', async () => {
    timelineResults.push(videoPreviewResult(`${transparentGif}#old-video`));
    refreshAssetUrlsMock.mockResolvedValueOnce(
      videoRefreshResult(`${transparentGif}#fresh-video`)
    );

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
        '#fresh-video'
      );
    });

    container
      .querySelector<HTMLImageElement>('img[alt="clip.mp4"]')
      ?.dispatchEvent(new Event('error'));

    await vi.waitFor(() => {
      expect(container.querySelector('img[alt="clip.mp4"]')).toBeNull();
    });
    expect(container.querySelector('.uil--play')).not.toBeNull();
    expect(container.textContent).toContain('Video');
  });
});
