import { redirect } from '@sveltejs/kit';
import { isSafeInternalPath } from '$lib/navigation/safeInternalPath';
import type { PageLoad } from './$types';

export const load: PageLoad = async ({ parent, url }) => {
  const { user } = await parent();
  const raw = url.searchParams.get('redirect') ?? '';
  const redirectUrl = isSafeInternalPath(raw) ? raw : '/';

  if (user) {
    redirect(302, redirectUrl.startsWith('/oauth/') ? redirectUrl : '/chat');
  }

  return {
    /** URL to redirect to after login (default: /). Must be a same-origin path. */
    redirectUrl,

    /** Known provider/auth redirect error code to render on the login page. */
    loginErrorCode: url.searchParams.get('error') ?? '',

    /** Whether the user just completed a password reset */
    passwordResetSuccess: url.searchParams.get('reset') === 'success'
  };
};
