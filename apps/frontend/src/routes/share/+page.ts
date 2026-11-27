import { preloadChatLocaleMessages } from '$lib/i18n/messages';
import type { PageLoad } from './$types';

/** Load conversation labels for the picker after startup selects the locale. */
export const load: PageLoad = async ({ parent }) => {
  await parent();
  await preloadChatLocaleMessages();
};
