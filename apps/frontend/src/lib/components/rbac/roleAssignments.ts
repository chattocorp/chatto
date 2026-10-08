import type { AdminMemberDetails } from '$lib/api/adminUsers';

/**
 * True when the role order keeps a viewer who may assign roles from changing
 * any role of the account.
 *
 * A viewer must outrank another account to change its roles
 * (`viewerOutranks`). For a bot, the server also checks the bot's owner. In
 * that case, the empty assignable and revocable lists from the server tell
 * that the role order blocks the change.
 */
export function roleOrderLocksRoles(
  details: AdminMemberDetails,
  isSelf: boolean,
  viewerOutranks: boolean
): boolean {
  const member = details.member;
  if (!member || isSelf || !details.viewerCanAssignRoles) return false;
  if (!viewerOutranks) return true;
  return (
    member.isBot === true &&
    details.assignableRoleNames?.length === 0 &&
    details.revocableRoleNames?.length === 0
  );
}
