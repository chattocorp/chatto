/**
 * Font texture for GLSL shader previews, bound as `iChannel0`.
 *
 * The layout follows Shadertoy's font texture: 16 × 16 cells of 64 × 64
 * pixels. ASCII code `c` is in column `c % 16` and row `c / 16`, counted from
 * the top of the image. In texture coordinates, its cell starts at
 * `vec2(c % 16, 15 - c / 16) / 16.0`. The red, green, and blue channels hold
 * glyph coverage. Alpha holds a signed distance field: 0.5 is the glyph edge,
 * smaller values are inside, and one unit spans `2 * SPREAD` pixels.
 *
 * Chatto draws the glyphs at runtime with the bundled IBM Plex Mono font, so
 * the texture needs no asset download. Glyphs share one baseline and the
 * monospace advance, so shaders can lay out text with fixed steps.
 */

export const FONT_ATLAS_SIZE = 1024;
const CELL = FONT_ATLAS_SIZE / 16;
/** Distance in pixels from the edge to the ends of the distance field. */
const SPREAD = 8;
/** Bundled with the frontend. The weight matches a loaded font file. */
export const FONT_ATLAS_FONT = `500 44px "IBM Plex Mono", monospace`;
const BASELINE = 46;
const INF = 1e20;

let atlas: Uint8Array | undefined;

/** Load the atlas font. Call this before the first `fontAtlas()`. */
export async function loadFontAtlasFont(): Promise<void> {
  try {
    await document.fonts.load(FONT_ATLAS_FONT);
  } catch {
    // The fallback monospace font still gives a usable texture.
  }
}

/**
 * RGBA pixels of the font texture, bottom row first, ready for `texImage2D`.
 * The result is computed once and shared.
 */
export function fontAtlas(): Uint8Array {
  if (atlas) return atlas;
  const canvas = document.createElement('canvas');
  canvas.width = canvas.height = FONT_ATLAS_SIZE;
  const context = canvas.getContext('2d', { willReadFrequently: true })!;
  context.font = FONT_ATLAS_FONT;
  context.fillStyle = '#fff';
  context.textAlign = 'center';
  context.textBaseline = 'alphabetic';
  for (let code = 33; code < 127; code++) {
    const x = (code % 16) * CELL + CELL / 2;
    const y = Math.floor(code / 16) * CELL + BASELINE;
    context.fillText(String.fromCharCode(code), x, y);
  }
  const coverage = context.getImageData(0, 0, FONT_ATLAS_SIZE, FONT_ATLAS_SIZE).data;
  const size = FONT_ATLAS_SIZE * FONT_ATLAS_SIZE;
  const outside = new Float64Array(size);
  const inside = new Float64Array(size);
  for (let i = 0; i < size; i++) {
    const filled = coverage[i * 4 + 3] >= 128;
    outside[i] = filled ? 0 : INF;
    inside[i] = filled ? INF : 0;
  }
  distanceTransform(outside);
  distanceTransform(inside);
  const pixels = new Uint8Array(size * 4);
  for (let row = 0; row < FONT_ATLAS_SIZE; row++) {
    // WebGL reads the first row as the bottom of the texture.
    const target = (FONT_ATLAS_SIZE - 1 - row) * FONT_ATLAS_SIZE;
    for (let column = 0; column < FONT_ATLAS_SIZE; column++) {
      const source = row * FONT_ATLAS_SIZE + column;
      const alpha = coverage[source * 4 + 3];
      const distance = Math.sqrt(outside[source]) - Math.sqrt(inside[source]);
      const field = Math.min(Math.max(0.5 + distance / (2 * SPREAD), 0), 1);
      const offset = (target + column) * 4;
      pixels[offset] = pixels[offset + 1] = pixels[offset + 2] = alpha;
      pixels[offset + 3] = Math.round(field * 255);
    }
  }
  atlas = pixels;
  return atlas;
}

/**
 * Replace each value with the squared Euclidean distance to the nearest zero,
 * in place (Felzenszwalb and Huttenlocher, 2012).
 */
function distanceTransform(grid: Float64Array) {
  const n = FONT_ATLAS_SIZE;
  const f = new Float64Array(n);
  const d = new Float64Array(n);
  const v = new Int32Array(n);
  const z = new Float64Array(n + 1);
  for (let x = 0; x < n; x++) {
    for (let y = 0; y < n; y++) f[y] = grid[y * n + x];
    transform1d(f, d, v, z, n);
    for (let y = 0; y < n; y++) grid[y * n + x] = d[y];
  }
  for (let y = 0; y < n; y++) {
    const row = y * n;
    for (let x = 0; x < n; x++) f[x] = grid[row + x];
    transform1d(f, d, v, z, n);
    for (let x = 0; x < n; x++) grid[row + x] = d[x];
  }
}

function transform1d(f: Float64Array, d: Float64Array, v: Int32Array, z: Float64Array, n: number) {
  let k = 0;
  v[0] = 0;
  z[0] = -INF;
  z[1] = INF;
  for (let q = 1; q < n; q++) {
    let s = (f[q] + q * q - (f[v[k]] + v[k] * v[k])) / (2 * q - 2 * v[k]);
    while (s <= z[k]) {
      k--;
      s = (f[q] + q * q - (f[v[k]] + v[k] * v[k])) / (2 * q - 2 * v[k]);
    }
    k++;
    v[k] = q;
    z[k] = s;
    z[k + 1] = INF;
  }
  k = 0;
  for (let q = 0; q < n; q++) {
    while (z[k + 1] < q) k++;
    const offset = q - v[k];
    d[q] = offset * offset + f[v[k]];
  }
}
