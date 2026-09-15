import '../../../app.css';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { tick } from 'svelte';
import type { HtmlViewerModalState } from '$lib/modal';
import type { RefreshedAttachmentUrls } from '$lib/attachments/attachmentUrls';

const mocks = vi.hoisted(() => ({
  refreshUrls: vi.fn(),
  getMetadata: vi.fn(),
  page: { state: {} as { modal?: HtmlViewerModalState } }
}));
vi.mock('$app/state', () => ({ page: mocks.page }));
vi.mock('$lib/state/server/registry.svelte', () => ({
  serverRegistry: { getServer: () => ({ url: 'https://remote.example' }) }
}));
vi.mock('$lib/state/server/serverConnection.svelte', () => ({
  serverConnectionManager: {
    getClient: () => ({
      queryScope: 'test-session',
      getAPI: () => ({ getMetadata: mocks.getMetadata })
    })
  }
}));
vi.mock('$lib/attachments/attachmentUrls', async (importOriginal) => ({
  ...(await importOriginal<typeof import('$lib/attachments/attachmentUrls')>()),
  refreshAttachmentUrlsForAssets: mocks.refreshUrls
}));

import HtmlViewerModal from './HtmlViewerModal.svelte';

function modalState(expired = false): HtmlViewerModalState {
  return {
    type: 'htmlViewer',
    serverId: 'remote',
    roomId: 'room',
    eventId: 'message',
    attachmentId: 'html',
    filename: 'report.html',
    contentType: 'text/html',
    assetUrl: {
      url: '/assets/files/html?access=ticket',
      expiresAt: expired ? '2000-01-01' : '2099-01-01'
    }
  };
}

function mount(modal = modalState()) {
  mocks.page.state.modal = modal;
  const onclose = vi.fn();
  return { ...render(HtmlViewerModal, { modal, onclose }), onclose };
}

function freshUrls(): Map<string, RefreshedAttachmentUrls> {
  return new Map([
    [
      'html',
      {
        // A local inert document avoids external requests in component tests.
        assetUrl: { url: 'about:blank', expiresAt: '2099-01-01' },
        thumbnailAssetUrl: null,
        videoThumbnailAssetUrl: null,
        variantAssetUrls: new Map()
      }
    ]
  ]);
}

describe('HTML viewer', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mocks.refreshUrls.mockResolvedValue(new Map());
    mocks.getMetadata.mockResolvedValue({ size: 1536 });
  });

  it('offers a remote download without loading or refreshing the document before consent', async () => {
    const view = mount();
    await expect.element(view.getByRole('dialog')).toBeVisible();
    await expect.element(view.getByRole('button', { name: 'Show preview' })).toBeVisible();
    expect(view.container.querySelector('iframe')).toBeNull();
    expect(mocks.refreshUrls).not.toHaveBeenCalled();
    await expect.element(view.getByText('HTML document', { exact: true })).toBeVisible();
    await expect.element(view.getByText('1.5 KiB', { exact: true })).toBeVisible();
    await expect
      .element(view.getByRole('link', { name: 'Download', exact: true }))
      .toHaveAttribute('href', 'https://remote.example/assets/files/html?access=ticket&download=1');
    await expect
      .element(view.getByText('These websites can see your IP address.', { exact: false }))
      .toBeVisible();
  });

  it('shows XHTML and a zero-byte size without loading the preview', async () => {
    mocks.getMetadata.mockResolvedValue({ size: 0 });
    const view = mount({
      ...modalState(),
      contentType: 'Application/XHTML+XML; charset=utf-8'
    });
    await expect.element(view.getByText('XHTML document', { exact: true })).toBeVisible();
    await expect.element(view.getByText('0 B', { exact: true })).toBeVisible();
    expect(view.container.querySelector('iframe')).toBeNull();
  });

  it('keeps download and preview available when file size cannot be read', async () => {
    mocks.getMetadata.mockRejectedValue(new Error('Metadata unavailable'));
    const view = mount();
    await expect.element(view.getByText('Unavailable', { exact: true })).toBeVisible();
    await expect.element(view.getByRole('link', { name: 'Download', exact: true })).toBeVisible();
    await expect.element(view.getByRole('button', { name: 'Show preview' })).toBeEnabled();
    expect(view.container.querySelector('iframe')).toBeNull();
  });

  it('refreshes on consent and creates an empty sandbox with no referrer', async () => {
    mocks.refreshUrls.mockResolvedValue(freshUrls());
    const view = mount(modalState(true));
    await view.getByRole('button', { name: 'Show preview' }).click();
    await expect
      .element(view.getByTitle('Preview of report.html'))
      .toHaveAttribute('src', 'about:blank');
    await expect.element(view.getByTitle('Preview of report.html')).toHaveAttribute('sandbox', '');
    await expect
      .element(view.getByTitle('Preview of report.html'))
      .toHaveAttribute('referrerpolicy', 'no-referrer');
    await expect.element(view.getByRole('link', { name: 'Download', exact: true })).toBeVisible();
    expect(mocks.refreshUrls).toHaveBeenCalledWith({ getMetadata: mocks.getMetadata }, 'room', [
      'html'
    ]);
    await view.unmount();
    const reopened = mount();
    expect(reopened.container.querySelector('iframe')).toBeNull();
    await expect.element(reopened.getByRole('button', { name: 'Show preview' })).toBeVisible();
  });

  it('reports refresh failures and leaves preview disabled until a successful retry', async () => {
    const view = mount(modalState(true));
    await view.getByRole('button', { name: 'Show preview' }).click();
    await expect.element(view.getByRole('alert')).toHaveTextContent('Could not load preview');
    expect(view.container.querySelector('iframe')).toBeNull();
    mocks.refreshUrls.mockResolvedValue(freshUrls());
    await view.getByRole('button', { name: 'Show preview' }).click();
    await expect.element(view.getByTitle('Preview of report.html')).toBeVisible();
  });

  it('reports a failed download refresh without enabling preview', async () => {
    const view = mount(modalState(true));
    await view.getByRole('link', { name: 'Download', exact: true }).click();
    await expect
      .element(view.getByRole('alert'))
      .toHaveTextContent('Could not refresh download link');
    expect(view.container.querySelector('iframe')).toBeNull();
  });

  it('refreshes a download link and activates it without enabling preview', async () => {
    const urls = freshUrls();
    urls.get('html')!.assetUrl!.url = '/assets/files/html?access=fresh';
    mocks.refreshUrls.mockResolvedValue(urls);
    const view = mount(modalState(true));
    const activations: string[] = [];
    const click = vi.spyOn(HTMLAnchorElement.prototype, 'click').mockImplementation(function (
      this: HTMLAnchorElement
    ) {
      activations.push(this.href);
    });
    try {
      await view.getByRole('link', { name: 'Download', exact: true }).click();
      await expect
        .poll(() => activations)
        .toEqual(['https://remote.example/assets/files/html?access=fresh&download=1']);
      expect(view.container.querySelector('iframe')).toBeNull();
    } finally {
      click.mockRestore();
    }
  });

  it('restores focus to the attachment trigger when removed by history navigation', async () => {
    const trigger = document.createElement('button');
    document.body.append(trigger);
    trigger.focus();
    try {
      const view = mount();
      await expect.element(view.getByRole('dialog')).toBeVisible();
      await view.unmount();
      expect(document.activeElement).toBe(trigger);
    } finally {
      trigger.remove();
    }
  });

  it.each(['close', 'replace', 'unmount'])(
    'discards a late preview refresh after %s',
    async (action) => {
      let resolve!: (urls: Map<string, RefreshedAttachmentUrls>) => void;
      mocks.refreshUrls.mockReturnValue(
        new Promise<Map<string, RefreshedAttachmentUrls>>((done) => {
          resolve = done;
        })
      );
      const view = mount(modalState(true));
      await view.getByRole('button', { name: 'Show preview' }).click();
      if (action === 'close') {
        await view.getByRole('button', { name: 'Close', exact: true }).click();
        await expect.poll(() => view.onclose.mock.calls.length).toBe(1);
      } else if (action === 'replace') mocks.page.state.modal = modalState();
      else await view.unmount();
      resolve(freshUrls());
      await tick();
      expect(view.container.querySelector('iframe')).toBeNull();
    }
  );
});
