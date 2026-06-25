import { redirect } from '@sveltejs/kit';
import type { PageLoad } from './$types';

export const load: PageLoad = async ({ parent, url }) => {
	const { user } = await parent();
	if (!user) redirect(302, `/${url.search}`);

	// Pass through welcome query param if present.
	return {
		welcome: url.searchParams.get('welcome') === 'true'
	};
};
