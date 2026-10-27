import { redirect } from '@sveltejs/kit';
import { resolve } from '$app/paths';
import type { PageLoad } from './$types';

/** Preserve old deletion links by sending them to the account controls. */
export const load: PageLoad = ({ params }) => {
  redirect(
    307,
    resolve('/chat/[serverId]/manage/server/members/[userId]/(sections)/account', params)
  );
};
