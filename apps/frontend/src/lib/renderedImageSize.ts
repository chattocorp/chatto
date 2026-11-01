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

/** Both loading placeholders and loaded previews use the same responsive geometry. */
export function renderedImageWidth({ width, height }: RenderedImageSize): string {
  return `min(${width}px, calc(min(30rem, 60vh) * ${width / height}))`;
}
