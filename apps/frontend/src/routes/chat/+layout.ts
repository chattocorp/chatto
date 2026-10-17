import { preloadChatLocaleMessages } from '$lib/i18n/messages';
import type { LayoutLoad } from './$types';

/** Load shared chat sections without depending on child route changes. */
export const load: LayoutLoad = async ({ parent }) => {
  // The root layout selects the locale and loads the public catalog first.
  await parent();
  await preloadChatLocaleMessages();

  return {};
};
