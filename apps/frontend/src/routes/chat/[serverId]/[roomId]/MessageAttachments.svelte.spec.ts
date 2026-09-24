import '../../../../app.css';
import { ImageFitMode } from '@chatto/api-types/api/v1/common_pb';
import { tick } from 'svelte';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import MessageAttachments from './MessageAttachments.svelte';
import { VideoProcessingStatus, type MessageAttachmentView } from '$lib/render/messageAttachments';
import type { RefreshedAttachmentUrls } from '$lib/attachments/attachmentUrls';

const attachmentMocks = vi.hoisted(() => ({
  pushState: vi.fn(),
  refreshAssetUrls: vi.fn(),
  videoPlayerModuleLoaded: vi.fn()
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

vi.mock('$lib/components/chat/VideoPlayer.svelte', async () => {
  attachmentMocks.videoPlayerModuleLoaded();
  return {
    default: (await import('./MessageAttachmentsVideoPlayerStub.svelte')).default
  };
});

vi.mock('$lib/state/server/scope.svelte', () => ({
  useServerScope: () => ({
    serverId: 'server_1',
    store: {},
    connection: {
      serverId: 'server_1',
      connectBaseUrl: 'https://chat.example.test/api/connect',
      bearerToken: null,
      getAPI: (factory: (config: never) => unknown) => factory({} as never)
    },
    isCurrent: () => true
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
  options: { canDeleteAttachment?: boolean; canEditAttachmentDescription?: boolean } = {}
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
  options: { canDeleteAttachment?: boolean; canEditAttachmentDescription?: boolean } = {}
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
    attachmentMocks.videoPlayerModuleLoaded.mockReset();
    attachmentMocks.refreshAssetUrls.mockResolvedValue(new Map());
  });

  it.each([
    'text/html',
    'TEXT/HTML; charset=UTF-8',
    'application/xhtml+xml',
    ' Application/XHTML+XML ; charset=utf-8'
  ])('opens %s in the HTML viewer without fetching the document', async (contentType) => {
    const attachment = fileAttachment({ filename: 'report.html', contentType });
    const view = renderAttachment(attachment);
    await view.getByRole('button', { name: 'View report.html' }).click();
    expect(attachmentMocks.pushState).toHaveBeenCalledWith('', {
      modal: {
        type: 'attachmentViewer',
        serverId: 'server_1',
        roomId: 'room_1',
        eventId: 'event_1',
        items: [
          expect.objectContaining({ id: attachment.id, filename: attachment.filename, contentType })
        ],
        index: 0
      }
    });
    expect(attachmentMocks.refreshAssetUrls).not.toHaveBeenCalled();
    expect(view.container.querySelector('iframe')).toBeNull();
  });

  it.each(['audio/mpeg', 'video/mp4'])(
    'pauses only the selected %s before opening its viewer',
    async (contentType) => {
      const attachment = fileAttachment({ filename: 'selected-media', contentType });
      const view = renderAttachments([
        attachment,
        fileAttachment({ id: 'other', filename: 'other.mp3', contentType: 'audio/mpeg' })
      ]);
      const [selected, other] = view.container.querySelectorAll<HTMLMediaElement>('audio, video');
      const pause = vi.spyOn(selected, 'pause');
      const otherPause = vi.spyOn(other, 'pause');
      await view.getByRole('button', { name: 'View selected-media', exact: true }).click();
      expect(pause).toHaveBeenCalledOnce();
      expect(otherPause).not.toHaveBeenCalled();
      expect(attachmentMocks.pushState).toHaveBeenCalledWith('', {
        modal: expect.objectContaining({
          type: 'attachmentViewer',
          index: 0,
          items: [expect.objectContaining({ id: attachment.id })]
        })
      });
    }
  );

  it.each([
    hlsVideoAttachment(),
    hlsVideoAttachment({ filename: 'loop.gif', contentType: 'image/gif' }),
    fileAttachment({ filename: 'raw.mp4', contentType: 'video/mp4' }),
    fileAttachment({ filename: 'voice.mp3', contentType: 'audio/mpeg' })
  ])(
    'opens $filename through an icon with an accessible filename and tooltip',
    async (attachment) => {
      const view = renderAttachment(attachment);
      const button = view.getByRole('button', { name: `View ${attachment.filename}`, exact: true });
      await expect.element(button).toHaveAttribute('title', `View ${attachment.filename}`);
      await expect.element(button).toHaveTextContent('');
      expect(view.container.querySelector('bdi')).toBeNull();
      await button.click();
      expect(attachmentMocks.pushState).toHaveBeenCalledWith('', {
        modal: expect.objectContaining({
          type: 'attachmentViewer',
          items: [expect.objectContaining({ id: attachment.id })]
        })
      });
    }
  );

  it.each([
    [false, false],
    [true, false],
    [false, true],
    [true, true]
  ])(
    'keeps audio actions compact and visible (delete: %s, edit: %s)',
    async (canDeleteAttachment, canEditAttachmentDescription) => {
      const { container } = renderAttachment(
        fileAttachment({ filename: 'voice.mp3', contentType: 'audio/mpeg' }),
        { canDeleteAttachment, canEditAttachmentDescription }
      );
      const buttons = [...container.querySelectorAll<HTMLButtonElement>('button')];
      expect(buttons.map((button) => button.getAttribute('aria-label'))).toEqual([
        ...(canDeleteAttachment ? ['Delete attachment'] : []),
        'View voice.mp3',
        ...(canEditAttachmentDescription ? ['Add description'] : [])
      ]);
      const audio = container.querySelector('audio')!;
      const frame = audio.closest<HTMLElement>('[data-attachment-media]')!;
      for (const width of [240, 120, 640]) {
        container.style.width = `${width}px`;
        await expect.poll(() => container.scrollWidth).toBeLessThanOrEqual(width);
        const frameBounds = frame.getBoundingClientRect();
        const audioBounds = audio.getBoundingClientRect();
        for (const [index, button] of buttons.entries()) {
          const bounds = button.getBoundingClientRect();
          expect(bounds.left >= audioBounds.right || bounds.top >= audioBounds.bottom).toBe(true);
          expect(bounds.left).toBeGreaterThanOrEqual(frameBounds.left);
          expect(bounds.right).toBeLessThanOrEqual(frameBounds.right);
          expect(bounds.bottom).toBeLessThanOrEqual(frameBounds.bottom);
          expect(getComputedStyle(button.parentElement!).opacity).toBe('1');
          if (width === 640) {
            expect(frameBounds.height).toBeLessThanOrEqual(70);
            if (index > 0) {
              expect(bounds.left - buttons[index - 1].getBoundingClientRect().right).toBe(4);
            }
          }
        }
      }
    }
  );

  it('uses equal padding around audio and file card contents', () => {
    const { container } = renderAttachments(
      [
        fileAttachment({ id: 'audio', filename: 'voice.mp3', contentType: 'audio/mpeg' }),
        fileAttachment({ id: 'file', filename: 'report.pdf' })
      ],
      { canDeleteAttachment: true, canEditAttachmentDescription: true }
    );
    container.style.width = '640px';
    const cards = [...container.querySelectorAll<HTMLElement>('.attachment-card')];
    expect(cards).toHaveLength(2);
    for (const card of cards) {
      const frame = card.getBoundingClientRect();
      const first = card.firstElementChild!.getBoundingClientRect();
      const last = card.lastElementChild!.getBoundingClientRect();
      const border = parseFloat(getComputedStyle(card).borderTopWidth);
      expect(first.left - frame.left - border).toBe(12);
      expect(frame.right - last.right - border).toBe(12);
      for (const child of [first, last]) {
        expect(child.top - frame.top - border).toBe(12);
        expect(frame.bottom - child.bottom - border).toBe(12);
      }
    }
    expect(cards[0].getBoundingClientRect().height).toBe(cards[1].getBoundingClientRect().height);
  });

  it.each(['text/plain', 'application/pdf', 'application/xml'])(
    'opens %s in the shared viewer',
    async (contentType) => {
      const view = renderAttachment(fileAttachment({ filename: 'report.html', contentType }));
      await view.getByRole('button', { name: 'View report.html' }).click();
      expect(attachmentMocks.pushState).toHaveBeenCalledWith('', {
        modal: expect.objectContaining({
          type: 'attachmentViewer',
          items: [expect.objectContaining({ contentType })],
          index: 0
        })
      });
    }
  );

  it('keeps the video player module out of non-video attachment rendering', async () => {
    renderAttachment(fileAttachment({}));

    await tick();
    await Promise.resolve();

    expect(attachmentMocks.videoPlayerModuleLoaded).not.toHaveBeenCalled();
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
    expect(button.getBoundingClientRect().height).toBeLessThanOrEqual(202);
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

  it('keeps a stable fog frame until an image without recorded dimensions loads', async () => {
    const { container } = renderAttachment(
      imageAttachment({ filename: 'unknown-size.jpg', width: 0, height: 0 })
    );
    const { image, button } = imageFrame(container, 'unknown-size.jpg');

    expect(button.style.width).toBe('320px');
    expect(button.style.aspectRatio).toBe('320 / 200');
    expect(button.getBoundingClientRect().height).toBeCloseTo(200, 0);
    expect(image.className).toContain('object-contain');
    expect(button.querySelector('[data-loading-fog]')).not.toBeNull();

    image.dispatchEvent(new Event('load'));
    await vi.waitFor(() => expect(button.querySelector('[data-loading-fog]')).toBeNull());
    expect(button.getBoundingClientRect().height).toBeCloseTo(200, 0);
  });

  it('scales a single image with the message width and preserves its proportions', async () => {
    const { container } = renderAttachment(
      imageAttachment({ filename: 'wide.jpg', width: 1600, height: 800 })
    );
    const { button } = imageFrame(container, 'wide.jpg');

    for (const width of [240, 120, 640]) {
      container.style.width = `${width}px`;
      await vi.waitFor(() => {
        const bounds = button.getBoundingClientRect();
        expect(bounds.width).toBe(Math.min(width, 400));
        expect(bounds.height).toBeCloseTo(bounds.width / 2, 0);
        expect(container.scrollWidth).toBe(container.clientWidth);
      });
    }
    expect(container.querySelector('[data-testid="message-image-gallery"]')).toBeNull();
  });

  it.each([true, false])(
    'keeps video attachments and long filenames inside the message (processed: %s)',
    async (processed) => {
      const attachment = hlsVideoAttachment();
      attachment.filename =
        'A very long video attachment filename that must not widen the message.mp4';
      if (!processed) {
        attachment.videoProcessing = null;
        attachment.assetUrl = {
          url: 'https://chat.example.test/clip.mp4',
          expiresAt: '2099-01-01T00:00:00Z'
        };
      }
      const { container } = renderAttachment(attachment);
      await expect
        .poll(() =>
          container.querySelector(
            processed ? '[data-testid="message-attachments-video-player"]' : 'video'
          )
        )
        .toBeTruthy();
      const player = container.querySelector<HTMLElement>(
        processed ? '[data-testid="message-attachments-video-player"]' : 'video'
      )!;
      const wrapper = player.parentElement!;
      for (const width of [240, 120, 640]) {
        container.style.width = `${width}px`;
        await expect.poll(() => wrapper.getBoundingClientRect().width).toBeLessThanOrEqual(width);
        expect(player.getBoundingClientRect().width).toBeLessThanOrEqual(width);
        expect(container.scrollWidth).toBeLessThanOrEqual(width);
      }
    }
  );

  it.each([true, false])(
    'keeps actions clear of playback controls on narrow videos (processed: %s)',
    async (processed) => {
      const attachment = processed
        ? hlsVideoAttachment()
        : fileAttachment({ filename: 'raw.mp4', contentType: 'video/mp4' });
      const { container } = renderAttachment(attachment, {
        canDeleteAttachment: true,
        canEditAttachmentDescription: true
      });
      const playerSelector = processed
        ? '[data-testid="message-attachments-video-player"]'
        : 'video';
      await expect.poll(() => container.querySelector(playerSelector)).toBeTruthy();
      const player = container.querySelector<HTMLElement>(playerSelector)!;
      const edit = container.querySelector<HTMLElement>('[aria-label="Add description"]')!;
      for (const width of [120, 240, 320]) {
        container.style.width = `${width}px`;
        await expect.poll(() => player.getBoundingClientRect().width).toBeLessThanOrEqual(width);
        expect(
          player.getBoundingClientRect().bottom - edit.getBoundingClientRect().bottom
        ).toBeGreaterThanOrEqual(48);
      }
    }
  );

  it('uses descriptions as image alt text and sends them to the image viewer', async () => {
    const description = 'A chart with a rising blue line.';
    const { container } = renderAttachment(imageAttachment({ description }));
    const image = container.querySelector<HTMLImageElement>(`img[alt="${description}"]`)!;

    expect(image).not.toBeNull();
    expect(container.querySelector('button[aria-label="Show description"]')).toBeNull();
    expect(image.closest('button')?.getAttribute('aria-describedby')).toBe(
      'attachment-description-event_1-att_1'
    );
    image.closest('button')!.click();

    await vi.waitFor(() => {
      expect(attachmentMocks.pushState).toHaveBeenCalledWith('', {
        modal: {
          type: 'attachmentViewer',
          serverId: 'server_1',
          roomId: 'room_1',
          eventId: 'event_1',
          items: [expect.objectContaining({ id: 'att_1', filename: 'image.jpg', description })],
          index: 0
        }
      });
    });
  });

  it('stacks delete before edit and uses a file-edit icon for descriptions', () => {
    const { container } = renderAttachment(imageAttachment({ description: 'A chart.' }), {
      canDeleteAttachment: true,
      canEditAttachmentDescription: true
    });
    const edit = container.querySelector<HTMLButtonElement>(
      'button[aria-label="Edit description"]'
    )!;
    const remove = container.querySelector<HTMLButtonElement>(
      'button[aria-label="Delete attachment"]'
    )!;

    expect([
      ...container.querySelectorAll(
        'button[aria-label="Delete attachment"], button[aria-label="Edit description"]'
      )
    ]).toEqual([remove, edit]);
    expect(remove.getBoundingClientRect().bottom).toBeLessThanOrEqual(
      edit.getBoundingClientRect().top
    );
    expect(edit.querySelector('span')?.classList.contains('icon-[uil--file-edit-alt]')).toBe(true);
  });

  it('associates file controls with descriptions and opens the edit dialog', () => {
    const description = 'Quarterly results in PDF format.';
    const { container } = renderAttachment(fileAttachment({ description }), {
      canEditAttachmentDescription: true
    });
    const download = container.querySelector<HTMLButtonElement>(
      'button[aria-label^="View document"]'
    )!;

    expect(download.getAttribute('aria-describedby')).toBe('attachment-description-event_1-file_1');
    expect(container.querySelector('button[aria-label="Show description"]')).toBeNull();
    const edit = container.querySelector<HTMLButtonElement>(
      'button[aria-label="Edit description"]'
    )!;
    edit.click();

    expect(attachmentMocks.pushState).toHaveBeenCalledWith('', {
      modal: {
        type: 'editAttachmentDescription',
        serverId: 'server_1',
        roomId: 'room_1',
        eventId: 'event_1',
        attachmentId: 'file_1',
        description
      }
    });
  });

  it('uses the same management controls for images and ordinary files', () => {
    const { container } = renderAttachments(
      [
        imageAttachment({
          filename: 'delete-me.jpg'
        }),
        fileAttachment({ filename: 'delete-me.pdf' })
      ],
      { canDeleteAttachment: true, canEditAttachmentDescription: true }
    );

    const deleteControls = container.querySelectorAll<HTMLElement>(
      '[aria-label="Delete attachment"]'
    );

    expect(deleteControls).toHaveLength(2);
    expect(deleteControls[0].tagName).toBe('BUTTON');
    expect(deleteControls[1].tagName).toBe('BUTTON');
    expect(deleteControls[1].getAttribute('title')).toBe('Delete attachment');
    expect(deleteControls[1].className).toBe(deleteControls[0].className);
    expect(getComputedStyle(deleteControls[1].parentElement!).opacity).toBe('1');
    const editControls = container.querySelectorAll<HTMLElement>('[aria-label="Add description"]');
    expect(editControls).toHaveLength(2);
    expect(editControls[1].className).toBe(editControls[0].className);
    expect(editControls[1].parentElement).toBe(deleteControls[1].parentElement);
    expect(deleteControls[1].getBoundingClientRect().right).toBeLessThanOrEqual(
      editControls[1].getBoundingClientRect().left
    );
    for (const control of deleteControls) {
      expect(control.querySelector('span')?.classList.contains('icon-[uil--trash-alt]')).toBe(true);
    }

    deleteControls[1].click();
    expect(attachmentMocks.pushState).toHaveBeenCalledWith('', {
      modal: {
        type: 'deleteAttachment',
        serverId: 'server_1',
        roomId: 'room_1',
        eventId: 'event_1',
        attachmentId: 'file_1'
      }
    });
  });

  it('keeps processed GIFs autolooping and processed videos using standard playback', async () => {
    const gif = hlsVideoAttachment({
      id: 'gif_1',
      filename: 'animated.gif',
      contentType: 'image/gif',
      videoProcessing: {
        ...hlsVideoAttachment().videoProcessing!,
        hlsMasterPlaylistUrl: null
      }
    });
    const { container } = renderAttachments([gif, hlsVideoAttachment()]);

    await vi.waitFor(() => {
      expect(
        container.querySelectorAll('[data-testid="message-attachments-video-player"]')
      ).toHaveLength(2);
    });
    const players = container.querySelectorAll<HTMLElement>(
      '[data-testid="message-attachments-video-player"]'
    );
    expect(Array.from(players, (player) => player.dataset.autoLoop)).toEqual(['true', 'false']);
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

  it('refreshes stale attachment URLs when mounted', async () => {
    renderAttachment(
      imageAttachment({
        assetUrl: {
          url: transparentGif,
          expiresAt: '2026-01-01T00:00:00Z'
        }
      })
    );

    await vi.waitFor(() => {
      expect(attachmentMocks.refreshAssetUrls).toHaveBeenCalledWith('room_1', ['att_1'], {
        width: 960,
        height: 400,
        fit: ImageFitMode.CONTAIN
      });
    });
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

    let player: HTMLButtonElement | null = null;
    await vi.waitFor(() => {
      player = container.querySelector<HTMLButtonElement>(
        '[data-testid="message-attachments-video-player"]'
      );
      expect(player).not.toBeNull();
    });

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

  it('opens the requested image and preserves the complete gallery for the viewer', async () => {
    const view = renderAttachments([
      imageAttachment({ id: 'first', filename: 'first.jpg' }),
      imageAttachment({ id: 'second', filename: 'second.jpg' }),
      fileAttachment({ id: 'pdf', filename: 'report.pdf' })
    ]);
    await view.getByRole('button', { name: 'View second.jpg' }).click();
    expect(attachmentMocks.pushState).toHaveBeenCalledWith('', {
      modal: {
        type: 'attachmentViewer',
        serverId: 'server_1',
        roomId: 'room_1',
        eventId: 'event_1',
        items: [
          expect.objectContaining({ id: 'first' }),
          expect.objectContaining({ id: 'second' })
        ],
        index: 1
      }
    });
    expect(attachmentMocks.refreshAssetUrls).not.toHaveBeenCalled();
  });

  it('updates gallery fades as its viewport scrolls and resizes', async () => {
    const { container } = renderAttachments([
      imageAttachment({ id: 'first', width: 1600, height: 900 }),
      imageAttachment({ id: 'second', width: 1600, height: 900 })
    ]);
    const gallery = container.querySelector<HTMLElement>('[data-testid="message-image-gallery"]')!;
    const fades = () =>
      ['left', 'right'].map(
        (edge) =>
          !container
            .querySelector(`[data-testid="message-image-gallery-${edge}-fade"]`)!
            .classList.contains('opacity-0')
      );
    gallery.style.width = '200px';
    await vi.waitFor(() => expect(fades()).toEqual([false, true]));
    gallery.scrollLeft = gallery.scrollWidth;
    await vi.waitFor(() => expect(fades()).toEqual([true, false]));
    gallery.style.width = '1000px';
    await vi.waitFor(() => expect(fades()).toEqual([false, false]));
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
    expect(gallery!.firstElementChild!.className).toContain('gap-3');
    expect(gallery!.firstElementChild!.className).toContain('p-1');
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
      'button[aria-label^="View document"]'
    );

    expect(gallery).not.toBeNull();
    expect(gallery!.querySelectorAll('button[aria-label^="View"]')).toHaveLength(2);
    expect(downloadButton).not.toBeNull();
    expect(
      gallery!.compareDocumentPosition(downloadButton!) & Node.DOCUMENT_POSITION_FOLLOWING
    ).toBeTruthy();
  });
});
