/**
 * Stable profile-band hue for a user. The same user ID always gives the same
 * hue on every client, so the band helps people recognize a profile at a glance.
 * The theme supplies lightness and chroma through `--profile-band-lightness` and
 * `--profile-band-chroma`.
 */
export function profileBandHue(userId: string): number {
  // 32-bit FNV-1a over UTF-16 code units: fast, dependency-free, and well spread.
  let hash = 0x811c9dc5;
  for (let index = 0; index < userId.length; index++) {
    hash ^= userId.charCodeAt(index);
    hash = Math.imul(hash, 0x01000193);
  }
  return (hash >>> 0) % 360;
}

/** Inline background colour for a user's profile band in the active theme. */
export function profileBandColor(userId: string): string {
  return `oklch(var(--profile-band-lightness) var(--profile-band-chroma) ${profileBandHue(userId)})`;
}
