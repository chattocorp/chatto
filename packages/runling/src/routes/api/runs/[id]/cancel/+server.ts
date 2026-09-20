import { getRunStore } from "$lib/server/run-store.ts";
import type { RequestHandler } from "./$types";

export const POST: RequestHandler = async ({ params }) => {
  const store = await getRunStore();
  if (store.cancel(params.id)) {
    return Response.json({ accepted: true }, { status: 202 });
  }
  const run = await store.get(params.id);
  return run
    ? Response.json({ error: "This run is no longer running." }, { status: 409 })
    : Response.json({ error: "Run not found." }, { status: 404 });
};
