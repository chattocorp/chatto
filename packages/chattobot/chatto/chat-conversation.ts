/** Route one Chatto thread and serialize all assistant and host-owned posts. */
import { Type, type WorkflowContext } from 'runling';
import { runConversationTask, type ConversationActivityHandler } from './conversation-task.ts';
import type { ChattoTyping } from '@chatto/client';
import {
  createChattoRouter,
  configuredChattoClient,
  postToChatto,
  type ChattoPost,
  type Delivery,
  type ConversationState
} from './routing.ts';

/** Settings supplied to a conversation task by its host. */
export type ConversationOptions<Settings> = Settings & {
  onBusy: ConversationActivityHandler;
  delivery: Delivery;
  /** Deliver a tool announcement to Chatto before beginning long-running work. */
  announce: (text: string, signal: AbortSignal) => Promise<void>;
  /** Post a host-owned background result in order with assistant replies. */
  postUpdate?: (text: string, signal: AbortSignal) => Promise<void>;
  /** Select the human message that prompted the next response; notifications have no direct target. */
  setReplyContext: (message: string, origin: 'user' | 'notification') => void;
};

/** Connect a string-in/string-out task to a Chatto thread. */
export function chattoConversation<Settings>({
  name,
  task,
  settings,
  acknowledge,
  post = postToChatto,
  typing = (destination, signal) => configuredChattoClient().refreshTyping(destination, signal),
  state
}: {
  name: string;
  task: (
    ctx: WorkflowContext<string, string>,
    prompt: string,
    options: ConversationOptions<Settings>
  ) => Promise<string>;
  settings: Settings;
  acknowledge: (delivery: Delivery, signal: AbortSignal) => Promise<void>;
  post?: ChattoPost;
  typing?: ChattoTyping;
  state?: ConversationState;
}) {
  return createChattoRouter({
    name,
    output: Type.Object({ reply: Type.String() }),
    post,
    state,
    async run(ctx, delivery, destination, inbox) {
      const messages = [delivery];
      let inReplyTo: string | undefined = delivery.message.id;
      const setReplyContext = (text: string, origin: 'user' | 'notification') => {
        inReplyTo = undefined;
        if (origin !== 'user') return;
        const index = messages.findIndex((message) => message.message.body === text);
        if (index !== -1) inReplyTo = messages.splice(index, 1)[0]!.message.id;
      };
      // Assistant text and tool announcements can arrive concurrently. Serialize
      // their posts. An assistant message in this turn already acknowledges the
      // work, so a tool does not need to announce it again in different words.
      let pending = Promise.resolve();
      let turn = 0;
      let assistantTurn = -1;
      let last: { text: string; origin: 'assistant' | 'announcement'; at: number } | undefined;
      const send = (
        text: string,
        signal: AbortSignal,
        origin: 'assistant' | 'announcement',
        replyToCurrent = true
      ) => {
        const sentTurn = turn;
        // Capture before awaiting other posts; a later input must not retarget this reply.
        const target = { ...destination, ...(replyToCurrent && inReplyTo ? { inReplyTo } : {}) };
        const next = pending.then(async () => {
          signal.throwIfAborted();
          if (origin === 'announcement' && assistantTurn === sentTurn) return;
          const normalized = text.trim().replace(/\s+/g, ' ');
          if (last?.text === normalized && last.origin !== origin && Date.now() - last.at < 10_000)
            return;
          await post(target, text, signal);
          if (origin === 'assistant') assistantTurn = sentTurn;
          last = { text: normalized, origin, at: Date.now() };
        });
        pending = next.catch(() => {});
        return next;
      };
      const reply = await runConversationTask(
        ctx,
        inbox,
        (childCtx, onBusy) => {
          return task(childCtx, delivery.message.body, {
            ...settings,
            onBusy: (busy) => {
              if (busy) turn++;
              onBusy(busy);
            },
            delivery,
            setReplyContext,
            announce: (text, signal) =>
              send(
                text,
                AbortSignal.any([childCtx.signal, signal, AbortSignal.timeout(10_000)]),
                'announcement'
              ),
            postUpdate: (text, signal) =>
              send(
                text,
                AbortSignal.any([childCtx.signal, signal, AbortSignal.timeout(10_000)]),
                'assistant',
                false
              )
          });
        },
        {
          destination,
          post: (_destination, text, signal) => send(text, signal, 'assistant'),
          typing,
          acknowledge,
          delivery,
          onMessage: (message) => messages.push(message)
        }
      );

      // Updates already posted the text; this value is only the saved result.
      return { reply };
    }
  });
}
