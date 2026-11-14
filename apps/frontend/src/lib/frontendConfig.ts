/**
 * Frontend settings that the serving Chatto server supplies. The server
 * writes them as meta elements into the app document when it serves the
 * bundled frontend. The standalone frontend and the desktop app get no
 * meta elements and use the defaults below.
 */

/** Meta element name that `cli/internal/http_server/frontend_config.go` writes. */
const GUTTER_META_NAME = 'chatto-gutter';

/**
 * Whether this frontend shows the Server Gutter and the Server Directory, so
 * users can add and switch between servers. A Chatto server that serves the
 * bundled frontend sets this from `frontend.gutter.enabled`, which is off by
 * default. Without the meta element, for example in the standalone frontend
 * and the desktop app, the gutter stays on.
 */
export function readGutterEnabled(doc: Document | undefined): boolean {
  const meta = doc?.querySelector<HTMLMetaElement>(`meta[name="${GUTTER_META_NAME}"]`);
  return meta?.content !== 'false';
}

/** The value for this page load. The document does not change it later. */
export const gutterEnabled = readGutterEnabled(
  typeof document === 'undefined' ? undefined : document
);
