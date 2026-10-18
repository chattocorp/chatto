/** Select advisory rows from server-resolved role decisions; never resolve user access here. */
import type { MatrixScope, RolePermissionMatrix } from '@chatto/client/api/permissions';

/** Return rooms with posting but no reading grant, or null for incomplete room decisions. */
export function roomsWithoutRead(matrix: RolePermissionMatrix): MatrixScope[] | null {
  const cells = new Map(matrix.cells.map((cell) => [`${cell.scopeId}::${cell.permission}`, cell]));
  const warnings: MatrixScope[] = [];
  for (const scope of matrix.scopes) {
    if (scope.kind !== 'ROOM') continue;
    const post = cells.get(`${scope.id}::message.post`);
    const read = cells.get(`${scope.id}::message.read`);
    // An incomplete response cannot support an all-clear result.
    if (!post || !read) return null;
    if (post.effective === 'ALLOW' && read.effective !== 'ALLOW') warnings.push(scope);
  }
  return warnings;
}
