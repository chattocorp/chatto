import { describe, expect, it } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { q } from '$lib/test-utils';

import ServerBanner from './ServerBanner.svelte';

describe('ServerBanner', () => {
  it('renders a single cover-fitted banner image without decorative duplicates', async () => {
    const url = 'https://cdn.example.com/server-banner.webp';
    const { container } = render(ServerBanner, { props: { url } });

    const images = container.querySelectorAll('img');
    expect(images).toHaveLength(1);
    expect(container.querySelector('[aria-hidden="true"]')).toBeNull();

    const image = q(container, 'img[alt="Server banner"]');
    await expect.element(image).toBeInTheDocument();
    await expect.element(image).toHaveAttribute('src', url);
    await expect.element(image).toHaveClass('object-cover');
    await expect.element(image).not.toHaveClass('object-contain');
  });
});
