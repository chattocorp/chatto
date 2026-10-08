import type { AdminMemberDetails } from '$lib/api/adminUsers';

/**
 * True when the role order keeps a viewer who may assign roles from changing
 * any role of the account.
 *
 * A viewer must outrank another account to change its roles
 * (`viewerOutranks`).
 */
export function roleOrderLocksRoles(
  details: AdminMemberDetails,
  isSelf: boolean,
  viewerOutranks: boolean
): boolean {
  const member = details.member;
  if (!member || isSelf || !details.viewerCanAssignRoles) return false;
  return !viewerOutranks;
}
