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
import type { ExtensionFactory } from '@earendil-works/pi-coding-agent';
import type { Settings } from './settings.ts';
import { StartupError, startupStep } from './diagnostics.ts';
import { log } from './log.ts';
import { mcpExtension, mcpObserver } from './mcp.ts';
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
      userIds: readonly string[] = [],
      tools: ExtensionFactory[] = []
    ): Promise<string> {
      signal.throwIfAborted();
      const memory = settings.memory
        ? await openMemory(identity.at(-1), userIds, signal)
        : undefined;
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
        // Only host-selected extensions are loaded; local extensions remain disabled.
        extensionFactories: [
          ...tools,
          mcpExtension(settings, memory?.directory),
          mcpObserver(signal, identity.at(-1), memory?.acquire),
          ...(context ? [threadContextExtension(context)] : [])
        ],
        systemPromptOverride: () =>
          `Your name is ${settings.name}.\n\n${settings.systemPrompt}\n\n${CAPABILITIES}\n${
            settings.mcp
              ? `You have Chatto MCP tools. Current room ID: ${identity.at(-2)}. Current thread root ID: ${identity.at(-1)}.`
              : 'Chatto MCP tools are disabled.'
          }\n${
            settings.web
              ? 'Exa web tools use the mcp__exa__ prefix. Cite the source URLs you use.'
              : 'Web access is disabled.'
          }\n${memory ? `Local memory tools use the mcp__memory__ prefix. Memory document IDs: ${JSON.stringify(memory.documents)}.\n${memory.instructions ?? ''}` : 'Local memory is disabled.'}`,
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
        noTools: 'builtin',
        sessionManager: SessionManager.open(
          join(settings.directory, transcriptName(identity)),
          settings.directory,
          settings.directory
        )
      });
      // Pi prepares every parallel call before executing any of them. A memory
      // lock held by a tool_call hook would deadlock the next call in that batch.
      if (memory) session.agent.toolExecution = 'sequential';
      // Pi totals include persisted history and compaction. Subtract the restored
      // baseline so this log reports only the current turn, including failed work.
      const usageBefore = session.getSessionStats();
      let shutdown: Promise<void> | undefined;
      const shutdownExtensions = () =>
        (shutdown ??= session.extensionRunner.emit({ type: 'session_shutdown', reason: 'quit' }));
      const abort = () => {
        void session.abort().catch(() => {});
        void shutdownExtensions().catch(() => {});
      };
      try {
        signal.throwIfAborted();
        await session.bindExtensions({});
        // Binding starts native MCP. Do not send shutdown before session_start.
        signal.addEventListener('abort', abort, { once: true });
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
          try {
            await shutdownExtensions();
          } finally {
            try {
              const usage = session.getSessionStats();
              log('info', 'Pi token usage', {
                threadId: identity.at(-1),
                sessionId: session.sessionId,
                inputTokens: usage.tokens.input - usageBefore.tokens.input,
                outputTokens: usage.tokens.output - usageBefore.tokens.output,
                cacheReadTokens: usage.tokens.cacheRead - usageBefore.tokens.cacheRead,
                cacheWriteTokens: usage.tokens.cacheWrite - usageBefore.tokens.cacheWrite,
                estimatedCostUsd: Number((usage.cost - usageBefore.cost).toFixed(8))
              });
            } finally {
              session.dispose();
            }
          }
        }
      }
    }
  };
}
