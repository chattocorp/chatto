import { describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import MessageAttachments from './MessageAttachments.svelte';
import type { MessageAttachmentView } from '$lib/render/types';

vi.mock('$app/navigation', () => ({
  goto: vi.fn(),
  pushState: vi.fn(),
  replaceState: vi.fn()
}));

vi.mock('$lib/api/attachments', () => ({
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

function renderAttachment(attachment: MessageAttachmentView) {
  return render(MessageAttachments, {
    props: {
      attachments: [attachment],
      serverId: 'server_1',
      roomId: 'room_1',
      eventId: 'event_1'
    }
  });
}

function imageFrame(container: HTMLElement, filename: string) {
  const image = container.querySelector<HTMLImageElement>(`img[alt="${filename}"]`);
  expect(image).not.toBeNull();
  const button = image?.closest('button');
  expect(button).not.toBeNull();
  return { image: image!, button: button! };
}

describe('MessageAttachments', () => {
  it('renders extreme portrait images in a cropped portrait thumbnail frame', () => {
    const { container } = renderAttachment(
      imageAttachment({
        filename: 'portrait.jpg',
        width: 320,
        height: 1200
      })
    );

    const { image, button } = imageFrame(container, 'portrait.jpg');

    expect(button.getAttribute('style')).toContain('width: 320px');
    expect(button.getAttribute('style')).toContain('aspect-ratio: 320 / 400');
    expect(image.className).toContain('object-cover');
    expect(image.className).toContain('h-full');
    expect(image.className).toContain('w-full');
  });

  it('renders extreme landscape images in a cropped landscape thumbnail frame', () => {
    const { container } = renderAttachment(
      imageAttachment({
        filename: 'landscape.jpg',
        width: 1200,
        height: 320
      })
    );

    const { image, button } = imageFrame(container, 'landscape.jpg');

    expect(button.getAttribute('style')).toContain('width: 480px');
    expect(button.getAttribute('style')).toContain('aspect-ratio: 480 / 320');
    expect(image.className).toContain('object-cover');
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

    expect(button.getAttribute('style')).toContain('width: 480px');
    expect(button.getAttribute('style')).toContain('aspect-ratio: 480 / 270');
    expect(image.className).toContain('object-cover');
    expect(image.className).toContain('h-full');
    expect(image.className).toContain('w-full');
  });
});
