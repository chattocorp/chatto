import { afterEach, expect, it, vi } from 'vitest';
import { copyImageToClipboard } from './copyImage';

const onePixelGif = 'data:image/gif;base64,R0lGODlhAQABAIAAAAAAAP///ywAAAAAAQABAAACAUwAOw==';

afterEach(() => {
  vi.restoreAllMocks();
});

it('writes the displayed image as PNG to the clipboard', async () => {
  const write = vi.spyOn(navigator.clipboard, 'write').mockImplementation(async (items) => {
    expect(items).toHaveLength(1);
    const png = await items[0].getType('image/png');
    expect(png.type).toBe('image/png');
    expect(png.size).toBeGreaterThan(0);
  });

  await copyImageToClipboard(onePixelGif);

  expect(write).toHaveBeenCalledOnce();
});

it('rejects when the image request fails', async () => {
  vi.spyOn(globalThis, 'fetch').mockResolvedValue(new Response(null, { status: 403 }));
  vi.spyOn(navigator.clipboard, 'write').mockImplementation(async (items) => {
    await items[0].getType('image/png');
  });

  await expect(copyImageToClipboard('https://example.com/image')).rejects.toThrow();
});
