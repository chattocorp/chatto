import { loadCurrentUser, type CurrentUser } from '$lib/auth/loadAuth';
import type { LayoutLoad } from './$types';

// SPA mode - no server-side rendering
export const ssr = false;

export const load: LayoutLoad = async ({ url }) => {
  // Track route changes so client-side navigation re-checks cookie auth.
  // loadCurrentUser preserves the cached user on transient network errors,
  // but a clean `viewer: null` response must clear authenticated UI.
  const authNavigationKey = url.pathname + url.search;

  // loadCurrentUser handles !browser case internally
  const user = await loadCurrentUser();
  return { user, authNavigationKey };
};

// Re-export for child routes to use in their types
export type { CurrentUser };
