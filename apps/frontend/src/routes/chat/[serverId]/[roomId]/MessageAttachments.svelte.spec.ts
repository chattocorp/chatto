import { beforeEach, describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import MessageAttachments from './MessageAttachments.svelte';
import {
  FitMode,
  VideoProcessingStatus,
  type MessageAttachmentView
} from '$lib/render/types';
import type { RefreshedAttachmentUrls } from '$lib/attachments/attachmentUrls';

const attachmentMocks = vi.hoisted(() => ({
  pushState: vi.fn(),
  refreshAssetUrls: vi.fn()
}));

vi.mock('$app/navigation', () => ({
  goto: vi.fn(),
  pushState: attachmentMocks.pushState,
  replaceState: vi.fn()
}));

vi.mock('$lib/api-client/attachments', async (importActual) => ({
  ...(await importActual<typeof import('$lib/api-client/attachments')>()),
  createAttachmentAPI: vi.fn(() => ({
    refreshAssetUrls: attachmentMocks.refreshAssetUrls
  }))
}));

vi.mock('$lib/components/chat/VideoPlayer.svelte', async () => ({
  default: (await import('./MessageAttachmentsVideoPlayerStub.svelte')).default
}));

vi.mock('$lib/state/server/connection.svelte', () => ({
  useConnection: () => () => ({
    serverId: 'server_1',
    connectBaseUrl: 'https://chat.example.test/api/connect',
    bearerToken: null
  })
}));

const transparentGif = 'data:image/gif;base64,R0lGODlhAQABAIAAAAAAAP///ywAAAAAAQABAAACAUwAOw==';

function emptyRefreshedUrls(): RefreshedAttachmentUrls {
  return {
    assetUrl: null,
    thumbnailAssetUrl: null,
    videoThumbnailAssetUrl: null,
    variantAssetUrls: new Map()
  };
}

function imageAttachment(overrides: Partial<MessageAttachmentView>): MessageAttachmentView {
  return {
    id: 'att_1',
    filename: 'image.jpg',
    contentType: 'image/jpeg',
    width: 800,
    height: 600,
    assetUrl: {
      url: transparentGif,
      expiresAt: '2027-05-29T15:00:00Z'
    },
    thumbnailAssetUrl: {
      url: `${transparentGif}#thumb`,
      expiresAt: '2027-05-29T15:00:00Z'
    },
    videoProcessing: null,
    ...overrides
  };
}

function fileAttachment(overrides: Partial<MessageAttachmentView>): MessageAttachmentView {
  return {
    id: 'file_1',
    filename: 'document.pdf',
    contentType: 'application/pdf',
    width: 0,
    height: 0,
    assetUrl: {
      url: 'https://chat.example.test/document.pdf',
      expiresAt: '2027-05-29T15:00:00Z'
    },
    thumbnailAssetUrl: null,
    videoProcessing: null,
    ...overrides
  };
}

function hlsVideoAttachment(overrides: Partial<MessageAttachmentView> = {}): MessageAttachmentView {
  return {
    id: 'video_1',
    filename: 'clip.mp4',
    contentType: 'video/mp4',
    width: 1280,
    height: 720,
    assetUrl: null,
    thumbnailAssetUrl: null,
    videoProcessing: {
      status: VideoProcessingStatus.Completed,
      durationMs: 12_000,
      width: 1280,
      height: 720,
      thumbnailAssetUrl: null,
      sourceAvailable: true,
      variants: [],
      hlsMasterPlaylistUrl: {
        url: 'https://chat.example.test/assets/hls/video_1/master.m3u8?access=expired',
        expiresAt: '2099-01-01T00:00:00Z'
      },
      reasonCode: null
    },
    ...overrides
  };
}

function renderAttachments(
  attachments: MessageAttachmentView[],
  options: { canDeleteAttachment?: boolean } = {}
) {
  return render(MessageAttachments, {
    props: {
      attachments,
      serverId: 'server_1',
      roomId: 'room_1',
      eventId: 'event_1',
      ...options
    }
  });
}

function renderAttachment(
  attachment: MessageAttachmentView,
  options: { canDeleteAttachment?: boolean } = {}
) {
  return renderAttachments([attachment], options);
}

function imageFrame(container: HTMLElement, filename: string) {
  const image = container.querySelector<HTMLImageElement>(`img[alt="${filename}"]`);
  expect(image).not.toBeNull();
  const button = image?.closest('button');
  expect(button).not.toBeNull();
  return { image: image!, button: button! };
}

describe('MessageAttachments', () => {
  beforeEach(() => {
    attachmentMocks.pushState.mockReset();
    attachmentMocks.refreshAssetUrls.mockReset();
    attachmentMocks.refreshAssetUrls.mockResolvedValue(new Map());
  });

  it('renders very tall portrait images as contained narrow strips', () => {
    const { container } = renderAttachment(
      imageAttachment({
        filename: 'tall.jpg',
        width: 320,
        height: 1600
      })
    );

    const { image, button } = imageFrame(container, 'tall.jpg');

    expect(button.getAttribute('style')).toContain('width: 40px');
    expect(button.getAttribute('style')).toContain('aspect-ratio: 40 / 200');
    expect(image.className).toContain('object-contain');
    expect(image.className).not.toContain('object-cover');
    expect(image.className).toContain('h-full');
    expect(image.className).toContain('w-full');
  });

  it('renders ultra-wide landscape images as contained shallow strips', () => {
    const { container } = renderAttachment(
      imageAttachment({
        filename: 'ultra-wide.jpg',
        width: 2000,
        height: 100
      })
    );

    const { image, button } = imageFrame(container, 'ultra-wide.jpg');

    expect(button.getAttribute('style')).toContain('width: 480px');
    expect(button.getAttribute('style')).toContain('aspect-ratio: 480 / 24');
    expect(image.className).toContain('object-contain');
    expect(image.className).not.toContain('object-cover');
    expect(image.className).toContain('h-full');
    expect(image.className).toContain('w-full');
  });

  it('keeps ordinary images proportionally sized', () => {
    const { container } = renderAttachment(
      imageAttachment({
        filename: 'ordinary.jpg',
        width: 1600,
        height: 900
      })
    );

    const { image, button } = imageFrame(container, 'ordinary.jpg');

    expect(button.getAttribute('style')).toContain('width: 356px');
    expect(button.getAttribute('style')).toContain('aspect-ratio: 356 / 200');
    expect(image.className).toContain('object-cover');
    expect(image.className).toContain('h-full');
    expect(image.className).toContain('w-full');
  });

  it('uses a subtle attachment remove control when deletion is allowed', () => {
    const { container } = renderAttachment(
      imageAttachment({
        filename: 'delete-me.jpg'
      }),
      { canDeleteAttachment: true }
    );

    const deleteControl = container.querySelector<HTMLElement>('[aria-label="Delete attachment"]');

    expect(deleteControl).not.toBeNull();
    expect(deleteControl!.getAttribute('title')).toBe('Delete attachment');
    expect(deleteControl!.className).toContain('attachment-remove-button');
    expect(deleteControl!.className).not.toContain('embed-control-button');
  });

  it('does not render empty media URLs for attachments that are missing asset URLs', () => {
    const { container } = renderAttachment(
      imageAttachment({
        filename: 'pending.jpg',
        assetUrl: null,
        thumbnailAssetUrl: null
      })
    );

    expect(container.querySelector('img[src=""]')).toBeNull();
    expect(container.querySelector('video[src=""]')).toBeNull();
    expect(container.querySelector('audio[src=""]')).toBeNull();
    expect(container.querySelector('img[alt="pending.jpg"]')).toBeNull();
  });

  it('retries HLS URL recovery after an earlier refresh request fails', async () => {
    attachmentMocks.refreshAssetUrls
      .mockRejectedValueOnce(new Error('network failed'))
      .mockResolvedValueOnce(
        new Map([
          [
            'video_1',
            {
              ...emptyRefreshedUrls(),
              hlsMasterPlaylistUrl: {
                url: 'https://chat.example.test/assets/hls/video_1/master.m3u8?access=fresh',
                expiresAt: '2099-01-02T00:00:00Z'
              }
            }
          ]
        ])
    );
    const { container } = renderAttachment(hlsVideoAttachment());

    const player = container.querySelector<HTMLButtonElement>(
      '[data-testid="message-attachments-video-player"]'
    );
    expect(player).not.toBeNull();

    player!.click();
    await vi.waitFor(() => expect(attachmentMocks.refreshAssetUrls).toHaveBeenCalledTimes(1));
    await new Promise((resolve) => window.setTimeout(resolve, 0));

    player!.click();
    await vi.waitFor(() => expect(attachmentMocks.refreshAssetUrls).toHaveBeenCalledTimes(2));
    await expect.poll(() => player!.dataset.hlsUrl?.includes('access=fresh') ?? false).toBe(true);
  });

  it('clears stale image URLs when refresh returns null asset URLs', async () => {
    attachmentMocks.refreshAssetUrls.mockResolvedValue(new Map([['att_1', emptyRefreshedUrls()]]));
    const { container } = renderAttachment(
      imageAttachment({
        filename: 'expired.jpg',
        thumbnailAssetUrl: null
      })
    );

    const image = container.querySelector<HTMLImageElement>('img[alt="expired.jpg"]');
    expect(image).not.toBeNull();
    image!.dispatchEvent(new Event('error'));

    await vi.waitFor(() => {
      expect(container.querySelector('img[alt="expired.jpg"]')).toBeNull();
    });
  });

  it('does not open a different gallery image when the clicked image URL is cleared', async () => {
    attachmentMocks.refreshAssetUrls.mockResolvedValue(
      new Map([['cleared', emptyRefreshedUrls()]])
    );
    const { container } = renderAttachments([
      imageAttachment({
        id: 'cleared',
        filename: 'cleared.jpg'
      }),
      imageAttachment({
        id: 'kept',
        filename: 'kept.jpg'
      })
    ]);

    const { button } = imageFrame(container, 'cleared.jpg');
    button.click();

    await vi.waitFor(() => {
      expect(attachmentMocks.refreshAssetUrls).toHaveBeenCalled();
    });
    await vi.waitFor(() => {
      expect(container.querySelector('img[alt="cleared.jpg"]')).toBeNull();
    });
    expect(attachmentMocks.pushState).not.toHaveBeenCalled();
  });

  it('opens the lightbox with a compressed display URL and a separate original URL', async () => {
    attachmentMocks.refreshAssetUrls.mockResolvedValue(
      new Map([
        [
          'att_1',
          {
            assetUrl: {
              url: 'https://cdn.example.test/original.jpg',
              expiresAt: '2027-05-29T15:00:00Z'
            },
            thumbnailAssetUrl: {
              url: 'https://cdn.example.test/lightbox.jpg',
              expiresAt: '2027-05-29T15:00:00Z'
            },
            videoThumbnailAssetUrl: null,
            variantAssetUrls: new Map()
          }
        ]
      ])
    );
    const { container } = renderAttachment(imageAttachment({ filename: 'large.jpg' }));

    imageFrame(container, 'large.jpg').button.click();

    await vi.waitFor(() => {
      expect(attachmentMocks.refreshAssetUrls).toHaveBeenCalledWith('room_1', ['att_1'], {
        width: 2048,
        height: 2048,
        fit: FitMode.Contain
      });
      expect(attachmentMocks.pushState).toHaveBeenCalledWith('', {
        modal: {
          type: 'imageViewer',
          roomId: 'room_1',
          eventId: 'event_1',
          imageItems: [
            {
              id: 'att_1',
              src: 'https://cdn.example.test/lightbox.jpg',
              originalSrc: 'https://cdn.example.test/original.jpg',
              alt: 'large.jpg',
              filename: 'large.jpg'
            }
          ],
          imageIndex: 0
        }
      });
    });
  });

  it('renders multiple images inside a horizontal gallery with equal-height frames', () => {
    const { container } = renderAttachments([
      imageAttachment({
        id: 'wide',
        filename: 'wide.jpg',
        width: 1600,
        height: 900
      }),
      imageAttachment({
        id: 'tall',
        filename: 'tall.jpg',
        width: 320,
        height: 1600
      })
    ]);

    const gallery = container.querySelector<HTMLElement>('[data-testid="message-image-gallery"]');
    expect(gallery).not.toBeNull();
    expect(gallery!.className).toContain('overflow-x-auto');
    expect(gallery!.className).toContain('overscroll-x-contain');
    expect(gallery!.className).toContain('gap-3');
    expect(gallery!.className).toContain('p-1');
    expect(gallery!.parentElement?.className).toContain('w-full');
    expect(gallery!.parentElement?.getAttribute('style')).toBeNull();
    expect(
      container.querySelector('[data-testid="message-image-gallery-left-fade"]')
    ).not.toBeNull();
    expect(
      container.querySelector('[data-testid="message-image-gallery-right-fade"]')
    ).not.toBeNull();

    const buttons = Array.from(gallery!.querySelectorAll<HTMLButtonElement>('button'));
    expect(buttons).toHaveLength(2);
    expect(buttons.map((button) => button.style.height)).toEqual(['180px', '180px']);
    expect(buttons.every((button) => Number.parseFloat(button.style.width) <= 320)).toBe(true);
  });

  it('fills moderately wide gallery image frames', () => {
    const { container } = renderAttachments([
      imageAttachment({
        id: 'moderately-wide',
        filename: 'moderately-wide.jpg',
        width: 1200,
        height: 600
      }),
      imageAttachment({
        id: 'ordinary',
        filename: 'ordinary.jpg',
        width: 800,
        height: 600
      })
    ]);

    const { image, button } = imageFrame(container, 'moderately-wide.jpg');

    expect(button.closest('[data-testid="message-image-gallery"]')).not.toBeNull();
    expect(button.getAttribute('style')).toContain('width: 320px');
    expect(button.getAttribute('style')).toContain('height: 180px');
    expect(image.className).toContain('object-cover');
    expect(image.className).not.toContain('object-contain');
  });

  it('fills moderately tall gallery image frames', () => {
    const { container } = renderAttachments([
      imageAttachment({
        id: 'moderately-tall',
        filename: 'moderately-tall.jpg',
        width: 400,
        height: 1000
      }),
      imageAttachment({
        id: 'ordinary',
        filename: 'ordinary.jpg',
        width: 800,
        height: 600
      })
    ]);

    const { image, button } = imageFrame(container, 'moderately-tall.jpg');

    expect(button.closest('[data-testid="message-image-gallery"]')).not.toBeNull();
    expect(button.getAttribute('style')).toContain('width: 72px');
    expect(button.getAttribute('style')).toContain('height: 180px');
    expect(image.className).toContain('object-cover');
    expect(image.className).not.toContain('object-contain');
  });

  it('contains ultra-wide gallery images instead of creating shallow thumbnails', () => {
    const { container } = renderAttachments([
      imageAttachment({
        id: 'ultra-wide',
        filename: 'ultra-wide.jpg',
        width: 2000,
        height: 100
      }),
      imageAttachment({
        id: 'ordinary',
        filename: 'ordinary.jpg',
        width: 1600,
        height: 900
      })
    ]);

    const { image, button } = imageFrame(container, 'ultra-wide.jpg');

    expect(button.closest('[data-testid="message-image-gallery"]')).not.toBeNull();
    expect(button.getAttribute('style')).toContain('width: 320px');
    expect(button.getAttribute('style')).toContain('height: 180px');
    expect(image.className).toContain('object-contain');
    expect(image.className).not.toContain('object-cover');
  });

  it('contains ultra-tall gallery images instead of cropping them', () => {
    const { container } = renderAttachments([
      imageAttachment({
        id: 'ultra-tall',
        filename: 'ultra-tall.jpg',
        width: 320,
        height: 1600
      }),
      imageAttachment({
        id: 'ordinary',
        filename: 'ordinary.jpg',
        width: 1600,
        height: 900
      })
    ]);

    const { image, button } = imageFrame(container, 'ultra-tall.jpg');

    expect(button.closest('[data-testid="message-image-gallery"]')).not.toBeNull();
    expect(button.getAttribute('style')).toContain('width: 72px');
    expect(button.getAttribute('style')).toContain('height: 180px');
    expect(image.className).toContain('object-contain');
    expect(image.className).not.toContain('object-cover');
  });

  it('renders image galleries before non-image attachments in mixed messages', () => {
    const { container } = renderAttachments([
      imageAttachment({
        id: 'first-image',
        filename: 'first.jpg'
      }),
      fileAttachment({
        id: 'document',
        filename: 'document.pdf'
      }),
      imageAttachment({
        id: 'second-image',
        filename: 'second.jpg'
      })
    ]);

    const gallery = container.querySelector<HTMLElement>('[data-testid="message-image-gallery"]');
    const downloadButton = container.querySelector<HTMLButtonElement>(
      'button[aria-label^="Download"]'
    );

    expect(gallery).not.toBeNull();
    expect(gallery!.querySelectorAll('button[aria-label^="View"]')).toHaveLength(2);
    expect(downloadButton).not.toBeNull();
    expect(
      gallery!.compareDocumentPosition(downloadButton!) & Node.DOCUMENT_POSITION_FOLLOWING
    ).toBeTruthy();
  });
});
