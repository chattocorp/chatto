import { createChannel } from '../channel.ts';
import { input } from '../input.ts';
import { TimeoutError, validateTimeout } from '../timeout.ts';
import { emitRunlingEvent } from '../events.ts';
import type { WorkflowContext } from '../context.ts';
import type { RunlingAgent } from '../agent.ts';
import { connectAgent, type AgentConnectionOptions } from './connection.ts';

/**
 * Keep an agent conversation open until its idle timeout expires.
 * Messages steer the active turn when possible; otherwise they start a new turn.
 * Replies are emitted through the task context. The caller owns the agent.
 */
export async function runAgentConversation(
  ctx: WorkflowContext<string, string>,
  agent: Pick<RunlingAgent, 'runOutcome' | 'steer'>,
  prompt: string,
  {
    timeout = 900,
    onBusy,
    notifications,
    keepAlive,
    prepareMessage
  }: {
    timeout?: number;
    onBusy?: (busy: boolean) => void;
    /** Background messages can steer an active turn or wake an idle agent. */
    notifications?: AsyncIterable<string>;
    /** Omit the idle deadline while work is active; its completion must send a notification. */
    keepAlive?: () => boolean;
    /** Prepare each input once with its trusted origin, including the initial user prompt. */
    prepareMessage?: AgentConnectionOptions['prepareMessage'];
  } = {}
): Promise<string> {
  validateTimeout(timeout);

  emitRunlingEvent({ type: 'conversation.started' });

  // The connection reads the task inbox even while we await an agent turn.
  // Keep messages it could not steer so they can become the next prompt.
  const pending = createChannel<string>({ signal: ctx.signal });
  const reader = pending[Symbol.asyncIterator]();
  let queuedMessages = 0;

  const readNextMessage = async (): Promise<string> => {
    const message = await reader.next();
    if (message.done) {
      throw new Error('Conversation inbox closed');
    }

    queuedMessages--;
    return message.value;
  };

  const connection = connectAgent(ctx, agent, {
    inbox: ctx.inbox,
    notifications,
    prepareMessage,
    onText: (text) => ctx.emit(text),
    onDelivery: async (text, consumed) => {
      if (consumed) {
        return;
      }

      await pending.send(text);
      queuedMessages++;
    }
  });

  try {
    if (prepareMessage) prompt = await prepareMessage(prompt, 'user');
    while (true) {
      onBusy?.(true);
      const result = await connection.runOutcome(prompt);
      ctx.signal.throwIfAborted();
      if (result.outcome !== 'completed') {
        throw new Error(result.summary);
      }

      onBusy?.(false);

      // Process queued messages immediately. Only an empty queue starts an
      // idle interval, so each completed interaction gets a fresh timeout.
      if (queuedMessages > 0) {
        prompt = await readNextMessage();
        continue;
      }

      try {
        // Use input() to show the idle wait on the timeline. Its answer comes
        // from our queue; the connection signal also reports inbox failures.
        prompt = await input(
          { ...ctx, signal: connection.signal, onInput: readNextMessage },
          'Waiting for a message',
          { timeout: keepAlive?.() ? undefined : timeout }
        );
      } catch (error) {
        // Idle expiry is normal completion. Cancellation and transport errors
        // must still fail the task, even if they are themselves timeout errors.
        if (error instanceof TimeoutError && !connection.signal.aborted) {
          return result.summary;
        }

        throw error;
      }
    }
  } finally {
    // Explicit cleanup also works on Node 22, which cannot parse `await using`.
    // Disconnecting releases callbacks, but does not dispose the caller's agent.
    try {
      onBusy?.(false);
    } finally {
      pending.close();
      await reader.return?.();
      await connection.dispose();
    }
  }
}
