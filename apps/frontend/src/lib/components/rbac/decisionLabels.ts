import { m } from '$lib/i18n/messages';
import type { CellState, MatrixScopeKind } from './SubjectPermissionsMatrix.svelte';

/**
 * Returns the decision as a word inside a sentence, such as the "allow" in
 * "Override allow for Moderator on room.join". Neutral decisions read as
 * "No decision".
 */
export function decisionWord(decision: CellState): string {
  if (decision === 'allow') return m('rbac.permissions.allow');
  if (decision === 'deny') return m('rbac.permissions.deny');
  return m('rbac.permissions.no_decision');
}

/** Returns the decision as a standalone label, such as "Allow" in a tooltip. */
export function decisionTitle(decision: CellState): string {
  if (decision === 'allow') return m('rbac.permissions.cell.allow');
  if (decision === 'deny') return m('rbac.permissions.cell.deny');
  return m('rbac.permissions.no_decision');
}

/** Returns the translated name of a permission scope level. */
export function scopeKindLabel(kind: MatrixScopeKind): string {
  switch (kind) {
    case 'SERVER':
      return m('rbac.permissions.level_server');
    case 'GROUP':
      return m('rbac.permissions.level_group');
    case 'ROOM':
      return m('rbac.permissions.level_room');
    case 'DM':
      return m('rbac.permissions.level_dm');
  }
}
