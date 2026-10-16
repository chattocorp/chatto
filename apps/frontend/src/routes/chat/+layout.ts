import { preloadChatLocaleMessages } from '$lib/i18n/messages';
import type { LayoutLoad } from './$types';

/** Load only the sections required by the target route before it renders. */
export const load: LayoutLoad = async ({ parent, route }) => {
  // The root layout selects the locale and loads the public catalog first.
  await parent();
  // Reading route.id also reruns this load when leaving management for chat.
  const management = route.id?.startsWith('/chat/[serverId]/manage') ?? false;
  await preloadChatLocaleMessages(management);

  return {};
};
