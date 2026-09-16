import { describe, expect, it } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { q } from '$lib/test-utils';

import ServerBanner from './ServerBanner.svelte';

describe('ServerBanner', () => {
  it('renders the complete banner at its original aspect ratio', async () => {
    const url = `data:image/svg+xml,${encodeURIComponent('<svg xmlns="http://www.w3.org/2000/svg" width="600" height="400"><rect width="600" height="400" fill="teal"/></svg>')}`;
    const { container } = render(ServerBanner, { props: { url } });

    const images = container.querySelectorAll('img');
    expect(images).toHaveLength(1);
    expect(container.querySelector('[aria-hidden="true"]')).toBeNull();

    const image = q(container, 'img[alt="Server banner"]');
    await expect.element(image).toBeInTheDocument();
    await expect.element(image).toHaveAttribute('src', url);
    if (!(image instanceof HTMLImageElement)) throw new Error('Server banner image is missing');
    await expect.poll(() => image.naturalWidth).toBe(600);
    await expect.poll(() => {
      const { width, height } = image.getBoundingClientRect();
      return width / height;
    }).toBeCloseTo(1.5, 2);
  });
});
