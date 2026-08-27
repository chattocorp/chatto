/**
 * Returns true when a value is safe to navigate to without validating against
 * a server-side allow-list because it points to this origin.
 *
 * This accepts root-relative paths only. It rejects protocol-relative URLs,
 * backslash or control-character variants that URL parsers can normalize to
 * protocol-relative URLs, absolute URLs, schemes such as `javascript:`, and
 * empty values.
 */
export function isSafeInternalPath(value: unknown): value is string {
  if (
    typeof value !== 'string' ||
    !value.startsWith('/') ||
    value.startsWith('//') ||
    value.includes('\\')
  ) {
    return false;
  }
  for (let index = 0; index < value.length; index += 1) {
    const codePoint = value.charCodeAt(index);
    if (codePoint <= 0x1f || codePoint === 0x7f) return false;
  }
  return true;
}
