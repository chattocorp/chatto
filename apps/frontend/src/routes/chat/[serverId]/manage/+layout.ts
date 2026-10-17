import { preloadChatLocaleMessages } from '$lib/i18n/messages';
import type { LayoutLoad } from './$types';

/** Await management translations on entry, after shared catalogs and access checks. */
export const load: LayoutLoad = async ({ parent }) => {
  await parent();
  await preloadChatLocaleMessages(true);
  return {};
};
