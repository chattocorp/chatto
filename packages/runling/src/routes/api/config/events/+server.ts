import { getConfigReloader } from "$lib/server/web-config.ts";
import { eventStream } from "$lib/server/stream.ts";
export const GET = async ({ request }: { request: Request }) => {
  const config = getConfigReloader();
  await config.load();
  return eventStream(request, (send) => {
    const update = () =>
      send("config", {
        revision: config.revision,
        error: config.error ?? null,
      });
    const unsubscribe = config.subscribe(update);
    update();
    return unsubscribe;
  });
};
