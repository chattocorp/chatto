import { loadCurrentUser, type CurrentUser } from '$lib/auth/loadAuth';
import { preloadActiveLocaleMessages } from '$lib/i18n/messages';
import type { LayoutLoad } from './$types';

// SPA mode - no server-side rendering
export const ssr = false;

export const load: LayoutLoad = async () => {
  await preloadActiveLocaleMessages();

  // loadCurrentUser handles !browser case internally
  const user = await loadCurrentUser();
  return { user };
};

// Re-export for child routes to use in their types
export type { CurrentUser };
