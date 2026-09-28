import { m } from '$lib/i18n/messages';
import type { DirectMessageLabels } from './users';

/** Returns the localized labels that DM names use for the viewer and deleted accounts. */
export function directMessageLabels(): DirectMessageLabels {
  return { currentUser: m('common.you'), deletedUser: m('common.deleted_user') };
}
