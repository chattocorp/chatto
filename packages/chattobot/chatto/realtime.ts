import { createApi, createClient, type AddressedMessage, type Server } from '@chatto/client';
import { parseServerUrl } from '@chatto/client/util/serverUrl';
import { createThreadReader } from '../thread.ts';
import { createEyesReaction } from '../reaction.ts';
import { ConfigurationError, setting, thinkingSetting } from '../settings.ts';
import { webSettings } from '../web.ts';
import type { EventSource } from 'runling/web';
import { log } from 'runling';
import { setTimeout as delay } from 'node:timers/promises';
import { createChattoBot } from '../workflows/chat.ts';
import { investigationSettings } from '../workflows/investigate.ts';
import { implementationSettings } from '../workflows/implement.ts';
import {
  createConversationState,
  RegistrationError,
  type ConversationState,
  type Delivery
} from './routing.ts';

interface Session {
  identity: string;
  conversations: ConversationState;
}

/** Convert authorized public message events into the bot's existing conversation input. */
export function messageDelivery(message: AddressedMessage, botId: string): Delivery {
  return {
    version: 1,
    id: message.id,
    type: 'message.created',
    triggers: message.reasons,
    occurred_at: message.occurredAt ?? '',
    bot_id: botId,
    room_id: message.roomId,
    thread_root_id: message.threadRootId ?? null,
    message: { id: message.id, author_id: message.authorId, body: message.body }
  };
}

/** Apply the client's server URL rules; the error never includes the value. */
function isServerUrl(value: string): boolean {
  try {
    parseServerUrl(value);
    return true;
  } catch {
    return false;
  }
}

/** Parse maintainer Chatto user IDs, separated by commas or spaces. They are matched against
 * server-authenticated message authors, never display names. */
function maintainerIds(): string[] {
  const ids = (setting('CHATTO_MAINTAINER_USER_IDS') ?? '').split(/[\s,]+/).filter(Boolean);
  if (ids.some((id) => !/^[A-Za-z0-9_-]{1,64}$/.test(id)))
    throw new ConfigurationError(
      'CHATTO_MAINTAINER_USER_IDS must list Chatto user IDs separated by commas'
    );
  return ids;
}

/** Read and validate this source generation's settings before contacting any service.
 * Runling does not log source failure messages, so configuration errors are logged here.
 * Their messages name settings, never their values. */
function sourceSettings() {
  try {
    const serverUrl = setting('CHATTO_URL');
    const apiKey = setting('CHATTO_API_KEY');
    if (!serverUrl || !apiKey) throw new ConfigurationError('Set CHATTO_URL and CHATTO_API_KEY');
    if (!isServerUrl(serverUrl))
      throw new ConfigurationError('CHATTO_URL must be an HTTP or HTTPS URL without credentials');
    const implementation = implementationSettings();
    const investigation = investigationSettings(implementation);
    const maintainers = maintainerIds();
    if ((investigation || implementation) && !maintainers.length)
      throw new ConfigurationError(
        'Source investigation and implementation require CHATTO_MAINTAINER_USER_IDS'
      );
    return {
      serverUrl,
      apiKey,
      // Match the server-authenticated actor ID, never a display name or user-supplied message field.
      allowedUserId: setting('CHATTO_ALLOWED_USER_ID'),
      model: setting('CHATTO_AGENT_MODEL'),
      thinkingLevel: thinkingSetting('CHATTO_AGENT_THINKING', 'low'),
      investigation,
      implementation,
      maintainers,
      web: webSettings()
    };
  } catch (error) {
    if (error instanceof ConfigurationError)
      console.error(`ChattoBot configuration error: ${error.message}`);
    throw error;
  }
}

/** Outbound-only bot source. Conversation state survives reloads for the same bot identity. */
export const chattoSource: EventSource = async (ctx) => {
  const { serverUrl, apiKey, allowedUserId, ...settings } = sourceSettings();
  // Each source generation owns its connection and closes it when it ends.
  // A new generation starts from a fresh realtime snapshot. Closing fails the
  // connection's requests in flight, so runs, which can outlive their
  // generation, send requests through a stateless API client instead.
  const client = createClient();
  try {
    const chatto = client.connect({ serverUrl, apiKey });
    await consume(ctx, chatto, apiKey, allowedUserId, settings, serverUrl);
  } finally {
    client.close();
  }
};

async function consume(
  ctx: Parameters<EventSource>[0],
  chatto: Server,
  apiKey: string,
  allowedUserId: string | undefined,
  settings: Omit<ReturnType<typeof sourceSettings>, 'serverUrl' | 'apiKey' | 'allowedUserId'>,
  serverUrl: string
): Promise<void> {
  const { viewerId: botId } = await chatto.ready({ signal: ctx.signal });
  const api = createApi({ serverUrl, apiKey, viewerId: botId });
  const identity = JSON.stringify([new URL(serverUrl).origin, botId]);
  let session = ctx.state.get('chatto') as Session | undefined;
  if (!session || session.identity !== identity) {
    session = { identity, conversations: createConversationState() };
    ctx.state.set('chatto', session);
  } else {
    // Each generation starts from a new snapshot; the previous one is closed.
    console.warn('ChattoBot reloaded: messages sent during the reload are not replayed.');
  }
  // Active runs keep this generation's credentials even if a reload changes them.
  const bot = createChattoBot({
    ...settings,
    state: session.conversations,
    post: async (destination, body, signal) => {
      await api.postMessage(destination, body, { signal });
    },
    typing: (destination, signal) => api.refreshTyping(destination, { signal }),
    readThread: createThreadReader(api),
    acknowledge: createEyesReaction(api)
  });
  await chatto.consumeEvents({
    signal: ctx.signal,
    onStatus(status) {
      // Only fixed local status fields are logged, never connection details or payloads.
      if (status.state === 'ready' && status.gap)
        console.warn('Chatto realtime recovery gap: some messages may have been missed.');
      else if (status.state === 'ready') log.success('Chatto realtime connected');
      else log.info(`Chatto realtime: ${status.state}`);
    },
    async onEvent(event) {
      // Ignore before routing: disallowed senders cannot start, steer, cancel,
      // or trigger acknowledgements for an existing conversation.
      if (allowedUserId && event.actorId !== allowedUserId) return;
      const message = await chatto.addressedMessage(event, { signal: ctx.signal }).catch(() => {
        ctx.signal.throwIfAborted();
        // Missing reply targets do not stop this bot's realtime source.
        console.warn(
          'ChattoBot could not verify a reply target; ignoring the unmentioned message.'
        );
        return undefined;
      });
      if (!message) return;
      const delivery = messageDelivery(message, botId);
      for (let attempt = 0; ; attempt++) {
        ctx.signal.throwIfAborted();
        try {
          await ctx.dispatch(bot.route, delivery);
          return;
        } catch (error) {
          // Runling wraps routing failures with their cause. Only failed run
          // registration is retryable; accepted inbox deliveries are never retried.
          const cause = error instanceof Error ? error.cause : undefined;
          if (!(cause instanceof RegistrationError) || attempt >= 2) throw error;
          await delay(250 * 2 ** attempt, undefined, { signal: ctx.signal });
        }
      }
    }
  });
}
