const skew = (Math.sqrt(3) - 1) / 2;
const unskew = (3 - Math.sqrt(3)) / 6;
const gradients = [
  [1, 1],
  [-1, 1],
  [1, -1],
  [-1, -1],
  [1, 0],
  [-1, 0],
  [0, 1],
  [0, -1]
];

function contribution(ix: number, iy: number, x: number, y: number, seed: number): number {
  const falloff = 0.5 - x * x - y * y;
  if (falloff <= 0) return 0;
  let hash = Math.imul(ix ^ seed ^ Math.imul(iy, 0x9e3779b9), 0x45d9f3b);
  hash = Math.imul(hash ^ (hash >>> 16), 0x45d9f3b);
  const gradient = gradients[(hash ^ (hash >>> 16)) & 7];
  return falloff ** 4 * (gradient[0] * x + gradient[1] * y);
}

/**
 * Seeded two-dimensional simplex noise. Three gradient contributions on a
 * triangular lattice form a continuous field without square grid patterns.
 * The compact falloff keeps transitions smooth across triangle boundaries.
 */
export function simplexNoise2D(x: number, y: number, seed: number): number {
  const s = (x + y) * skew;
  const ix = Math.floor(x + s);
  const iy = Math.floor(y + s);
  const t = (ix + iy) * unskew;
  const dx = x - ix + t;
  const dy = y - iy + t;
  const stepX = dx > dy ? 1 : 0;
  const stepY = 1 - stepX;
  return (
    70 *
    (contribution(ix, iy, dx, dy, seed) +
      contribution(ix + stepX, iy + stepY, dx - stepX + unskew, dy - stepY + unskew, seed) +
      contribution(ix + 1, iy + 1, dx - 1 + 2 * unskew, dy - 1 + 2 * unskew, seed))
  );
}
