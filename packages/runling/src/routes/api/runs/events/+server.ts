import { getRunStore } from "$lib/server/run-store.ts";
import { eventStream } from "$lib/server/stream.ts";
import type { RequestHandler } from "./$types";

export const GET: RequestHandler = async ({ request }) => {
  const store = await getRunStore();
  return eventStream(request, (send) => {
    let timer: ReturnType<typeof setTimeout> | undefined;
    const unsubscribe = store.subscribe(() => {
      timer ??= setTimeout(() => {
        timer = undefined;
        send("runs", store.list());
      }, 100);
    });
    send("runs", store.list());
    return () => {
      clearTimeout(timer);
      unsubscribe();
    };
  });
};
