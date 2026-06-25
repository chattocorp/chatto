/**
 * Generate deterministic gradients from strings (e.g., space names).
 * Used for fallback backgrounds when no banner image is configured.
 */

// Curated gradient color pairs that look good together in light and dark modes
const GRADIENT_PAIRS = [
  ['#6366f1', '#8b5cf6'], // indigo -> violet
  ['#14b8a6', '#06b6d4'], // teal -> cyan
  ['#f59e0b', '#f97316'], // amber -> orange
  ['#ec4899', '#f43f5e'], // pink -> rose
  ['#22c55e', '#10b981'], // green -> emerald
  ['#3b82f6', '#6366f1'], // blue -> indigo
  ['#8b5cf6', '#ec4899'], // violet -> pink
  ['#f97316', '#ef4444'] // orange -> red
];

function hashString(str: string): number {
  let hash = 0;
  for (let i = 0; i < str.length; i++) {
    const char = str.charCodeAt(i);
    hash = (hash << 5) - hash + char;
    hash = hash & hash; // Convert to 32-bit integer
  }
  return Math.abs(hash);
}

/**
 * Generate a deterministic CSS gradient from a string.
 * The same string will always produce the same gradient.
 */
export function getGradientForName(name: string): string {
  const hash = hashString(name);
  const [color1, color2] = GRADIENT_PAIRS[hash % GRADIENT_PAIRS.length];
  // Vary the gradient angle slightly based on hash for more variety
  const angle = 135 + (hash % 3) * 15; // 135, 150, or 165 degrees
  return `linear-gradient(${angle}deg, ${color1}, ${color2})`;
}
