import '../../../app.css';
import { describe, it, expect, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import type { LinkPreviewView } from '@chatto/client/timeline/linkPreviews';
import ComposerLinkPreviewCard from './ComposerLinkPreviewCard.svelte';

vi.mock('$app/navigation', () => ({ goto: vi.fn() }));
vi.mock('$lib/client', async () => (await import('$lib/test-utils/clientMock')).clientMockDefaults);

function preview(o: Partial<LinkPreviewView> = {}): LinkPreviewView {
  return {
    url: 'https://example.com/story',
    title: null,
    description: null,
    imageUrl: null,
    siteName: null,
    embedType: 'generic',
    embedId: null,
    ...o
  };
}

describe('ComposerLinkPreviewCard', () => {
  it('shows the site, title, and thumbnail in one short row without the description', async () => {
    const { container, getByText } = render(ComposerLinkPreviewCard, {
      preview: preview({
        title: 'Story title',
        description: 'A long description',
        siteName: 'Example',
        imageUrl: 'https://example.com/image.png'
      }),
      onDismiss: vi.fn()
    });

    await expect.element(getByText('Example')).toBeInTheDocument();
    await expect.element(getByText('Story title')).toBeInTheDocument();
    expect(container.textContent).not.toContain('A long description');
    expect(container.querySelector('img')?.getAttribute('src')).toBe(
      'https://example.com/image.png'
    );
    const card = container.querySelector<HTMLElement>('[data-testid="composer-link-preview"]')!;
    expect(card.getBoundingClientRect().height).toBeLessThanOrEqual(72);
  });

  it('falls back to the host name when the preview has no site name or title', async () => {
    const { getByText } = render(ComposerLinkPreviewCard, {
      preview: preview(),
      onDismiss: vi.fn()
    });

    await expect.element(getByText('example.com').first()).toBeInTheDocument();
  });

  it('shows a YouTube preview as a row without loading the player', async () => {
    const { container, getByText } = render(ComposerLinkPreviewCard, {
      preview: preview({
        url: 'https://www.youtube.com/watch?v=dQw4w9WgXcQ',
        title: 'YouTube Video',
        embedType: 'youtube',
        embedId: 'dQw4w9WgXcQ'
      }),
      onDismiss: vi.fn()
    });

    await expect.element(getByText('YouTube Video')).toBeInTheDocument();
    expect(container.querySelector('iframe')).toBeNull();
  });

  it('calls onDismiss from the dismiss button', async () => {
    const onDismiss = vi.fn();
    const { getByRole } = render(ComposerLinkPreviewCard, {
      preview: preview({ title: 'Story title' }),
      onDismiss
    });

    await getByRole('button', { name: 'Dismiss preview' }).click();

    expect(onDismiss).toHaveBeenCalledOnce();
  });
});
