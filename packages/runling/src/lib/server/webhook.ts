import { serverLog } from "../../runtime/server-log.ts";
import { validateSchema } from "runling";
import { dispatchRoute, RoutingError } from "../../runtime/routing.ts";
import {
  describeRouterSchemas,
  type StartedRun,
  type WebhookContext,
  type WebhookRouter,
  type WebConfig,
} from "runling/web";

export interface WebhookDependencies {
  config: WebConfig;
  start: WebhookContext["start"];
}

export function describeWebhook(name: string, { config }: Pick<WebhookDependencies, "config">): Response {
  if (!Object.hasOwn(config.webhooks, name)) {
    return Response.json({ error: `Unknown webhook ${JSON.stringify(name)}.` }, { status: 404 });
  }
  return Response.json(describeRouterSchemas(config.webhooks[name]!));
}

export async function prepareWebhook(
  name: string,
  request: Request,
  config: WebConfig,
): Promise<Response | { input: unknown; route: WebhookRouter<any> }> {
  if (!Object.hasOwn(config.webhooks, name)) {
    return Response.json({ error: `Unknown webhook ${JSON.stringify(name)}.` }, { status: 404 });
  }

  let body: unknown;
  try {
    body = await request.json();
  } catch {
    return Response.json({ error: "The request body must be valid JSON." }, { status: 400 });
  }

  const route = config.webhooks[name]!;
  if (route.input !== undefined) {
    const result = await validateSchema(route.input, body);
    if (result.issues) {
      return Response.json({
        error: "The request body does not match the webhook input schema.",
        issues: result.issues,
      }, { status: 400 });
    }
  }

  // The task or custom router owns parsing; do not pass a transformed value twice.
  return { input: body, route };
}

/** Register all starts initiated during routing, including calls not awaited by the router. */
export async function handleWebhook(
  name: string,
  request: Request,
  { config, start }: WebhookDependencies,
): Promise<Response> {
  const prepared = await prepareWebhook(name, request, config);
  if (prepared instanceof Response) return prepared;

  let runs: StartedRun[];
  try {
    runs = await dispatchRoute(prepared.route, prepared.input, start);
  } catch (error) {
    runs = error instanceof RoutingError ? error.runs : [];
    const failure = error instanceof RoutingError ? error.cause : error;
    serverLog("error", "webhook.route_failed", { webhook: name, runs, error: failure });
    return Response.json({
      error: failure instanceof Error ? failure.message : "Webhook routing failed.",
      runs,
    }, { status: 500 });
  }

  if (!runs.length) serverLog("info", "webhook.handled", { webhook: name, startedRun: false });
  return Response.json({ runs }, { status: 202 });
}
