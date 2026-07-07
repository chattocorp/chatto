/**
 * Returns true when a value is safe to navigate to without validating against
 * a server-side allow-list because it points to this origin.
 *
 * This accepts root-relative paths only. It rejects protocol-relative URLs,
 * backslash variants that some routers normalize to protocol-relative URLs,
 * absolute URLs, schemes such as `javascript:`, and empty values.
 */
export function isSafeInternalPath(value: unknown): value is string {
  return (
    typeof value === 'string' &&
    value.startsWith('/') &&
    !value.startsWith('//') &&
    !value.startsWith('/\\')
  );
}
