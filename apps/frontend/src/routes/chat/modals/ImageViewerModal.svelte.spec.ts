import { afterEach, describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import type { ImageViewerModalState } from '$lib/modal';

const mocks = vi.hoisted(() => ({
  refreshUrls: vi.fn(async () => new Map()),
  getAPI: vi.fn(() => ({})),
  replaceState: vi.fn(),
  page: { state: {} }
}));

vi.mock('$lib/state/server/registry.svelte', () => ({
  serverRegistry: { getServer: () => undefined }
}));
vi.mock('$app/state', () => ({ page: mocks.page }));
vi.mock('$app/navigation', () => ({ replaceState: mocks.replaceState }));
vi.mock('$lib/state/server/serverConnection.svelte', () => ({
  serverConnectionManager: { getClient: () => ({ getAPI: mocks.getAPI }) }
}));
vi.mock('$lib/attachments/attachmentUrls', async (importOriginal) => ({
  ...(await importOriginal<typeof import('$lib/attachments/attachmentUrls')>()),
  refreshAttachmentUrlsForAssets: mocks.refreshUrls
}));

import ImageViewerModal from './ImageViewerModal.svelte';

const refreshInterval = 22 * 60 * 60 * 1000;

describe('image viewer URL refresh', () => {
  afterEach(() => {
    vi.useRealTimers();
    vi.clearAllMocks();
  });

  it('refreshes attachment URLs periodically and stops after unmount', async () => {
    vi.useFakeTimers();
    const modal: ImageViewerModalState = {
      type: 'imageViewer',
      serverId: 'origin',
      roomId: 'room-1',
      eventId: 'event-1',
      imageIndex: 0,
      imageItems: [
        {
          id: 'image-1',
          src: 'data:image/gif;base64,R0lGODlhAQABAIAAAAAAAP///yH5BAEAAAAALAAAAAABAAEAAAIBRAA7',
          alt: 'Test image'
        }
      ]
    };
    const rendered = render(ImageViewerModal, { props: { modal, onclose: vi.fn() } });

    await expect.element(rendered.getByRole('img', { name: 'Test image' })).toBeVisible();
    await vi.advanceTimersByTimeAsync(refreshInterval - 1);
    expect(mocks.refreshUrls).not.toHaveBeenCalled();
    await vi.advanceTimersByTimeAsync(1);
    expect(mocks.refreshUrls).toHaveBeenCalledOnce();
    expect(mocks.refreshUrls).toHaveBeenLastCalledWith(
      {},
      'room-1',
      ['image-1'],
      expect.any(Object)
    );
    await vi.advanceTimersByTimeAsync(refreshInterval);
    expect(mocks.refreshUrls).toHaveBeenCalledTimes(2);

    await rendered.unmount();
    await vi.advanceTimersByTimeAsync(refreshInterval);
    expect(mocks.refreshUrls).toHaveBeenCalledTimes(2);
  });
});
