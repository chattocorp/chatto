/** Shared geometry for the inline player and its lazy-loading placeholder. */
export function videoFrameStyle(width?: number | null, height?: number | null): string {
  const w = positiveVideoDimension(width) ?? 480;
  const h = positiveVideoDimension(height) ?? 270;
  // Keep controls usable for extreme ratios; the media uses object-fit: contain.
  const canvasWidth = Math.max(w, h * (9 / 16));
  const canvasHeight = Math.max(h, w / (16 / 9));
  const scale = Math.min(480 / canvasWidth, 320 / canvasHeight, 1);
  const displayWidth = Math.max(1, Math.round(canvasWidth * scale));
  const displayHeight = Math.max(1, Math.round(canvasHeight * scale));
  return `width: ${displayWidth}px; max-width: 100%; aspect-ratio: ${displayWidth} / ${displayHeight};`;
}

/** Reject missing or invalid dimensions before choosing a frame fallback. */
export function positiveVideoDimension(value: number | null | undefined): number | null {
  return typeof value === 'number' && Number.isFinite(value) && value > 0 ? value : null;
}
