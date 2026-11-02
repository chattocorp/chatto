/** Detached, loaded previews follow message identity across virtualized row remounts. */
const images = new WeakMap<object, Map<string, HTMLImageElement>>();
// Markdown permits four SVG and four Mermaid previews per message.
const maximumEntries = 8;

/** Check without claiming the image, so source tabs can remain lazy. */
export function hasRenderedImage(owner: object, key: string): boolean {
  return images.get(owner)?.has(key) ?? false;
}

/** Claim exclusive use: simultaneous copies of a message must not move each other's DOM. */
export function takeRenderedImage(owner: object, key: string): HTMLImageElement | undefined {
  const entries = images.get(owner);
  const image = entries?.get(key);
  entries?.delete(key);
  return image;
}

/** Call after canvas cleanup, when no old listeners or parent nodes remain. */
export function rememberRenderedImage(owner: object, key: string, image: HTMLImageElement): void {
  if (image.isConnected || !image.complete || !image.naturalWidth) return;
  let entries = images.get(owner);
  if (!entries) images.set(owner, (entries = new Map()));
  entries.delete(key);
  entries.set(key, image);
  if (entries.size > maximumEntries) entries.delete(entries.keys().next().value!);
}
