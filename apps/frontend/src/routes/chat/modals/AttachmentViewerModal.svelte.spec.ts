import '../../../app.css';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { tick } from 'svelte';
import { VideoProcessingStatus } from '@chatto/client/timeline/messageAttachments';
import type { AttachmentViewerModalState } from '$lib/modal';
import type { RefreshedAttachmentUrls } from '@chatto/client/attachments/attachmentUrls';

const mocks = vi.hoisted(() => ({
  refreshUrls: vi.fn(),
  getMetadata: vi.fn(),
  page: { state: {} as { modal?: AttachmentViewerModalState } }
}));
// Asset URLs resolve against the server that owns the attachment's server ID.
vi.mock('@chatto/client/server/serverIds', async (importOriginal) => ({
  ...(await importOriginal<typeof import('@chatto/client/server/serverIds')>()),
  ownerOfServerId: () => ({ getServer: () => ({ url: 'https://remote.example' }) })
}));
vi.mock('$lib/client', async () => ({
  ...(await import('$lib/test-utils/clientMock')).clientMockDefaults,
  serverRegistry: { getServer: () => ({ url: 'https://remote.example' }) },
  serverConnectionManager: {
    getClient: () => ({
      queryScope: 'test-session',
      getAPI: () => ({ getMetadata: mocks.getMetadata })
    })
  }
}));

vi.mock('$app/state', () => ({ page: mocks.page }));
vi.mock('@chatto/client/attachments/attachmentUrls', async (importOriginal) => ({
  ...(await importOriginal<typeof import('@chatto/client/attachments/attachmentUrls')>()),
  refreshAttachmentUrlsForAssets: mocks.refreshUrls
}));

import AttachmentViewerModal from './AttachmentViewerModal.svelte';

function modalState(expired = false): AttachmentViewerModalState {
  return {
    type: 'attachmentViewer',
    index: 0,
    items: [
      {
        id: 'html',
        width: 0,
        height: 0,

        filename: 'report.html',
        contentType: 'text/html',
        assetUrl: {
          url: '/assets/files/html?access=ticket',
          expiresAt: expired ? '2000-01-01' : '2099-01-01'
        }
      }
    ],
    serverId: 'remote',
    roomId: 'room',
    eventId: 'message'
  };
}

function mount(modal = modalState()) {
  mocks.page.state.modal = modal;
  const onclose = vi.fn();
  return { ...render(AttachmentViewerModal, { modal, onclose }), onclose };
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

  it.each(['text/html', 'application/pdf', 'audio/mpeg', 'video/mp4'])(
    'shows a plain-text description for %s without treating it as document content',
    async (contentType) => {
      const modal = modalState();
      const description = '<strong>Quarterly report</strong>\nNotes from the author.';
      modal.items[0] = { ...modal.items[0], contentType, description };
      const view = mount(modal);
      const captionId = view.container.querySelector('dialog')!.getAttribute('aria-describedby')!;
      const caption = document.getElementById(captionId)!;
      expect(caption.textContent?.trim()).toBe(description);
      expect(caption.querySelector('strong')).toBeNull();
      await expect
        .element(view.getByRole('dialog'))
        .toHaveAttribute('aria-describedby', caption.id);
      await expect.element(view.getByRole('link', { name: 'Download', exact: true })).toBeVisible();
      expect(view.container.querySelector('iframe')).toBeNull();
      if (contentType === 'text/html') {
        expect(mocks.refreshUrls).not.toHaveBeenCalled();
        mocks.refreshUrls.mockResolvedValue(freshUrls());
        await view.getByRole('button', { name: 'Show preview' }).click();
        await expect.element(view.getByTitle('Preview of report.html')).toBeVisible();
        await expect.element(caption).toBeVisible();
      }
    }
  );

  it('shows XHTML and a zero-byte size without loading the preview', async () => {
    mocks.getMetadata.mockResolvedValue({ size: 0 });
    const view = mount({
      ...modalState(),
      items: [{ ...modalState().items[0], contentType: 'Application/XHTML+XML; charset=utf-8' }]
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
    expect(mocks.refreshUrls).toHaveBeenCalledWith(
      { getMetadata: mocks.getMetadata },
      'room',
      ['html'],
      expect.anything()
    );
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

/** Viewer tests use controlled byte responses, including responses that ignore abort. */
describe('Markdown attachment previews', () => {
  const fetchDocument = vi.fn<typeof fetch>();

  function documentModal(expired = false) {
    const modal = modalState(expired);
    modal.items[0].filename = 'report.md';
    modal.items[0].contentType = 'text/markdown';
    return modal;
  }

  beforeEach(() => {
    vi.clearAllMocks();
    fetchDocument.mockReset();
    vi.stubGlobal('fetch', fetchDocument);
    fetchDocument.mockResolvedValue(new Response('# Report\n\nDocument body.'));
    mocks.refreshUrls.mockResolvedValue(freshUrls());
    mocks.getMetadata.mockResolvedValue({ size: 1536 });
  });
  afterEach(() => vi.unstubAllGlobals());

  it('automatically renders Markdown and preserves metadata and the original download', async () => {
    fetchDocument.mockResolvedValue(
      new Response(
        '# Report\n\n**Summary**\n\n- First item\n- Second item\n\n' +
          '| Item | Status |\n| --- | --- |\n| Preview | Ready |\n\n' +
          '[Reference](https://example.com)\n\n```text\nexample code\n```\n\n@alice'
      )
    );
    const modal = documentModal();
    modal.items[0].description = 'Notes from the author.';
    const view = mount(modal);
    await expect.element(view.getByRole('heading', { name: 'Report', exact: true })).toBeVisible();
    expect(view.container.querySelector('strong')?.textContent).toBe('Summary');
    expect(view.container.querySelectorAll('li')).toHaveLength(2);
    expect(view.container.querySelector('table')).not.toBeNull();
    expect(view.container.querySelector('pre code')?.textContent).toContain('example code');
    expect(view.container.querySelector('[data-mention-handle]')).toBeNull();
    await expect
      .element(view.getByRole('link', { name: 'Reference' }))
      .toHaveAttribute('rel', 'noopener noreferrer');
    await expect.element(view.getByText('Notes from the author.')).toBeVisible();
    await expect.element(view.getByText('1.5 KiB', { exact: true })).toBeVisible();
    await expect
      .element(view.getByRole('link', { name: 'Download', exact: true }))
      .toHaveAttribute('href', 'https://remote.example/assets/files/html?access=ticket&download=1');
    expect(view.container.querySelector('iframe')).toBeNull();
    expect(fetchDocument).toHaveBeenCalledExactlyOnceWith(
      'https://remote.example/assets/files/html?access=ticket',
      { signal: expect.any(AbortSignal), credentials: 'omit', referrerPolicy: 'no-referrer' }
    );
    expect(mocks.refreshUrls).not.toHaveBeenCalled();
  });

  it.each(['text/plain', 'application/octet-stream', ''])(
    'automatically previews a Markdown filename with type %s',
    async (contentType) => {
      const modal = documentModal();
      modal.items[0].contentType = contentType;
      const view = mount(modal);
      await expect
        .element(view.getByRole('heading', { name: 'Report', exact: true }))
        .toBeVisible();
    }
  );

  it('keeps raw HTML and embedded images inert', async () => {
    fetchDocument.mockResolvedValue(
      new Response(
        '# Safe\n\n<script>alert(1)</script>\n\n<img src="https://example.com/tracker">\n\n' +
          '![Tracker](https://example.com/image.png)\n\n[Unsafe](javascript:alert(1))'
      )
    );
    const view = mount(documentModal());
    await expect.element(view.getByRole('heading', { name: 'Safe' })).toBeVisible();
    expect(view.container.querySelector('script, img, iframe')).toBeNull();
    expect(view.container.querySelector('a[href^="javascript:"]')).toBeNull();
    expect(fetchDocument).toHaveBeenCalledTimes(1);
  });

  it('shows an empty file as a valid preview', async () => {
    fetchDocument.mockResolvedValue(new Response(''));
    const view = mount(documentModal());
    await expect.poll(() => view.container.querySelector('.markdown-html') !== null).toBe(true);
    expect(view.container.textContent).not.toContain('No preview is available');
  });

  it('keeps loading active until the document body has been read and rendered', async () => {
    let resolve!: (body: string) => void;
    const response = new Response();
    const read = vi.spyOn(response, 'text').mockImplementation(
      () =>
        new Promise((done) => {
          resolve = done;
        })
    );
    fetchDocument.mockResolvedValue(response);
    const view = mount(documentModal());
    await expect.poll(() => read.mock.calls.length).toBe(1);
    await expect
      .element(view.getByRole('link', { name: 'Download', exact: true }))
      .toHaveAttribute('aria-disabled', 'true');
    expect(view.container.querySelector('.markdown-html')).toBeNull();
    resolve('# Loaded document');
    await expect
      .element(view.getByRole('heading', { name: 'Loaded document', exact: true }))
      .toBeVisible();
    await expect
      .element(view.getByRole('link', { name: 'Download', exact: true }))
      .toHaveAttribute('aria-disabled', 'false');
  });

  it('refreshes an expired URL before requesting document bytes', async () => {
    const view = mount(documentModal(true));
    await expect.element(view.getByRole('heading', { name: 'Report', exact: true })).toBeVisible();
    expect(mocks.refreshUrls).toHaveBeenCalledTimes(1);
    expect(fetchDocument.mock.calls[0][0]).toBe('about:blank');
  });

  it('refreshes and retries once after a failed byte request', async () => {
    fetchDocument.mockResolvedValueOnce(new Response('', { status: 403 }));
    const view = mount(documentModal());
    await expect.element(view.getByRole('heading', { name: 'Report', exact: true })).toBeVisible();
    expect(mocks.refreshUrls).toHaveBeenCalledTimes(1);
    expect(fetchDocument).toHaveBeenCalledTimes(2);
  });

  it('bounds automatic recovery and allows a manual retry', async () => {
    fetchDocument.mockRejectedValue(new TypeError('Request failed'));
    const view = mount(documentModal());
    await expect.element(view.getByRole('alert')).toBeVisible();
    expect(fetchDocument).toHaveBeenCalledTimes(2);
    expect(mocks.refreshUrls).toHaveBeenCalledTimes(1);
    fetchDocument.mockResolvedValue(new Response('# Recovered'));
    await view.getByRole('button', { name: 'Try Again', exact: true }).click();
    await expect.element(view.getByRole('heading', { name: 'Recovered' })).toBeVisible();
    expect(fetchDocument).toHaveBeenCalledTimes(3);
    expect(mocks.refreshUrls).toHaveBeenCalledTimes(2);
  });

  it.each(['navigate', 'close', 'unmount'])(
    'aborts pending bytes and discards late content after %s',
    async (action) => {
      let resolve!: (response: Response) => void;
      fetchDocument.mockImplementationOnce(
        () =>
          new Promise((done) => {
            resolve = done;
          })
      );
      const modal = documentModal();
      modal.items.push({ ...modal.items[0], id: 'next', filename: 'next.md' });
      const view = mount(modal);
      await expect.poll(() => fetchDocument.mock.calls.length).toBe(1);
      const signal = (fetchDocument.mock.calls[0][1] as RequestInit).signal!;
      if (action === 'navigate') {
        await view.getByRole('button', { name: 'Next image' }).click();
        await expect
          .element(view.getByRole('heading', { name: 'Report', exact: true }))
          .toBeVisible();
      } else if (action === 'close') {
        await view.getByRole('button', { name: 'Close', exact: true }).click();
      } else await view.unmount();
      await expect.poll(() => signal.aborted).toBe(true);
      const response = new Response('# Stale document');
      const read = vi.spyOn(response, 'text');
      resolve(response);
      await expect.poll(() => read.mock.settledResults[0]?.type).toBe('fulfilled');
      await tick();
      await expect
        .poll(() => view.container.textContent?.includes('Stale document') ?? false)
        .toBe(false);
      expect(mocks.refreshUrls).not.toHaveBeenCalled();
    }
  );
});

const pixel = 'data:image/gif;base64,R0lGODlhAQABAIAAAAAAAP///yH5BAEAAAAALAAAAAABAAEAAAIBRAA7';
function gallery() {
  const modal = modalState();
  modal.items = ['first', 'second'].map((id) => ({
    ...modal.items[0],
    id,
    filename: `${id}.gif`,
    contentType: 'image/gif',
    assetUrl: { url: `/assets/files/${id}?access=original`, expiresAt: '2099-01-01' }
  }));
  return modal;
}
function imageUrls(id: string): Map<string, RefreshedAttachmentUrls> {
  return new Map([
    [
      id,
      {
        assetUrl: { url: `/assets/files/${id}?access=fresh`, expiresAt: '2099-01-01' },
        thumbnailAssetUrl: { url: pixel, expiresAt: '2099-01-01' },
        videoThumbnailAssetUrl: null,
        variantAssetUrls: new Map()
      }
    ]
  ]);
}

describe('shared attachment previews', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mocks.getMetadata.mockImplementation((_room, id) =>
      Promise.resolve({ size: id === 'second' ? 2048 : 1024 })
    );
    mocks.refreshUrls.mockImplementation((_api, _room, ids) => Promise.resolve(imageUrls(ids[0])));
  });

  it('navigates image galleries and updates original download and metadata together', async () => {
    const view = mount(gallery());
    await expect.element(view.getByAltText('first.gif')).toHaveAttribute('src', pixel);
    await view.getByRole('button', { name: 'Details' }).click();
    await expect.element(view.getByText('File type')).toBeVisible();
    await expect.element(view.getByText('1 KiB', { exact: true })).toBeVisible();
    await view.getByRole('button', { name: 'Next image' }).click();
    await expect.element(view.getByAltText('second.gif')).toBeVisible();
    await expect.element(view.getByText('2 KiB', { exact: true })).toBeVisible();
    await expect
      .element(view.getByRole('link', { name: 'Download', exact: true }))
      .toHaveAttribute(
        'href',
        'https://remote.example/assets/files/second?access=fresh&download=1'
      );
    window.dispatchEvent(new KeyboardEvent('keydown', { key: 'ArrowLeft', bubbles: true }));
    await expect.element(view.getByAltText('first.gif')).toBeVisible();
  });

  it('resets zoom on gallery selection and keeps it during an image URL refresh', async () => {
    const view = mount(gallery());
    await expect.element(view.getByAltText('first.gif')).toBeVisible();
    await view.getByRole('button', { name: 'Zoom in' }).click();
    await expect.element(view.getByText('125%')).toBeVisible();

    view.container.querySelector('img')!.dispatchEvent(new Event('error'));
    await expect.poll(() => mocks.refreshUrls.mock.calls.length).toBe(2);
    await expect.element(view.getByText('125%')).toBeVisible();

    await view.getByRole('button', { name: 'Next image' }).click();
    await expect.element(view.getByText('100%')).toBeVisible();
    await expect.element(view.getByAltText('second.gif')).toBeVisible();
  });

  it('supports zoom keys without changing gallery navigation', async () => {
    const view = mount(gallery());
    await expect.element(view.getByAltText('first.gif')).toBeVisible();
    window.dispatchEvent(
      new KeyboardEvent('keydown', { key: '+', bubbles: true, cancelable: true })
    );
    await expect.element(view.getByText('125%')).toBeVisible();
    window.dispatchEvent(
      new KeyboardEvent('keydown', { key: '-', bubbles: true, cancelable: true })
    );
    await expect.element(view.getByText('100%')).toBeVisible();
    window.dispatchEvent(
      new KeyboardEvent('keydown', { key: '+', bubbles: true, cancelable: true })
    );
    window.dispatchEvent(
      new KeyboardEvent('keydown', { key: '0', bubbles: true, cancelable: true })
    );
    await expect.element(view.getByText('100%')).toBeVisible();
    window.dispatchEvent(new KeyboardEvent('keydown', { key: 'ArrowRight', bubbles: true }));
    await expect.element(view.getByAltText('second.gif')).toBeVisible();
  });

  it('mirrors gallery arrow keys in RTL', async () => {
    document.documentElement.dir = 'rtl';
    try {
      const view = mount(gallery());
      await expect.element(view.getByAltText('first.gif')).toBeVisible();
      window.dispatchEvent(new KeyboardEvent('keydown', { key: 'ArrowLeft', bubbles: true }));
      await expect.element(view.getByAltText('second.gif')).toBeVisible();
    } finally {
      document.documentElement.dir = 'ltr';
    }
  });

  it('preserves gallery descriptions as alt text and visible captions', async () => {
    const modal = gallery();
    modal.items[0].description = 'Blue line rising across the chart.';
    modal.items[1].description = 'Red line falling across the chart.';
    const view = mount(modal);
    await expect.element(view.getByAltText(modal.items[0].description)).toBeVisible();
    await expect
      .element(view.getByRole('button', { name: 'Details' }))
      .toHaveAttribute('aria-expanded', 'false');
    await view.getByRole('button', { name: 'Details' }).click();
    await expect.element(view.getByText(modal.items[0].description, { exact: true })).toBeVisible();
    await view.getByRole('button', { name: 'Next image' }).click();
    await expect.element(view.getByAltText(modal.items[1].description)).toBeVisible();
    await expect.element(view.getByText(modal.items[1].description, { exact: true })).toBeVisible();
  });

  it('removes the caption and accessible description for an undescribed gallery image', async () => {
    const modal = gallery();
    modal.items[0].description = 'Blue line rising across the chart.';
    const view = mount(modal);
    await view.getByRole('button', { name: 'Details' }).click();
    await expect.element(view.getByText(modal.items[0].description, { exact: true })).toBeVisible();
    await view.getByRole('button', { name: 'Next image' }).click();
    await expect.element(view.getByAltText('second.gif')).toBeVisible();
    expect(view.container.querySelector('p[id]')).toBeNull();
    await expect.element(view.getByRole('dialog')).not.toHaveAttribute('aria-describedby');
    await view.getByRole('button', { name: 'Previous image' }).click();
    await expect.element(view.getByText(modal.items[0].description, { exact: true })).toBeVisible();
  });

  it('discards an earlier image refresh after changing the selection', async () => {
    let resolve!: (value: Map<string, RefreshedAttachmentUrls>) => void;
    mocks.refreshUrls.mockImplementationOnce(() => new Promise((done) => (resolve = done)));
    const view = mount(gallery());
    await expect.poll(() => mocks.refreshUrls.mock.calls.length).toBe(1);
    await view.getByRole('button', { name: 'Next image' }).click();
    await expect.element(view.getByAltText('second.gif')).toBeVisible();
    resolve(imageUrls('first'));
    await tick();
    expect(view.container.querySelector('img')?.getAttribute('alt')).toBe('second.gif');
    await expect
      .element(view.getByRole('link', { name: 'Download', exact: true }))
      .toHaveAttribute(
        'href',
        'https://remote.example/assets/files/second?access=fresh&download=1'
      );
  });

  it('bounds automatic recovery for a broken image', async () => {
    const view = mount(gallery());
    await expect.element(view.getByAltText('first.gif')).toBeVisible();
    view.container.querySelector('img')!.dispatchEvent(new Event('error'));
    await expect.poll(() => mocks.refreshUrls.mock.calls.length).toBe(2);
    await tick();
    view.container.querySelector('img')!.dispatchEvent(new Event('error'));
    await expect.element(view.getByRole('alert')).toBeVisible();
    expect(mocks.refreshUrls).toHaveBeenCalledTimes(2);
    await expect
      .element(view.getByRole('button', { name: 'Try Again', exact: true }))
      .toBeVisible();
    await view.getByRole('button', { name: 'Try Again', exact: true }).click();
    await expect.poll(() => mocks.refreshUrls.mock.calls.length).toBe(3);
  });

  it.each(['application/pdf', 'application/zip', 'text/plain'])(
    'does not request unsupported %s document bytes',
    async (contentType) => {
      const modal = modalState();
      modal.items[0].contentType = contentType;
      const view = mount(modal);
      await expect
        .element(view.getByText('No preview is available', { exact: false }))
        .toBeVisible();
      await expect.element(view.getByRole('link', { name: 'Download', exact: true })).toBeVisible();
      expect(view.container.querySelector('iframe, img, video, audio')).toBeNull();
      expect(mocks.refreshUrls).not.toHaveBeenCalled();
    }
  );

  it('previews the original video while processing is pending', async () => {
    const modal = modalState();
    modal.items[0] = {
      ...modal.items[0],
      contentType: 'video/mp4',
      assetUrl: { url: 'data:video/mp4,', expiresAt: '2099-01-01' },
      videoProcessing: {
        status: VideoProcessingStatus.Pending,
        variants: [],
        sourceAvailable: true
      }
    };
    const view = mount(modal);
    await expect
      .poll(() => view.container.querySelector('video')?.hasAttribute('controls'))
      .toBe(true);
    expect(view.container.querySelector('media-player')).toBeNull();
  });

  it.each(['audio/wav', 'video/webm'])('uses a native %s preview', async (contentType) => {
    const modal = modalState();
    modal.items[0].contentType = contentType;
    modal.items[0].assetUrl!.url = 'data:application/octet-stream,';
    const view = mount(modal);
    await expect
      .poll(() => view.container.querySelector('audio, video')?.hasAttribute('controls'))
      .toBe(true);
    await view.unmount();
    expect(view.container.querySelector('audio, video')).toBeNull();
  });
});
