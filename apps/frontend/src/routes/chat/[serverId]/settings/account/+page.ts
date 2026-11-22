import { redirect } from '@sveltejs/kit';
import { resolve } from '$app/paths';
import { hasIdentityLinkParams } from '$lib/auth/identityLink';
import type { PageLoad } from './$types';

/**
 * Older clients open this page with link parameters to link a provider on
 * this server, and older link starts return here. Forward both to the
 * identity-link window, which shows the link steps without the chat shell.
 */
export const load: PageLoad = ({ url }) => {
  if (hasIdentityLinkParams(url)) redirect(302, resolve('/sso/link') + url.search);
};
