import { redirect } from '@sveltejs/kit';
import { isSafeInternalPath } from '$lib/navigation/safeInternalPath';
import type { PageLoad } from './$types';
import { serverRegistry } from '$lib/client';

export const load: PageLoad = async ({ parent, url }) => {
  const { user } = await parent();
  const raw = url.searchParams.get('redirect') ?? '';
  const redirectUrl = isSafeInternalPath(raw) ? raw : '/';

  if (user) {
    redirect(302, redirectUrl === '/' ? '/chat' : redirectUrl);
  }

  // A standalone host has no local account to sign in to. Keep explicit
  // server-owned continuations on this route if origin discovery is unavailable.
  if (!serverRegistry.originServer && redirectUrl === '/') redirect(302, '/chat/servers');

  return {
    /** URL to redirect to after login (default: /). Must be a same-origin path. */
    redirectUrl,

    /** Known provider/auth redirect error code to render on the login page. */
    loginErrorCode: url.searchParams.get('error') ?? '',

    /** Whether the user just completed a password reset */
    passwordResetSuccess: url.searchParams.get('reset') === 'success'
  };
};
