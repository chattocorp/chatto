import { redirect } from '@sveltejs/kit';
import type { PageLoad } from './$types';
import { isSafeInternalPath } from '$lib/navigation/safeInternalPath';

export const load: PageLoad = async ({ parent, url }) => {
  const { user } = await parent();
  const raw = url.searchParams.get('redirect') ?? '';
  const redirectUrl = isSafeInternalPath(raw) ? raw : '/';
  if (user) redirect(302, redirectUrl === '/' ? '/chat' : redirectUrl);
  return {
    /** Safe continuation for signup inside an authorization window. */
    redirectUrl,
    inviteAccepted: url.searchParams.get('invited') === '1',
    inviteError: url.searchParams.get('error') === 'invalid_invitation'
  };
};
