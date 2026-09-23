import { dispatchRoute } from "./routing.ts";
import { serverLog } from "./server-log.ts";
import type { EventSource, WebhookContext, WebConfig } from "./web-config.ts";

interface RunningSource {
  controller: AbortController;
  completion: Promise<void>;
}

/** Serializes source generations while retaining adapter-owned state by source name. */
export class SourceManager {
  private states = new Map<string, Map<string, unknown>>();
  private running: RunningSource[] = [];
  private pending = Promise.resolve();
  private closed = false;

  constructor(private start: (name: string) => WebhookContext["start"]) {}

  replace(config: WebConfig): Promise<void> {
    this.pending = this.pending.then(async () => {
      await this.stop();
      if (this.closed) return;
      const sources = config.sources ?? {};
      for (const name of this.states.keys()) if (!Object.hasOwn(sources, name)) this.states.delete(name);
      for (const [name, source] of Object.entries(sources)) this.launch(name, source);
    });
    return this.pending;
  }

  private launch(name: string, source: EventSource) {
    const controller = new AbortController();
    const state = this.states.get(name) ?? new Map<string, unknown>();
    this.states.set(name, state);
    const deliveries = new Set<Promise<unknown>>();
    let accepting = true;
    const completion = Promise.resolve().then(() => source({
      signal: controller.signal, state,
      dispatch: (route, input) => {
        if (!accepting || controller.signal.aborted) return Promise.reject(new Error("Event source has stopped"));
        const delivery = dispatchRoute(route, input, this.start(name));
        deliveries.add(delivery);
        void delivery.then(() => deliveries.delete(delivery), () => deliveries.delete(delivery));
        return delivery;
      },
    })).catch(() => {
      if (!controller.signal.aborted) serverLog("error", "source.failed", { source: name });
    }).finally(async () => {
      accepting = false;
      controller.abort();
      await Promise.allSettled(deliveries);
    });
    this.running.push({ controller, completion });
  }

  private async stop() {
    for (const source of this.running) source.controller.abort();
    await Promise.all(this.running.map(source => source.completion));
    this.running = [];
  }

  /** Wait for source cleanup and accepted dispatches; active workflows remain host-owned. */
  async close() {
    this.closed = true;
    for (const source of this.running) source.controller.abort();
    await this.pending;
    await this.stop();
    this.states.clear();
  }
}
