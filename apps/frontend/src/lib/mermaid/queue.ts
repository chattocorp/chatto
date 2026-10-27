/** One render at a time across visible messages; no message cache outlives its DOM owner. */
import MermaidWorker from './renderer.worker?worker';

interface Job {
  start: () => void;
}
const queue: Job[] = [];
let active: Job | undefined;

function next() {
  if (active) return;
  active = queue.shift();
  active?.start();
}

/** Resolve SVG or null on failure, overload or cancellation. Abort also removes queued source. */
export function requestMermaid(source: string, signal: AbortSignal): Promise<string | null> {
  if (signal.aborted || queue.length >= 32) return Promise.resolve(null);
  return new Promise((resolve) => {
    let worker: Worker | undefined;
    let timeout: ReturnType<typeof setTimeout> | undefined;
    let done = false;
    function finish(svg: string | null = null) {
      if (done) return;
      done = true;
      clearTimeout(timeout);
      worker?.terminate();
      signal.removeEventListener('abort', abort);
      if (active === job) active = undefined;
      const index = queue.indexOf(job);
      if (index !== -1) queue.splice(index, 1);
      resolve(svg);
      next();
    }
    const abort = () => finish();
    const job: Job = {
      start() {
        try {
          worker = new MermaidWorker();
          timeout = setTimeout(finish, 3_000);
          worker.onmessage = (event: MessageEvent<unknown>) =>
            finish(typeof event.data === 'string' ? event.data : null);
          worker.onerror = () => finish();
          worker.onmessageerror = () => finish();
          worker.postMessage(source);
        } catch {
          finish();
        }
      }
    };
    signal.addEventListener('abort', abort, { once: true });
    queue.push(job);
    next();
  });
}
