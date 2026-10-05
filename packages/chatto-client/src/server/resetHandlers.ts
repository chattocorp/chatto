/** Run every cleanup even when another consumer fails. Return whether all succeeded. */
export function runResetHandlers(handlers: Iterable<() => void>): boolean {
  let complete = true;
  for (const handler of handlers) {
    try {
      handler();
    } catch {
      complete = false;
      console.error('Private-data cleanup failed');
    }
  }
  return complete;
}
