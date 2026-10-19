/**
 * The investigator: a read-only subagent that answers questions about the code in a directory.
 * Each investigation keeps its agent session in a file, so a caller can resume it later with a
 * follow-up question, also after the investigation ended. The caller chooses the directory, for
 * example a checkout or a worktree.
 *
 * Run it on its own: `runling run tasks/investigate.ts --input '{"directory":
 * "/path/to/repo", "question": "..."}'`.
 */
import { stat } from 'node:fs/promises';
import { resolve } from 'node:path';
import {
  createAgentSession,
  AgentSessionStore,
  task,
  Type,
  type AgentOptions,
  type RunlingAgent,
  type TaskContext,
  type ThinkingLevel
} from 'runling';
import { ConfigurationError, setting, thinkingSetting } from '../settings.ts';
import { checkCitations, findCitations } from '../investigation/citations.ts';
import { confineExtension } from '../investigation/confine.ts';
import { GIT_TOOLS, gitExtension, runGit } from '../investigation/git.ts';

export interface InvestigatorSettings {
  model: string;
  thinkingLevel?: ThinkingLevel;
  /** Runling storage for resumable investigations, scoped by the host to this caller. */
  sessions: AgentSessionStore;
}

const Input = Type.Object({
  directory: Type.String({ minLength: 1, description: 'The directory to investigate' }),
  question: Type.String({ minLength: 1, maxLength: 20_000 }),
  session: Type.Optional(
    Type.String({
      pattern: '^[0-9a-f-]{36}$',
      description: 'The ID of an earlier investigation to continue'
    })
  ),
  id: Type.Optional(
    Type.String({
      pattern: '^[0-9a-f-]{36}$',
      description: 'The ID for a new investigation, so the caller knows it before it ends'
    })
  )
});

/** An event of the investigator's agent session. */
type AgentEvent = Parameters<NonNullable<AgentOptions['onEvent']>>[0];

/** A progress update: what the investigator does now, such as "Reading src/a.ts". */
const Update = Type.String();

const Output = Type.Object({
  /** Pass it as `session` to continue this investigation. */
  session: Type.String(),
  answer: Type.String(),
  /** The commit of the investigated directory, when it is a Git checkout. */
  commit: Type.Optional(Type.String()),
  /** File references in the answer that name existing lines. */
  citations: Type.Array(
    Type.Object({ path: Type.String(), start: Type.Integer(), end: Type.Integer() })
  )
});

const INSTRUCTIONS = [
  'You investigate the repository in your working directory to answer questions from another agent. You can only read the files and history of the repository: you cannot change files, run programs, or read files outside the repository or ignored by Git.',
  'Start from what the repository documents about itself where it exists: AGENTS.md files, architecture and decision-record indexes, and the glossary. Then read the code that the question is about. Use the git tools to see when and why code changed.',
  'gitShow shows a commit message and diff, not a historical file snapshot. Give the revision and optional path as separate arguments; revision:path syntax is unsupported, and path only filters the diff. To inspect a bounded range of historical file content, use gitBlame with revision, path, start, and end. The read tool reads the current checkout. If the available tools cannot establish historical behavior, state that limit instead of treating current code as evidence for an older version.',
  'Answer the question directly. Support each claim with a reference to lines that you read, as `path:line` or `path:start-end` with the full path relative to the repository root, also when you cite the same file again. Say which statements are inferences.',
  'Lead with the strongest conclusion the evidence supports. If you found a matching fix, say so first and identify the commit or PR, then explain whether its release status is known. A fix in the checkout does not establish that it was released, backported, or included in the reported version. Local release records alone do not establish the latest published release.',
  'End with "Checked:" (what you read) and "Not checked:" (what could change the answer). If you cannot answer from the code, say so.',
  'Be concise. The reader is an agent that will summarize your answer.'
];

/** Create the investigator task. `createAgent` is injectable for tests. */
export function createInvestigator(
  settings: InvestigatorSettings,
  createAgent: (options: AgentOptions) => Promise<RunlingAgent> = createAgentSession
) {
  return task(
    { name: 'Investigate', input: Input, result: Output, output: Update },
    async (ctx, input) => {
      const directory = resolve(input.directory);
      if (!(await stat(directory).catch(() => undefined))?.isDirectory())
        throw new Error('The directory to investigate does not exist');
      if (input.session && input.id)
        throw new Error('Give either the ID of an earlier investigation or a new ID, not both');
      await using saved = await settings.sessions.open({
        id: input.session ?? input.id,
        resume: !!input.session
      });
      const session = saved.id;
      await using investigator = await createAgent({
        label: 'investigate',
        model: settings.model,
        thinkingLevel: settings.thinkingLevel ?? 'medium',
        output: 'text',
        cwd: directory,
        sessionFile: saved.file,
        tools: ['read', 'grep', 'find', 'ls', ...GIT_TOOLS],
        extensions: [confineExtension(directory), gitExtension(directory)],
        // Repository instructions help; host extensions and skills do not belong here.
        resources: { extensions: false, skills: false, promptTemplates: false, themes: false },
        instructions: INSTRUCTIONS,
        // Progress for the caller. A caller that does not read updates must not stop the work.
        onEvent: (event) => {
          const update = describeEvent(event);
          if (update) void ctx.emit(update).catch(() => {});
        }
      });
      let answer = await answerOf(investigator, ctx, input.question);
      let checked = await checkCitations(directory, findCitations(answer));
      const problems = checked.flatMap((citation) => citation.problem ?? []);
      // One chance to correct invalid references; the answer is kept either way.
      if (problems.length) {
        answer = await answerOf(
          investigator,
          ctx,
          `Some references in your answer are wrong:\n${problems.map((problem) => `- ${problem}`).join('\n')}\nCheck them, then write your complete answer again with correct references.`
        );
        checked = await checkCitations(directory, findCitations(answer));
      }
      const commit = await runGit(directory, ['rev-parse', 'HEAD'], ctx.signal).then(
        (output) => output.trim(),
        () => undefined
      );
      return {
        session,
        answer,
        ...(commit ? { commit } : {}),
        citations: checked
          .filter((citation) => !citation.problem)
          .map(({ path, start, end }) => ({ path, start, end }))
      };
    }
  );
}

async function answerOf(investigator: RunlingAgent, ctx: TaskContext<any, any>, prompt: string) {
  const result = await investigator.runOutcome(ctx, prompt);
  if (result.outcome !== 'completed') throw new Error(result.summary);
  return result.summary;
}

/** Investigator settings from the environment. */
export function investigatorSettings(): InvestigatorSettings {
  const model = setting('CHATTO_INVESTIGATION_MODEL') ?? setting('CHATTO_AGENT_MODEL');
  if (!model) throw new ConfigurationError('Set CHATTO_INVESTIGATION_MODEL or CHATTO_AGENT_MODEL');
  return {
    model,
    thinkingLevel: thinkingSetting('CHATTO_INVESTIGATION_THINKING', 'medium'),
    // The old investigations directory holds worktrees of the previous bot; keep them apart.
    sessions: new AgentSessionStore('.runling/subagents/investigate')
  };
}

/** The investigator with settings from the environment, for `runling run`. */
export default task(
  { name: 'Investigate code', input: Input, result: Output, output: Update },
  async (ctx, input) => {
    const investigation = ctx.start(createInvestigator(investigatorSettings()), input);
    for await (const update of investigation.output) await ctx.emit(update);
    return investigation.result;
  }
);

/** A short progress line for an agent event: a tool call or a reasoning step. Tool calls inside
 * a tool are left out, and so is everything else. */
export function describeEvent(event: AgentEvent): string | undefined {
  if (event.type === 'tool_execution_start' && !event.parentToolCallId) {
    const args = (event.args ?? {}) as Record<string, unknown>;
    const text = (value: unknown) => String(value ?? '').slice(0, 200);
    switch (event.toolName) {
      case 'read':
        return `Reading ${text(args.path)}${Number(args.offset) > 1 ? ` from line ${text(args.offset)}` : ''}`;
      case 'grep':
        return `Searching for ${JSON.stringify(text(args.pattern))}${args.path ? ` in ${text(args.path)}` : ''}`;
      case 'find':
        return `Finding files: ${text(args.pattern)}${args.path ? ` in ${text(args.path)}` : ''}`;
      case 'ls':
        return `Listing ${text(args.path ?? '.')}`;
      case 'gitLog':
        return `Reading the Git log${args.path ? ` of ${text(args.path)}` : ''}${args.added ? ` for ${JSON.stringify(text(args.added))}` : ''}`;
      case 'gitBlame':
        return `Reading the Git blame of ${text(args.path)}:${text(args.start)}-${text(args.end)}`;
      case 'gitShow':
        return `Reading commit ${text(args.revision)}${args.path ? ` for ${text(args.path)}` : ''}`;
      default:
        return `Using ${event.toolName}`;
    }
  }
  if (event.type === 'message_end' && event.message.role === 'assistant') {
    // Reasoning summaries start with a short heading, such as "**Checking the renderer**".
    const heading = event.message.content
      .flatMap((part) => (part.type === 'thinking' && !part.redacted ? [part.thinking] : []))
      .map((thinking) => thinking.trim().split('\n', 1)[0]!.replace(/\*\*/g, '').trim())
      .find(Boolean);
    return heading ? `Thinking: ${heading.slice(0, 200)}` : undefined;
  }
  return undefined;
}
