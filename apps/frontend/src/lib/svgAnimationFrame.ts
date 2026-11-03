/** Policy that must be present before author content enters the playback frame. */
export const SVG_ANIMATION_FRAME_POLICY =
  "default-src 'none'; style-src 'unsafe-inline'; base-uri 'none'; form-action 'none'";

/**
 * Fixed host document for SVG playback. Scripts, network requests, nested frames,
 * forms and base URLs are blocked. Inline CSS belongs only to this host document;
 * sanitizeSvg removes all author CSS before its XML tree enters the frame.
 */
export const SVG_ANIMATION_FRAME_DOCUMENT = `<!doctype html><html><head><meta http-equiv="Content-Security-Policy" content="${SVG_ANIMATION_FRAME_POLICY}"><style>html,body{margin:0;width:100%;height:100%;overflow:hidden}body{display:flex;align-items:center;justify-content:center}body>svg{display:block}</style></head><body></body></html>`;

/** Encode SVG for display or download. The caller must sanitize the source first. */
export function svgDataUrl(source: string): string {
  return `data:image/svg+xml;charset=utf-8,${encodeURIComponent(source)}`;
}

/**
 * Return a static poster from sanitized SVG. Dimension measurement must not run
 * a second animation timeline in a hidden image. Keep the animated source for
 * playback and download.
 */
export function svgAnimationPoster(source: string): string {
  const document = new DOMParser().parseFromString(source, 'image/svg+xml');
  for (const animation of document.querySelectorAll('animate, animateTransform'))
    animation.remove();
  return new XMLSerializer().serializeToString(document.documentElement);
}
