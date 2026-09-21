import { getRunStore } from "$lib/server/run-store.ts";
import { eventStream } from "$lib/server/stream.ts";
import type { RequestHandler } from "./$types";

export const GET: RequestHandler = async ({ params, request }) => {
  const store = await getRunStore();
  const run = await store.get(params.id);
  if (!run)
    return Response.json({ error: "Run not found." }, { status: 404 });
  return eventStream(request, (send) => {
    const unsubscribe = store.subscribe((id, record) => {
      if (id === params.id) send("record", record);
    });
    // A new snapshot on reconnect includes any events missed while disconnected.
    send("snapshot", run);
    return unsubscribe;
  });
};
