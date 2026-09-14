import { afterEach, describe, expect, it, vi } from 'vitest';
import { loadPublicServerImage, publicServerImageURL } from './publicServerImage';

afterEach(() => {
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
});

describe('publicServerImageURL', () => {
  it('accepts relative and absolute images from the advertised server origin', () => {
    expect(publicServerImageURL('https://chat.example', '/assets/logo.webp')).toBe(
      'https://chat.example/assets/logo.webp'
    );
    expect(
      publicServerImageURL('https://chat.example:8443', 'https://chat.example:8443/banner.png')
    ).toBe('https://chat.example:8443/banner.png');
  });

  it('rejects external, credentialed, and unsupported image URLs', () => {
    expect(
      publicServerImageURL('https://chat.example', 'https://cdn.example/logo.webp')
    ).toBeNull();
    expect(
      publicServerImageURL('https://chat.example', 'https://user@chat.example/logo.webp')
    ).toBeNull();
    expect(publicServerImageURL('https://chat.example', 'data:image/png;base64,AA==')).toBeNull();
    expect(publicServerImageURL('not an origin', '/logo.webp')).toBeNull();
  });
});

describe('loadPublicServerImage', () => {
  it('loads an image without credentials, referrer data, or redirects', async () => {
    let requestSignal: AbortSignal | undefined;
    const browserFetch = vi.fn(async (_input: RequestInfo | URL, init?: RequestInit) => {
      requestSignal = init?.signal ?? undefined;
      return new Response(new Blob(['image'], { type: 'image/png' }), {
        status: 200,
        headers: { 'Content-Type': 'image/png' }
      });
    });
    vi.stubGlobal('fetch', browserFetch);
    const createObjectURL = vi.spyOn(URL, 'createObjectURL').mockReturnValue('blob:public-image');
    const revokeObjectURL = vi.spyOn(URL, 'revokeObjectURL').mockImplementation(() => {});
    const removeAttribute = vi.fn();
    const image = { src: 'stale', removeAttribute } as unknown as HTMLImageElement;

    const cleanup = loadPublicServerImage('https://chat.example/logo.webp')(image);

    await vi.waitFor(() => expect(image.src).toBe('blob:public-image'));
    expect(removeAttribute).toHaveBeenCalledWith('src');
    expect(browserFetch).toHaveBeenCalledWith('https://chat.example/logo.webp', {
      credentials: 'omit',
      redirect: 'error',
      referrerPolicy: 'no-referrer',
      signal: expect.any(AbortSignal)
    });
    expect(createObjectURL).toHaveBeenCalledWith(expect.any(Blob));

    cleanup?.();

    expect(requestSignal?.aborted).toBe(true);
    expect(revokeObjectURL).toHaveBeenCalledWith('blob:public-image');
  });

  it('does not expose an unsupported image response', async () => {
    const browserFetch = vi.fn().mockResolvedValue(
      new Response('<svg xmlns="http://www.w3.org/2000/svg"></svg>', {
        status: 200,
        headers: { 'Content-Type': 'image/svg+xml' }
      })
    );
    vi.stubGlobal('fetch', browserFetch);
    const createObjectURL = vi.spyOn(URL, 'createObjectURL');
    const image = {
      src: '',
      removeAttribute: vi.fn()
    } as unknown as HTMLImageElement;

    loadPublicServerImage('https://chat.example/not-an-image')(image);

    await vi.waitFor(() => expect(browserFetch).toHaveBeenCalledOnce());
    expect(createObjectURL).not.toHaveBeenCalled();
    expect(image.src).toBe('');
  });
});
