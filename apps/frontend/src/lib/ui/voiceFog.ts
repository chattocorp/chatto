import { simplexNoise2D as noise } from './simplexNoise';

// Independent scales, paths, and seeds keep the translucent layers from moving
// as one texture. The broad rear veil drifts more slowly than the foreground.
const layers = [
  { scale: 0.65, dx: 0.07, dy: -0.025, opacity: 0.23 },
  { scale: 0.9, dx: -0.11, dy: 0.04, opacity: 0.28 },
  { scale: 1.2, dx: 0.15, dy: 0.015, opacity: 0.32 }
];

/**
 * Fill a small alpha texture with three transparent layers of flowing fog.
 * Distorted simplex noise forms broad wisps that drift through one another.
 * The caller tints and enlarges this mask, avoiding per-pixel work at device resolution.
 */
export function paintVoiceFog(
  pixels: Uint8ClampedArray,
  width: number,
  height: number,
  time: number,
  seed: number
) {
  for (let y = 0; y < height; y++) {
    const v = y / (height - 1);
    for (let x = 0; x < width; x++) {
      const u = x / (width - 1);
      let density = 0;
      for (let layer = 0; layer < layers.length; layer++) {
        const { scale, dx, dy, opacity } = layers[layer];
        const layerSeed = seed + layer * 109;
        const px = u * 2 * scale + time * dx + layer * 7.3;
        const py = v * 1.5 * scale + time * dy + layer * 3.7;
        const warp = noise(px * 0.65 - time * dy, py * 0.6, layerSeed + 11);
        const field = noise(px, py + warp * 0.45, layerSeed);
        const veil = (noise(px * 0.7, py * 0.65 + time * dx * 0.3, layerSeed + 37) + 1) * 0.5;
        // Use filled density, not ridges around zero crossings: those produce
        // bright electrical filaments instead of soft volumes of mist.
        const cloud = Math.max(0, 0.5 + field * 0.65) ** 1.7;
        const alpha = cloud * (0.7 + veil * 0.6) * opacity;
        // Source-over alpha composition, as if these were separate canvases.
        density += alpha * (1 - density);
      }
      // Feather into the card rather than exposing the texture's rectangular edge.
      const edge = Math.sin(Math.PI * v) ** 0.6 * Math.min(1, u * 12, (1 - u) * 12);
      const offset = (y * width + x) * 4;
      pixels[offset] = pixels[offset + 1] = pixels[offset + 2] = 255;
      pixels[offset + 3] = Math.round(density * edge * 255);
    }
  }
}
