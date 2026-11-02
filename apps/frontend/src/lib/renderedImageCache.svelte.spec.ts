import { expect, it } from 'vitest';
import { hasRenderedImage, rememberRenderedImage, takeRenderedImage } from './renderedImageCache';

it('keeps only loaded detached images, bounds retention, and claims images exclusively', async () => {
  const owner = {};
  const image = new Image();
  rememberRenderedImage(owner, 'unloaded', image);
  expect(hasRenderedImage(owner, 'unloaded')).toBe(false);
  image.src =
    'data:image/svg+xml,' +
    encodeURIComponent('<svg xmlns="http://www.w3.org/2000/svg" width="10" height="10"/>');
  await image.decode();
  document.body.append(image);
  try {
    rememberRenderedImage(owner, 'connected', image);
    expect(hasRenderedImage(owner, 'connected')).toBe(false);
  } finally {
    image.remove();
  }
  rememberRenderedImage(owner, 'loaded', image);
  expect(takeRenderedImage({}, 'loaded')).toBeUndefined();
  expect(takeRenderedImage(owner, 'loaded')).toBe(image);
  expect(takeRenderedImage(owner, 'loaded')).toBeUndefined();
  for (let index = 0; index < 9; index++) {
    const entry = new Image();
    entry.src = image.src;
    await entry.decode();
    rememberRenderedImage(owner, String(index), entry);
  }
  expect(hasRenderedImage(owner, '0')).toBe(false);
  expect(hasRenderedImage(owner, '1')).toBe(true);
  expect(hasRenderedImage(owner, '8')).toBe(true);
});
