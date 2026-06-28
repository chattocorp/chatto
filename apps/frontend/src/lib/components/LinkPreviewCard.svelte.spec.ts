import { describe, it, expect } from 'vitest';
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
});
