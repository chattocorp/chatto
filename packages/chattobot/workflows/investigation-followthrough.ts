/** Conversation-local provenance for issue filing after a source investigation.
 * Notifications select existing requests; they never supply authorization text. */
import type { AgentTaskState } from 'runling/agents';

// Stay inside Runling's authorization-classifier prompt limits. Retained messages
// must reach that classifier unchanged; exceeding either limit disables filing.
const MAX_MESSAGES = 20;
const MAX_MESSAGE = 4_000;

/** Retain the initiating human request and reserve at most one issue creation attempt per task.
 * A failed transport can have applied the write, so reservations are never rolled back. */
export function createInvestigationFollowthrough() {
  const requests = new Map<string, { messages: string[]; attempted: boolean; overflow: boolean }>();
  let selected: string | undefined;
  const current = () => (selected ? requests.get(selected) : undefined);
  const issueCreate = (args: unknown): args is string[] =>
    Array.isArray(args) && args[0] === 'issue' && args[1] === 'create';
  return {
    started(id: string, messages: readonly string[]) {
      if (!requests.has(id)) {
        const overflow =
          messages.length > MAX_MESSAGES ||
          messages.some((message) => message.length > MAX_MESSAGE);
        requests.set(id, { messages: overflow ? [] : [...messages], attempted: false, overflow });
      }
    },
    /** Keep withdrawals and corrections for every pending request. Stop rather than lose context. */
    update(message: string) {
      for (const request of requests.values()) {
        if (request.attempted || request.overflow) continue;
        if (request.messages.length >= MAX_MESSAGES || message.length > MAX_MESSAGE) {
          request.overflow = true;
          continue;
        }
        request.messages.push(message);
      }
    },
    /** Use observer state, never the task snapshot supplied in a notification. */
    select(id: string | undefined, tasks: readonly AgentTaskState[]) {
      selected = undefined;
      const task = tasks.find((task) => task.id === id);
      if (!task || task.status !== 'completed' || !requests.has(task.id)) return;
      try {
        if (JSON.parse(task.result ?? '').outcome === 'completed') selected = task.id;
      } catch {
        // Missing or malformed results cannot authorize follow-through.
      }
    },
    canFile(args: unknown) {
      const request = current();
      return !!request && !request.attempted && !request.overflow && issueCreate(args);
    },
    /** Facts for the supervisor, not an authorization grant. */
    context() {
      const request = current();
      return request
        ? {
            maintainerMessages: [...request.messages],
            issueCreationAttempted: request.attempted,
            retentionLimitExceeded: request.overflow
          }
        : undefined;
    },
    messages(recent: readonly string[]) {
      const request = current();
      return request ? [...request.messages] : [...recent];
    },
    reserve(args: readonly string[]) {
      const request = current();
      if (!request || request.attempted || request.overflow || !issueCreate(args))
        throw new Error(
          'No unused completed investigation permits this issue creation. Check GitHub before retrying a write.'
        );
      request.attempted = true;
    }
  };
}
