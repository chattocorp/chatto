import { describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import MessageAttachments from './MessageAttachments.svelte';
import type { MessageAttachmentView } from '$lib/render/types';

vi.mock('$app/navigation', () => ({
  goto: vi.fn(),
  pushState: vi.fn(),
  replaceState: vi.fn()
}));

vi.mock('@chatto/api-client/attachments', () => ({
  createAttachmentAPI: vi.fn(() => ({
    refreshMessageAttachmentUrls: vi.fn().mockResolvedValue(new Map())
  }))
}));

vi.mock('$lib/state/server/connection.svelte', () => ({
  useConnection: () => () => ({
    serverId: 'server_1',
    connectBaseUrl: 'https://chat.example.test/api/connect',
    bearerToken: null
  })
}));

const transparentGif = 'data:image/gif;base64,R0lGODlhAQABAIAAAAAAAP///ywAAAAAAQABAAACAUwAOw==';

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

function renderAttachments(attachments: MessageAttachmentView[]) {
  return render(MessageAttachments, {
    props: {
      attachments,
      serverId: 'server_1',
      roomId: 'room_1',
      eventId: 'event_1'
    }
  });
}

function renderAttachment(attachment: MessageAttachmentView) {
  return renderAttachments([attachment]);
}

function imageFrame(container: HTMLElement, filename: string) {
  const image = container.querySelector<HTMLImageElement>(`img[alt="${filename}"]`);
  expect(image).not.toBeNull();
  const button = image?.closest('button');
  expect(button).not.toBeNull();
  return { image: image!, button: button! };
}

describe('MessageAttachments', () => {
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
