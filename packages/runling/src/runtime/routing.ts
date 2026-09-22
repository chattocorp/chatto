import type { StartedRun, WebhookContext, WebhookRouter } from "./web-config.ts";

/** Preserve partial registration results for HTTP responses and source failure handling. */
export class RoutingError extends Error {
  constructor(readonly runs: StartedRun[], cause: unknown) {
    super("Event routing failed", { cause });
  }
}

/** Each delivery gets a context that expires when its router returns. */
export async function dispatchRoute<Input>(
  route: WebhookRouter<Input>, input: Input, start: WebhookContext["start"],
): Promise<StartedRun[]> {
  const pending: Promise<StartedRun>[] = [];
  let accepting = true;
  let failure: unknown;
  let failed = false;
  const ctx: WebhookContext = {
    start(task, options) {
      if (!accepting) {
        const rejection = Promise.reject<StartedRun>(new Error("Webhook routing has finished"));
        void rejection.catch(() => {});
        return rejection;
      }
      const registration = Promise.resolve().then(() => start(task, options));
      pending.push(registration);
      void registration.catch(() => {});
      return registration;
    },
  };
  try { await route(ctx, input); }
  catch (error) { failed = true; failure = error; }
  finally { accepting = false; }
  const runs: StartedRun[] = [];
  for (const result of await Promise.allSettled(pending)) {
    if (result.status === "fulfilled") runs.push({ id: result.value.id });
    else if (!failed) { failed = true; failure = result.reason; }
  }
  if (failed) throw new RoutingError(runs, failure);
  return runs;
}
