import { afterEach, describe, expect, it, vi } from 'vitest';
import {
  loadPublicServerImage,
  MAX_PUBLIC_SERVER_IMAGE_BYTES,
  publicServerImageURL
} from './publicServerImage';

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
      cache: 'no-cache',
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
    let bodyCancelled = false;
    const browserFetch = vi.fn().mockResolvedValue(
      new Response(
        new ReadableStream<Uint8Array>({
          start(controller) {
            controller.enqueue(new TextEncoder().encode('<svg></svg>'));
          },
          cancel() {
            bodyCancelled = true;
          }
        }),
        {
          status: 200,
          headers: { 'Content-Type': 'image/svg+xml' }
        }
      )
    );
    vi.stubGlobal('fetch', browserFetch);
    const createObjectURL = vi.spyOn(URL, 'createObjectURL');
    const image = {
      src: '',
      dispatchEvent: vi.fn(),
      removeAttribute: vi.fn()
    } as unknown as HTMLImageElement;

    loadPublicServerImage('https://chat.example/not-an-image')(image);

    await vi.waitFor(() => expect(bodyCancelled).toBe(true));
    expect(createObjectURL).not.toHaveBeenCalled();
    expect(image.src).toBe('');
    expect(image.dispatchEvent).toHaveBeenCalledWith(expect.objectContaining({ type: 'error' }));
  });

  it('stops reading an image that exceeds the public-image byte limit', async () => {
    let bodyCancelled = false;
    const browserFetch = vi.fn().mockResolvedValue(
      new Response(
        new ReadableStream<Uint8Array>({
          start(controller) {
            controller.enqueue(new Uint8Array(MAX_PUBLIC_SERVER_IMAGE_BYTES + 1));
          },
          cancel() {
            bodyCancelled = true;
          }
        }),
        {
          status: 200,
          headers: { 'Content-Type': 'image/png' }
        }
      )
    );
    vi.stubGlobal('fetch', browserFetch);
    const createObjectURL = vi.spyOn(URL, 'createObjectURL');
    const image = {
      src: '',
      dispatchEvent: vi.fn(),
      removeAttribute: vi.fn()
    } as unknown as HTMLImageElement;

    loadPublicServerImage('https://chat.example/oversized.png')(image);

    await vi.waitFor(() => expect(bodyCancelled).toBe(true));
    expect(createObjectURL).not.toHaveBeenCalled();
    expect(image.src).toBe('');
    expect(image.dispatchEvent).toHaveBeenCalledWith(expect.objectContaining({ type: 'error' }));
  });

  it('shows a fallback on network failure but ignores a disposed request', async () => {
    let rejectRequest!: (reason: Error) => void;
    vi.stubGlobal(
      'fetch',
      vi.fn(
        () =>
          new Promise<Response>((_, reject) => {
            rejectRequest = reject;
          })
      )
    );
    const image = {
      removeAttribute: vi.fn(),
      dispatchEvent: vi.fn()
    } as unknown as HTMLImageElement;
    loadPublicServerImage('https://chat.example/logo.png')(image);
    rejectRequest(new Error('Network unavailable'));
    await vi.waitFor(() => expect(image.dispatchEvent).toHaveBeenCalledOnce());

    vi.mocked(image.dispatchEvent).mockClear();
    const cleanup = loadPublicServerImage('https://chat.example/logo.png')(image);
    cleanup?.();
    rejectRequest(new Error('Request aborted'));
    await Promise.resolve();
    expect(image.dispatchEvent).not.toHaveBeenCalled();
  });
});
