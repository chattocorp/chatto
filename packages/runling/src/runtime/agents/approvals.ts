/** Process-local approval requests owned by one parent. Decisions release only the waiting call;
 * they do not grant new tools, change host policy, or survive cancellation and restart. */
import { randomUUID } from 'node:crypto';
import { Type } from 'typebox';
import { defineAgentExtension, type AgentExtension } from '../agent.ts';
import type { AgentTaskData } from './tasks.ts';

/** Host-selected description of an exact proposed action. Exclude secrets and personal data. */
export interface ApprovalAction {
  action: string;
  details: { [key: string]: AgentTaskData };
}

/** An opaque, single-use request in this owner's queue. */
export interface ApprovalRequest extends ApprovalAction {
  id: string;
}

/** Only an explicit allow decision releases an action. Reasons are data for the child model. */
export interface ApprovalDecision {
  decision: 'allow' | 'deny';
  reason: string;
}

/** A conversation-local owner queue. Notify through the requesting child's parent channel.
 * Pending requests are bounded, expire, and fail closed when either participant is cancelled. */
export function createApprovalQueue({
  signal,
  timeoutMs = 5 * 60_000,
  maxPending = 32
}: {
  signal: AbortSignal;
  timeoutMs?: number;
  maxPending?: number;
}) {
  if (
    !Number.isSafeInteger(timeoutMs) ||
    timeoutMs < 1 ||
    !Number.isSafeInteger(maxPending) ||
    maxPending < 1
  )
    throw new Error('Invalid approval queue limits');
  const pending = new Map<
    string,
    { request: ApprovalRequest; expiresAt: number; finish: (decision: ApprovalDecision) => void }
  >();
  let closed = false;
  const denied = (reason: string): ApprovalDecision => ({ decision: 'deny', reason });
  const dispose = () => {
    if (closed) return;
    closed = true;
    signal.removeEventListener('abort', dispose);
    for (const entry of pending.values()) entry.finish(denied('The approval owner stopped.'));
  };
  signal.addEventListener('abort', dispose, { once: true });
  if (signal.aborted) dispose();
  return {
    /** Immutable snapshots for the owning agent. A request ID alone cannot be used in another queue. */
    list: (): ApprovalRequest[] =>
      [...pending.values()].map(({ request }) => structuredClone(request)),
    /** Resolve one live request. Unknown, expired, or already decided IDs return false. */
    decide(id: string, decision: ApprovalDecision): boolean {
      if (
        !['allow', 'deny'].includes(decision.decision) ||
        !decision.reason.trim() ||
        decision.reason.length > 1000
      )
        throw new Error('Invalid approval decision');
      const entry = pending.get(id);
      if (!entry) return false;
      if (Date.now() >= entry.expiresAt) {
        entry.finish(denied('The approval request expired.'));
        return false;
      }
      entry.finish({ ...decision });
      return true;
    },
    /** Pause an action until the owner decides. Notify failure, timeout, and cancellation deny it.
     * The notification receives a copy; neither participant can mutate the stored proposal. */
    async request(
      action: ApprovalAction,
      options: {
        signal: AbortSignal;
        notify: (request: ApprovalRequest) => void | Promise<void>;
      }
    ): Promise<ApprovalDecision> {
      if (closed || signal.aborted || options.signal.aborted)
        return denied('The approval participant stopped.');
      if (pending.size >= maxPending) return denied('The approval queue is full.');
      const encoded = JSON.stringify(action);
      if (!action.action.trim() || encoded.length > 16_000)
        return denied('The approval action is invalid or too large.');
      const request: ApprovalRequest = { ...JSON.parse(encoded), id: randomUUID() };
      return new Promise<ApprovalDecision>((resolve) => {
        let timer: ReturnType<typeof setTimeout>;
        const abort = () => finish(denied('The requesting action was cancelled.'));
        const finish = (decision: ApprovalDecision) => {
          if (!pending.delete(request.id)) return;
          clearTimeout(timer);
          options.signal.removeEventListener('abort', abort);
          resolve(decision);
        };
        pending.set(request.id, { request, expiresAt: Date.now() + timeoutMs, finish });
        options.signal.addEventListener('abort', abort, { once: true });
        timer = setTimeout(() => finish(denied('The approval request expired.')), timeoutMs);
        void Promise.resolve()
          .then(() => {
            if (pending.has(request.id)) return options.notify(structuredClone(request));
          })
          .catch(() => finish(denied('The approval request could not reach its owner.')));
      });
    },
    dispose
  };
}

export type ApprovalQueue = ReturnType<typeof createApprovalQueue>;

/** Expose decisions only to the owning agent. Never install this extension on a child. */
export function approvalDecisionExtension(queue: ApprovalQueue): AgentExtension {
  return defineAgentExtension((pi) => {
    pi.registerTool({
      name: 'decideApproval',
      label: 'Decide child approval',
      description:
        'Allow or deny one pending child action by its approval ID. Allow only within the user-authorized delegated scope; otherwise ask the user and leave it pending until they answer. A decision grants only this exact action.',
      parameters: Type.Object({
        id: Type.String(),
        decision: Type.Union([Type.Literal('allow'), Type.Literal('deny')]),
        reason: Type.String({ minLength: 1, maxLength: 1000 })
      }),
      async execute(_id, { id, decision, reason }) {
        const decided = queue.decide(id, { decision, reason });
        return {
          content: [{ type: 'text' as const, text: JSON.stringify({ decided }) }],
          details: {}
        };
      }
    });
  });
}

/** Gate selected tools with owner approval. Other tools retain their existing host policy.
 * The host describes requests without exposing raw inputs. Approval applies to the input
 * captured at the hook; changes while it waits block execution. Install after hard policy gates.
 * The request callback must propagate cancellation to its owner queue. */
export function toolApprovalGate(options: {
  /** Lifecycle of the requesting child; checked again after the owner decision. */
  signal: AbortSignal;
  tools: Readonly<Record<string, (input: Record<string, unknown>) => ApprovalAction>>;
  request: (action: ApprovalAction) => Promise<ApprovalDecision>;
}): AgentExtension {
  return defineAgentExtension((pi) => {
    pi.on('tool_call', async (event) => {
      const describe = Object.hasOwn(options.tools, event.toolName)
        ? options.tools[event.toolName]
        : undefined;
      if (!describe) return;
      const encoded = JSON.stringify(event.input);
      let decision: ApprovalDecision;
      try {
        decision = await options.request(describe(JSON.parse(encoded)));
      } catch {
        return { block: true, reason: 'The tool approval owner could not decide.' };
      }
      if (options.signal.aborted) return { block: true, reason: 'The requesting child stopped.' };
      if (encoded !== JSON.stringify(event.input))
        return { block: true, reason: 'The tool input changed while approval was pending.' };
      if (decision.decision !== 'allow')
        return {
          block: true,
          reason: `The owner did not approve this tool call: ${decision.reason}`
        };
    });
  });
}
