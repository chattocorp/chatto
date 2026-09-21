import { getRunStore } from "$lib/server/run-store.ts";
import { loadWebConfig } from "$lib/server/web-config.ts";
import { describeWebhook, handleWebhook } from "$lib/server/webhook.ts";
import type { RequestHandler } from "./$types";

export const GET: RequestHandler = async ({ params }) =>
  describeWebhook(params.name, { config: await loadWebConfig() });

export const POST: RequestHandler = async ({ params, request }) => {
  const store = await getRunStore();
  return handleWebhook(params.name, request, {
    config: await loadWebConfig(),
    start: async (task, { input }) => {
      const { id } = await store.start(params.name, task, input, "webhook");
      return { id };
    },
  });
};
