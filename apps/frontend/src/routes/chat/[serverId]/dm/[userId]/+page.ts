import { redirect } from '@sveltejs/kit';
import { resolve } from '$app/paths';
import type { PageLoad } from './$types';

/** Preserve bookmarks for the former DM entry route. */
export const load: PageLoad = ({ params, url }) => {
  redirect(302, `${resolve('/chat/[serverId]/user/[userId]', params)}${url.search}${url.hash}`);
};
