/** Open a Pi transcript for one turn, then release all live session resources. */
import { createHash } from 'node:crypto';
import { mkdir } from 'node:fs/promises';
import { join } from 'node:path';
import {
  createAgentSession,
  DefaultResourceLoader,
  ModelRuntime,
  SessionManager,
  SettingsManager
} from '@earendil-works/pi-coding-agent';
import type { Settings } from './settings.ts';
import { StartupError, startupStep } from './diagnostics.ts';
import { log } from './log.ts';
import { connectChattoMcp } from './mcp.ts';
import { createMemory } from './memory.ts';
import { threadContextExtension, type ThreadContext } from './thread-context.ts';

/** Runtime facts supplement the configurable behavior instructions. */
const CAPABILITIES = `You cannot see attachments.
Your final answer is posted to this thread automatically.`;

/** Use opaque file names and bind transcripts to the server, bot, room, and thread. */
export function transcriptName(identity: readonly string[]): string {
  return `${createHash('sha256').update(JSON.stringify(identity)).digest('hex')}.jsonl`;
}

/** Prepare model access once. Credentials use Pi's normal auth store and provider environment. */
export async function createAgent(settings: Settings) {
  const openMemory = createMemory(settings);
  await startupStep('Cannot create the Botto data directory. Check runtime.data_directory.', () =>
    mkdir(settings.directory, { recursive: true, mode: 0o700 })
  );
  const runtime = await startupStep('Cannot load Pi model settings and authentication.', () =>
    ModelRuntime.create()
  );
  let model = runtime.getModel(settings.provider, settings.modelId);
  if (!model && process.env.PI_OFFLINE === undefined) {
    await startupStep(
      'Cannot refresh the Pi model catalog. Check provider connectivity and credentials.',
      () =>
        runtime.refresh({
          providers: [settings.provider],
          allowNetwork: true,
          force: true,
          signal: AbortSignal.timeout(10_000)
        })
    );
    model = runtime.getModel(settings.provider, settings.modelId);
  }
  if (!model)
    throw new StartupError(
      'agent.model was not found in the Pi model catalog. Check the provider/model ID and PI_OFFLINE.'
    );

  return {
    /** The caller must serialize calls for the same identity. Abort stops model generation. */
    async reply(
      identity: readonly string[],
      prompt: string,
      signal: AbortSignal,
      context?: ThreadContext,
      userIds: readonly string[] = []
    ): Promise<string> {
      signal.throwIfAborted();
      const mcp = settings.mcp ? await connectChattoMcp(settings, signal) : undefined;
      let memory: Awaited<ReturnType<typeof openMemory>> | undefined;
      try {
        if (settings.memory) memory = await openMemory(identity, userIds, signal);
        const tools = [...(mcp?.tools ?? []), ...(memory?.tools ?? [])];
        const settingsManager = SettingsManager.inMemory();
        const resourceLoader = new DefaultResourceLoader({
          cwd: settings.directory,
          agentDir: settings.directory,
          settingsManager,
          noExtensions: true,
          noSkills: true,
          noPromptTemplates: true,
          noThemes: true,
          noContextFiles: true,
          // Only this host-owned extension is loaded; local extensions remain disabled.
          extensionFactories: context ? [threadContextExtension(context)] : [],
          systemPromptOverride: () =>
            `Your name is ${settings.name}.\n\n${settings.systemPrompt}\n\n${CAPABILITIES}\n${
              mcp
                ? `You have Chatto MCP tools. Current room ID: ${identity.at(-2)}. Current thread root ID: ${identity.at(-1)}.`
                : 'Chatto MCP tools are disabled.'
            }\n${memory ? `Local memory tools use the memory_ prefix. Memory document IDs: ${JSON.stringify(memory.documents)}.\n${memory.client.getInstructions() ?? ''}` : 'Local memory is disabled.'}`,
          appendSystemPromptOverride: () => []
        });
        await resourceLoader.reload();
        signal.throwIfAborted();
        const { session } = await createAgentSession({
          cwd: settings.directory,
          agentDir: settings.directory,
          modelRuntime: runtime,
          model,
          settingsManager,
          resourceLoader,
          tools: tools.map((tool) => tool.name),
          customTools: tools,
          sessionManager: SessionManager.open(
            join(settings.directory, transcriptName(identity)),
            settings.directory,
            settings.directory
          )
        });
        const abort = () => {
          void session.abort().catch(() => {});
        };
        signal.addEventListener('abort', abort, { once: true });
        try {
          signal.throwIfAborted();
          const historyMessages = session.messages.length;
          log('info', historyMessages ? 'Pi session resumed' : 'Pi session created', {
            threadId: identity.at(-1),
            sessionId: session.sessionId,
            historyMessages
          });
          await session.prompt(prompt);
          signal.throwIfAborted();
          const last = session.messages.at(-1);
          if (
            last?.role !== 'assistant' ||
            last.stopReason === 'error' ||
            last.stopReason === 'aborted'
          )
            throw new Error('Pi did not complete the turn');
          const text = session.getLastAssistantText()?.trim();
          if (!text) throw new Error('Pi returned no answer');
          return text;
        } finally {
          signal.removeEventListener('abort', abort);
          try {
            await session.abort();
          } finally {
            session.dispose();
          }
        }
      } finally {
        try {
          await memory?.close();
        } finally {
          await mcp?.close();
        }
      }
    }
  };
}
