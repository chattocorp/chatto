import { preloadHelpLocaleMessages } from '$lib/i18n/messages';
import type { LayoutLoad } from './$types';

/** The parent settles the shell locale before the lazy help catalog loads. */
export const load: LayoutLoad = async ({ parent }) => {
  await parent();
  await preloadHelpLocaleMessages();
};
