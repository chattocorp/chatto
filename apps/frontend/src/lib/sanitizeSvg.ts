/** Restrict user-authored diagram previews to a bounded, static SVG subset. */
import createDOMPurify from 'dompurify';

const svgNamespace = 'http://www.w3.org/2000/svg';
const purifier = createDOMPurify(window);

// Paint references may name a local gradient, but must not load another document.
purifier.addHook('uponSanitizeAttribute', (_node, attribute) => {
  if (['fill', 'stroke'].includes(attribute.attrName)) {
    const value = attribute.attrValue.trim();
    attribute.keepAttr =
      value === 'none' || CSS.supports('color', value) || /^url\(#[a-zA-Z_][\w.-]*\)$/.test(value);
  }
});

/**
 * Return a sanitized SVG for an image resource, or throw for invalid/oversized input.
 * Call only in the browser, before creating the image URL. Keep the original source
 * separately for editing. These bounds reduce rendering cost; they are not a CPU limit.
 */
export function sanitizeSvg(source: string): string {
  // Reject declarations before an XML parser can expand entities. The source cap
  // also bounds parsing work before we can inspect the document's structure.
  if (source.length > 100_000 || /<!\s*(?:DOCTYPE|ENTITY)\b/i.test(source)) {
    throw new Error('SVG source exceeds preview policy');
  }
  const document = new DOMParser().parseFromString(source, 'image/svg+xml');
  const root = document.documentElement;
  if (
    document.querySelector('parsererror') ||
    root.localName !== 'svg' ||
    root.namespaceURI !== svgNamespace
  ) {
    throw new Error('Invalid SVG document');
  }
  const pending: { element: Element; depth: number }[] = [{ element: root, depth: 1 }];
  let count = 0;
  while (pending.length) {
    const { element, depth } = pending.pop()!;
    if (++count > 1_000 || depth > 32) throw new Error('SVG structure exceeds preview policy');
    for (const child of element.children) pending.push({ element: child, depth: depth + 1 });
  }

  // An explicit subset excludes use expansion, filters, patterns, embedded images,
  // CSS, HTML and animation. Do not combine ALLOWED_TAGS with USE_PROFILES: the
  // profile would override this allowlist.
  purifier.sanitize(root, {
    IN_PLACE: true,
    ALLOWED_TAGS: [
      'svg',
      'g',
      'defs',
      'title',
      'desc',
      'path',
      'rect',
      'circle',
      'ellipse',
      'line',
      'polygon',
      'polyline',
      'text',
      'tspan',
      'linearGradient',
      'radialGradient',
      'stop'
    ],
    ALLOWED_ATTR: [
      'xmlns',
      'id',
      'viewBox',
      'preserveAspectRatio',
      'width',
      'height',
      'x',
      'y',
      'x1',
      'y1',
      'x2',
      'y2',
      'cx',
      'cy',
      'r',
      'rx',
      'ry',
      'fx',
      'fy',
      'fr',
      'd',
      'points',
      'transform',
      'fill',
      'fill-opacity',
      'fill-rule',
      'stroke',
      'stroke-width',
      'stroke-opacity',
      'stroke-linecap',
      'stroke-linejoin',
      'stroke-miterlimit',
      'stroke-dasharray',
      'stroke-dashoffset',
      'opacity',
      'font-family',
      'font-size',
      'font-weight',
      'font-style',
      'text-anchor',
      'dominant-baseline',
      'dx',
      'dy',
      'rotate',
      'textLength',
      'lengthAdjust',
      'gradientUnits',
      'gradientTransform',
      'spreadMethod',
      'offset',
      'stop-color',
      'stop-opacity'
    ],
    ALLOW_DATA_ATTR: false,
    ALLOW_ARIA_ATTR: false
  });
  return new XMLSerializer().serializeToString(root);
}
