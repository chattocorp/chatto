/** One render at a time across visible messages; no message cache outlives its DOM owner. */
import MermaidWorker from './renderer.worker?worker';

interface Job {
  start: () => void;
}
const queue: Job[] = [];
let active: Job | undefined;
let worker: Worker | undefined;
let ready = false;

/** Discard a busy or broken worker; successful jobs reuse its loaded renderer. */
function discardWorker() {
  worker?.terminate();
  worker = undefined;
  ready = false;
}

function next() {
  if (active) return;
  active = queue.shift();
  active?.start();
}

/** Resolve SVG or null on failure, overload or cancellation. Abort also removes queued source. */
export function requestMermaid(source: string, signal: AbortSignal): Promise<string | null> {
  if (signal.aborted || queue.length >= 32) return Promise.resolve(null);
  return new Promise((resolve) => {
    let timeout: ReturnType<typeof setTimeout> | undefined;
    let done = false;
    function finish(svg: string | null = null, discard = false) {
      if (done) return;
      done = true;
      clearTimeout(timeout);
      signal.removeEventListener('abort', abort);
      if (active === job) {
        // Do not retain completed message source in idle worker callbacks.
        if (worker) worker.onmessage = worker.onerror = worker.onmessageerror = null;
        if (discard) discardWorker();
        active = undefined;
      }
      const index = queue.indexOf(job);
      if (index !== -1) queue.splice(index, 1);
      resolve(svg);
      next();
    }
    const abort = () => finish(null, true);
    const job: Job = {
      start() {
        try {
          worker ??= new MermaidWorker();
          const run = () => {
            clearTimeout(timeout);
            timeout = setTimeout(() => finish(null, true), 3_000);
            try {
              worker!.postMessage(source);
            } catch {
              finish(null, true);
            }
          };
          worker.onmessage = (event: MessageEvent<unknown>) => {
            if (done) return;
            if (
              !ready &&
              event.data &&
              typeof event.data === 'object' &&
              'type' in event.data &&
              event.data.type === 'ready'
            ) {
              ready = true;
              run();
            } else if (ready && (typeof event.data === 'string' || event.data === null)) {
              finish(event.data);
            } else finish(null, true);
          };
          worker.onerror = () => finish(null, true);
          worker.onmessageerror = () => finish(null, true);
          if (ready) run();
          // Loading has a separate bound, so slow downloads do not consume
          // the parser/layout budget or leave the queue blocked indefinitely.
          else timeout = setTimeout(() => finish(null, true), 30_000);
        } catch {
          finish(null, true);
        }
      }
    };
    signal.addEventListener('abort', abort, { once: true });
    queue.push(job);
    next();
  });
}
