/** Retain only bounded image geometry so virtualized diagrams do not collapse on remount. */
import { sha256 } from 'js-sha256';

export type RenderedImageSize = { width: number; height: number };
const sizes = new Map<string, RenderedImageSize>();
const maximumEntries = 256;

/** No message source, SVG, or image URL is retained in this cache. */
export function renderedImageSizeKey(language: string, source: string): string {
  return sha256(`${language}:${source}`);
}

/** Return the last intrinsic dimensions and keep recently used entries. */
export function getRenderedImageSize(key: string): RenderedImageSize | undefined {
  const size = sizes.get(key);
  if (size) {
    sizes.delete(key);
    sizes.set(key, size);
  }
  return size;
}

/** Keep successful dimensions only; failed images must not replace known geometry. */
export function rememberRenderedImageSize(key: string, image: HTMLImageElement): void {
  const { naturalWidth: width, naturalHeight: height } = image;
  if (!width || !height) return;
  sizes.delete(key);
  sizes.set(key, { width, height });
  if (sizes.size > maximumEntries) sizes.delete(sizes.keys().next().value!);
}

/** Fill the message width up to the height limit, also for small vector images. */
export function renderedImageWidth({ width, height }: RenderedImageSize): string {
  return `calc(min(30rem, 60vh) * ${width / height})`;
}
