import { describe, it, expect, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { makeRenderData } from '$lib/render/data';
import LinkPreviewCard, { LinkPreviewViewData } from './LinkPreviewCard.svelte';

type PreviewData = {
  url: string;
  title?: string | null;
  description?: string | null;
  imageUrl?: string | null;
  siteName?: string | null;
  embedType?: string | null;
  embedId?: string | null;
  socialPost?: {
    provider: string;
    url?: string | null;
    author?: {
      displayName: string;
      handle: string;
      avatarUrl?: string | null;
    } | null;
    text: string;
    publishedAt?: string | null;
    externalLink?: {
      url: string;
      title?: string | null;
      description?: string | null;
      imageUrl?: string | null;
    } | null;
    contentWarning?: string | null;
    images: Array<{
      url: string;
      alt?: string | null;
      width?: number | null;
      height?: number | null;
    }>;
    quotedPost?: PreviewData['socialPost'];
  } | null;
};

function preview(o: Partial<PreviewData> = {}) {
  return makeRenderData(
    {
      url: 'https://example.com',
      title: null,
      description: null,
      imageUrl: null,
      siteName: null,
      embedType: 'generic',
      embedId: null,
      ...o
    },
    LinkPreviewViewData
  );
}

describe('LinkPreviewCard', () => {
  it('renders nothing when no metadata is available', () => {
    const { container } = render(LinkPreviewCard, {
      props: { preview: preview() }
    });
    expect(container.querySelector('[data-testid="link-preview-card"]')).toBeNull();
  });

  it('renders the card when only a title is present', () => {
    const { container } = render(LinkPreviewCard, {
      props: { preview: preview({ title: 'Hello' }) }
    });
    const card = container.querySelector('[data-testid="link-preview-card"]');
    expect(card).not.toBeNull();
    expect(card?.textContent).toContain('Hello');
  });

  it('renders the card when only an image is present', () => {
    const { container } = render(LinkPreviewCard, {
      props: { preview: preview({ imageUrl: 'https://example.com/img.png' }) }
    });
    expect(container.querySelector('[data-testid="link-preview-card"]')).not.toBeNull();
  });

  it('renders the card when only a description is present', () => {
    const { container } = render(LinkPreviewCard, {
      props: { preview: preview({ description: 'A description' }) }
    });
    expect(container.querySelector('[data-testid="link-preview-card"]')).not.toBeNull();
  });

  it('renders the card when only a site name is present', () => {
    const { container } = render(LinkPreviewCard, {
      props: { preview: preview({ siteName: 'Example' }) }
    });
    expect(container.querySelector('[data-testid="link-preview-card"]')).not.toBeNull();
  });

  it('renders the YouTube embed when embedType is youtube', () => {
    const { container } = render(LinkPreviewCard, {
      props: {
        preview: preview({
          url: 'https://www.youtube.com/watch?v=abc123',
          embedType: 'youtube',
          embedId: 'abc123'
        })
      }
    });
    expect(container.querySelector('[data-testid="link-preview-card"]')).toBeNull();
    expect(container.querySelector('iframe')).not.toBeNull();
  });

  it('renders a native social-post snapshot and conceals a warned quote', async () => {
    const { container, rerender } = render(LinkPreviewCard, {
      props: {
        preview: preview({
          url: 'https://bsky.app/profile/bsky.app/post/3kq7aeuwbg42k',
          title: 'Bluesky (@bsky.app)',
          embedType: 'bluesky',
          embedId: 'at://did:plc:z72i7hdynmk6r22z27h6tvur/app.bsky.feed.post/3kq7aeuwbg42k',
          socialPost: {
            provider: 'bluesky',
            author: {
              displayName: 'Bluesky',
              handle: 'bsky.app'
            },
            text: 'A post rendered by Chatto.',
            images: [],
            quotedPost: {
              provider: 'bluesky',
              url: 'https://bsky.app/profile/quoted.example/post/quoted',
              author: {
                displayName: 'Quoted Author',
                handle: 'quoted.example'
              },
              text: 'A quoted post with an attached image.',
              contentWarning: 'Spoilers',
              images: [
                {
                  url: 'https://example.com/quoted.jpg',
                  alt: 'Quoted image'
                }
              ]
            }
          }
        })
      }
    });
    expect(container.querySelector('[data-testid="link-preview-card"]')).toBeNull();
    const card = container.querySelector<HTMLElement>('[data-testid="social-post-embed"]');
    expect(card).not.toBeNull();
    expect(card?.dataset.provider).toBe('bluesky');
    expect(card?.querySelector<HTMLAnchorElement>('a')?.href).toBe(
      'https://bsky.app/profile/bsky.app/post/3kq7aeuwbg42k'
    );
    expect(card?.textContent).toContain('Bluesky');
    expect(card?.textContent).toContain('@bsky.app');
    expect(card?.textContent).toContain('A post rendered by Chatto.');
    expect(card?.querySelector('[data-testid="quoted-social-post"]')).not.toBeNull();
    expect(card?.textContent).not.toContain('A quoted post with an attached image.');
    expect(card?.textContent).toContain('Spoilers');
    expect(card?.querySelector<HTMLImageElement>('img[alt="Quoted image"]')).toBeNull();

    const reveal = Array.from(card?.querySelectorAll('button') ?? []).find((button) =>
      button.textContent?.includes('Show content')
    );
    reveal?.click();
    await vi.waitFor(() => {
      expect(card?.textContent).toContain('A quoted post with an attached image.');
      expect(card?.querySelector<HTMLImageElement>('img[alt="Quoted image"]')).not.toBeNull();
    });
    expect(card?.querySelector('iframe')).toBeNull();

    await rerender({
      preview: preview({
        url: 'https://social.example/@alice/next',
        socialPost: {
          provider: 'mastodon',
          text: 'Outer post',
          images: [],
          quotedPost: {
            provider: 'mastodon',
            url: 'https://remote.example/@bob/next',
            text: 'A different warned quote.',
            contentWarning: 'A different warning',
            images: []
          }
        }
      })
    });
    expect(card?.textContent).toContain('A different warning');
    expect(card?.textContent).not.toContain('A different warned quote.');
  });

  it('conceals a warned social post until the reader reveals it', async () => {
    const { container, rerender } = render(LinkPreviewCard, {
      props: {
        preview: preview({
          url: 'https://social.example/@alice/123',
          socialPost: {
            provider: 'mastodon',
            text: 'Hidden post text',
            contentWarning: 'Sensitive topic',
            images: [{ url: 'https://example.com/hidden.jpg', alt: 'Hidden image' }]
          }
        })
      }
    });

    const card = container.querySelector<HTMLElement>('[data-testid="social-post-embed"]');
    expect(card?.textContent).toContain('Sensitive topic');
    expect(card?.textContent).not.toContain('Hidden post text');
    expect(card?.querySelector<HTMLImageElement>('img[alt="Hidden image"]')).toBeNull();

    const reveal = Array.from(card?.querySelectorAll('button') ?? []).find((button) =>
      button.textContent?.includes('Show content')
    );
    reveal?.click();
    await vi.waitFor(() => {
      expect(card?.textContent).toContain('Hidden post text');
      expect(card?.querySelector<HTMLImageElement>('img[alt="Hidden image"]')).not.toBeNull();
    });

    await rerender({
      preview: preview({
        url: 'https://social.example/@bob/456',
        socialPost: {
          provider: 'mastodon',
          text: 'A different hidden post',
          contentWarning: 'Another sensitive topic',
          images: []
        }
      })
    });
    expect(card?.textContent).toContain('Another sensitive topic');
    expect(card?.textContent).not.toContain('A different hidden post');
  });
});
