import { preloadChatLocaleMessages } from '$lib/i18n/messages';
import type { LayoutLoad } from './$types';

/** Load the complete selected locale at the authenticated application boundary. */
export const load: LayoutLoad = async ({ parent }) => {
  // The root layout selects the locale and loads the public catalog first.
  await parent();
  await preloadChatLocaleMessages();

  return {};
};
