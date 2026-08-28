import { redirect } from '@sveltejs/kit';
import type { PageLoad } from './$types';

export const load: PageLoad = async ({ parent, url }) => {
	const { user } = await parent();
	if (!user) redirect(302, `/${url.search}`);

	// The origin segment does not depend on client registration or realtime
	// projection state. The server route owns the subsequent room redirect.
	const welcome = url.searchParams.get('welcome') === 'true';
	redirect(302, welcome ? '/chat/-?welcome=true' : '/chat/-');
};
