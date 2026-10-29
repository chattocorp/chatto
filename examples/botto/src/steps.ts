/** Botto's privacy boundary for errors in observed operations. Runling owns all logging. */
import { log, step, type LogFields } from 'runling';
import { errorHint } from './diagnostics.ts';

/** Observe work without copying its result or external error into the journal.
 * The caller receives the original error for retry and delivery decisions. */
export async function safeStep<T>(
  stage: string,
  work: () => Promise<T>,
  fields: LogFields = {}
): Promise<T> {
  let failure: { error: unknown } | undefined;
  try {
    return await log.withFields(fields, () =>
      step(stage, async () => {
        try {
          return await work();
        } catch (error) {
          failure = { error };
          throw new Error(`${stage} failed.${errorHint(error)}`);
        }
      })
    );
  } catch (error) {
    throw failure ? failure.error : error;
  }
}
