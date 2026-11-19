import { getRunStore } from '$lib/server/run-store.ts';
import type { RequestHandler } from './$types';

export const GET: RequestHandler = async ({ params, url }) => {
  const store = await getRunStore();
  const run = await (url.searchParams.get('view') === 'console'
    ? store.console(params.id)
    : store.get(params.id));
  return run ? Response.json(run) : Response.json({ error: 'Run not found.' }, { status: 404 });
};
