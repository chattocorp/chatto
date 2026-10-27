import { runtime } from '$lib/server/runtime.ts';
const { ChannelClosedError, ChannelFullError } = runtime;
import { getRunStore, RunNotActiveError } from '$lib/server/run-store.ts';
import type { RequestHandler } from './$types';

/** Send one JSON message to an active run's top-level task. */
export const POST: RequestHandler = async ({ params, request }) => {
  const store = await getRunStore();
  let message: unknown;
  try {
    ({ message } = (await request.json()) as { message: unknown });
  } catch {
    return Response.json({ error: 'The request body must be valid JSON.' }, { status: 400 });
  }
  try {
    await store.send(params.id, message);
    return Response.json({ accepted: true }, { status: 202 });
  } catch (error) {
    if (error instanceof RunNotActiveError)
      return Response.json({ error: error.message }, { status: 409 });
    if (error instanceof ChannelClosedError)
      return Response.json({ error: 'The run does not accept more messages.' }, { status: 409 });
    if (error instanceof ChannelFullError)
      return Response.json({ error: 'The run has too many unread messages.' }, { status: 503 });
    if (error instanceof TypeError) return Response.json({ error: error.message }, { status: 400 });
    throw error;
  }
};
