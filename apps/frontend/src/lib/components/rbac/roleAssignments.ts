import type { AdminMemberDetails } from '$lib/api/adminUsers';

/**
 * True when the role order keeps a viewer who may assign roles from changing
 * any role of the account.
 *
 * A viewer must outrank another account to change its roles. A bot owner
 * outranks their bot for other actions (`viewerOutranks`), but role changes on
 * a bot need outranking the bot itself. In that case, the empty assignable and
 * revocable lists from the server tell that the bot ranks at or above the viewer.
 */
export function roleOrderLocksRoles(details: AdminMemberDetails, isSelf: boolean): boolean {
  const member = details.member;
  if (!member || isSelf || !details.viewerCanAssignRoles) return false;
  if (!member.viewerOutranks) return true;
  return (
    member.isBot === true &&
    details.assignableRoleNames?.length === 0 &&
    details.revocableRoleNames?.length === 0
  );
}
