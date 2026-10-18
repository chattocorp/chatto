/** A recording parent for delegation-tool tests. Conversation lifecycle is tested through
 * createConversation; this fixture records tool handoffs and their formatted answers. */
import type { PendingWork, WorkOutcome, WorkOwner } from './work.ts';

export function recordWork(onFinish?: (outcome: WorkOutcome) => void) {
  const pending = new Map<string, PendingWork>();
  const finished: [PendingWork, WorkOutcome][] = [];
  const progress: [string, string][] = [];
  const owner: WorkOwner = {
    pending,
    track(child, details, formatResult) {
      child.closeInput();
      const job: PendingWork = {
        ...details,
        id: child.id,
        startedAt: Date.now(),
        child,
        done: Promise.resolve()
      };
      pending.set(job.id, job);
      const updates = (async () => {
        for await (const update of child.updates) {
          job.latest = String(update);
          progress.push([job.id, job.latest]);
        }
      })();
      job.done = Promise.all([child.result, updates])
        .then(
          ([result]) => ({ result: formatResult(result) }),
          (error: Error) => ({ error: error.message })
        )
        .then((outcome) => {
          pending.delete(job.id);
          finished.push([job, outcome]);
          onFinish?.(outcome);
        });
      return job;
    }
  };
  return { owner, pending, finished, progress };
}
