import { redirect } from '@sveltejs/kit';
import {
  isReturnNavigationInProgress,
  takeReturnNavigationTarget
} from '$lib/auth/returnNavigation';
import type { PageLoad } from './$types';

export const load: PageLoad = async ({ parent, url }) => {
	const { user } = await parent();
	if (!user) redirect(302, `/${url.search}`);

	// Take return navigation before choosing the default landing route. This
	// keeps the route load from racing an authentication redirect.
	if (isReturnNavigationInProgress()) return {};
	const returnTarget = takeReturnNavigationTarget();
	if (returnTarget?.startsWith('/oauth/')) {
		window.location.href = returnTarget;
		return {};
	}
	if (returnTarget && returnTarget !== url.pathname + url.search) redirect(302, returnTarget);

	// The origin segment does not depend on client registration or realtime
	// projection state. The server route owns the subsequent room redirect.
	const welcome = url.searchParams.get('welcome') === 'true';
	redirect(302, welcome ? '/chat/-?welcome=true' : '/chat/-');
};
