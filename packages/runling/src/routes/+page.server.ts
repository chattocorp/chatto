import { loadWebConfig } from "$lib/server/web-config.ts";
import { getRunStore } from "$lib/server/run-store.ts";
import type { PageServerLoad } from "./$types";
import type { WebhookInfo } from "$lib/runs.ts";
import { describeRouterSchemas } from "runling/web";

export const load: PageServerLoad = async () => {
  const config = await loadWebConfig();
  const webhooks = Object.entries(config.webhooks).map(
    ([name, definition]) => ({
      name,
      workflow: definition.label ?? name,
      path: `/api/webhooks/${encodeURIComponent(name)}`,
      ...describeRouterSchemas(definition),
    }),
  );
  // Strip non-JSON metadata before SvelteKit serializes the page data.
  return {
    webhooks: JSON.parse(JSON.stringify(webhooks)) as WebhookInfo[],
    runs: (await getRunStore()).list(),
  };
};
