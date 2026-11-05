import { redirect } from '@sveltejs/kit';
import { resolve } from '$app/paths';
import type { PageLoad } from './$types';

/** Preserve links to the former role management location. */
export const load: PageLoad = ({ params, url }) => {
  redirect(
    308,
    `${resolve('/chat/[serverId]/manage/server/roles/[name]', { serverId: params.serverId, name: params.name })}${url.search}`
  );
};
