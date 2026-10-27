import { getRunStore } from '$lib/server/run-store.ts';
import { loadWebConfig } from '$lib/server/web-config.ts';
import { handleWebhook } from '$lib/server/webhook.ts';
import type { RequestHandler } from './$types';

export const POST: RequestHandler = async ({ params, request }) => {
  const store = await getRunStore();
  return handleWebhook(params.name, request, {
    config: await loadWebConfig(),
    start: (task, { input, runId }) => store.start(params.name, task, input, 'web', runId)
  });
};
