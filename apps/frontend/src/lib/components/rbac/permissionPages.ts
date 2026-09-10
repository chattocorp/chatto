import type { MatrixData } from '$lib/api-client/permissions';

/** Join live scope pages. Later copies replace earlier ones after layout changes. */
export function mergePermissionPages<T extends MatrixData>(pages: T[]): T | null {
  if (!pages.length) return null;
  const scopes = new Map<string, T['scopes'][number]>();
  const cells = new Map<string, T['cells'][number]>();
  for (const page of pages) {
    for (const scope of page.scopes) scopes.set(scope.id, scope);
    for (const cell of page.cells) cells.set(`${cell.scopeId}|${cell.permission}`, cell);
  }
  return { ...pages[0], scopes: [...scopes.values()], cells: [...cells.values()] };
}
