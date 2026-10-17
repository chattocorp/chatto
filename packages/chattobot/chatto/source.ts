/** The realtime Chatto source: route addressed messages and configured report intake. */
import { statSync } from 'node:fs';
import { resolve } from 'node:path';
import { createApi, createClient, type Server } from '@chatto/client';
import { UserService } from '@chatto/api-types/api/v1/user_service_connect';
import { parseServerUrl } from '@chatto/client/util/serverUrl';
import { log } from 'runling';
import type { EventSource } from 'runling/web';
import { createConversation, type ConversationSettings } from '../tasks/conversation.ts';
import { createTokenSource, githubSettings } from '../github/app.ts';
import { createEyesReaction, createHandledReaction } from '../reaction.ts';
import {
  ConfigurationError,
  maintainerLoginSetting,
  setting,
  thinkingSetting,
  autoEngageRoomIds
} from '../settings.ts';
import { createAttachmentReader, createThreadReader } from '../thread.ts';
import { webSettings } from '../web.ts';
import { createInvestigator, investigatorSettings } from '../tasks/investigate.ts';
import { incomingMessage, intakeMessage } from './message.ts';
import { createChattoRouter, createRouterState, type RouterState } from './router.ts';
import { createIntakeClassifier } from '../tasks/classify-intake.ts';
import { createThreadWorkflow } from '../tasks/thread.ts';
import { createFollowUpReader } from './follow-up.ts';
import { paseoSettings } from '../paseo/settings.ts';

/** Parse maintainer Chatto user IDs, separated by commas or spaces. They are matched against
 * server-authenticated message authors, never display names. */
function maintainerIds(): Set<string> {
  const ids = (setting('CHATTO_MAINTAINER_USER_IDS') ?? '').split(/[\s,]+/).filter(Boolean);
  if (ids.some((id) => !/^[A-Za-z0-9_-]{1,64}$/.test(id)))
    throw new ConfigurationError(
      'CHATTO_MAINTAINER_USER_IDS must list Chatto user IDs separated by commas'
    );
  return new Set(ids);
}

/** Read and validate this source generation's settings before contacting any service.
 * Runling does not log source failure messages, so configuration errors are logged here.
 * Their messages name settings, never their values. */
export function sourceSettings() {
  try {
    const serverUrl = setting('CHATTO_URL');
    const apiKey = setting('CHATTO_API_KEY');
    if (!serverUrl || !apiKey) throw new ConfigurationError('Set CHATTO_URL and CHATTO_API_KEY');
    try {
      parseServerUrl(serverUrl);
    } catch {
      throw new ConfigurationError('CHATTO_URL must be an HTTP or HTTPS URL without credentials');
    }
    const model = setting('CHATTO_AGENT_MODEL');
    if (!model) throw new ConfigurationError('Set CHATTO_AGENT_MODEL');
    const github = githubSettings();
    const source = setting('CHATTO_SOURCE_DIRECTORY');
    if (source && !statSync(resolve(source), { throwIfNoEntry: false })?.isDirectory())
      throw new ConfigurationError('CHATTO_SOURCE_DIRECTORY must name a directory');
    const investigator = source ? investigatorSettings() : undefined;
    const conversation: ConversationSettings = {
      model,
      thinkingLevel: thinkingSetting('CHATTO_AGENT_THINKING', 'medium'),
      maintainers: maintainerIds(),
      maintainerLogin: maintainerLoginSetting(),
      web: webSettings(),
      paseo: paseoSettings(),
      // One token source per generation, so conversations share cached installation tokens.
      github: github && { settings: github, tokens: createTokenSource(github) },
      ...(source
        ? {
            investigation: {
              directory: resolve(source),
              investigator: (scope: string) =>
                createInvestigator({
                  ...investigator!,
                  sessions: resolve(investigator!.sessions, scope)
                }),
              ...(github ? { repository: github.repository } : {})
            }
          }
        : {})
    };
    return {
      serverUrl,
      apiKey,
      // Match the server-authenticated actor ID, never a display name.
      allowedUserId: setting('CHATTO_ALLOWED_USER_ID'),
      autoEngageRooms: autoEngageRoomIds(),
      conversation
    };
  } catch (error) {
    if (error instanceof ConfigurationError)
      console.error(`ChattoBot configuration error: ${error.message}`);
    throw error;
  }
}

/** Outbound-only bot source. Routing state survives reloads for the same bot identity. */
export const chattoSource: EventSource = async (ctx) => {
  const settings = sourceSettings();
  // Each source generation owns its connection and closes it when it ends. Runs can outlive
  // their generation, so they send requests through a stateless API client instead.
  const client = createClient();
  try {
    const server = client.connect({ serverUrl: settings.serverUrl, apiKey: settings.apiKey });
    await consume(ctx, server, settings);
  } finally {
    client.close();
  }
};

async function consume(
  ctx: Parameters<EventSource>[0],
  server: Server,
  {
    serverUrl,
    apiKey,
    allowedUserId,
    autoEngageRooms,
    conversation: conversationSettings
  }: ReturnType<typeof sourceSettings>
) {
  const { viewerId: botId } = await server.ready({ signal: ctx.signal });
  const api = createApi({ serverUrl, apiKey, viewerId: botId });
  const identity = JSON.stringify([new URL(serverUrl).origin, botId]);
  let session = ctx.state.get('chatto') as { identity: string; state: RouterState } | undefined;
  if (session?.identity !== identity) {
    session = { identity, state: createRouterState() };
    ctx.state.set('chatto', session);
  } else {
    // Each generation starts from a new snapshot; the previous one is closed.
    console.warn('ChattoBot reloaded: messages sent during the reload are not replayed.');
  }
  const conversation = createConversation(conversationSettings, {
    post: async (destination, body, signal) =>
      (await api.postMessage(destination, body, { signal })).ids[0],
    typing: (destination, signal) => api.refreshTyping(destination, { signal }),
    acknowledge: createEyesReaction(api),
    setHandled: createHandledReaction(api),
    readThread: createThreadReader(api),
    readAttachment: createAttachmentReader(api)
  });
  const workflow = createThreadWorkflow({
    classifier: createIntakeClassifier(conversationSettings.model),
    conversation,
    readFollowUp: createFollowUpReader(api, allowedUserId),
    async read(message, signal) {
      const { users } = await api
        .service(UserService)
        .batchGetUsers({ userIds: [message.authorId] }, { signal, timeoutMs: 10_000 });
      const author = users.find((row) => row.user?.id === message.authorId)?.user;
      if (!author || author.bot || author.deleted) return;
      // The thread reader includes its root and current attachments; retracted roots are absent.
      const thread = await api.readThread(
        { roomId: message.roomId, threadRootId: message.id },
        { signal, limit: 1 }
      );
      const root = thread.messages.find((item) => item.id === message.id);
      if (!root || root.authorId !== message.authorId) return;
      return {
        body: root.body,
        attachmentTypes: (root.attachments ?? []).map((item) => item.contentType)
      };
    }
  });
  const route = createChattoRouter({ botId, workflow, state: session.state });

  await server.consumeEvents({
    signal: ctx.signal,
    onStatus(status) {
      // Only fixed local status fields are logged, never connection details or payloads.
      if (status.state === 'ready' && status.gap)
        console.warn('Chatto realtime recovery gap: some messages may have been missed.');
      else if (status.state === 'ready') log.success('Chatto realtime connected');
      else log.info(`Chatto realtime: ${status.state}`);
    },
    async onEvent(event) {
      // Disallowed senders cannot start or continue a conversation.
      if (allowedUserId && event.actorId !== allowedUserId) return;
      const message = await server.addressedMessage(event, { signal: ctx.signal }).catch(() => {
        ctx.signal.throwIfAborted();
        console.warn('ChattoBot could not verify a reply target; ignoring the message.');
        return undefined;
      });
      if (message?.authorId === botId) return;
      try {
        if (message) await ctx.dispatch(route, incomingMessage(message));
        else {
          const candidate = intakeMessage(event, autoEngageRooms, botId);
          if (candidate) await ctx.dispatch(route, candidate);
        }
      } catch {
        ctx.signal.throwIfAborted();
        // One failed message must not stop the bot. The error can contain message content.
        console.error('ChattoBot could not route a message.');
      }
    }
  });
}
