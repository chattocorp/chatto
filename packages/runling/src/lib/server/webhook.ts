import { serverLog } from "../../runtime/server-log.ts";
import { validateSchema } from "runling";
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

  const pending: Promise<StartedRun>[] = [];
  let accepting = true;
  let failure: unknown;
  let failed = false;
  const ctx: WebhookContext = {
    start(task, options) {
      if (!accepting) {
        const rejection = Promise.reject<StartedRun>(new Error("Webhook routing has finished"));
        // A detached callback must not create an unhandled rejection in the host.
        void rejection.catch(() => {});
        return rejection;
      }


      const registration = Promise.resolve().then(() => start(task, options));
      pending.push(registration);
      // Observe failures even when the routing function forgets to await a start.
      void registration.catch(() => {});
      return registration;
    },
  };

  try {
    await prepared.route(ctx, prepared.input);
  } catch (error) {
    failed = true;
    failure = error;
  } finally {
    accepting = false;
  }

  const registrations = await Promise.allSettled(pending);
  const runs: StartedRun[] = [];
  for (const registration of registrations) {
    if (registration.status === "fulfilled") {
      runs.push({ id: registration.value.id });
    } else if (!failed) {
      failed = true;
      failure = registration.reason;
    }
  }

  if (failed) {
    serverLog("error", "webhook.route_failed", { webhook: name, runs, error: failure });
    return Response.json({
      error: failure instanceof Error ? failure.message : "Webhook routing failed.",
      runs,
    }, { status: 500 });
  }

  if (!runs.length) serverLog("info", "webhook.handled", { webhook: name, startedRun: false });
  return Response.json({ runs }, { status: 202 });
}
