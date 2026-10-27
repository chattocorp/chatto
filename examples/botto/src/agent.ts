/** Open the conversation transcript through Runling for one turn, then release its resources. */
import { createHash } from 'node:crypto';
import { mkdir } from 'node:fs/promises';
import { join } from 'node:path';
import { createAgentSession, type TaskContext } from 'runling';
import type { ExtensionFactory } from '@earendil-works/pi-coding-agent';
import type { Settings } from './settings.ts';
import { startupStep } from './diagnostics.ts';
import { mcpExtension, memoryLockExtension } from './mcp.ts';
import { createMemory } from './memory.ts';
import { threadContextExtension, type ThreadContext } from './thread-context.ts';
import { systemPrompt, type PromptCapability } from './prompts.ts';
import { skillReadExtension } from './skills.ts';

/** Private input for one model turn. The task context always comes from its conversation. */
export interface ReplyOptions {
  identity: readonly string[];
  prompt: string;
  signal: AbortSignal;
  context?: ThreadContext;
  /** Deferred until Pi exposes the active model's image capability. */
  loadImages?: Parameters<typeof threadContextExtension>[1];
  /** Receives completed parent assistant messages, including the final answer, never child output. */
  onText?: (text: string) => void;
  userIds?: readonly string[];
  tools?: ExtensionFactory[];
  /** Prompt modules for the optional tools installed by the caller. */
  capabilities?: readonly PromptCapability[];
}

/** Preserve existing transcript names, bound to the server, bot, room, and thread. */
export function transcriptName(identity: readonly string[]): string {
  return `${createHash('sha256').update(JSON.stringify(identity)).digest('hex')}.jsonl`;
}

/** Prepare host-owned memory and storage. Runling owns model discovery and live sessions. */
export async function createAgent(settings: Settings) {
  const openMemory = createMemory(settings);
  await startupStep('Cannot create the Botto data directory. Check runtime.data_directory.', () =>
    mkdir(settings.directory, { recursive: true, mode: 0o700 })
  );
  return {
    /** The caller serializes turns for one identity; idle conversations hold no live agent. */
    async reply(
      ctx: TaskContext,
      {
        identity,
        prompt,
        signal,
        context,
        loadImages,
        onText,
        userIds = [],
        tools = [],
        capabilities = []
      }: ReplyOptions
    ): Promise<string> {
      signal.throwIfAborted();
      const memory = settings.memory
        ? await openMemory(identity.at(-1), userIds, signal)
        : undefined;
      await using session = await createAgentSession({
        cwd: settings.directory,
        model: `${settings.provider}/${settings.modelId}`,
        thinkingLevel: settings.reasoningLevel,
        output: 'text',
        logContent: false,
        tools: 'extensions',
        // Pi prepares all parallel calls before executing them. Memory locks
        // acquired by preparation hooks require sequential execution.
        toolExecution: memory ? 'sequential' : 'parallel',
        sessionFile: join(settings.directory, transcriptName(identity)),
        resources: {
          agentDir: settings.directory,
          extensions: false,
          skills: [settings.skillsDirectory],
          promptTemplates: false,
          themes: false,
          contextFiles: false
        },
        extensions: [
          skillReadExtension(settings.skillsDirectory),
          ...tools,
          mcpExtension(settings, memory?.directory),
          memoryLockExtension(signal, memory?.acquire),
          ...(context || memory || loadImages
            ? [
                threadContextExtension(
                  {
                    ...context,
                    messages: context?.messages ?? [],
                    olderContextOmitted: context?.olderContextOmitted ?? false,
                    ...(memory ? { memoryDocuments: memory.documents } : {})
                  },
                  loadImages
                )
              ]
            : [])
        ],
        systemPrompt: systemPrompt(settings, {
          role: 'main',
          identity,
          memory: Boolean(memory),
          capabilities
        })
      });
      signal.throwIfAborted();
      // A native MCP call can wait on its transport. Close extensions on
      // cancellation as well as cancelling the model, then await disposal.
      const abort = () => {
        void session.dispose().catch(() => {});
      };
      signal.addEventListener('abort', abort, { once: true });
      try {
        signal.throwIfAborted();
        const { summary } = await session.run(ctx, prompt, {
          signal,
          ...(onText ? { onText } : {})
        });
        signal.throwIfAborted();
        if (!summary.trim()) throw new Error('The agent returned no answer');
        return summary.trim();
      } finally {
        signal.removeEventListener('abort', abort);
      }
    }
  };
}
