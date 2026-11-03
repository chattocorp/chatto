/** Restrict diagram previews to bounded shapes and simple SVG animations. */
import createDOMPurify from 'dompurify';
import { MAX_SVG_SOURCE_LENGTH } from './svgPolicy';

const svgNamespace = 'http://www.w3.org/2000/svg';
const purifier = createDOMPurify(window);

function isPaintColor(value: string): boolean {
  // CSS.supports accepts unresolved substitutions, whose fallback could be a
  // resource URL when used as SVG paint. Keep colours independent of CSS input.
  return (
    !/\\|\/\*|\b(?:var|env|attr)\s*\(/i.test(value) &&
    (value === 'none' || CSS.supports('color', value))
  );
}

const numericAnimationTargets = new Set([
  'x',
  'y',
  'cx',
  'cy',
  'r',
  'rx',
  'ry',
  'width',
  'height',
  'opacity',
  'fill-opacity',
  'stroke-opacity',
  'stroke-width',
  'stroke-dashoffset'
]);
const animationAttributes = new Set([
  'attributeName',
  'type',
  'from',
  'to',
  'by',
  'values',
  'dur',
  'begin',
  'repeatCount',
  'calcMode',
  'fill'
]);

/** Accept clock offsets only. Event and cross-element timing are not supported. */
function clockSeconds(value: string): number {
  const match = /^(\d+(?:\.\d+)?|\.\d+)(ms|s)$/.exec(value);
  return match ? Number(match[1]) / (match[2] === 'ms' ? 1000 : 1) : NaN;
}

/** Validate animated values before they can change an already sanitized shape. */
function isAllowedAnimation(element: Element): boolean {
  const target = element.getAttribute('attributeName') ?? '';
  const transform = element.localName === 'animateTransform';
  const type = element.getAttribute('type') ?? '';
  if (
    transform
      ? target !== 'transform' || !['translate', 'scale', 'rotate'].includes(type)
      : !numericAnimationTargets.has(target) && !['fill', 'stroke'].includes(target)
  )
    return false;
  const duration = clockSeconds(element.getAttribute('dur') ?? '');
  if (!(duration >= 0.1 && duration <= 3600)) return false;
  if (element.hasAttribute('begin')) {
    const begin = clockSeconds(element.getAttribute('begin')!);
    if (!(begin >= 0 && begin <= 3600)) return false;
  }
  const repeat = element.getAttribute('repeatCount');
  if (repeat !== null && repeat !== 'indefinite' && !/^(?:[1-9]\d?|100)$/.test(repeat))
    return false;
  if (
    element.hasAttribute('calcMode') &&
    !['linear', 'discrete'].includes(element.getAttribute('calcMode')!)
  )
    return false;
  if (element.hasAttribute('fill') && !['freeze', 'remove'].includes(element.getAttribute('fill')!))
    return false;
  const values = [
    ...(element.hasAttribute('values') ? element.getAttribute('values')!.split(';') : []),
    ...['from', 'to', 'by'].flatMap((name) =>
      element.hasAttribute(name) ? [element.getAttribute(name)!] : []
    )
  ];
  if (!values.length || values.length > 64) return false;
  return values.every((value) => {
    value = value.trim();
    if (!transform && ['fill', 'stroke'].includes(target)) return isPaintColor(value);
    const numbers = value.split(/[\s,]+/);
    const lengths = transform ? (type === 'rotate' ? [1, 3] : [1, 2]) : [1];
    return (
      lengths.includes(numbers.length) &&
      numbers.every(
        (number) =>
          /^[-+]?(?:\d+(?:\.\d*)?|\.\d+)(?:e[-+]?\d+)?$/i.test(number) &&
          Math.abs(Number(number)) <= 8192
      )
    );
  });
}

// Paint references may name a local gradient, but must not load another document.
purifier.addHook('uponSanitizeAttribute', (node, attribute) => {
  if (['animate', 'animateTransform'].includes(node.localName) && attribute.attrName === 'fill')
    return;
  if (['fill', 'stroke'].includes(attribute.attrName)) {
    const value = attribute.attrValue.trim();
    // Match paired quotes and an optional fallback colour, but never an external URL.
    const reference = /^url\(\s*(["']?)#[a-zA-Z_][\w.-]*\1\s*\)(?:\s+(.+))?$/i.exec(value);
    attribute.keepAttr =
      isPaintColor(value) || (reference !== null && (!reference[2] || isPaintColor(reference[2])));
  }
});

/**
 * Return SVG for an image or scriptless playback frame, or throw for invalid input.
 * Call only in the browser, before creating the image URL or frame content.
 * Keep the original source for editing. These bounds reduce rendering cost;
 * they are not a CPU limit.
 */
export function sanitizeSvg(source: string): string {
  // Reject declarations before an XML parser can expand entities. The source cap
  // also bounds parsing work before we can inspect the document's structure.
  if (source.length > MAX_SVG_SOURCE_LENGTH || /<!\s*(?:DOCTYPE|ENTITY)\b/i.test(source)) {
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
  let animations = 0;
  while (pending.length) {
    const { element, depth } = pending.pop()!;
    if (++count > 1_000 || depth > 32) throw new Error('SVG structure exceeds preview policy');
    for (const child of element.children) pending.push({ element: child, depth: depth + 1 });
    if (['animate', 'animateTransform'].includes(element.localName)) {
      if (++animations > 32 || !isAllowedAnimation(element)) {
        element.remove();
      } else {
        // Animation targets stay on the parent shape. Drop references and all
        // attributes outside this subset before DOMPurify checks the tree.
        for (const attribute of [...element.attributes]) {
          if (!animationAttributes.has(attribute.name)) element.removeAttributeNode(attribute);
        }
      }
    }
  }

  // An explicit subset excludes use expansion, filters, patterns, embedded images,
  // CSS and HTML. Do not combine ALLOWED_TAGS with USE_PROFILES: the
  // profile would override this allowlist.
  purifier.sanitize(root, {
    IN_PLACE: true,
    // IDs enter only an image or a scriptless sandbox, never the application DOM.
    // Preserve gradient IDs such as "title" and "images" within that boundary.
    SANITIZE_DOM: false,
    ALLOWED_TAGS: [
      'svg',
      'g',
      'defs',
      'title',
      'desc',
      'animate',
      'animateTransform',
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
      'attributeName',
      'type',
      'from',
      'to',
      'by',
      'values',
      'dur',
      'begin',
      'repeatCount',
      'calcMode',
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
