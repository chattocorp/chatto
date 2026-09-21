import { getRunStore } from "$lib/server/run-store.ts";
import type { RequestHandler } from "./$types";

export const GET: RequestHandler = async ({ params }) => {
  const run = await (await getRunStore()).get(params.id);
  return run
    ? Response.json(run)
    : Response.json({ error: "Run not found." }, { status: 404 });
};
