import { describe, it, expect } from 'vitest';
import { render } from 'vitest-browser-svelte';
import ImageModal from './ImageModal.svelte';

describe('ImageModal', () => {
  it('keeps the original image action as a native link', async () => {
    const { container } = render(ImageModal, {
      props: {
        items: [
          {
            src: 'https://cdn.example.com/display.jpg',
            originalSrc: 'https://cdn.example.com/original.jpg',
            filename: 'image.jpg'
          }
        ],
        onclose: () => {}
      }
    });

    const link = container.querySelector<HTMLAnchorElement>('a')!;

    await expect.element(link).toHaveAttribute('href', 'https://cdn.example.com/original.jpg');
    await expect.element(link).toHaveAttribute('target', '_blank');
    await expect.element(link).toHaveAttribute('rel', 'noopener noreferrer');
  });

  it('falls back to the display image when no separate original URL exists', async () => {
    const { container } = render(ImageModal, {
      props: {
        items: [{ src: 'https://cdn.example.com/display.jpg', filename: 'image.jpg' }],
        onclose: () => {}
      }
    });

    await expect
      .element(container.querySelector<HTMLAnchorElement>('a')!)
      .toHaveAttribute('href', 'https://cdn.example.com/display.jpg');
  });
});
