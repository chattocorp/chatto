import { describe, it, expect } from 'vitest';
import { render } from 'vitest-browser-svelte';
import ImageModal from './ImageModal.svelte';

describe('ImageModal', () => {
  it('keeps the original image action as a native link', async () => {
    const { container } = render(ImageModal, {
      props: {
        items: [
          {
            src: 'https://cdn.example.com/current.jpg',
            filename: 'image.jpg'
          }
        ],
        onclose: () => {}
      }
    });

    const link = container.querySelector<HTMLAnchorElement>('a')!;

    await expect.element(link).toHaveAttribute('href', 'https://cdn.example.com/current.jpg');
    await expect.element(link).toHaveAttribute('target', '_blank');
    await expect.element(link).toHaveAttribute('rel', 'noopener noreferrer');
  });
});
