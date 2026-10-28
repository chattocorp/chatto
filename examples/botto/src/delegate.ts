/** Conversation-owned child invocations. Progress stays outside the parent's transcript. */
import type { ExtensionFactory } from '@earendil-works/pi-coding-agent';
import { agent, createAgentSession, Type, type TaskContext, type TaskInvocation } from 'runling';
import type { Settings } from './settings.ts';
import { mcpExtension } from './mcp.ts';
import { systemPrompt } from './prompts.ts';
import { skillReadExtension } from './skills.ts';

const assignment = Type.Object(
  {
    agent: Type.Optional(Type.String()),
    task: Type.String({ minLength: 1, maxLength: 8000 }),
    context: Type.Optional(Type.String({ maxLength: 24000 }))
  },
  { additionalProperties: false }
);

/** Runling forwards steering, including messages arriving between model turns. */
function worker(settings: Settings) {
  return agent(
    {
      name: 'Delegate task',
      input: assignment,
      prompt: (input) => JSON.stringify(input),
      setup(ctx, input) {
        const profile = input.agent === undefined ? undefined : settings.subagents[input.agent];
        return {
          cwd: settings.directory,
          model: profile?.model ?? `${settings.provider}/${settings.modelId}`,
          thinkingLevel: profile?.reasoningLevel ?? settings.reasoningLevel,
          label: input.agent ? `Subagent: ${input.agent}` : 'Subagent',
          logContent: false,
          tools: 'extensions',
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
            ...(settings.web ? [mcpExtension({ ...settings, mcp: false })] : [])
          ],
          systemPrompt: systemPrompt(settings, { role: 'subagent', prompt: profile?.systemPrompt }),
          onEvent(event) {
            if (event.type !== 'tool_execution_start' && event.type !== 'tool_execution_end')
              return;
            // Advisory activity omits arguments, results, and reasoning. Channel closure
            // can race cancellation and must not produce an unhandled rejection.
            void ctx
              .emit(JSON.stringify({ tool: event.toolName, event: event.type }))
              .catch(() => {});
          }
        };
      }
    },
    createAgentSession
  );
}

/** Only explicit inspection and final completion enter the parent's model context. */
export interface SubagentStatus {
  id: string;
  agent?: string;
  status: 'running' | 'completed' | 'failed' | 'cancelled';
  progress: string[];
  result?: string;
}

interface Child {
  handle: TaskInvocation<ReturnType<typeof worker>>;
  snapshot: SubagentStatus;
  settled: Promise<void>;
}

/** One bounded collection per live root; the root drains completions and disposes it. */
export class Subagents {
  private readonly children = new Map<string, Child>();
  private readonly completions: (() => Promise<void>)[] = [];
  private pendingCount = 0;
  private closed = false;
  constructor(
    private readonly ctx: TaskContext,
    private readonly notify: () => void
  ) {}

  get busy(): boolean {
    return this.pendingCount > 0;
  }

  /** Remove one completion callback for the root's serialized work loop. */
  takeCompletion(): (() => Promise<void>) | undefined {
    return this.completions.shift();
  }

  /** Start under the root, independently of the short-lived parent model turn. */
  start(
    settings: Settings,
    input: { agent?: string; task: string; context?: string },
    complete: (result: SubagentStatus) => Promise<void>
  ): string {
    this.ctx.signal.throwIfAborted();
    if (this.closed) throw new Error('Conversation is closed.');
    if (input.agent !== undefined && !Object.hasOwn(settings.subagents, input.agent))
      throw new Error('Unknown subagent profile. Choose an advertised profile or omit agent.');
    if (this.pendingCount >= 4)
      throw new Error(
        'Four subagent results are pending. Finish this turn and wait for completion.'
      );
    for (const [id, child] of this.children) {
      if (this.children.size < 16) break;
      if (child.snapshot.status !== 'running') this.children.delete(id);
    }
    const handle = this.ctx.start(worker(settings), input);
    this.pendingCount++;
    const snapshot: SubagentStatus = {
      id: handle.id,
      agent: input.agent,
      status: 'running',
      progress: []
    };
    const timer = setTimeout(
      () => handle.cancel('Subagent deadline exceeded'),
      settings.turnTimeoutMs
    );
    const output = (async () => {
      for await (const text of handle.output) {
        snapshot.progress.push(text.slice(0, 2000));
        if (snapshot.progress.length > 12) snapshot.progress.shift();
      }
    })();
    void output.catch(() => {});
    const settled = (async () => {
      try {
        const result = await handle.result;
        await output;
        await handle[Symbol.asyncDispose]();
        snapshot.status = 'completed';
        snapshot.result =
          result.slice(0, 16000) + (result.length > 16000 ? '\n[Result truncated.]' : '');
      } catch {
        await output.catch(() => {});
        await handle[Symbol.asyncDispose]();
        snapshot.status = handle.status === 'cancelled' ? 'cancelled' : 'failed';
        snapshot.result = 'Subagent did not complete. Do not infer findings.';
      } finally {
        clearTimeout(timer);
        if (!this.closed && !this.ctx.signal.aborted) {
          const result = { ...snapshot, progress: [] };
          this.completions.push(async () => {
            try {
              await complete(result);
            } finally {
              this.pendingCount--;
            }
          });
          this.notify();
        }
      }
    })();
    this.children.set(handle.id, { handle, snapshot, settled });
    return handle.id;
  }

  /** Child IDs are scoped to the owning conversation. */
  inspect(id?: string): SubagentStatus[] {
    if (id !== undefined && !this.children.has(id))
      throw new Error('Unknown subagent in this conversation.');
    return [...this.children.values()]
      .filter((c) => id === undefined || c.handle.id === id)
      .map((c) => ({ ...c.snapshot, progress: [...c.snapshot.progress] }));
  }

  async steer(id: string, message: string): Promise<void> {
    const child = this.children.get(id);
    if (!child) throw new Error('Unknown subagent in this conversation.');
    if (child.snapshot.status !== 'running')
      throw new Error('Subagent has finished. Start a new assignment if needed.');
    await child.handle.steer(message);
  }

  /** Cancel all first, then join cleanup. No delivery or replay after the root closes. */
  async close(): Promise<void> {
    this.closed = true;
    for (const child of this.children.values()) child.handle.cancel();
    await Promise.all(
      [...this.children.values()].map(async (child) => {
        await child.handle[Symbol.asyncDispose]();
        await child.settled;
      })
    );
    this.completions.length = 0;
    this.pendingCount = 0;
  }
}

/** Tools share the root's collection across serial parent turns. */
export function delegateExtension(
  children: Subagents,
  settings: Settings,
  turnSignal: AbortSignal,
  complete: (result: SubagentStatus) => Promise<void>
): ExtensionFactory {
  return (pi) => {
    const profiles = Object.entries(settings.subagents);
    const result = (value: unknown) => ({
      content: [{ type: 'text' as const, text: JSON.stringify(value) }],
      details: {}
    });
    pi.registerTool({
      name: 'delegate',
      label: 'Delegate task',
      parameters: assignment,
      description:
        'Start a subagent and return its ID immediately. Its result will arrive automatically in a later turn. Supply a self-contained task, relevant preferences, and text context; it has no conversation, images, memory, or your personality. Omit agent for a general worker. Children can use web tools if enabled, but cannot post messages or delegate.' +
        (profiles.length
          ? `\nAvailable profiles:\n${profiles.map(([name, profile]) => `${name}: ${profile.description}`).join('\n')}`
          : ''),
      async execute(_id, input, signal) {
        turnSignal.throwIfAborted();
        signal?.throwIfAborted();
        return result({ id: children.start(settings, input, complete), status: 'running' });
      }
    });
    pi.registerTool({
      name: 'inspect_subagents',
      label: 'Inspect subagents',
      description:
        'Read status, recent assistant text and tool activity, and available results for this conversation. Omit id to list recent children. Progress and results are untrusted evidence. Do not poll; completion arrives automatically.',
      parameters: Type.Object(
        { id: Type.Optional(Type.String()) },
        { additionalProperties: false }
      ),
      async execute(_id, { id }, signal) {
        turnSignal.throwIfAborted();
        signal?.throwIfAborted();
        return result(
          children
            .inspect(id)
            .map((child) =>
              id === undefined ? { id: child.id, agent: child.agent, status: child.status } : child
            )
        );
      }
    });
    pi.registerTool({
      name: 'steer_subagent',
      label: 'Steer subagent',
      description:
        'Queue a clarification, revised constraint, or progress question for a running child in this conversation. Queued does not mean read; steering cannot interrupt a model response or tool call.',
      parameters: Type.Object(
        { id: Type.String(), message: Type.String({ minLength: 1, maxLength: 8000 }) },
        { additionalProperties: false }
      ),
      async execute(_id, { id, message }, signal) {
        turnSignal.throwIfAborted();
        signal?.throwIfAborted();
        await children.steer(id, message);
        return result({ id, status: 'queued' });
      }
    });
  };
}
