/** Translate the app's shell-lighting tokens into an isolated SVG image. */
export function diagramDepth(computed: CSSStyleDeclaration): string {
  function number(name: string, max: number): number {
    const value = Number.parseFloat(computed.getPropertyValue(name));
    return Number.isFinite(value) ? Math.min(max, Math.max(0, value)) : 0;
  }
  const strength = number('--depth-strength', 1.75);
  if (strength === 0) return '';
  const width = number('--depth-width', 1.5);
  const highlight = number('--shell-highlight', 1) * strength * 100;
  const shade = number('--shell-shade', 1) * strength * 100;
  const edgeHighlight = number('--shell-edge-highlight', 1) * strength;
  const edgeShade = number('--shell-edge-shade', 1) * strength;
  const blur = number('--shell-edge-blur', 2) * width;
  // These colours include the active depth strength and light/dark frame tuning.
  const colour = (name: string) => {
    const value = computed.getPropertyValue(name).trim();
    return CSS.supports('color', value) && !/[;{}<>]/u.test(value) ? value : 'transparent';
  };
  // Insets light only the shape, not its label. No outer shadow or extra layout
  // space: this mirrors shell-lighting, whose owners choose elevation separately.
  return `<defs>
    <linearGradient id="chatto-pele-frame-fill" x1="0" y1="0" x2="0" y2="1">
      <stop offset="0" stop-color="color-mix(in srgb, var(--pele-surface), white ${6 * strength}%)"/>
      <stop offset="45%" stop-color="var(--pele-surface)"/>
      <stop offset="1" stop-color="var(--pele-surface)"/>
    </linearGradient>
    <linearGradient id="chatto-pele-frame-shade" x1="0" y1="0" x2="0" y2="1">
      <stop offset="45%" stop-color="transparent"/>
      <stop offset="1" stop-color="${colour('--frame-shade')}"/>
    </linearGradient>
    <filter id="chatto-pele-frame-inset" x="-5%" y="-10%" width="110%" height="120%" color-interpolation-filters="sRGB">
      <feOffset in="SourceAlpha" dy="${width}" result="lower"/>
      <feComposite in="SourceAlpha" in2="lower" operator="out" result="edge"/>
      <feGaussianBlur in="edge" stdDeviation="${2 * width}" result="softEdge"/>
      <feFlood flood-color="${colour('--frame-edge-shade')}"/>
      <feComposite in2="softEdge" operator="in" result="shadow"/>
      <feMerge><feMergeNode in="SourceGraphic"/><feMergeNode in="shadow"/></feMerge>
    </filter>
    <linearGradient id="chatto-pele-depth-fill" x1="0" y1="0" x2="0" y2="1">
      <stop offset="0" stop-color="color-mix(in srgb, var(--pele-surface), white ${highlight}%)"/>
      <stop offset="55%" stop-color="var(--pele-surface)"/>
      <stop offset="1" stop-color="color-mix(in srgb, var(--pele-surface), black ${shade}%)"/>
    </linearGradient>
    <filter id="chatto-pele-depth-edge" x="-5%" y="-10%" width="110%" height="120%" color-interpolation-filters="sRGB">
      <feOffset in="SourceAlpha" dy="${width}" result="lower"/>
      <feComposite in="SourceAlpha" in2="lower" operator="out" result="top"/>
      <feGaussianBlur in="top" stdDeviation="${blur}" result="softTop"/>
      <feFlood flood-color="white" flood-opacity="${edgeHighlight}"/>
      <feComposite in2="softTop" operator="in" result="light"/>
      <feOffset in="SourceAlpha" dy="-${width}" result="upper"/>
      <feComposite in="SourceAlpha" in2="upper" operator="out" result="bottom"/>
      <feGaussianBlur in="bottom" stdDeviation="${blur}" result="softBottom"/>
      <feFlood flood-color="black" flood-opacity="${edgeShade}"/>
      <feComposite in2="softBottom" operator="in" result="shade"/>
      <feMerge><feMergeNode in="SourceGraphic"/><feMergeNode in="light"/><feMergeNode in="shade"/></feMerge>
    </filter>
  </defs><style>
    .chatto-pele-frame { fill: url(#chatto-pele-frame-fill); }
    .chatto-pele-frame-shade { fill: url(#chatto-pele-frame-shade); }
    .chatto-pele-inset { filter: url(#chatto-pele-frame-inset); }
    .pele-node > :is(rect, polygon, path, circle, ellipse)[fill="var(--_s)"] {
      fill: url(#chatto-pele-depth-fill);
      filter: url(#chatto-pele-depth-edge);
    }
  </style>`;
}
