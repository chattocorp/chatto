import { emitRunlingEvent, withRunlingActivity } from './events.ts';
import { log, logStep } from './log.ts';

export type StepWork<T> = () => T;

/**
 * Runs a named workflow step, indenting any log output produced by its work.
 */
export function step<T>(label: string, work: StepWork<T>): T {
  const id = crypto.randomUUID();
  const startedAt = performance.now();
  emitRunlingEvent({
    type: 'step.started',
    id,
    label
  });
  logStep(label);

  const finish = (status: 'completed' | 'failed', error?: unknown) =>
    emitRunlingEvent({
      type: 'step.finished',
      id,
      status,
      durationMs: performance.now() - startedAt,
      ...(status === 'failed' ? { error: errorMessage(error) } : {})
    });

  try {
    const result = withRunlingActivity(id, () => log.indented(work));
    if (isPromiseLike(result)) {
      void Promise.resolve(result).then(
        () => finish('completed'),
        (error) => finish('failed', error)
      );
    } else {
      finish('completed');
    }
    return result;
  } catch (error) {
    finish('failed', error);
    throw error;
  }
}

/** A failed step's error message for its journal, bounded. The server log does not include it. */
const errorMessage = (error: unknown) =>
  (error instanceof Error ? `${error.name}: ${error.message}` : String(error)).slice(0, 2000);

const isPromiseLike = (value: unknown): value is PromiseLike<unknown> =>
  (typeof value === 'object' || typeof value === 'function') &&
  value !== null &&
  'then' in value &&
  typeof value.then === 'function';
