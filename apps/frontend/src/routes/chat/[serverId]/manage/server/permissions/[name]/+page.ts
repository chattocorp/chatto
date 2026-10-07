import { redirect } from '@sveltejs/kit';
import { resolve } from '$app/paths';
import type { PageLoad } from './$types';

/**
 * Earlier releases opened role pages below Permissions. Send those links to
 * the Roles pages: `permissions/new` to `roles/new`, `permissions/[name]` to
 * `roles/[name]`.
 */
export const load: PageLoad = ({ params, url }) => {
  redirect(
    308,
    `${resolve('/chat/[serverId]/manage/server/roles/[name]', {
      serverId: params.serverId,
      name: params.name
    })}${url.search}`
  );
};
