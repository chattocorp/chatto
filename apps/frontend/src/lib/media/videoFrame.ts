/**
 * Shared geometry for the inline player and its loading placeholder.
 * GIF loops use the source proportions, with a 24 px minimum for the rim. Other
 * videos reserve a small canvas for controls only when the media is too thin.
 * Apply the result to a preview-frame with preview-media-frame so its first
 * child keeps the media ratio inside the rim.
 */
export function videoFrameStyle(
  width?: number | null,
  height?: number | null,
  autoLoop = false
): string {
  const w = positiveVideoDimension(width) ?? 480;
  const h = positiveVideoDimension(height) ?? 270;
  const scale = Math.min(480 / w, 320 / h, 1);
  const displayWidth = Math.max(autoLoop ? 24 : 180, Math.round(w * scale));
  const displayHeight = Math.max(autoLoop ? 24 : 128, Math.round(h * scale));
  return `width: ${displayWidth}px; max-width: 100%; --preview-media-ratio: ${displayWidth} / ${displayHeight};`;
}

/** Reject missing or invalid dimensions before choosing a frame fallback. */
export function positiveVideoDimension(value: number | null | undefined): number | null {
  return typeof value === 'number' && Number.isFinite(value) && value > 0 ? value : null;
}
