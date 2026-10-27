/** Attribute-free inline tags supported in Chatto message bodies. */
export const TEXT_DECORATIONS = [
  'color-red',
  'color-orange',
  'color-yellow',
  'color-green',
  'color-blue',
  'color-purple',
  'text-glow',
  'text-rainbow',
  'text-wave',
  'text-shake'
] as const;

/** A portable message tag, independent of the host's visual treatment. */
export type TextDecoration = (typeof TEXT_DECORATIONS)[number];

/** Check the exact, case-sensitive tag vocabulary; attributes are not supported. */
export function isTextDecoration(value: string): value is TextDecoration {
  return (TEXT_DECORATIONS as readonly string[]).includes(value);
}
